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
	ErrPolicyNotFound       = errors.New("policy not found")
	ErrPolicyInvalidType    = errors.New("invalid memory type in policy")
	ErrPolicyMaxMemories    = errors.New("max_memories must be positive")
	ErrPolicyPriorityWeight = errors.New("priority_weight must be positive")
)

type PolicyService struct {
	policyStore  domain.PolicyStore
	memoryStore  domain.MemoryStore
	agentStore   domain.AgentStore
	llmClient    domain.LLMClient
	embClient    domain.EmbeddingClient
	logger       *zap.Logger
}

func NewPolicyService(ps domain.PolicyStore, ms domain.MemoryStore, as domain.AgentStore, lc domain.LLMClient, ec domain.EmbeddingClient, logger *zap.Logger) *PolicyService {
	return &PolicyService{
		policyStore:  ps,
		memoryStore:  ms,
		agentStore:   as,
		llmClient:    lc,
		embClient:    ec,
		logger:       logger,
	}
}

func (s *PolicyService) GetPolicies(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Policy, error) {
	// Verify agent belongs to tenant
	_, err := s.agentStore.GetByID(ctx, agentID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	policies, err := s.policyStore.GetByAgentID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if policies == nil {
		policies = []domain.Policy{}
	}
	return policies, nil
}

func (s *PolicyService) UpsertPolicies(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, policies []domain.Policy) ([]domain.Policy, error) {
	// Verify agent belongs to tenant
	_, err := s.agentStore.GetByID(ctx, agentID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Validate all policies before upserting
	for _, p := range policies {
		if !domain.ValidMemoryType(string(p.MemoryType)) {
			return nil, ErrPolicyInvalidType
		}
		if p.MaxMemories <= 0 {
			return nil, ErrPolicyMaxMemories
		}
		if p.PriorityWeight <= 0 {
			return nil, ErrPolicyPriorityWeight
		}
	}

	var result []domain.Policy
	for i := range policies {
		policies[i].AgentID = agentID
		if err := s.policyStore.Upsert(ctx, &policies[i]); err != nil {
			return nil, err
		}
		result = append(result, policies[i])
	}

	return result, nil
}

// EnforceOnCreate checks and enforces policy limits after a memory is created.
// If the count exceeds max_memories and auto_summarize is enabled, it summarizes
// the oldest memories and replaces them with a single summarized memory.
// If auto_summarize is disabled, it deletes the oldest memories exceeding the limit.
func (s *PolicyService) EnforceOnCreate(ctx context.Context, m *domain.Memory) error {
	policy, err := s.policyStore.GetByAgentIDAndType(ctx, m.AgentID, m.Type)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// No policy for this type — no enforcement needed
			return nil
		}
		return err
	}

	count, err := s.memoryStore.CountByAgentAndType(ctx, m.AgentID, m.Type)
	if err != nil {
		return err
	}

	if count <= policy.MaxMemories {
		return nil
	}

	excess := count - policy.MaxMemories
	oldest, err := s.memoryStore.ListOldestByAgentAndType(ctx, m.AgentID, m.Type, excess)
	if err != nil {
		return err
	}

	if policy.AutoSummarize && s.llmClient != nil && len(oldest) > 0 {
		summary, err := s.llmClient.Summarize(ctx, oldest)
		if err != nil {
			s.logger.Warn("failed to summarize memories during policy enforcement", zap.Error(err))
		} else {
			// Create a summarized memory to replace the ones being deleted
			summarized := &domain.Memory{
				AgentID:    m.AgentID,
				TenantID:   m.TenantID,
				Type:       m.Type,
				Content:    summary,
				Source:     "auto-summarize",
				Confidence: 0.8,
			}
			if s.embClient != nil {
				emb, err := s.embClient.Embed(ctx, summary)
				if err != nil {
					s.logger.Warn("failed to embed summarized memory", zap.Error(err))
				} else {
					summarized.Embedding = emb
				}
			}
			if err := s.memoryStore.Create(ctx, summarized); err != nil {
				s.logger.Warn("failed to store summarized memory", zap.Error(err))
			}
		}
	}

	// Delete the excess oldest memories
	for _, old := range oldest {
		if err := s.memoryStore.Delete(ctx, old.ID, old.TenantID); err != nil {
			s.logger.Warn("failed to delete excess memory during enforcement",
				zap.String("memory_id", old.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

func (s *PolicyService) GetTypeWeights(ctx context.Context, agentID uuid.UUID) map[domain.MemoryType]float64 {
	policies, err := s.policyStore.GetByAgentID(ctx, agentID)
	if err != nil {
		s.logger.Debug("failed to load policies for type weights", zap.Error(err))
		return nil
	}

	weights := make(map[domain.MemoryType]float64)
	for _, p := range policies {
		if p.PriorityWeight > 0 {
			weights[p.MemoryType] = p.PriorityWeight
		}
	}

	if len(weights) == 0 {
		return nil
	}
	return weights
}
