package service

import (
	"context"
	"errors"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrEpisodeNotFound       = errors.New("episode not found")
	ErrEpisodeContentEmpty   = errors.New("raw_content is required")
	ErrEpisodeAgentIDMissing = errors.New("agent_id is required")
	ErrInvalidOutcomeType    = errors.New("invalid outcome type")
)

const (
	// EpisodeSimilarityThreshold for finding related episodes
	EpisodeSimilarityThreshold = 0.7
	// MaxAssociationsPerEpisode limits how many associations to create
	MaxAssociationsPerEpisode = 5
	// ExtractionConfidenceDiscount is applied to auto-extracted beliefs
	ExtractionConfidenceDiscount = 0.8
)

type EpisodeService struct {
	episodeStore    domain.EpisodeStore
	agentStore      domain.AgentStore
	memoryStore     domain.MemoryStore
	embeddingClient domain.EmbeddingClient
	llmClient       domain.LLMClient
	logger          *zap.Logger
}

func NewEpisodeService(
	es domain.EpisodeStore,
	as domain.AgentStore,
	ec domain.EmbeddingClient,
	lc domain.LLMClient,
	logger *zap.Logger,
) *EpisodeService {
	return &EpisodeService{
		episodeStore:    es,
		agentStore:      as,
		embeddingClient: ec,
		llmClient:       lc,
		logger:          logger,
	}
}

func (s *EpisodeService) SetMemoryStore(ms domain.MemoryStore) {
	s.memoryStore = ms
}

// EncodeInput is the input for encoding a new episode.
type EncodeInput struct {
	AgentID        uuid.UUID
	TenantID       uuid.UUID
	RawContent     string
	ConversationID *uuid.UUID
	OccurredAt     time.Time
	Outcome        *domain.OutcomeType
}

// Encode creates a richly-encoded episode from raw input.
func (s *EpisodeService) Encode(ctx context.Context, input EncodeInput) (*domain.Episode, error) {
	if input.RawContent == "" {
		return nil, ErrEpisodeContentEmpty
	}
	if input.AgentID == uuid.Nil {
		return nil, ErrEpisodeAgentIDMissing
	}

	// Verify agent exists
	_, err := s.agentStore.GetByID(ctx, input.AgentID, input.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Set defaults
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now()
	}

	episode := &domain.Episode{
		AgentID:             input.AgentID,
		TenantID:            input.TenantID,
		RawContent:          input.RawContent,
		ConversationID:      input.ConversationID,
		OccurredAt:          input.OccurredAt,
		ConsolidationStatus: domain.ConsolidationRaw,
		MemoryStrength:      1.0,
		DecayRate:           0.1,
		AccessCount:         1,
		LastAccessedAt:      time.Now(),
		ImportanceScore:     0.5,
	}

	if input.Outcome != nil {
		episode.Outcome = *input.Outcome
	}

	// Extract temporal context
	episode.TimeOfDay = extractTimeOfDay(input.OccurredAt)
	episode.DayOfWeek = input.OccurredAt.Weekday().String()

	// Generate embedding
	if s.embeddingClient != nil {
		emb, err := s.embeddingClient.Embed(ctx, input.RawContent)
		if err != nil {
			s.logger.Warn("failed to generate episode embedding", zap.Error(err))
		} else {
			episode.Embedding = emb
		}
	}

	// LLM extraction (entities, emotions, causal links)
	if s.llmClient != nil {
		extraction, err := s.llmClient.ExtractEpisodeStructure(ctx, input.RawContent)
		if err != nil {
			s.logger.Warn("failed to extract episode structure", zap.Error(err))
		} else if extraction != nil {
			episode.Entities = extraction.Entities
			episode.Topics = extraction.Topics
			episode.CausalLinks = extraction.CausalLinks
			episode.EmotionalValence = extraction.EmotionalValence
			episode.EmotionalIntensity = extraction.EmotionalIntensity
			if extraction.ImportanceScore > 0 {
				episode.ImportanceScore = extraction.ImportanceScore
			}
		}
	}

	// Save episode
	if err := s.episodeStore.Create(ctx, episode); err != nil {
		return nil, err
	}

	// Find and create associations with similar episodes
	if len(episode.Embedding) > 0 {
		s.createAssociations(ctx, episode)
	}

	// Extract beliefs from the episode asynchronously
	if s.llmClient != nil && s.memoryStore != nil {
		go s.extractBeliefsFromEpisode(context.Background(), episode)
	}

	return episode, nil
}

// createAssociations finds similar episodes and creates associations.
func (s *EpisodeService) createAssociations(ctx context.Context, episode *domain.Episode) {
	similar, err := s.episodeStore.FindSimilar(
		ctx,
		episode.AgentID,
		episode.TenantID,
		episode.Embedding,
		EpisodeSimilarityThreshold,
		MaxAssociationsPerEpisode,
	)
	if err != nil {
		s.logger.Warn("failed to find similar episodes", zap.Error(err))
		return
	}

	for _, sim := range similar {
		if sim.ID == episode.ID {
			continue // Skip self
		}

		assoc := &domain.EpisodeAssociation{
			EpisodeAID:          episode.ID,
			EpisodeBID:          sim.ID,
			AssociationType:     domain.AssociationThematic,
			AssociationStrength: sim.Score,
		}

		if err := s.episodeStore.CreateAssociation(ctx, assoc); err != nil {
			s.logger.Warn("failed to create episode association",
				zap.String("episode_a", episode.ID.String()),
				zap.String("episode_b", sim.ID.String()),
				zap.Error(err))
		}
	}
}

