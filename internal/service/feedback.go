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
	ErrFeedbackMemoryIDMissing = errors.New("memory_id is required")
	ErrFeedbackAgentIDMissing  = errors.New("agent_id is required")
	ErrFeedbackInvalidSignal   = errors.New("invalid signal_type")
)

type FeedbackService struct {
	feedbackStore    domain.FeedbackStore
	memoryStore      domain.MemoryStore
	agentStore       domain.AgentStore
	mutationLogStore domain.MutationLogStore
	uow              *store.UnitOfWork
	logger           *zap.Logger
}

func NewFeedbackService(fs domain.FeedbackStore, ms domain.MemoryStore, as domain.AgentStore) *FeedbackService {
	return &FeedbackService{
		feedbackStore: fs,
		memoryStore:   ms,
		agentStore:    as,
		logger:        zap.NewNop(),
	}
}

func (s *FeedbackService) SetMutationLogStore(mls domain.MutationLogStore) {
	s.mutationLogStore = mls
}

func (s *FeedbackService) SetUnitOfWork(uow *store.UnitOfWork) {
	s.uow = uow
}

func (s *FeedbackService) SetLogger(logger *zap.Logger) {
	s.logger = logger
}

func (s *FeedbackService) Create(ctx context.Context, f *domain.Feedback, tenantID uuid.UUID) error {
	if f.MemoryID == uuid.Nil {
		return ErrFeedbackMemoryIDMissing
	}
	if f.AgentID == uuid.Nil {
		return ErrFeedbackAgentIDMissing
	}
	if !domain.ValidFeedbackType(string(f.SignalType)) {
		return ErrFeedbackInvalidSignal
	}

	// Verify agent belongs to tenant
	_, err := s.agentStore.GetByID(ctx, f.AgentID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrAgentNotFound
		}
		return err
	}

	// Verify memory exists and get current state
	memory, err := s.memoryStore.GetByID(ctx, f.MemoryID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrMemoryNotFound
		}
		return err
	}

	if err := s.feedbackStore.Create(ctx, f); err != nil {
		return err
	}

	effect, hasEffect := domain.FeedbackEffects[f.SignalType]
	if hasEffect {
		s.applyFeedbackEffect(ctx, f, memory, effect)
	}

	return nil
}

func (s *FeedbackService) applyFeedbackEffect(ctx context.Context, f *domain.Feedback, memory *domain.Memory, effect domain.FeedbackEffect) {
	oldConfidence := memory.Confidence
	oldReinforcement := memory.ReinforcementCount

	newConfidence := ApplyLogOddsDelta(memory.Confidence, effect.LogOddsDelta)

	newReinforcement := memory.ReinforcementCount + effect.ReinforcementDelta
	if newReinforcement < 0 {
		newReinforcement = 0
	}

	mutation := &domain.MutationLog{
		MemoryID:              memory.ID,
		AgentID:               f.AgentID,
		TenantID:              &memory.TenantID,
		MutationType:          domain.MutationFeedback,
		SourceType:            domain.MutationSourceExplicit,
		SourceID:              &f.ID,
		OldConfidence:         &oldConfidence,
		NewConfidence:         &newConfidence,
		OldReinforcementCount: &oldReinforcement,
		NewReinforcementCount: &newReinforcement,
		Reason:                "feedback: " + string(f.SignalType),
	}

	if s.uow != nil {
		// Atomic path: memory update, review flag, and audit row commit together.
		if err := s.uow.Do(ctx, func(st *store.TxStores) error {
			if err := st.Memory.UpdateReinforcement(ctx, memory.ID, newConfidence, newReinforcement); err != nil {
				return err
			}
			if effect.TriggerReview {
				if err := st.Memory.SetNeedsReview(ctx, memory.ID, true); err != nil {
					return err
				}
			}
			return st.MutationLog.Create(ctx, mutation)
		}); err != nil {
			s.logger.Warn("failed to apply feedback effect", zap.Error(err))
			return
		}
	} else {
		// Fallback (no unit of work, e.g. unit tests): non-atomic.
		if err := s.memoryStore.UpdateReinforcement(ctx, memory.ID, newConfidence, newReinforcement); err != nil {
			s.logger.Warn("failed to update memory on feedback", zap.Error(err))
			return
		}
		if effect.TriggerReview {
			if err := s.memoryStore.SetNeedsReview(ctx, memory.ID, true); err != nil {
				s.logger.Warn("failed to set needs_review flag", zap.Error(err))
			}
		}
		if s.mutationLogStore != nil {
			if err := s.mutationLogStore.Create(ctx, mutation); err != nil {
				s.logger.Warn("failed to log mutation", zap.Error(err))
			}
		}
	}

	s.logger.Debug("applied feedback effect",
		zap.String("memory_id", memory.ID.String()),
		zap.String("signal_type", string(f.SignalType)),
		zap.Float32("old_confidence", oldConfidence),
		zap.Float32("new_confidence", newConfidence),
		zap.Int("old_reinforcement", oldReinforcement),
		zap.Int("new_reinforcement", newReinforcement),
	)
}

func (s *FeedbackService) GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.Feedback, error) {
	return s.feedbackStore.GetByAgentID(ctx, agentID)
}
