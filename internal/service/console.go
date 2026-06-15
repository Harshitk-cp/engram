package service

import (
	"context"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ConsoleService aggregates existing reads into console/dashboard-friendly
// summaries. It owns no new state — it composes memory, contradiction, and
// learning-stats stores.
type ConsoleService struct {
	memoryStore        domain.MemoryStore
	contradictionStore domain.ContradictionStore
	learningStatsStore domain.LearningStatsStore
	logger             *zap.Logger
}

func NewConsoleService(ms domain.MemoryStore, cs domain.ContradictionStore, lss domain.LearningStatsStore, logger *zap.Logger) *ConsoleService {
	return &ConsoleService{memoryStore: ms, contradictionStore: cs, learningStatsStore: lss, logger: logger}
}

// DashboardSummary is the single-call knowledge-health overview for an agent.
type DashboardSummary struct {
	AgentID            uuid.UUID                 `json:"agent_id"`
	TierCounts         map[domain.MemoryTier]int `json:"tier_counts"`
	TotalMemories      int                       `json:"total_memories"`
	NeedsReviewCount   int                       `json:"needs_review_count"`
	ContradictionCount int                       `json:"contradiction_count"`
	LearningVelocity   *float32                  `json:"learning_velocity,omitempty"`
	StabilityScore     *float32                  `json:"stability_score,omitempty"`
}

// Dashboard returns a consolidated knowledge-health summary for an agent.
func (s *ConsoleService) Dashboard(ctx context.Context, agentID, tenantID uuid.UUID) (*DashboardSummary, error) {
	tierCounts, err := s.memoryStore.GetTierCounts(ctx, agentID, tenantID)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, c := range tierCounts {
		total += c
	}

	reviewCount, err := s.memoryStore.CountNeedsReview(ctx, agentID, tenantID)
	if err != nil {
		return nil, err
	}

	contradictionCount, err := s.contradictionStore.CountByAgent(ctx, agentID, tenantID)
	if err != nil {
		return nil, err
	}

	summary := &DashboardSummary{
		AgentID:            agentID,
		TierCounts:         tierCounts,
		TotalMemories:      total,
		NeedsReviewCount:   reviewCount,
		ContradictionCount: contradictionCount,
	}

	if s.learningStatsStore != nil {
		if latest, err := s.learningStatsStore.GetLatest(ctx, agentID); err == nil && latest != nil {
			summary.LearningVelocity = latest.LearningVelocity
			summary.StabilityScore = latest.StabilityScore
		}
	}

	return summary, nil
}

// MemoryPage is a paginated slice of an agent's memories.
type MemoryPage struct {
	Items  []domain.Memory `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// Memories lists an agent's memories with optional filters + pagination.
func (s *ConsoleService) Memories(ctx context.Context, agentID, tenantID uuid.UUID, f domain.MemoryFilter, limit, offset int) (*MemoryPage, error) {
	items, total, err := s.memoryStore.ListByAgentFiltered(ctx, agentID, tenantID, f, limit, offset)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []domain.Memory{}
	}
	return &MemoryPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// Snapshot is the agent's belief set reconstructed as of a past instant.
type Snapshot struct {
	AsOf    time.Time             `json:"as_of"`
	Total   int                   `json:"total"`
	Beliefs []domain.BeliefAtTime `json:"beliefs"`
}

// SnapshotAsOf reconstructs what the agent believed at `at` (transaction time).
func (s *ConsoleService) SnapshotAsOf(ctx context.Context, agentID, tenantID uuid.UUID, at time.Time, limit int) (*Snapshot, error) {
	beliefs, total, err := s.memoryStore.BeliefsAsOf(ctx, agentID, tenantID, at, limit)
	if err != nil {
		return nil, err
	}
	if beliefs == nil {
		beliefs = []domain.BeliefAtTime{}
	}
	return &Snapshot{AsOf: at, Total: total, Beliefs: beliefs}, nil
}

// ContradictingBelief is a belief that conflicts with one under review.
type ContradictingBelief struct {
	MemoryID   uuid.UUID `json:"memory_id"`
	Content    string    `json:"content,omitempty"`
	Confidence float32   `json:"confidence,omitempty"`
}

// ReviewItem is a flagged belief plus the beliefs it conflicts with, so an
// operator can resolve it via the admin endpoints.
type ReviewItem struct {
	Memory         domain.Memory         `json:"memory"`
	Tier           domain.MemoryTier     `json:"tier"`
	Contradictions []ContradictingBelief `json:"contradictions,omitempty"`
}

// Contradictions returns all detected contradiction pairs for an agent (the
// source of truth — independent of the needs_review flag).
func (s *ConsoleService) Contradictions(ctx context.Context, agentID, tenantID uuid.UUID, limit int) ([]domain.ContradictionPair, error) {
	pairs, err := s.contradictionStore.ListByAgent(ctx, agentID, tenantID, limit)
	if err != nil {
		return nil, err
	}
	if pairs == nil {
		pairs = []domain.ContradictionPair{}
	}
	return pairs, nil
}

// ReviewQueue returns memories flagged needs_review, each with the beliefs it
// contradicts.
func (s *ConsoleService) ReviewQueue(ctx context.Context, agentID, tenantID uuid.UUID, limit int) ([]ReviewItem, error) {
	if limit <= 0 {
		limit = 50
	}
	mems, err := s.memoryStore.GetNeedsReview(ctx, agentID, tenantID, limit)
	if err != nil {
		return nil, err
	}

	items := make([]ReviewItem, 0, len(mems))
	for i := range mems {
		mem := mems[i]
		item := ReviewItem{Memory: mem, Tier: domain.ComputeTier(float64(mem.Confidence))}

		if contradictions, err := s.contradictionStore.GetByBeliefID(ctx, mem.ID); err == nil {
			for _, c := range contradictions {
				cb := ContradictingBelief{MemoryID: c.ContradictedByID}
				if other, err := s.memoryStore.GetByIDOnly(ctx, c.ContradictedByID); err == nil && other != nil {
					cb.Content = other.Content
					cb.Confidence = other.Confidence
				}
				item.Contradictions = append(item.Contradictions, cb)
			}
		}
		items = append(items, item)
	}
	return items, nil
}
