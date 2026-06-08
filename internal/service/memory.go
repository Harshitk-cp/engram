package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service/contradiction"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var datePatternRE = regexp.MustCompile(`\[DATE:\s*([^\]]+)\]`)

var eventDateFormats = []string{
	"2006/01/02 (Mon) 15:04",
	"2006/01/02 (Mon)",
	"2006/01/02",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"January 2, 2006",
}

func extractEventDate(content string, metadata map[string]any) *time.Time {
	if metadata != nil {
		if dateStr, ok := metadata["session_date"].(string); ok && dateStr != "" {
			if t := parseEventDate(dateStr); t != nil {
				return t
			}
		}
		if dateStr, ok := metadata["event_date"].(string); ok && dateStr != "" {
			if t := parseEventDate(dateStr); t != nil {
				return t
			}
		}
	}

	if m := datePatternRE.FindStringSubmatch(content); len(m) > 1 {
		if t := parseEventDate(m[1]); t != nil {
			return t
		}
	}

	return nil
}

func parseEventDate(s string) *time.Time {
	s = fmt.Sprintf("%s", s)
	for _, layout := range eventDateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

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
	// SimilarityThreshold is the minimum embedding similarity for recall deduplication.
	SimilarityThreshold = 0.60
	// ContradictionCandidateThreshold is the lower threshold used when fetching candidates
	ContradictionCandidateThreshold = 0.25
	// typeAwareCandidateLimit is the maximum number of same-type memories retrieved as
	typeAwareCandidateLimit = 15
	ReinforcementThreshold = 0.85
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
	// SessionPromotionThreshold is the reinforcement count at which a recurring
	SessionPromotionThreshold = 3
)

func sameAnchorCandidates(candidates []domain.MemoryWithScore, anchorID *uuid.UUID) []domain.MemoryWithScore {
	out := candidates[:0]
	for _, c := range candidates {
		if sameAnchor(c.AnchorID, anchorID) {
			out = append(out, c)
		}
	}
	return out
}

