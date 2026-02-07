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
	ErrMemoryNotFound       = errors.New("memory not found")
	ErrInvalidMemoryType    = errors.New("invalid memory type")
	ErrMemoryContentEmpty   = errors.New("content is required")
	ErrMemoryAgentIDMissing = errors.New("agent_id is required")
	ErrRecallQueryEmpty     = errors.New("query is required")
	ErrRecallAgentIDMissing = errors.New("agent_id is required for recall")
)

type PolicyEnforcer interface {
	EnforceOnCreate(ctx context.Context, m *domain.Memory) error
}

const (
	// SimilarityThreshold is the minimum embedding similarity for reinforcement detection.
	SimilarityThreshold = 0.85
	// ReinforcementConfidenceBoost is added to confidence when a belief is reinforced.
	ReinforcementConfidenceBoost = 0.05
	// MaxConfidence is the maximum confidence value.
	MaxConfidence = 0.99
	// ContradictionConfidencePenalty is subtracted from old belief on contradiction.
	ContradictionConfidencePenalty = 0.2
	// MinConfidence is the minimum confidence value.
	MinConfidence = 0.1
	// NewContradictingBeliefConfidence is the starting confidence for a contradicting belief.
	NewContradictingBeliefConfidence = 0.7
	// DefaultRecallMinConfidence is the default minimum confidence for recall.
	DefaultRecallMinConfidence = 0.6
	// UsageReinforcementBoost is the small boost applied when a memory is recalled.
	UsageReinforcementBoost = 0.02
)

type GraphBuilder interface {
	OnMemoryCreated(ctx context.Context, memory *domain.Memory) error
}

type MemoryService struct {
	memoryStore        domain.MemoryStore
	agentStore         domain.AgentStore
	embeddingClient    domain.EmbeddingClient
	llmClient          domain.LLMClient
	contradictionStore domain.ContradictionStore
	policyEnforcer     PolicyEnforcer
	graphBuilder       GraphBuilder
	logger             *zap.Logger
}

func NewMemoryService(ms domain.MemoryStore, as domain.AgentStore, ec domain.EmbeddingClient, lc domain.LLMClient, logger *zap.Logger) *MemoryService {
	return &MemoryService{
		memoryStore:     ms,
		agentStore:      as,
		embeddingClient: ec,
		llmClient:       lc,
		logger:          logger,
	}
}

func (s *MemoryService) SetContradictionStore(cs domain.ContradictionStore) {
	s.contradictionStore = cs
}

func (s *MemoryService) SetPolicyEnforcer(pe PolicyEnforcer) {
	s.policyEnforcer = pe
}

func (s *MemoryService) SetGraphBuilder(gb GraphBuilder) {
	s.graphBuilder = gb
}

// CreateResult contains additional info about a memory creation.
type CreateResult struct {
	Reinforced         bool      `json:"reinforced"`
	ReinforcedMemoryID uuid.UUID `json:"reinforced_memory_id,omitempty"`
}

func (s *MemoryService) Create(ctx context.Context, m *domain.Memory) (*CreateResult, error) {
	return s.createWithOptions(ctx, m, true)
}

// CreateWithoutBeliefLogic creates a memory without reinforcement/contradiction checks.
// Used internally when we want to force creation of a new belief.
func (s *MemoryService) CreateWithoutBeliefLogic(ctx context.Context, m *domain.Memory) error {
	_, err := s.createWithOptions(ctx, m, false)
	return err
}

