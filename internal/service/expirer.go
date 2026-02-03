package service

import (
	"context"
	"sync"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"go.uber.org/zap"
)

const defaultExpirerInterval = 1 * time.Hour

type ExpirerService struct {
	memoryStore domain.MemoryStore
	policyStore domain.PolicyStore
	feedbackStore domain.FeedbackStore
	logger      *zap.Logger

	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewExpirerService(ms domain.MemoryStore, ps domain.PolicyStore, fs domain.FeedbackStore, logger *zap.Logger) *ExpirerService {
	return &ExpirerService{
		memoryStore:   ms,
		policyStore:   ps,
		feedbackStore: fs,
		logger:        logger,
		interval:      defaultExpirerInterval,
		stopCh:        make(chan struct{}),
	}
}

func (s *ExpirerService) SetInterval(d time.Duration) {
	s.interval = d
}

// Start runs the expirer on a periodic schedule in a background goroutine.
func (s *ExpirerService) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.logger.Info("memory expirer started", zap.Duration("interval", s.interval))

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				s.run(ctx)
				cancel()
			case <-s.stopCh:
				s.logger.Info("memory expirer stopped")
				return
			}
		}
	}()
}

// Stop gracefully stops the expirer.
func (s *ExpirerService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *ExpirerService) run(ctx context.Context) {
	// 1. Delete memories past their explicit expires_at timestamp
	deleted, err := s.memoryStore.DeleteExpired(ctx)
	if err != nil {
		s.logger.Error("failed to delete expired memories", zap.Error(err))
	} else if deleted > 0 {
		s.logger.Info("deleted expired memories", zap.Int64("count", deleted))
	}

	// 2. Delete memories past retention_days based on policies
	// Get all agents that have feedback (they have policies to enforce)
	agentIDs, err := s.feedbackStore.ListDistinctAgentIDs(ctx)
	if err != nil {
		s.logger.Error("failed to list agent IDs for retention", zap.Error(err))
		return
	}

	for _, agentID := range agentIDs {
		policies, err := s.policyStore.GetByAgentID(ctx, agentID)
		if err != nil {
			s.logger.Warn("failed to get policies for retention check",
				zap.String("agent_id", agentID.String()),
				zap.Error(err))
			continue
		}

		for _, policy := range policies {
			if policy.RetentionDays == nil || *policy.RetentionDays <= 0 {
				continue
			}

			deleted, err := s.memoryStore.DeleteByRetention(ctx, agentID, policy.MemoryType, *policy.RetentionDays)
			if err != nil {
				s.logger.Warn("failed to delete memories by retention",
					zap.String("agent_id", agentID.String()),
					zap.String("memory_type", string(policy.MemoryType)),
					zap.Error(err))
			} else if deleted > 0 {
				s.logger.Info("deleted memories past retention",
					zap.String("agent_id", agentID.String()),
					zap.String("memory_type", string(policy.MemoryType)),
					zap.Int("retention_days", *policy.RetentionDays),
					zap.Int64("count", deleted))
			}
		}
	}
}