func sameAnchor(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

type GraphBuilder interface {
	OnMemoryCreated(ctx context.Context, memory *domain.Memory) error
}

type boostJob struct {
	id    uuid.UUID
	boost float32
}

type MemoryService struct {
	memoryStore           domain.MemoryStore
	agentStore            domain.AgentStore
	embeddingClient       domain.EmbeddingClient
	llmClient             domain.LLMClient
	contradictionDetector contradiction.Detector
	contradictionStore    domain.ContradictionStore
	policyEnforcer        PolicyEnforcer
	graphBuilder          GraphBuilder
	logger                *zap.Logger
	boostCh               chan boostJob
}

func NewMemoryService(ms domain.MemoryStore, as domain.AgentStore, ec domain.EmbeddingClient, lc domain.LLMClient, logger *zap.Logger) *MemoryService {
	var detector contradiction.Detector
	if lc != nil && os.Getenv("CONTRADICTION_MODE") != "embedding" {
		detector = contradiction.NewLLMDetector(lc)
	} else {
		detector = contradiction.NewEmbeddingDetector()
	}

	svc := &MemoryService{
		memoryStore:           ms,
		agentStore:            as,
		embeddingClient:       ec,
		llmClient:             lc,
		contradictionDetector: detector,
		logger:                logger,
		boostCh:               make(chan boostJob, 500),
	}
	for i := 0; i < 3; i++ {
		go svc.runBoostWorker()
	}
	return svc
}

func (s *MemoryService) runBoostWorker() {
	for job := range s.boostCh {
		if err := s.memoryStore.IncrementAccessAndBoost(context.Background(), job.id, job.boost); err != nil {
			s.logger.Debug("failed to reinforce memory on usage", zap.String("memory_id", job.id.String()), zap.Error(err))
		}
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

func (s *MemoryService) CreateCanon(ctx context.Context, m *domain.Memory) (*CreateResult, error) {
	m.Binding = domain.BindingCanon
	m.AnchorID = nil
	m.SessionID = nil
	return s.createWithOptions(ctx, m, false)
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

	if m.EventDate == nil {
		m.EventDate = extractEventDate(m.Content, m.Metadata)
	}

	// Classify memory type. Use LLM when available; fall back to keyword heuristic.
	// The heuristic is always fast (<1ms) and handles ~85% of cases correctly.
	if m.Type == "" {
		if s.llmClient != nil {
			classified, err := s.llmClient.Classify(ctx, m.Content)
			if err != nil {
				s.logger.Warn("LLM classification failed, using heuristic", zap.Error(err))
				m.Type = contradiction.ClassifyHeuristic(m.Content)
			} else {
				m.Type = classified
			}
		} else {
			m.Type = contradiction.ClassifyHeuristic(m.Content)
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

	const maxBeliefContentLen = 2000
	if len(m.Content) > maxBeliefContentLen {
		enableBeliefLogic = false
	}

	if enableBeliefLogic && m.Metadata != nil {
		if src, ok := m.Metadata["ingest_source"].(string); ok && src == "conversation" {
			enableBeliefLogic = false
		}
	}

	if enableBeliefLogic && len(m.Embedding) > 0 {
		similar, err := s.memoryStore.FindSimilar(ctx, m.AgentID, m.TenantID, m.Embedding, ContradictionCandidateThreshold)
		if err != nil {
			s.logger.Warn("failed to find similar beliefs", zap.Error(err))
		}

		if m.Type == domain.MemoryTypePreference || m.Type == domain.MemoryTypeConstraint {
			typed, err := s.memoryStore.GetRecentByType(ctx, m.AgentID, m.TenantID, m.Type, typeAwareCandidateLimit)
			if err != nil {
				s.logger.Warn("failed to fetch type-based candidates", zap.Error(err))
			} else {
				seen := make(map[string]bool, len(similar))
				for _, s := range similar {
					seen[s.ID.String()] = true
				}
				for _, t := range typed {
					if !seen[t.ID.String()] {
						similar = append(similar, t)
					}
				}
			}
		}

		similar = sameAnchorCandidates(similar, m.AnchorID)

		if len(similar) > 0 {
			s.logger.Info("contradiction candidates found", zap.Int("count", len(similar)))
			var reinforcementCandidate *domain.MemoryWithScore
			for i, existing := range similar {
				tension, err := s.contradictionDetector.CheckTension(
					ctx,
					existing.Content, m.Content,
					existing.Embedding, m.Embedding,
				)
				if err != nil {
					s.logger.Warn("tension check failed", zap.Error(err))
					continue
				}

				s.logger.Info("tension result",
					zap.String("existing", existing.Content),
					zap.String("incoming", m.Content),
					zap.Float32("similarity", existing.Score),
					zap.String("tension_type", string(tension.Type)),
					zap.Float32("tension_score", tension.TensionScore),
				)

				handled, err := s.handleTension(ctx, tension, &existing, m)
				if err != nil {
					return nil, err
				}
				if handled {
					s.logger.Info("contradiction handled", zap.String("type", string(tension.Type)), zap.String("existing_id", existing.ID.String()))
					return result, nil
				}

				if reinforcementCandidate == nil && existing.Score >= ReinforcementThreshold {
					reinforcementCandidate = &similar[i]
				}
			}

			if reinforcementCandidate != nil {
				newConfidence := reinforcementCandidate.Confidence + ReinforcementConfidenceBoost
				if newConfidence > MaxConfidence {
					newConfidence = MaxConfidence
				}
				newCount := reinforcementCandidate.ReinforcementCount + 1
				if err := s.memoryStore.UpdateReinforcement(ctx, reinforcementCandidate.ID, newConfidence, newCount); err != nil {
					s.logger.Warn("failed to reinforce belief", zap.Error(err))
				} else {
					m.ID = reinforcementCandidate.ID
					m.Confidence = newConfidence
					m.ReinforcementCount = newCount
					m.CreatedAt = reinforcementCandidate.CreatedAt
					m.UpdatedAt = reinforcementCandidate.UpdatedAt
					result.Reinforced = true
					result.ReinforcedMemoryID = reinforcementCandidate.ID


					if reinforcementCandidate.Binding == domain.BindingSession &&
						reinforcementCandidate.AnchorID != nil &&
						newCount >= SessionPromotionThreshold {
						if promoted, err := s.memoryStore.PromoteSessionToAnchor(ctx, reinforcementCandidate.ID); err != nil {
							s.logger.Warn("failed to promote session memory", zap.Error(err))
						} else if promoted {
							m.Binding = domain.BindingAnchored
							m.SessionID = nil
							s.logger.Info("promoted session memory to anchor",
								zap.String("memory_id", reinforcementCandidate.ID.String()))
						}
					}
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

func (s *MemoryService) Restore(ctx context.Context, id uuid.UUID) error {
	err := s.memoryStore.Restore(ctx, id)
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

	// Usage reinforcement: recalled memories get a small confidence boost (best-effort, non-blocking)
	for _, mem := range memories {
		tier := domain.ComputeTier(float64(mem.Confidence))
		behavior := domain.GetTierBehavior(tier)
		if behavior.SummarizeOnAccess {
			s.logger.Debug("cold tier memory accessed, candidate for summarization",
				zap.String("memory_id", mem.ID.String()),
				zap.String("tier", string(tier)))
		}
		select {
		case s.boostCh <- boostJob{id: mem.ID, boost: UsageReinforcementBoost}:
		default:
		}
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
		select {
		case s.boostCh <- boostJob{id: mem.ID, boost: UsageReinforcementBoost}:
		default:
		}
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
func (s *MemoryService) handleTension(ctx context.Context, tension *domain.TensionResult, existing *domain.MemoryWithScore, m *domain.Memory) (bool, error) {
	if tension == nil {
		return false, nil
	}

	switch tension.Type {
	case domain.ContradictionHard:
		if tension.TensionScore <= 0.25 {
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
		newOldConfidence := existing.Confidence - (ContradictionConfidencePenalty / 2)
		if newOldConfidence < MinConfidence {
			newOldConfidence = MinConfidence
		}
		if err := s.memoryStore.UpdateConfidence(ctx, existing.ID, newOldConfidence); err != nil {
			s.logger.Warn("failed to reduce soft-contradicted belief confidence", zap.Error(err))
		}
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
	HotCount     int                            `json:"hot_count"`
	WarmCount    int                            `json:"warm_count"`
	ColdCount    int                            `json:"cold_count"`
	ArchiveCount int                            `json:"archive_count"`
	Distribution map[domain.MemoryType]TierDist `json:"tier_distribution,omitempty"`
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
