package service

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

// mockFeedbackStore implements domain.FeedbackStore for testing.
type mockFeedbackStore struct {
	feedbacks  []domain.Feedback
	aggregates map[uuid.UUID][]domain.FeedbackAggregate
}

func newMockFeedbackStore() *mockFeedbackStore {
	return &mockFeedbackStore{
		aggregates: make(map[uuid.UUID][]domain.FeedbackAggregate),
	}
}

func (m *mockFeedbackStore) Create(ctx context.Context, f *domain.Feedback) error {
	f.ID = uuid.New()
	m.feedbacks = append(m.feedbacks, *f)
	return nil
}

func (m *mockFeedbackStore) GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.Feedback, error) {
	var result []domain.Feedback
	for _, f := range m.feedbacks {
		if f.AgentID == agentID {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockFeedbackStore) GetByMemoryID(ctx context.Context, memoryID uuid.UUID) ([]domain.Feedback, error) {
	var result []domain.Feedback
	for _, f := range m.feedbacks {
		if f.MemoryID == memoryID {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockFeedbackStore) GetAggregatesByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.FeedbackAggregate, error) {
	return m.aggregates[agentID], nil
}

func (m *mockFeedbackStore) CountByAgentID(ctx context.Context, agentID uuid.UUID) (int, error) {
	count := 0
	for _, f := range m.feedbacks {
		if f.AgentID == agentID {
			count++
		}
	}
	return count, nil
}

func (m *mockFeedbackStore) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]bool)
	var ids []uuid.UUID
	for _, f := range m.feedbacks {
		if !seen[f.AgentID] {
			seen[f.AgentID] = true
			ids = append(ids, f.AgentID)
		}
	}
	return ids, nil
}

func TestTunerService_RunForAgent_IgnoredReducesWeight(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID := uuid.New()

	// Set up initial policy
	initialPolicy := &domain.Policy{
		AgentID:        agentID,
		MemoryType:     domain.MemoryTypePreference,
		MaxMemories:    100,
		PriorityWeight: 1.0,
	}
	_ = policyStore.Upsert(context.Background(), initialPolicy)

	// Add enough feedback to trigger tuner (minFeedbackCount = 10)
	for i := 0; i < 15; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:         uuid.New(),
			MemoryID:   uuid.New(),
			AgentID:    agentID,
			SignalType: domain.FeedbackTypeIgnored,
		})
	}

	// Set aggregates: 80% ignored (12/15), 20% other (3/15)
	feedbackStore.aggregates[agentID] = []domain.FeedbackAggregate{
		{AgentID: agentID, MemoryType: domain.MemoryTypePreference, SignalType: domain.FeedbackTypeIgnored, Count: 12},
		{AgentID: agentID, MemoryType: domain.MemoryTypePreference, SignalType: domain.FeedbackTypeUsed, Count: 3},
	}

	svc := NewTunerService(feedbackStore, policyStore, logger)
	if err := svc.RunForAgent(context.Background(), agentID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check policy was adjusted
	updated, err := policyStore.GetByAgentIDAndType(context.Background(), agentID, domain.MemoryTypePreference)
	if err != nil {
		t.Fatalf("expected policy to exist, got %v", err)
	}
	expectedWeight := 1.0 - weightDelta
	if updated.PriorityWeight != expectedWeight {
		t.Fatalf("expected priority_weight %.1f, got %.1f", expectedWeight, updated.PriorityWeight)
	}
}

func TestTunerService_RunForAgent_HelpfulIncreasesWeight(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID := uuid.New()

	initialPolicy := &domain.Policy{
		AgentID:        agentID,
		MemoryType:     domain.MemoryTypeFact,
		MaxMemories:    100,
		PriorityWeight: 1.0,
	}
	_ = policyStore.Upsert(context.Background(), initialPolicy)

	// Add enough feedback
	for i := 0; i < 15; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:         uuid.New(),
			MemoryID:   uuid.New(),
			AgentID:    agentID,
			SignalType: domain.FeedbackTypeHelpful,
		})
	}

	// Set aggregates: 80% helpful
	feedbackStore.aggregates[agentID] = []domain.FeedbackAggregate{
		{AgentID: agentID, MemoryType: domain.MemoryTypeFact, SignalType: domain.FeedbackTypeHelpful, Count: 12},
		{AgentID: agentID, MemoryType: domain.MemoryTypeFact, SignalType: domain.FeedbackTypeUsed, Count: 3},
	}

	svc := NewTunerService(feedbackStore, policyStore, logger)
	if err := svc.RunForAgent(context.Background(), agentID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	updated, err := policyStore.GetByAgentIDAndType(context.Background(), agentID, domain.MemoryTypeFact)
	if err != nil {
		t.Fatalf("expected policy to exist, got %v", err)
	}
	expectedWeight := 1.0 + weightDelta
	if updated.PriorityWeight != expectedWeight {
		t.Fatalf("expected priority_weight %.1f, got %.1f", expectedWeight, updated.PriorityWeight)
	}
}

