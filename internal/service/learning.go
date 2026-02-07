package service

import (
	"context"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type LearningService struct {
	memoryStore          domain.MemoryStore
	episodeStore         domain.EpisodeStore
	episodeMemUsageStore domain.EpisodeMemoryUsageStore
	mutationLogStore     domain.MutationLogStore
	learningStatsStore   domain.LearningStatsStore
	logger               *zap.Logger
}

func NewLearningService(
	memoryStore domain.MemoryStore,
	episodeStore domain.EpisodeStore,
	logger *zap.Logger,
) *LearningService {
	return &LearningService{
		memoryStore:  memoryStore,
		episodeStore: episodeStore,
		logger:       logger,
	}
}

func (s *LearningService) SetEpisodeMemoryUsageStore(store domain.EpisodeMemoryUsageStore) {
	s.episodeMemUsageStore = store
}

func (s *LearningService) SetMutationLogStore(store domain.MutationLogStore) {
	s.mutationLogStore = store
}

func (s *LearningService) SetLearningStatsStore(store domain.LearningStatsStore) {
	s.learningStatsStore = store
}

// RecordMemoryUsage records which memories were used during an episode.
func (s *LearningService) RecordMemoryUsage(ctx context.Context, episodeID uuid.UUID, memories []domain.MemoryWithScore, usageType domain.MemoryUsageType) error {
	if s.episodeMemUsageStore == nil {
		return nil
	}

	for _, mem := range memories {
		usage := &domain.EpisodeMemoryUsage{
			EpisodeID:      episodeID,
			MemoryID:       mem.ID,
			UsageType:      usageType,
			RelevanceScore: &mem.Score,
		}
		if err := s.episodeMemUsageStore.Create(ctx, usage); err != nil {
			s.logger.Warn("failed to record memory usage",
				zap.String("episode_id", episodeID.String()),
				zap.String("memory_id", mem.ID.String()),
				zap.Error(err),
			)
		}
	}

	return nil
}

// RecordOutcome records the outcome of an episode and propagates effects to used memories.
func (s *LearningService) RecordOutcome(ctx context.Context, record domain.OutcomeRecord) error {
	// Update episode outcome
	if s.episodeStore != nil {
		if err := s.episodeStore.UpdateOutcome(ctx, record.EpisodeID, record.Outcome, ""); err != nil {
			s.logger.Warn("failed to update episode outcome", zap.Error(err))
		}
	}

	// Determine feedback effect based on outcome
	var effect domain.FeedbackEffect
	var feedbackType domain.FeedbackType

	switch record.Outcome {
	case domain.OutcomeSuccess:
		effect = domain.FeedbackEffects[domain.FeedbackTypeHelpful]
		feedbackType = domain.FeedbackTypeHelpful
	case domain.OutcomeFailure:
		effect = domain.FeedbackEffects[domain.FeedbackTypeUnhelpful]
		feedbackType = domain.FeedbackTypeUnhelpful
	default:
		// Neutral outcome - no effect
		return nil
	}

	// Apply effect to all memories used
	for _, memID := range record.MemoriesUsed {
		if err := s.applyOutcomeEffect(ctx, memID, effect, feedbackType, record.EpisodeID); err != nil {
			s.logger.Warn("failed to apply outcome effect to memory",
				zap.String("memory_id", memID.String()),
				zap.Error(err),
			)
		}
	}

	return nil
}

func (s *LearningService) applyOutcomeEffect(ctx context.Context, memID uuid.UUID, effect domain.FeedbackEffect, feedbackType domain.FeedbackType, episodeID uuid.UUID) error {
	// Get current memory state - we need tenant ID but don't have it, so we search all
	memory, err := s.getMemoryByIDWithoutTenant(ctx, memID)
	if err != nil {
		return err
	}

	oldConfidence := memory.Confidence
	oldReinforcement := memory.ReinforcementCount

	newConfidence := memory.Confidence + effect.ConfidenceDelta
	if newConfidence > MaxConfidence {
		newConfidence = MaxConfidence
	}
	if newConfidence < MinConfidence {
		newConfidence = MinConfidence
	}

	newReinforcement := memory.ReinforcementCount + effect.ReinforcementDelta
	if newReinforcement < 0 {
		newReinforcement = 0
	}

	// Update memory
	if err := s.memoryStore.UpdateReinforcement(ctx, memory.ID, newConfidence, newReinforcement); err != nil {
		return err
	}

	// Log mutation
	if s.mutationLogStore != nil {
		mutation := &domain.MutationLog{
			MemoryID:              memory.ID,
			AgentID:               memory.AgentID,
			MutationType:          domain.MutationOutcome,
			SourceType:            domain.MutationSourceSystem,
			SourceID:              &episodeID,
			OldConfidence:         &oldConfidence,
			NewConfidence:         &newConfidence,
			OldReinforcementCount: &oldReinforcement,
			NewReinforcementCount: &newReinforcement,
			Reason:                "outcome: " + string(feedbackType),
		}
		if err := s.mutationLogStore.Create(ctx, mutation); err != nil {
			s.logger.Warn("failed to log mutation", zap.Error(err))
		}
	}

	s.logger.Debug("applied outcome effect to memory",
		zap.String("memory_id", memory.ID.String()),
		zap.String("feedback_type", string(feedbackType)),
		zap.Float32("old_confidence", oldConfidence),
		zap.Float32("new_confidence", newConfidence),
	)

	return nil
}

// getMemoryByIDWithoutTenant is a helper to find a memory by ID.
func (s *LearningService) getMemoryByIDWithoutTenant(ctx context.Context, memID uuid.UUID) (*domain.Memory, error) {
	// Try with nil tenant ID first
	mem, err := s.memoryStore.GetByID(ctx, memID, uuid.Nil)
	if err == nil {
		return mem, nil
	}

	// For testing/compatibility, iterate agent memories
	agentIDs, err := s.memoryStore.ListDistinctAgentIDs(ctx)
	if err != nil {
		return nil, err
	}

	for _, agentID := range agentIDs {
		memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
		if err != nil {
			continue
		}
		for i := range memories {
			if memories[i].ID == memID {
				return &memories[i], nil
			}
		}
	}

	return nil, ErrMemoryNotFound
}

// GetLearningStats returns aggregated learning statistics for an agent.
func (s *LearningService) GetLearningStats(ctx context.Context, agentID uuid.UUID) (*domain.LearningStats, error) {
	if s.learningStatsStore == nil {
		return nil, nil
	}
	return s.learningStatsStore.GetLatest(ctx, agentID)
}

// ComputeLearningStats computes and stores learning statistics for an agent over a time period.
func (s *LearningService) ComputeLearningStats(ctx context.Context, agentID uuid.UUID, periodStart, periodEnd time.Time) (*domain.LearningStats, error) {
	if s.mutationLogStore == nil || s.learningStatsStore == nil {
		return nil, nil
	}

	// Get mutations in the period
	mutations, err := s.mutationLogStore.GetByAgentID(ctx, agentID, periodStart, 1000)
	if err != nil {
		return nil, err
	}

	stats := &domain.LearningStats{
		AgentID:     agentID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	}

	for _, m := range mutations {
		if m.CreatedAt.After(periodEnd) {
			continue
		}

		// Count confidence changes
		if m.OldConfidence != nil && m.NewConfidence != nil {
			if *m.NewConfidence > *m.OldConfidence {
				stats.ConfidenceIncreases++
			} else if *m.NewConfidence < *m.OldConfidence {
				stats.ConfidenceDecreases++
			}
		}

		// Count by mutation type
		switch m.MutationType {
		case domain.MutationReinforcement:
			stats.MemoriesReinforced++
		case domain.MutationFeedback:
			// Parse feedback type from reason
			if len(m.Reason) > 10 && m.Reason[:9] == "feedback:" {
				fbType := m.Reason[10:]
				switch domain.FeedbackType(fbType) {
				case domain.FeedbackTypeHelpful:
					stats.HelpfulCount++
				case domain.FeedbackTypeUnhelpful:
					stats.UnhelpfulCount++
				case domain.FeedbackTypeIgnored:
					stats.IgnoredCount++
				case domain.FeedbackTypeContradicted:
					stats.ContradictedCount++
				case domain.FeedbackTypeOutdated:
					stats.OutdatedCount++
				}
			}
		case domain.MutationOutcome:
			// Parse outcome type
			if len(m.Reason) > 9 && m.Reason[:8] == "outcome:" {
				outcomeType := m.Reason[9:]
				switch domain.FeedbackType(outcomeType) {
				case domain.FeedbackTypeHelpful:
					stats.SuccessCount++
				case domain.FeedbackTypeUnhelpful:
					stats.FailureCount++
				default:
					stats.NeutralCount++
				}
			}
		}
	}

	// Compute derived metrics
	totalChanges := float32(stats.ConfidenceIncreases + stats.ConfidenceDecreases)
	if totalChanges > 0 {
		velocity := float32(stats.ConfidenceIncreases-stats.ConfidenceDecreases) / totalChanges
		stats.LearningVelocity = &velocity
	}

	// Store stats
	if err := s.learningStatsStore.Upsert(ctx, stats); err != nil {
		return nil, err
	}

	return stats, nil
}
