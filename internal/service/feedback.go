package service

import (
	"context"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

var (
	ErrFeedbackMemoryIDMissing = errors.New("memory_id is required")
	ErrFeedbackAgentIDMissing  = errors.New("agent_id is required")
	ErrFeedbackInvalidSignal   = errors.New("invalid signal_type")
)

type FeedbackService struct {
	feedbackStore domain.FeedbackStore
	memoryStore   domain.MemoryStore
	agentStore    domain.AgentStore
}

func NewFeedbackService(fs domain.FeedbackStore, ms domain.MemoryStore, as domain.AgentStore) *FeedbackService {
	return &FeedbackService{
		feedbackStore: fs,
		memoryStore:   ms,
		agentStore:    as,
	}
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

	// Verify memory exists and belongs to tenant
	_, err = s.memoryStore.GetByID(ctx, f.MemoryID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrMemoryNotFound
		}
		return err
	}

	return s.feedbackStore.Create(ctx, f)
}

func (s *FeedbackService) GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.Feedback, error) {
	return s.feedbackStore.GetByAgentID(ctx, agentID)
}
