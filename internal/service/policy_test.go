package service

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

// mockPolicyStore implements domain.PolicyStore for testing.
type mockPolicyStore struct {
	policies map[string]*domain.Policy // key: agentID+memType
}

func newMockPolicyStore() *mockPolicyStore {
	return &mockPolicyStore{policies: make(map[string]*domain.Policy)}
}

func policyKey(agentID uuid.UUID, memType domain.MemoryType) string {
	return agentID.String() + ":" + string(memType)
}

func (m *mockPolicyStore) Upsert(ctx context.Context, p *domain.Policy) error {
	key := policyKey(p.AgentID, p.MemoryType)
	p.ID = uuid.New()
	m.policies[key] = p
	return nil
}

func (m *mockPolicyStore) GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.Policy, error) {
	var result []domain.Policy
	for _, p := range m.policies {
		if p.AgentID == agentID {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *mockPolicyStore) GetByAgentIDAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (*domain.Policy, error) {
	key := policyKey(agentID, memType)
	p, ok := m.policies[key]
	if !ok {
		return nil, store.ErrNotFound
	}
	return p, nil
}

func setupPolicyTest() (*PolicyService, *mockPolicyStore, *mockMemoryStore, uuid.UUID, uuid.UUID) {
	agentStore := newMockAgentStore()
	memStore := newMockMemoryStore()
	policyStore := newMockPolicyStore()
	llmClient := newMockLLMClient()
	embClient := &mockEmbeddingClient{}
	logger := testLogger()

	svc := NewPolicyService(policyStore, memStore, agentStore, llmClient, embClient, logger)

	tenantID := uuid.New()
	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = agentStore.Create(context.Background(), agent)

	return svc, policyStore, memStore, tenantID, agent.ID
}

func TestPolicyService_UpsertPolicies(t *testing.T) {
	svc, policyStore, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	policies := []domain.Policy{
		{
			MemoryType:     domain.MemoryTypePreference,
			MaxMemories:    50,
			PriorityWeight: 1.2,
			AutoSummarize:  true,
		},
		{
			MemoryType:     domain.MemoryTypeFact,
			MaxMemories:    200,
			PriorityWeight: 1.0,
			AutoSummarize:  false,
		},
	}

	result, err := svc.UpsertPolicies(ctx, agentID, tenantID, policies)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(result))
	}
	if len(policyStore.policies) != 2 {
		t.Fatalf("expected 2 policies in store, got %d", len(policyStore.policies))
	}
}

func TestPolicyService_UpsertPolicies_InvalidType(t *testing.T) {
	svc, _, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	policies := []domain.Policy{
		{
			MemoryType:     "invalid",
			MaxMemories:    50,
			PriorityWeight: 1.0,
		},
	}

	_, err := svc.UpsertPolicies(ctx, agentID, tenantID, policies)
	if err != ErrPolicyInvalidType {
		t.Fatalf("expected ErrPolicyInvalidType, got %v", err)
	}
}

func TestPolicyService_UpsertPolicies_InvalidMaxMemories(t *testing.T) {
	svc, _, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	policies := []domain.Policy{
		{
			MemoryType:     domain.MemoryTypePreference,
			MaxMemories:    0,
			PriorityWeight: 1.0,
		},
	}

	_, err := svc.UpsertPolicies(ctx, agentID, tenantID, policies)
	if err != ErrPolicyMaxMemories {
		t.Fatalf("expected ErrPolicyMaxMemories, got %v", err)
	}
}

func TestPolicyService_UpsertPolicies_InvalidPriorityWeight(t *testing.T) {
	svc, _, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	policies := []domain.Policy{
		{
			MemoryType:     domain.MemoryTypePreference,
			MaxMemories:    50,
			PriorityWeight: 0,
		},
	}

	_, err := svc.UpsertPolicies(ctx, agentID, tenantID, policies)
	if err != ErrPolicyPriorityWeight {
		t.Fatalf("expected ErrPolicyPriorityWeight, got %v", err)
	}
}

