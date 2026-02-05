package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Procedural learning constants
const (
	ProcedureSimilarityThreshold     = 0.75 // For finding similar procedures
	ProcedureReinforcementBoost      = 0.05 // Confidence boost on reinforcement
	ProcedureMinSuccessRate          = 0.5  // Minimum success rate to return
	ProcedureMinConfidenceDefault    = 0.4  // Minimum confidence for applicable procedures
	NewProcedureInitialConfidence    = 0.5  // Initial confidence for new procedures
	RecencyBoostDecayDays            = 30.0 // Days after which recency boost is minimal
)

var (
	ErrProcedureNotFound     = errors.New("procedure not found")
	ErrProcedureContentEmpty = errors.New("episode content is empty")
	ErrInvalidProcedureID    = errors.New("invalid procedure ID")
)

// ProceduralService handles learning and retrieval of procedural memories (skills).
type ProceduralService struct {
	procedureStore  domain.ProcedureStore
	episodeStore    domain.EpisodeStore
	agentStore      domain.AgentStore
	embeddingClient domain.EmbeddingClient
	llmClient       domain.LLMClient
	logger          *zap.Logger
}

// NewProceduralService creates a new procedural service.
func NewProceduralService(
	procedureStore domain.ProcedureStore,
	episodeStore domain.EpisodeStore,
	agentStore domain.AgentStore,
	embeddingClient domain.EmbeddingClient,
	llmClient domain.LLMClient,
	logger *zap.Logger,
) *ProceduralService {
	return &ProceduralService{
		procedureStore:  procedureStore,
		episodeStore:    episodeStore,
		agentStore:      agentStore,
		embeddingClient: embeddingClient,
		llmClient:       llmClient,
		logger:          logger,
	}
}

// ProcedureMatchInput contains the input for finding applicable procedures.
type ProcedureMatchInput struct {
	AgentID        uuid.UUID
	TenantID       uuid.UUID
	Situation      string  // Current situation/context to match against
	MinSuccessRate float32 // Minimum success rate (defaults to ProcedureMinSuccessRate)
	MinConfidence  float32 // Minimum confidence (defaults to ProcedureMinConfidenceDefault)
	Limit          int     // Maximum results (defaults to 5)
}

// LearnFromOutcome extracts procedures from successful episodes.
func (s *ProceduralService) LearnFromOutcome(ctx context.Context, episodeID uuid.UUID, tenantID uuid.UUID, outcome domain.OutcomeType) error {
	// Get the episode
	episode, err := s.episodeStore.GetByID(ctx, episodeID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrEpisodeNotFound
		}
		return err
	}

	if outcome != domain.OutcomeSuccess {
		return s.recordFailureFromEpisode(ctx, episode)
	}

	// Only extract procedures from important episodes
	if episode.ImportanceScore < 0.6 {
		return nil
	}

	if s.llmClient == nil {
		s.logger.Debug("no LLM client, skipping procedure extraction")
		return nil
	}

	pattern, err := s.llmClient.ExtractProcedure(ctx, episode.RawContent)
	if err != nil {
		s.logger.Debug("failed to extract procedure", zap.Error(err))
		return nil // Don't fail the whole operation
	}

	if pattern == nil || pattern.TriggerPattern == "" || pattern.ActionTemplate == "" {
		s.logger.Debug("no valid procedure pattern extracted")
		return nil
	}

	// Generate embedding for the trigger pattern
	var embedding []float32
	if s.embeddingClient != nil {
		embedding, err = s.embeddingClient.Embed(ctx, pattern.TriggerPattern)
		if err != nil {
			s.logger.Debug("failed to generate trigger embedding", zap.Error(err))
		}
	}

	// Check if a similar procedure already exists
	if len(embedding) > 0 {
		similar, err := s.procedureStore.FindByTriggerSimilarity(
			ctx, episode.AgentID, episode.TenantID,
			embedding, ProcedureSimilarityThreshold, 1,
		)
		if err == nil && len(similar) > 0 {
			// Reinforce existing procedure
			s.logger.Debug("reinforcing existing procedure",
				zap.String("procedure_id", similar[0].ID.String()),
				zap.Float32("similarity", similar[0].Score))
			return s.reinforceProcedure(ctx, similar[0].ID, episodeID)
		}
	}

	// Create new procedure
	procedure := &domain.Procedure{
		AgentID:             episode.AgentID,
		TenantID:            episode.TenantID,
		TriggerPattern:      pattern.TriggerPattern,
		TriggerKeywords:     pattern.TriggerKeywords,
		TriggerEmbedding:    embedding,
		ActionTemplate:      pattern.ActionTemplate,
		ActionType:          pattern.ActionType,
		DerivedFromEpisodes: []uuid.UUID{episodeID},
		Confidence:          NewProcedureInitialConfidence,
		MemoryStrength:      1.0,
	}

	now := time.Now()
	procedure.LastVerifiedAt = &now

	if err := s.procedureStore.Create(ctx, procedure); err != nil {
		return err
	}

	// Link the episode to this procedure
	if err := s.episodeStore.LinkDerivedMemory(ctx, episodeID, procedure.ID, "procedural"); err != nil {
		s.logger.Debug("failed to link episode to procedure", zap.Error(err))
	}

	s.logger.Info("created new procedure",
		zap.String("procedure_id", procedure.ID.String()),
		zap.String("trigger", pattern.TriggerPattern),
		zap.String("action_type", string(pattern.ActionType)))

	return nil
}