func (s *EpisodeService) extractBeliefsFromEpisode(ctx context.Context, episode *domain.Episode) {
	extracted, err := s.llmClient.Extract(ctx, []domain.Message{
		{Role: "user", Content: episode.RawContent},
	})
	if err != nil {
		s.logger.Debug("failed to extract beliefs from episode", zap.Error(err))
		return
	}

	for _, belief := range extracted {
		mem := &domain.Memory{
			AgentID:    episode.AgentID,
			TenantID:   episode.TenantID,
			Content:    belief.Content,
			Type:       belief.Type,
			Confidence: belief.Confidence * ExtractionConfidenceDiscount,
			Source:     "episode:" + episode.ID.String(),
		}

		// Generate embedding
		if s.embeddingClient != nil {
			emb, err := s.embeddingClient.Embed(ctx, belief.Content)
			if err == nil {
				mem.Embedding = emb
			}
		}

		if err := s.memoryStore.Create(ctx, mem); err != nil {
			s.logger.Debug("failed to store extracted belief",
				zap.String("content", belief.Content),
				zap.Error(err))
			continue
		}

		// Link the derived memory to the episode
		if err := s.episodeStore.LinkDerivedMemory(ctx, episode.ID, mem.ID, "semantic"); err != nil {
			s.logger.Debug("failed to link derived memory to episode", zap.Error(err))
		}
	}

	// Mark episode as processed
	if err := s.episodeStore.UpdateConsolidationStatus(ctx, episode.ID, domain.ConsolidationProcessed); err != nil {
		s.logger.Debug("failed to update episode consolidation status", zap.Error(err))
	}
}

// GetByID retrieves an episode by ID and records access.
func (s *EpisodeService) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Episode, error) {
	episode, err := s.episodeStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrEpisodeNotFound
		}
		return nil, err
	}

	// Record access (best effort)
	if err := s.episodeStore.RecordAccess(ctx, id); err != nil {
		s.logger.Warn("failed to record episode access", zap.Error(err))
	}

	return episode, nil
}

// RecallOpts contains options for episode recall.
type EpisodeRecallOpts struct {
	Query         string
	StartTime     *time.Time
	EndTime       *time.Time
	MinImportance float32
	Limit         int
}

// Recall retrieves episodes based on various criteria.
func (s *EpisodeService) Recall(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, opts EpisodeRecallOpts) ([]domain.EpisodeWithScore, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

	// Verify agent exists
	_, err := s.agentStore.GetByID(ctx, agentID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// If query is provided, do semantic search
	if opts.Query != "" {
		if s.embeddingClient == nil {
			return nil, errors.New("embedding client not configured")
		}

		emb, err := s.embeddingClient.Embed(ctx, opts.Query)
		if err != nil {
			return nil, err
		}

		results, err := s.episodeStore.FindSimilar(ctx, agentID, tenantID, emb, 0.5, opts.Limit)
		if err != nil {
			return nil, err
		}

		// Record access for retrieved episodes
		for _, ep := range results {
			_ = s.episodeStore.RecordAccess(ctx, ep.ID)
		}

		return results, nil
	}

	// If time range is provided, use time-based retrieval
	if opts.StartTime != nil && opts.EndTime != nil {
		episodes, err := s.episodeStore.GetByTimeRange(ctx, agentID, tenantID, *opts.StartTime, *opts.EndTime)
		if err != nil {
			return nil, err
		}

		var results []domain.EpisodeWithScore
		for i, ep := range episodes {
			if i >= opts.Limit {
				break
			}
			results = append(results, domain.EpisodeWithScore{
				Episode: ep,
				Score:   1.0, // No similarity score for time-based retrieval
			})
			_ = s.episodeStore.RecordAccess(ctx, ep.ID)
		}
		return results, nil
	}

	// If min importance is provided, use importance-based retrieval
	if opts.MinImportance > 0 {
		episodes, err := s.episodeStore.GetByImportance(ctx, agentID, tenantID, opts.MinImportance, opts.Limit)
		if err != nil {
			return nil, err
		}

		var results []domain.EpisodeWithScore
		for _, ep := range episodes {
			results = append(results, domain.EpisodeWithScore{
				Episode: ep,
				Score:   ep.ImportanceScore,
			})
			_ = s.episodeStore.RecordAccess(ctx, ep.ID)
		}
		return results, nil
	}

	return nil, errors.New("at least one of query, time range, or min_importance is required")
}

// RecordOutcome records the outcome of an episode.
func (s *EpisodeService) RecordOutcome(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, outcome domain.OutcomeType, description string) error {
	if !domain.ValidOutcomeType(string(outcome)) {
		return ErrInvalidOutcomeType
	}

	// Verify episode exists
	_, err := s.episodeStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrEpisodeNotFound
		}
		return err
	}

	return s.episodeStore.UpdateOutcome(ctx, id, outcome, description)
}

// GetByConversationID retrieves all episodes for a conversation.
func (s *EpisodeService) GetByConversationID(ctx context.Context, conversationID uuid.UUID, tenantID uuid.UUID) ([]domain.Episode, error) {
	return s.episodeStore.GetByConversationID(ctx, conversationID, tenantID)
}

// GetAssociations retrieves associations for an episode.
func (s *EpisodeService) GetAssociations(ctx context.Context, episodeID uuid.UUID) ([]domain.EpisodeAssociation, error) {
	return s.episodeStore.GetAssociations(ctx, episodeID)
}

// extractTimeOfDay returns the time of day category for a given time.
func extractTimeOfDay(t time.Time) string {
	hour := t.Hour()
	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}