func TestTunerService_RunForAgent_UnhelpfulReducesMaxMemories(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID := uuid.New()

	initialPolicy := &domain.Policy{
		AgentID:        agentID,
		MemoryType:     domain.MemoryTypeConstraint,
		MaxMemories:    100,
		PriorityWeight: 1.0,
	}
	_ = policyStore.Upsert(context.Background(), initialPolicy)

	// Add enough feedback
	for i := 0; i < 15; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:         uuid.New(),
			MemoryID:   uuid.New(),
			AgentID:    agentID,
			SignalType: domain.FeedbackTypeUnhelpful,
		})
	}

	// Set aggregates: 80% unhelpful
	feedbackStore.aggregates[agentID] = []domain.FeedbackAggregate{
		{AgentID: agentID, MemoryType: domain.MemoryTypeConstraint, SignalType: domain.FeedbackTypeUnhelpful, Count: 12},
		{AgentID: agentID, MemoryType: domain.MemoryTypeConstraint, SignalType: domain.FeedbackTypeUsed, Count: 3},
	}

	svc := NewTunerService(feedbackStore, policyStore, logger)
	if err := svc.RunForAgent(context.Background(), agentID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	updated, err := policyStore.GetByAgentIDAndType(context.Background(), agentID, domain.MemoryTypeConstraint)
	if err != nil {
		t.Fatalf("expected policy to exist, got %v", err)
	}
	expectedMax := 100 - maxMemoriesReduce
	if updated.MaxMemories != expectedMax {
		t.Fatalf("expected max_memories %d, got %d", expectedMax, updated.MaxMemories)
	}
}

func TestTunerService_RunForAgent_InsufficientFeedback(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID := uuid.New()

	initialPolicy := &domain.Policy{
		AgentID:        agentID,
		MemoryType:     domain.MemoryTypePreference,
		MaxMemories:    100,
		PriorityWeight: 1.0,
	}
	_ = policyStore.Upsert(context.Background(), initialPolicy)

	// Only 5 feedback signals (below minFeedbackCount of 10)
	for i := 0; i < 5; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:         uuid.New(),
			MemoryID:   uuid.New(),
			AgentID:    agentID,
			SignalType: domain.FeedbackTypeIgnored,
		})
	}

	svc := NewTunerService(feedbackStore, policyStore, logger)
	if err := svc.RunForAgent(context.Background(), agentID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Policy should not have changed
	updated, err := policyStore.GetByAgentIDAndType(context.Background(), agentID, domain.MemoryTypePreference)
	if err != nil {
		t.Fatalf("expected policy to exist, got %v", err)
	}
	if updated.PriorityWeight != 1.0 {
		t.Fatalf("expected unchanged priority_weight 1.0, got %.1f", updated.PriorityWeight)
	}
}

func TestTunerService_RunForAgent_MinWeightFloor(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID := uuid.New()

	// Start with weight already at minimum
	initialPolicy := &domain.Policy{
		AgentID:        agentID,
		MemoryType:     domain.MemoryTypePreference,
		MaxMemories:    100,
		PriorityWeight: minPriorityWeight,
	}
	_ = policyStore.Upsert(context.Background(), initialPolicy)

	for i := 0; i < 15; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:       uuid.New(),
			MemoryID: uuid.New(),
			AgentID:  agentID,
			SignalType: domain.FeedbackTypeIgnored,
		})
	}

	feedbackStore.aggregates[agentID] = []domain.FeedbackAggregate{
		{AgentID: agentID, MemoryType: domain.MemoryTypePreference, SignalType: domain.FeedbackTypeIgnored, Count: 15},
	}

	svc := NewTunerService(feedbackStore, policyStore, logger)
	_ = svc.RunForAgent(context.Background(), agentID)

	updated, _ := policyStore.GetByAgentIDAndType(context.Background(), agentID, domain.MemoryTypePreference)
	if updated.PriorityWeight != minPriorityWeight {
		t.Fatalf("expected weight at floor %.1f, got %.1f", minPriorityWeight, updated.PriorityWeight)
	}
}

