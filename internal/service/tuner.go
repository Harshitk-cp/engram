package service

import (
	"context"
	"sync"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// Thresholds for policy adjustment
	ignoredThreshold  = 0.7
	helpfulThreshold  = 0.7
	unhelpfulThreshold = 0.7

	// Adjustment amounts
	weightDelta       = 0.1
	maxMemoriesReduce = 10

	// Minimum values to prevent going to zero
	minPriorityWeight = 0.1
	minMaxMemories    = 10

	// Default tuner interval
	defaultTunerInterval = 1 * time.Hour

	// Minimum feedback count before tuner acts
	minFeedbackCount = 10
)

type TunerService struct {
	feedbackStore domain.FeedbackStore
	policyStore   domain.PolicyStore
	logger        *zap.Logger

	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewTunerService(fs domain.FeedbackStore, ps domain.PolicyStore, logger *zap.Logger) *TunerService {
	return &TunerService{
		feedbackStore: fs,
		policyStore:   ps,
		logger:        logger,
		interval:      defaultTunerInterval,
		stopCh:        make(chan struct{}),
	}
}

func (s *TunerService) SetInterval(d time.Duration) {
	s.interval = d
}

// Start runs the tuner on a periodic schedule in a background goroutine.
func (s *TunerService) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.logger.Info("policy tuner started", zap.Duration("interval", s.interval))

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := s.RunAll(ctx); err != nil {
					s.logger.Error("policy tuner run failed", zap.Error(err))
				}
				cancel()
			case <-s.stopCh:
				s.logger.Info("policy tuner stopped")
				return
			}
		}
	}()
}

// Stop gracefully stops the tuner.
func (s *TunerService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// RunAll runs the tuner for all agents that have feedback.
func (s *TunerService) RunAll(ctx context.Context) error {
	agentIDs, err := s.feedbackStore.ListDistinctAgentIDs(ctx)
	if err != nil {
		return err
	}

	for _, agentID := range agentIDs {
		if err := s.RunForAgent(ctx, agentID); err != nil {
			s.logger.Warn("tuner failed for agent",
				zap.String("agent_id", agentID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// RunForAgent analyzes feedback for a single agent and adjusts policies.
func (s *TunerService) RunForAgent(ctx context.Context, agentID uuid.UUID) error {
	// Check if there's enough feedback to act on
	totalCount, err := s.feedbackStore.CountByAgentID(ctx, agentID)
	if err != nil {
		return err
	}
	if totalCount < minFeedbackCount {
		return nil
	}

	aggregates, err := s.feedbackStore.GetAggregatesByAgentID(ctx, agentID)
	if err != nil {
		return err
	}

	// Group aggregates by memory type
	type typeStats struct {
		used      int
		ignored   int
		helpful   int
		unhelpful int
		total     int
	}

	statsByType := make(map[domain.MemoryType]*typeStats)
	for _, agg := range aggregates {
		stats, ok := statsByType[agg.MemoryType]
		if !ok {
			stats = &typeStats{}
			statsByType[agg.MemoryType] = stats
		}
		switch agg.SignalType {
		case domain.FeedbackTypeUsed:
			stats.used += agg.Count
		case domain.FeedbackTypeIgnored:
			stats.ignored += agg.Count
		case domain.FeedbackTypeHelpful:
			stats.helpful += agg.Count
		case domain.FeedbackTypeUnhelpful:
			stats.unhelpful += agg.Count
		}
		stats.total += agg.Count
	}

	// Apply rules for each memory type
	for memType, stats := range statsByType {
		if stats.total == 0 {
			continue
		}

		// Get current policy (or create default)
		policy, err := s.policyStore.GetByAgentIDAndType(ctx, agentID, memType)
		if err != nil {
			// No policy exists — create a default one to tune
			policy = &domain.Policy{
				AgentID:        agentID,
				MemoryType:     memType,
				MaxMemories:    100,
				PriorityWeight: 1.0,
				AutoSummarize:  false,
			}
		}

		adjusted := false

		// Rule 1: >70% ignored → decrease priority_weight
		ignoredRate := float64(stats.ignored) / float64(stats.total)
		if ignoredRate > ignoredThreshold {
			newWeight := policy.PriorityWeight - weightDelta
			if newWeight < minPriorityWeight {
				newWeight = minPriorityWeight
			}
			if newWeight != policy.PriorityWeight {
				s.logger.Info("tuner: reducing priority_weight",
					zap.String("agent_id", agentID.String()),
					zap.String("memory_type", string(memType)),
					zap.Float64("old", policy.PriorityWeight),
					zap.Float64("new", newWeight))
				policy.PriorityWeight = newWeight
				adjusted = true
			}
		}

		// Rule 2: >70% helpful → increase priority_weight
		helpfulRate := float64(stats.helpful) / float64(stats.total)
		if helpfulRate > helpfulThreshold {
			newWeight := policy.PriorityWeight + weightDelta
			s.logger.Info("tuner: increasing priority_weight",
				zap.String("agent_id", agentID.String()),
				zap.String("memory_type", string(memType)),
				zap.Float64("old", policy.PriorityWeight),
				zap.Float64("new", newWeight))
			policy.PriorityWeight = newWeight
			adjusted = true
		}

		// Rule 3: >70% unhelpful → reduce max_memories
		unhelpfulRate := float64(stats.unhelpful) / float64(stats.total)
		if unhelpfulRate > unhelpfulThreshold {
			newMax := policy.MaxMemories - maxMemoriesReduce
			if newMax < minMaxMemories {
				newMax = minMaxMemories
			}
			if newMax != policy.MaxMemories {
				s.logger.Info("tuner: reducing max_memories",
					zap.String("agent_id", agentID.String()),
					zap.String("memory_type", string(memType)),
					zap.Int("old", policy.MaxMemories),
					zap.Int("new", newMax))
				policy.MaxMemories = newMax
				adjusted = true
			}
		}

		if adjusted {
			if err := s.policyStore.Upsert(ctx, policy); err != nil {
				s.logger.Warn("tuner: failed to update policy",
					zap.String("agent_id", agentID.String()),
					zap.String("memory_type", string(memType)),
					zap.Error(err))
			}
		}
	}

	return nil
}
