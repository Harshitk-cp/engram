package service

import (
	"context"
	"errors"

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

type MemoryService struct {
	memoryStore        domain.MemoryStore
	agentStore         domain.AgentStore
	embeddingClient    domain.EmbeddingClient
	llmClient          domain.LLMClient
	contradictionStore domain.ContradictionStore
	policyEnforcer     PolicyEnforcer
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
			// Check for reinforcement or contradiction
			for _, existing := range similar {
				// Check for contradiction using LLM
				if s.llmClient != nil {
					contradicts, err := s.llmClient.CheckContradiction(ctx, existing.Content, m.Content)
					if err != nil {
						s.logger.Warn("contradiction check failed", zap.Error(err))
						continue
					}

					if contradicts {
						// Handle contradiction: decrease old belief confidence, create new with lower confidence
						newOldConfidence := existing.Confidence - ContradictionConfidencePenalty
						if newOldConfidence < MinConfidence {
							newOldConfidence = MinConfidence
						}
						if err := s.memoryStore.UpdateConfidence(ctx, existing.ID, newOldConfidence); err != nil {
							s.logger.Warn("failed to update contradicted belief confidence", zap.Error(err))
						}

						// Set new belief confidence
						m.Confidence = NewContradictingBeliefConfidence

						// Create the new contradicting belief
						if err := s.memoryStore.Create(ctx, m); err != nil {
							return nil, err
						}

						// Record the contradiction
						if s.contradictionStore != nil {
							if err := s.contradictionStore.Create(ctx, existing.ID, m.ID); err != nil {
								s.logger.Warn("failed to record contradiction", zap.Error(err))
							}
						}

						// Enforce policies
						if s.policyEnforcer != nil {
							if err := s.policyEnforcer.EnforceOnCreate(ctx, m); err != nil {
								s.logger.Warn("policy enforcement failed after memory creation", zap.Error(err))
							}
						}

						return result, nil
					}
				}

				// No contradiction detected with this similar belief - reinforce it
				newConfidence := existing.Confidence + ReinforcementConfidenceBoost
				if newConfidence > MaxConfidence {
					newConfidence = MaxConfidence
				}
				newCount := existing.ReinforcementCount + 1

				if err := s.memoryStore.UpdateReinforcement(ctx, existing.ID, newConfidence, newCount); err != nil {
					s.logger.Warn("failed to reinforce belief", zap.Error(err))
				} else {
					// Return the existing memory info as reinforced
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

	emb, err := s.embeddingClient.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	memories, err := s.memoryStore.Recall(ctx, emb, agentID, tenantID, opts)
	if err != nil {
		return nil, err
	}

	// Usage reinforcement: recalled memories get a small confidence boost
	for _, mem := range memories {
		go func(id uuid.UUID) {
			if err := s.memoryStore.IncrementAccessAndBoost(context.Background(), id, UsageReinforcementBoost); err != nil {
				s.logger.Debug("failed to reinforce memory on usage", zap.String("memory_id", id.String()), zap.Error(err))
			}
		}(mem.ID)
	}

	return memories, nil
}

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
		result := ExtractResult{
			Type:       e.Type,
			Content:    e.Content,
			Confidence: e.Confidence,
			Stored:     false,
		}

		if autoStore {
			mem := &domain.Memory{
				AgentID:    agentID,
				TenantID:   tenantID,
				Type:       e.Type,
				Content:    e.Content,
				Confidence: e.Confidence,
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
