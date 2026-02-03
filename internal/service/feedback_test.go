package service

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

func setupFeedbackTest() (*FeedbackService, *mockFeedbackStore, *mockMemoryStore, uuid.UUID, uuid.UUID) {
	agentStore := newMockAgentStore()
	memStore := newMockMemoryStore()
	feedbackStore := newMockFeedbackStore()

	svc := NewFeedbackService(feedbackStore, memStore, agentStore)

	tenantID := uuid.New()
	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = agentStore.Create(context.Background(), agent)

	return svc, feedbackStore, memStore, tenantID, agent.ID
}

func TestFeedbackService_Create(t *testing.T) {
	svc, feedbackStore, memStore, tenantID, agentID := setupFeedbackTest()
	ctx := context.Background()

	// Create a memory first
	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "test", Type: domain.MemoryTypeFact}
	_ = memStore.Create(ctx, mem)

	feedback := &domain.Feedback{
		MemoryID:   mem.ID,
		AgentID:    agentID,
		SignalType: domain.FeedbackTypeHelpful,
	}

	if err := svc.Create(ctx, feedback, tenantID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if feedback.ID == uuid.Nil {
		t.Fatal("expected feedback ID to be set")
	}
	if len(feedbackStore.feedbacks) != 1 {
		t.Fatalf("expected 1 feedback in store, got %d", len(feedbackStore.feedbacks))
	}
}

func TestFeedbackService_Create_MissingMemoryID(t *testing.T) {
	svc, _, _, tenantID, agentID := setupFeedbackTest()

	feedback := &domain.Feedback{
		AgentID:    agentID,
		SignalType: domain.FeedbackTypeHelpful,
	}

	err := svc.Create(context.Background(), feedback, tenantID)
	if err != ErrFeedbackMemoryIDMissing {
		t.Fatalf("expected ErrFeedbackMemoryIDMissing, got %v", err)
	}
}

func TestFeedbackService_Create_MissingAgentID(t *testing.T) {
	svc, _, _, tenantID, _ := setupFeedbackTest()

	feedback := &domain.Feedback{
		MemoryID:   uuid.New(),
		SignalType: domain.FeedbackTypeHelpful,
	}

	err := svc.Create(context.Background(), feedback, tenantID)
	if err != ErrFeedbackAgentIDMissing {
		t.Fatalf("expected ErrFeedbackAgentIDMissing, got %v", err)
	}
}

func TestFeedbackService_Create_InvalidSignal(t *testing.T) {
	svc, _, _, tenantID, agentID := setupFeedbackTest()

	feedback := &domain.Feedback{
		MemoryID:   uuid.New(),
		AgentID:    agentID,
		SignalType: "invalid",
	}

	err := svc.Create(context.Background(), feedback, tenantID)
	if err != ErrFeedbackInvalidSignal {
		t.Fatalf("expected ErrFeedbackInvalidSignal, got %v", err)
	}
}

func TestFeedbackService_Create_AgentNotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupFeedbackTest()

	feedback := &domain.Feedback{
		MemoryID:   uuid.New(),
		AgentID:    uuid.New(),
		SignalType: domain.FeedbackTypeHelpful,
	}

	err := svc.Create(context.Background(), feedback, tenantID)
	if err != ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestFeedbackService_Create_MemoryNotFound(t *testing.T) {
	svc, _, _, tenantID, agentID := setupFeedbackTest()

	feedback := &domain.Feedback{
		MemoryID:   uuid.New(), // nonexistent
		AgentID:    agentID,
		SignalType: domain.FeedbackTypeHelpful,
	}

	err := svc.Create(context.Background(), feedback, tenantID)
	if err != ErrMemoryNotFound {
		t.Fatalf("expected ErrMemoryNotFound, got %v", err)
	}
}

func TestFeedbackService_Create_AllSignalTypes(t *testing.T) {
	svc, _, memStore, tenantID, agentID := setupFeedbackTest()
	ctx := context.Background()

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "test", Type: domain.MemoryTypeFact}
	_ = memStore.Create(ctx, mem)

	signalTypes := []domain.FeedbackType{
		domain.FeedbackTypeUsed,
		domain.FeedbackTypeIgnored,
		domain.FeedbackTypeHelpful,
		domain.FeedbackTypeUnhelpful,
	}

	for _, st := range signalTypes {
		feedback := &domain.Feedback{
			MemoryID:   mem.ID,
			AgentID:    agentID,
			SignalType: st,
		}
		if err := svc.Create(ctx, feedback, tenantID); err != nil {
			t.Fatalf("expected no error for signal type %s, got %v", st, err)
		}
	}
}