func TestTunerService_RunForAgent_NoPolicyCreatesDefault(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID := uuid.New()

	// No initial policy — tuner should create a default and adjust it
	for i := 0; i < 15; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:       uuid.New(),
			MemoryID: uuid.New(),
			AgentID:  agentID,
			SignalType: domain.FeedbackTypeHelpful,
		})
	}

	feedbackStore.aggregates[agentID] = []domain.FeedbackAggregate{
		{AgentID: agentID, MemoryType: domain.MemoryTypeFact, SignalType: domain.FeedbackTypeHelpful, Count: 12},
		{AgentID: agentID, MemoryType: domain.MemoryTypeFact, SignalType: domain.FeedbackTypeUsed, Count: 3},
	}

	svc := NewTunerService(feedbackStore, policyStore, logger)
	if err := svc.RunForAgent(context.Background(), agentID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have created a policy with adjusted weight
	updated, err := policyStore.GetByAgentIDAndType(context.Background(), agentID, domain.MemoryTypeFact)
	if err != nil {
		t.Fatalf("expected policy to be created, got %v", err)
	}
	// Default weight 1.0 + 0.1 = 1.1
	if updated.PriorityWeight != 1.1 {
		t.Fatalf("expected priority_weight 1.1, got %.1f", updated.PriorityWeight)
	}
}

func TestTunerService_RunAll(t *testing.T) {
	feedbackStore := newMockFeedbackStore()
	policyStore := newMockPolicyStore()
	logger := testLogger()

	agentID1 := uuid.New()
	agentID2 := uuid.New()

	// Add feedback for two agents
	for i := 0; i < 15; i++ {
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:       uuid.New(),
			MemoryID: uuid.New(),
			AgentID:  agentID1,
			SignalType: domain.FeedbackTypeHelpful,
		})
		feedbackStore.feedbacks = append(feedbackStore.feedbacks, domain.Feedback{
			ID:       uuid.New(),
			MemoryID: uuid.New(),
			AgentID:  agentID2,
			SignalType: domain.FeedbackTypeIgnored,
		})
	}

	feedbackStore.aggregates[agentID1] = []domain.FeedbackAggregate{
		{AgentID: agentID1, MemoryType: domain.MemoryTypePreference, SignalType: domain.FeedbackTypeHelpful, Count: 15},
	}
	feedbackStore.aggregates[agentID2] = []domain.FeedbackAggregate{
		{AgentID: agentID2, MemoryType: domain.MemoryTypePreference, SignalType: domain.FeedbackTypeIgnored, Count: 15},
	}

	// Set initial policies
	_ = policyStore.Upsert(context.Background(), &domain.Policy{
		AgentID: agentID1, MemoryType: domain.MemoryTypePreference, MaxMemories: 100, PriorityWeight: 1.0,
	})
	_ = policyStore.Upsert(context.Background(), &domain.Policy{
		AgentID: agentID2, MemoryType: domain.MemoryTypePreference, MaxMemories: 100, PriorityWeight: 1.0,
	})

	svc := NewTunerService(feedbackStore, policyStore, logger)
	if err := svc.RunAll(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Agent 1 should have increased weight
	p1, _ := policyStore.GetByAgentIDAndType(context.Background(), agentID1, domain.MemoryTypePreference)
	if p1.PriorityWeight != 1.1 {
		t.Fatalf("expected agent1 weight 1.1, got %.1f", p1.PriorityWeight)
	}

	// Agent 2 should have decreased weight
	p2, _ := policyStore.GetByAgentIDAndType(context.Background(), agentID2, domain.MemoryTypePreference)
	if p2.PriorityWeight != 0.9 {
		t.Fatalf("expected agent2 weight 0.9, got %.1f", p2.PriorityWeight)
	}
}