func (s *MemoryService) createWithOptions(ctx context.Context, m *domain.Memory, enableBeliefLogic bool) (*CreateResult, error) {
	if m.Content == "" {
		return nil, ErrMemoryContentEmpty
	}
	if m.AgentID == uuid.Nil {
		return nil, ErrMemoryAgentIDMissing
	}
	if m.Type != "" && !domain.ValidMemoryType(string(m.Type)) {
		return nil, ErrInvalidMemoryType
	}

	// Classify via LLM if type not provided
	if m.Type == "" {
		if s.llmClient != nil {
			classified, err := s.llmClient.Classify(ctx, m.Content)
			if err != nil {
				s.logger.Warn("LLM classification failed, defaulting to fact", zap.Error(err))
				m.Type = domain.MemoryTypeFact
			} else {
				m.Type = classified
			}
		} else {
			m.Type = domain.MemoryTypeFact
		}
	}

	// Default confidence
	if m.Confidence == 0 {
		m.Confidence = 1.0
	}

	// Verify agent exists and belongs to tenant
	_, err := s.agentStore.GetByID(ctx, m.AgentID, m.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Generate embedding
	if s.embeddingClient != nil {
		emb, err := s.embeddingClient.Embed(ctx, m.Content)
		if err != nil {
			s.logger.Warn("embedding generation failed", zap.Error(err))
			// Continue without embedding — recall won't find it, but storage still works
		} else {
			m.Embedding = emb
		}
	}

	result := &CreateResult{}

	// Belief reinforcement and contradiction logic
	if enableBeliefLogic && len(m.Embedding) > 0 {
		similar, err := s.memoryStore.FindSimilar(ctx, m.AgentID, m.TenantID, m.Embedding, SimilarityThreshold)
		if err != nil {
			s.logger.Warn("failed to find similar beliefs", zap.Error(err))
		} else if len(similar) > 0 {
			for _, existing := range similar {
				if s.llmClient != nil {
					tension, err := s.llmClient.CheckTension(ctx, existing.Content, m.Content)
					if err != nil {
						s.logger.Warn("tension check failed", zap.Error(err))
						continue
					}

					handled, err := s.handleTension(ctx, tension, &existing, m, result)
					if err != nil {
						return nil, err
					}
					if handled {
						return result, nil
					}
				}

				// No tension detected - reinforce
				newConfidence := existing.Confidence + ReinforcementConfidenceBoost
				if newConfidence > MaxConfidence {
					newConfidence = MaxConfidence
				}
				newCount := existing.ReinforcementCount + 1

				if err := s.memoryStore.UpdateReinforcement(ctx, existing.ID, newConfidence, newCount); err != nil {
					s.logger.Warn("failed to reinforce belief", zap.Error(err))
				} else {
					m.ID = existing.ID
					m.Confidence = newConfidence
					m.ReinforcementCount = newCount
					m.CreatedAt = existing.CreatedAt
					m.UpdatedAt = existing.UpdatedAt
					result.Reinforced = true
					result.ReinforcedMemoryID = existing.ID
					return result, nil
				}
			}
		}
	}

	// No similar beliefs found or belief logic disabled - create new
	if err := s.memoryStore.Create(ctx, m); err != nil {
		return nil, err
	}

	// Enforce policies after creation (non-blocking — log errors but don't fail the create)
	if s.policyEnforcer != nil {
		if err := s.policyEnforcer.EnforceOnCreate(ctx, m); err != nil {
			s.logger.Warn("policy enforcement failed after memory creation", zap.Error(err))
		}
	}

	// Build graph edges for hybrid retrieval (non-blocking)
	if s.graphBuilder != nil {
		if err := s.graphBuilder.OnMemoryCreated(ctx, m); err != nil {
			s.logger.Warn("graph building failed after memory creation", zap.Error(err))
		}
	}

	return result, nil
}

func (s *MemoryService) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	m, err := s.memoryStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrMemoryNotFound
		}
		return nil, err
	}
	return m, nil
}

func (s *MemoryService) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := s.memoryStore.Delete(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrMemoryNotFound
		}
		return err
	}
	return nil
}

