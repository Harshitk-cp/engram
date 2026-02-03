package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

// mockProcedureStore implements domain.ProcedureStore for testing.
type mockProcedureStore struct {
	procedures map[uuid.UUID]*domain.Procedure
}

func newMockProcedureStore() *mockProcedureStore {
	return &mockProcedureStore{procedures: make(map[uuid.UUID]*domain.Procedure)}
}

func (m *mockProcedureStore) Create(ctx context.Context, p *domain.Procedure) error {
	p.ID = uuid.New()
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	m.procedures[p.ID] = p
	return nil
}

func (m *mockProcedureStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Procedure, error) {
	p, ok := m.procedures[id]
	if !ok || p.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return p, nil
}

func (m *mockProcedureStore) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Procedure, error) {
	var results []domain.Procedure
	for _, p := range m.procedures {
		if p.AgentID == agentID && p.TenantID == tenantID {
			results = append(results, *p)
		}
	}
	return results, nil
}

func (m *mockProcedureStore) FindByTriggerSimilarity(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.ProcedureWithScore, error) {
	// Return empty by default - tests can set specific behavior
	return []domain.ProcedureWithScore{}, nil
}

func (m *mockProcedureStore) RecordUse(ctx context.Context, id uuid.UUID, success bool) error {
	p, ok := m.procedures[id]
	if !ok {
		return store.ErrNotFound
	}
	p.UseCount++
	if success {
		p.SuccessCount++
	} else {
		p.FailureCount++
	}
	if p.UseCount > 0 {
		p.SuccessRate = float32(p.SuccessCount) / float32(p.UseCount)
	}
	now := time.Now()
	p.LastUsedAt = &now
	return nil
}

func (m *mockProcedureStore) Reinforce(ctx context.Context, id uuid.UUID, episodeID uuid.UUID, confidenceBoost float32) error {
	p, ok := m.procedures[id]
	if !ok {
		return store.ErrNotFound
	}
	p.Confidence += confidenceBoost
	if p.Confidence > 0.99 {
		p.Confidence = 0.99
	}
	p.DerivedFromEpisodes = append(p.DerivedFromEpisodes, episodeID)
	now := time.Now()
	p.LastVerifiedAt = &now
	return nil
}

func (m *mockProcedureStore) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	p, ok := m.procedures[id]
	if !ok {
		return store.ErrNotFound
	}
	p.Confidence = confidence
	return nil
}

func (m *mockProcedureStore) Archive(ctx context.Context, id uuid.UUID) error {
	p, ok := m.procedures[id]
	if !ok {
		return store.ErrNotFound
	}
	p.MemoryStrength = 0
	return nil
}

func (m *mockProcedureStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Procedure, error) {
	var results []domain.Procedure
	for _, p := range m.procedures {
		if p.AgentID == agentID && p.MemoryStrength > 0 {
			results = append(results, *p)
		}
	}
	return results, nil
}

func (m *mockProcedureStore) CreateNewVersion(ctx context.Context, p *domain.Procedure) error {
	return m.Create(ctx, p)
}

func setupProceduralTest() (*ProceduralService, *mockProcedureStore, *mockEpisodeStore, uuid.UUID, uuid.UUID) {
	agentStore := newMockAgentStore()
	procedureStore := newMockProcedureStore()
	episodeStore := newMockEpisodeStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()
	svc := NewProceduralService(procedureStore, episodeStore, agentStore, embClient, llmClient, testLogger())

	tenantID := uuid.New()
	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = agentStore.Create(context.Background(), agent)

	return svc, procedureStore, episodeStore, tenantID, agent.ID
}

func TestProceduralService_LearnFromOutcome_Success(t *testing.T) {
	svc, procedureStore, episodeStore, tenantID, agentID := setupProceduralTest()
	ctx := context.Background()

	// Create an episode
	episode := &domain.Episode{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "User was frustrated about slow loading. I acknowledged their frustration and then explained the solution.",
	}
	_ = episodeStore.Create(ctx, episode)

	// Learn from successful outcome
	err := svc.LearnFromOutcome(ctx, episode.ID, tenantID, domain.OutcomeSuccess)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify a procedure was created
	if len(procedureStore.procedures) != 1 {
		t.Fatalf("expected 1 procedure, got %d", len(procedureStore.procedures))
	}

	// Verify procedure properties
	for _, p := range procedureStore.procedures {
		if p.AgentID != agentID {
			t.Fatalf("expected agent_id %s, got %s", agentID, p.AgentID)
		}
		if p.TenantID != tenantID {
			t.Fatalf("expected tenant_id %s, got %s", tenantID, p.TenantID)
		}
		if p.Confidence != NewProcedureInitialConfidence {
			t.Fatalf("expected confidence %f, got %f", NewProcedureInitialConfidence, p.Confidence)
		}
		if len(p.DerivedFromEpisodes) != 1 {
			t.Fatalf("expected 1 derived episode, got %d", len(p.DerivedFromEpisodes))
		}
	}
}

func TestProceduralService_LearnFromOutcome_NonSuccess(t *testing.T) {
	svc, procedureStore, episodeStore, tenantID, agentID := setupProceduralTest()
	ctx := context.Background()

	// Create an episode
	episode := &domain.Episode{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "User was confused and left unsatisfied.",
	}
	_ = episodeStore.Create(ctx, episode)

	// Learn from failure outcome (should not create new procedure)
	err := svc.LearnFromOutcome(ctx, episode.ID, tenantID, domain.OutcomeFailure)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify no procedure was created
	if len(procedureStore.procedures) != 0 {
		t.Fatalf("expected 0 procedures for failure, got %d", len(procedureStore.procedures))
	}
}