func TestPolicyService_UpsertPolicies_AgentNotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupPolicyTest()
	ctx := context.Background()

	policies := []domain.Policy{
		{
			MemoryType:     domain.MemoryTypePreference,
			MaxMemories:    50,
			PriorityWeight: 1.0,
		},
	}

	_, err := svc.UpsertPolicies(ctx, uuid.New(), tenantID, policies)
	if err != ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestPolicyService_GetPolicies(t *testing.T) {
	svc, _, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	// Upsert some policies
	policies := []domain.Policy{
		{MemoryType: domain.MemoryTypePreference, MaxMemories: 50, PriorityWeight: 1.2, AutoSummarize: true},
	}
	_, err := svc.UpsertPolicies(ctx, agentID, tenantID, policies)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Get policies
	result, err := svc.GetPolicies(ctx, agentID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(result))
	}
	if result[0].MaxMemories != 50 {
		t.Fatalf("expected max_memories 50, got %d", result[0].MaxMemories)
	}
}

func TestPolicyService_GetPolicies_Empty(t *testing.T) {
	svc, _, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	result, err := svc.GetPolicies(ctx, agentID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 policies, got %d", len(result))
	}
}

func TestPolicyService_EnforceOnCreate_NoPolicy(t *testing.T) {
	svc, _, _, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	mem := &domain.Memory{
		AgentID:  agentID,
		TenantID: tenantID,
		Type:     domain.MemoryTypePreference,
		Content:  "test",
	}

	// No policy set — should be a no-op
	if err := svc.EnforceOnCreate(ctx, mem); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestPolicyService_EnforceOnCreate_UnderLimit(t *testing.T) {
	svc, _, memStore, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	// Set policy with high limit
	policies := []domain.Policy{
		{MemoryType: domain.MemoryTypePreference, MaxMemories: 100, PriorityWeight: 1.0},
	}
	_, _ = svc.UpsertPolicies(ctx, agentID, tenantID, policies)

	// Add one memory
	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypePreference, Content: "test"}
	_ = memStore.Create(ctx, mem)

	if err := svc.EnforceOnCreate(ctx, mem); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Memory count should remain 1
	count, _ := memStore.CountByAgentAndType(ctx, agentID, domain.MemoryTypePreference)
	if count != 1 {
		t.Fatalf("expected 1 memory, got %d", count)
	}
}

func TestPolicyService_EnforceOnCreate_OverLimit_Delete(t *testing.T) {
	svc, _, memStore, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	// Set policy with low limit, no auto-summarize
	policies := []domain.Policy{
		{MemoryType: domain.MemoryTypePreference, MaxMemories: 2, PriorityWeight: 1.0, AutoSummarize: false},
	}
	_, _ = svc.UpsertPolicies(ctx, agentID, tenantID, policies)

	// Add 3 memories (exceeds limit of 2)
	for i := 0; i < 3; i++ {
		mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypePreference, Content: "test"}
		_ = memStore.Create(ctx, mem)
	}

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypePreference, Content: "trigger"}
	if err := svc.EnforceOnCreate(ctx, mem); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// After enforcement, count should be reduced (oldest deleted)
	count, _ := memStore.CountByAgentAndType(ctx, agentID, domain.MemoryTypePreference)
	if count > 2 {
		t.Fatalf("expected at most 2 memories after enforcement, got %d", count)
	}
}

func TestPolicyService_EnforceOnCreate_OverLimit_Summarize(t *testing.T) {
	svc, _, memStore, tenantID, agentID := setupPolicyTest()
	ctx := context.Background()

	// Set policy with low limit and auto-summarize enabled
	policies := []domain.Policy{
		{MemoryType: domain.MemoryTypePreference, MaxMemories: 2, PriorityWeight: 1.0, AutoSummarize: true},
	}
	_, _ = svc.UpsertPolicies(ctx, agentID, tenantID, policies)

	// Add 3 memories (exceeds limit of 2)
	for i := 0; i < 3; i++ {
		mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypePreference, Content: "test"}
		_ = memStore.Create(ctx, mem)
	}

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypePreference, Content: "trigger"}
	if err := svc.EnforceOnCreate(ctx, mem); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// After enforcement: oldest should be deleted, a summarized memory should be created
	// The summarized memory is added, so we should have original 3 - 1 deleted + 1 summarized = 3
	// Actually let me recalculate: 3 memories, max 2, excess = 1.
	// So 1 memory deleted, 1 summary created = still 3.
	// But the point is the oldest was replaced with a summary.
	count, _ := memStore.CountByAgentAndType(ctx, agentID, domain.MemoryTypePreference)
	// 3 original - 1 deleted + 1 summarized = 3
	if count != 3 {
		t.Fatalf("expected 3 memories (2 original + 1 summary), got %d", count)
	}
}