func (s *MemoryService) Recall(ctx context.Context, query string, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	if query == "" {
		return nil, ErrRecallQueryEmpty
	}
	if agentID == uuid.Nil {
		return nil, ErrRecallAgentIDMissing
	}

	if s.embeddingClient == nil {
		return nil, errors.New("embedding client not configured")
	}

	// Set default minimum confidence for belief retrieval
	if opts.MinConfidence == 0 {
		opts.MinConfidence = DefaultRecallMinConfidence
	}

	// Default scoring mode
	if opts.Scoring == "" {
		opts.Scoring = domain.ScoringWeighted
	}

	// Default to hot and warm tiers
	if len(opts.IncludeTiers) == 0 {
		opts.IncludeTiers = domain.DefaultIncludeTiers()
	}

	// For weighted scoring, fetch more candidates so re-ranking is meaningful
	storeOpts := opts
	if opts.Scoring == domain.ScoringWeighted {
		storeOpts.TopK = opts.TopK * 3
		if storeOpts.TopK < 30 {
			storeOpts.TopK = 30
		}
	}

	emb, err := s.embeddingClient.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	memories, err := s.memoryStore.Recall(ctx, emb, agentID, tenantID, storeOpts)
	if err != nil {
		return nil, err
	}

	// Apply tier-based filtering
	memories = s.filterByTier(memories, opts.IncludeTiers)

	// Apply composite scoring and re-ranking
	if opts.Scoring == domain.ScoringWeighted && len(memories) > 0 {
		scorer := s.buildScorer(ctx, agentID)
		scored := scorer.ScoreAndRank(memories, timeNow())
		memories = make([]domain.MemoryWithScore, 0, len(scored))
		for _, sm := range scored {
			memories = append(memories, sm.MemoryWithScore)
		}
	}

	// Truncate to requested TopK after re-ranking
	if len(memories) > opts.TopK {
		memories = memories[:opts.TopK]
	}

	// Usage reinforcement: recalled memories get a small confidence boost
	// Cold/archive tier memories are logged for potential summarization
	for _, mem := range memories {
		tier := domain.ComputeTier(float64(mem.Confidence))
		behavior := domain.GetTierBehavior(tier)

		go func(id uuid.UUID, shouldSummarize bool) {
			if err := s.memoryStore.IncrementAccessAndBoost(context.Background(), id, UsageReinforcementBoost); err != nil {
				s.logger.Debug("failed to reinforce memory on usage", zap.String("memory_id", id.String()), zap.Error(err))
			}
			if shouldSummarize {
				s.logger.Debug("cold tier memory accessed, candidate for summarization",
					zap.String("memory_id", id.String()),
					zap.String("tier", string(tier)))
			}
		}(mem.ID, behavior.SummarizeOnAccess)
	}

	return memories, nil
}