func TestProceduralService_LearnFromOutcome_EpisodeNotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupProceduralTest()
	ctx := context.Background()

	err := svc.LearnFromOutcome(ctx, uuid.New(), tenantID, domain.OutcomeSuccess)
	if err != ErrEpisodeNotFound {
		t.Fatalf("expected ErrEpisodeNotFound, got %v", err)
	}
}

func TestProceduralService_GetByID(t *testing.T) {
	svc, procedureStore, _, tenantID, agentID := setupProceduralTest()
	ctx := context.Background()

	// Create a procedure directly in the store
	procedure := &domain.Procedure{
		AgentID:        agentID,
		TenantID:       tenantID,
		TriggerPattern: "When user is frustrated",
		ActionTemplate: "Acknowledge feelings first",
		ActionType:     domain.ActionTypeCommunication,
		Confidence:     0.7,
		MemoryStrength: 1.0,
	}
	_ = procedureStore.Create(ctx, procedure)

	// Get the procedure
	found, err := svc.GetByID(ctx, procedure.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != procedure.ID {
		t.Fatalf("expected ID %s, got %s", procedure.ID, found.ID)
	}
	if found.TriggerPattern != procedure.TriggerPattern {
		t.Fatalf("expected trigger_pattern %q, got %q", procedure.TriggerPattern, found.TriggerPattern)
	}
}

func TestProceduralService_GetByID_NotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupProceduralTest()
	ctx := context.Background()

	_, err := svc.GetByID(ctx, uuid.New(), tenantID)
	if err != ErrProcedureNotFound {
		t.Fatalf("expected ErrProcedureNotFound, got %v", err)
	}
}

func TestProceduralService_RecordProcedureOutcome_Success(t *testing.T) {
	svc, procedureStore, _, tenantID, agentID := setupProceduralTest()
	ctx := context.Background()

	// Create a procedure
	procedure := &domain.Procedure{
		AgentID:        agentID,
		TenantID:       tenantID,
		TriggerPattern: "When user asks about X",
		ActionTemplate: "Respond with Y",
		ActionType:     domain.ActionTypeResponseStyle,
		Confidence:     0.6,
	}
	_ = procedureStore.Create(ctx, procedure)

	// Record success
	err := svc.RecordProcedureOutcome(ctx, procedure.ID, tenantID, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify use count and success count were updated
	updated := procedureStore.procedures[procedure.ID]
	if updated.UseCount != 1 {
		t.Fatalf("expected use_count 1, got %d", updated.UseCount)
	}
	if updated.SuccessCount != 1 {
		t.Fatalf("expected success_count 1, got %d", updated.SuccessCount)
	}
	if updated.SuccessRate != 1.0 {
		t.Fatalf("expected success_rate 1.0, got %f", updated.SuccessRate)
	}
}

func TestProceduralService_RecordProcedureOutcome_Failure(t *testing.T) {
	svc, procedureStore, _, tenantID, agentID := setupProceduralTest()
	ctx := context.Background()

	// Create a procedure
	procedure := &domain.Procedure{
		AgentID:        agentID,
		TenantID:       tenantID,
		TriggerPattern: "When user asks about X",
		ActionTemplate: "Respond with Y",
		ActionType:     domain.ActionTypeResponseStyle,
		Confidence:     0.6,
	}
	_ = procedureStore.Create(ctx, procedure)

	// Record failure
	err := svc.RecordProcedureOutcome(ctx, procedure.ID, tenantID, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify use count and failure count were updated
	updated := procedureStore.procedures[procedure.ID]
	if updated.UseCount != 1 {
		t.Fatalf("expected use_count 1, got %d", updated.UseCount)
	}
	if updated.FailureCount != 1 {
		t.Fatalf("expected failure_count 1, got %d", updated.FailureCount)
	}
	if updated.SuccessRate != 0.0 {
		t.Fatalf("expected success_rate 0.0, got %f", updated.SuccessRate)
	}
}

func TestProceduralService_RecordProcedureOutcome_NotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupProceduralTest()
	ctx := context.Background()

	err := svc.RecordProcedureOutcome(ctx, uuid.New(), tenantID, true)
	if err != ErrProcedureNotFound {
		t.Fatalf("expected ErrProcedureNotFound, got %v", err)
	}
}

func TestProceduralService_GetApplicableProcedures_Empty(t *testing.T) {
	svc, _, _, tenantID, agentID := setupProceduralTest()
	ctx := context.Background()

	input := ProcedureMatchInput{
		AgentID:   agentID,
		TenantID:  tenantID,
		Situation: "User is asking about something",
	}

	procedures, err := svc.GetApplicableProcedures(ctx, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(procedures) != 0 {
		t.Fatalf("expected 0 procedures, got %d", len(procedures))
	}
}

func TestProceduralService_RecencyBoost(t *testing.T) {
	svc, _, _, _, _ := setupProceduralTest()

	// Test nil last used (never used)
	boost := svc.recencyBoost(nil)
	if boost != 0.5 {
		t.Fatalf("expected boost 0.5 for nil, got %f", boost)
	}

	// Test recently used
	now := time.Now()
	boost = svc.recencyBoost(&now)
	if boost < 0.99 {
		t.Fatalf("expected boost ~1.0 for now, got %f", boost)
	}

	// Test old usage (30 days ago)
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	boost = svc.recencyBoost(&oldTime)
	if boost > 0.5 || boost < 0.3 {
		t.Fatalf("expected boost ~0.37 for 30 days ago, got %f", boost)
	}
}