// GetApplicableProcedures returns procedures relevant to the current situation.
func (s *ProceduralService) GetApplicableProcedures(ctx context.Context, input ProcedureMatchInput) ([]domain.ProcedureWithScore, error) {
	// Set defaults
	if input.MinSuccessRate == 0 {
		input.MinSuccessRate = ProcedureMinSuccessRate
	}
	if input.MinConfidence == 0 {
		input.MinConfidence = ProcedureMinConfidenceDefault
	}
	if input.Limit == 0 {
		input.Limit = 5
	}

	// Generate embedding for the situation
	if s.embeddingClient == nil {
		return nil, nil
	}

	embedding, err := s.embeddingClient.Embed(ctx, input.Situation)
	if err != nil {
		return nil, err
	}

	// Find procedures with similar triggers
	candidates, err := s.procedureStore.FindByTriggerSimilarity(
		ctx, input.AgentID, input.TenantID,
		embedding, ProcedureSimilarityThreshold, input.Limit*2, // Get more than needed for filtering
	)
	if err != nil {
		return nil, err
	}

	// Filter by minimum success rate and confidence
	var applicable []domain.ProcedureWithScore
	for _, p := range candidates {
		if p.SuccessRate >= input.MinSuccessRate && p.Confidence >= input.MinConfidence {
			applicable = append(applicable, p)
		}
	}

	// Sort by combined score: (success_rate * confidence * similarity * recency_boost)
	sort.Slice(applicable, func(i, j int) bool {
		scoreI := s.computeProcedureScore(&applicable[i])
		scoreJ := s.computeProcedureScore(&applicable[j])
		return scoreI > scoreJ
	})

	// Limit results
	if len(applicable) > input.Limit {
		applicable = applicable[:input.Limit]
	}

	return applicable, nil
}

// GetByID retrieves a procedure by ID.
func (s *ProceduralService) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Procedure, error) {
	procedure, err := s.procedureStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrProcedureNotFound
		}
		return nil, err
	}
	return procedure, nil
}

// RecordProcedureOutcome records the success/failure of using a procedure.
func (s *ProceduralService) RecordProcedureOutcome(ctx context.Context, procedureID uuid.UUID, tenantID uuid.UUID, success bool) error {
	// Verify the procedure exists and belongs to this tenant
	_, err := s.procedureStore.GetByID(ctx, procedureID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrProcedureNotFound
		}
		return err
	}

	return s.procedureStore.RecordUse(ctx, procedureID, success)
}

// reinforceProcedure increases confidence and links the episode.
func (s *ProceduralService) reinforceProcedure(ctx context.Context, procedureID uuid.UUID, episodeID uuid.UUID) error {
	return s.procedureStore.Reinforce(ctx, procedureID, episodeID, ProcedureReinforcementBoost)
}

// recordFailureFromEpisode finds similar procedures and records failure.
func (s *ProceduralService) recordFailureFromEpisode(ctx context.Context, episode *domain.Episode) error {
	if s.embeddingClient == nil {
		return nil
	}

	embedding, err := s.embeddingClient.Embed(ctx, episode.RawContent)
	if err != nil {
		return nil // Don't fail silently
	}

	similar, err := s.procedureStore.FindByTriggerSimilarity(
		ctx, episode.AgentID, episode.TenantID,
		embedding, ProcedureSimilarityThreshold, 1,
	)
	if err != nil || len(similar) == 0 {
		return nil
	}

	// Record failure on the most similar procedure
	return s.procedureStore.RecordUse(ctx, similar[0].ID, false)
}

// computeProcedureScore calculates the combined ranking score for a procedure.
func (s *ProceduralService) computeProcedureScore(p *domain.ProcedureWithScore) float64 {
	recencyBoost := s.recencyBoost(p.LastUsedAt)
	return float64(p.SuccessRate) * float64(p.Confidence) * float64(p.Score) * recencyBoost
}

// recencyBoost calculates a boost factor based on how recently the procedure was used.
func (s *ProceduralService) recencyBoost(lastUsedAt *time.Time) float64 {
	if lastUsedAt == nil {
		return 0.5 // Never used gets lower boost
	}

	daysSinceUse := time.Since(*lastUsedAt).Hours() / 24
	if daysSinceUse <= 0 {
		return 1.0
	}

	// Exponential decay: boost = exp(-days / decayDays)
	// At 30 days, boost is ~0.37; at 60 days, boost is ~0.14
	return math.Exp(-daysSinceUse / RecencyBoostDecayDays)
}