func (s *MemoryService) filterByTier(memories []domain.MemoryWithScore, includeTiers []domain.MemoryTier) []domain.MemoryWithScore {
	tierSet := make(map[domain.MemoryTier]bool)
	for _, t := range includeTiers {
		tierSet[t] = true
	}

	var filtered []domain.MemoryWithScore
	for _, m := range memories {
		tier := domain.ComputeTier(float64(m.Confidence))
		if !tierSet[tier] {
			continue
		}
		behavior := domain.GetTierBehavior(tier)
		if float64(m.Score) >= behavior.RetrievalThreshold {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// RecallWithExplain returns scored memories with detailed score breakdowns.
func (s *MemoryService) RecallWithExplain(ctx context.Context, query string, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]ScoredMemory, error) {
	if query == "" {
		return nil, ErrRecallQueryEmpty
	}
	if agentID == uuid.Nil {
		return nil, ErrRecallAgentIDMissing
	}

	if s.embeddingClient == nil {
		return nil, errors.New("embedding client not configured")
	}

	if opts.MinConfidence == 0 {
		opts.MinConfidence = DefaultRecallMinConfidence
	}
	if opts.Scoring == "" {
		opts.Scoring = domain.ScoringWeighted
	}
	if len(opts.IncludeTiers) == 0 {
		opts.IncludeTiers = domain.DefaultIncludeTiers()
	}

	// Fetch extra candidates for re-ranking
	storeOpts := opts
	storeOpts.TopK = opts.TopK * 3
	if storeOpts.TopK < 30 {
		storeOpts.TopK = 30
	}

	emb, err := s.embeddingClient.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	memories, err := s.memoryStore.Recall(ctx, emb, agentID, tenantID, storeOpts)
	if err != nil {
		return nil, err
	}

	// Apply tier-based filtering
	memories = s.filterByTier(memories, opts.IncludeTiers)

	scorer := s.buildScorer(ctx, agentID)
	scored := scorer.ScoreAndRank(memories, timeNow())

	if len(scored) > opts.TopK {
		scored = scored[:opts.TopK]
	}

	// Usage reinforcement
	for _, mem := range scored {
		go func(id uuid.UUID) {
			if err := s.memoryStore.IncrementAccessAndBoost(context.Background(), id, UsageReinforcementBoost); err != nil {
				s.logger.Debug("failed to reinforce memory on usage", zap.String("memory_id", id.String()), zap.Error(err))
			}
		}(mem.ID)
	}

	return scored, nil
}

// PolicyWeightProvider is an optional interface that PolicyEnforcer can implement
// to provide per-type importance weights for scoring.
type PolicyWeightProvider interface {
	GetTypeWeights(ctx context.Context, agentID uuid.UUID) map[domain.MemoryType]float64
}

// buildScorer creates a RecallScorer with per-agent policy weights if available.
func (s *MemoryService) buildScorer(ctx context.Context, agentID uuid.UUID) *RecallScorer {
	scorer := NewRecallScorer()

	if s.policyEnforcer == nil {
		return scorer
	}

	if ps, ok := s.policyEnforcer.(PolicyWeightProvider); ok {
		weights := ps.GetTypeWeights(ctx, agentID)
		if len(weights) > 0 {
			scorer.TypeWeights = weights
		}
	}

	return scorer
}

var timeNow = time.Now

type ExtractResult struct {
	ID         uuid.UUID         `json:"id,omitempty"`
	Type       domain.MemoryType `json:"type"`
	Content    string            `json:"content"`
	Confidence float32           `json:"confidence"`
	Stored     bool              `json:"stored"`
	Reinforced bool              `json:"reinforced,omitempty"`
}

func (s *MemoryService) Extract(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, conversation []domain.Message, autoStore bool) ([]ExtractResult, error) {
	if s.llmClient == nil {
		return nil, errors.New("LLM client not configured")
	}

	extracted, err := s.llmClient.Extract(ctx, conversation)
	if err != nil {
		return nil, err
	}

	var results []ExtractResult
	for _, e := range extracted {
		// Compute confidence from EvidenceType if present, otherwise use LLM confidence
		confidence := e.Confidence
		if e.EvidenceType != "" {
			confidence = e.EvidenceType.InitialConfidence()
		}

		result := ExtractResult{
			Type:       e.Type,
			Content:    e.Content,
			Confidence: confidence,
			Stored:     false,
		}

		if autoStore {
			mem := &domain.Memory{
				AgentID:    agentID,
				TenantID:   tenantID,
				Type:       e.Type,
				Content:    e.Content,
				Confidence: confidence,
				Source:     string(domain.SourceExtraction),
			}
			createResult, err := s.Create(ctx, mem)
			if err != nil {
				s.logger.Warn("failed to auto-store extracted memory",
					zap.String("content", e.Content),
					zap.Error(err))
			} else {
				result.ID = mem.ID
				result.Confidence = mem.Confidence
				result.Stored = true
				if createResult != nil {
					result.Reinforced = createResult.Reinforced
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}

func (s *MemoryService) Summarize(ctx context.Context, memories []domain.Memory) (string, error) {
	if s.llmClient == nil {
		return "", errors.New("LLM client not configured")
	}
	return s.llmClient.Summarize(ctx, memories)
}

// handleTension applies graded contradiction rules. Returns true if the tension was handled
// (meaning the caller should not proceed with reinforcement).
func (s *MemoryService) handleTension(ctx context.Context, tension *domain.TensionResult, existing *domain.MemoryWithScore, m *domain.Memory, result *CreateResult) (bool, error) {
	if tension == nil {
		return false, nil
	}

	switch tension.Type {
	case domain.ContradictionHard:
		if tension.TensionScore <= 0.7 {
			return false, nil
		}
		// Demote old belief significantly, create new
		newOldConfidence := existing.Confidence - ContradictionConfidencePenalty
		if newOldConfidence < MinConfidence {
			newOldConfidence = MinConfidence
		}
		if err := s.memoryStore.UpdateConfidence(ctx, existing.ID, newOldConfidence); err != nil {
			s.logger.Warn("failed to update contradicted belief confidence", zap.Error(err))
		}
		m.Confidence = NewContradictingBeliefConfidence
		if err := s.memoryStore.Create(ctx, m); err != nil {
			return false, err
		}
		if s.contradictionStore != nil {
			if err := s.contradictionStore.Create(ctx, existing.ID, m.ID); err != nil {
				s.logger.Warn("failed to record contradiction", zap.Error(err))
			}
		}
		if s.policyEnforcer != nil {
			if err := s.policyEnforcer.EnforceOnCreate(ctx, m); err != nil {
				s.logger.Warn("policy enforcement failed after memory creation", zap.Error(err))
			}
		}
		return true, nil

	case domain.ContradictionTemporal:
		// Archive old belief, create new with boosted confidence (time evolution)
		if err := s.memoryStore.Archive(ctx, existing.ID); err != nil {
			s.logger.Warn("failed to archive temporally superseded belief", zap.Error(err))
		}
		if m.Confidence == 0 {
			m.Confidence = NewContradictingBeliefConfidence
		}
		if err := s.memoryStore.Create(ctx, m); err != nil {
			return false, err
		}
		if s.policyEnforcer != nil {
			if err := s.policyEnforcer.EnforceOnCreate(ctx, m); err != nil {
				s.logger.Warn("policy enforcement failed after memory creation", zap.Error(err))
			}
		}
		return true, nil

	case domain.ContradictionContextual:
		// Both coexist - create the new belief without touching the old one
		if err := s.memoryStore.Create(ctx, m); err != nil {
			return false, err
		}
		if s.policyEnforcer != nil {
			if err := s.policyEnforcer.EnforceOnCreate(ctx, m); err != nil {
				s.logger.Warn("policy enforcement failed after memory creation", zap.Error(err))
			}
		}
		return true, nil

	case domain.ContradictionSoft:
		if tension.TensionScore < 0.3 {
			// Low tension - treat as reinforcement, fall through
			return false, nil
		}
		// Moderate soft tension - create new without demoting old
		if err := s.memoryStore.Create(ctx, m); err != nil {
			return false, err
		}
		return true, nil

	case domain.ContradictionNone:
		// Reinforce existing, fall through
		return false, nil
	}

	return false, nil
}

func (s *MemoryService) GetHotMemories(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.memoryStore.GetByTier(ctx, agentID, tenantID, domain.TierHot, limit)
}

type TierStats struct {
	HotCount      int                             `json:"hot_count"`
	WarmCount     int                             `json:"warm_count"`
	ColdCount     int                             `json:"cold_count"`
	ArchiveCount  int                             `json:"archive_count"`
	Distribution  map[domain.MemoryType]TierDist  `json:"tier_distribution,omitempty"`
}

type TierDist struct {
	Hot     int `json:"hot"`
	Warm    int `json:"warm"`
	Cold    int `json:"cold"`
	Archive int `json:"archive"`
}

func (s *MemoryService) GetTierStats(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (*TierStats, error) {
	counts, err := s.memoryStore.GetTierCounts(ctx, agentID, tenantID)
	if err != nil {
		return nil, err
	}

	return &TierStats{
		HotCount:     counts[domain.TierHot],
		WarmCount:    counts[domain.TierWarm],
		ColdCount:    counts[domain.TierCold],
		ArchiveCount: counts[domain.TierArchive],
	}, nil
}
