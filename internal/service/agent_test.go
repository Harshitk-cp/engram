package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

// mockAgentStore implements domain.AgentStore for testing.
type mockAgentStore struct {
	agents map[uuid.UUID]*domain.Agent
}

func newMockAgentStore() *mockAgentStore {
	return &mockAgentStore{agents: make(map[uuid.UUID]*domain.Agent)}
}

func (m *mockAgentStore) Create(ctx context.Context, a *domain.Agent) error {
	for _, existing := range m.agents {
		if existing.ExternalID == a.ExternalID && existing.TenantID == a.TenantID {
			return store.ErrConflict
		}
	}
	a.ID = uuid.New()
	m.agents[a.ID] = a
	return nil
}

func (m *mockAgentStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Agent, error) {
	a, ok := m.agents[id]
	if !ok || a.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return a, nil
}

func (m *mockAgentStore) GetByExternalID(ctx context.Context, externalID string, tenantID uuid.UUID) (*domain.Agent, error) {
	for _, a := range m.agents {
		if a.ExternalID == externalID && a.TenantID == tenantID {
			return a, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockAgentStore) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	n := 0
	for _, a := range m.agents {
		if a.TenantID == tenantID {
			n++
		}
	}
	return n, nil
}

func (m *mockAgentStore) ListByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]domain.Agent, error) {
	var result []domain.Agent
	for _, a := range m.agents {
		if a.TenantID == tenantID {
			result = append(result, *a)
		}
	}
	return result, nil
}

func (m *mockAgentStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	a, ok := m.agents[id]
	if !ok || a.TenantID != tenantID {
		return store.ErrNotFound
	}
	delete(m.agents, id)
	return nil
}

func TestAgentService_Create(t *testing.T) {
	s := NewAgentService(newMockAgentStore())
	ctx := context.Background()
	tenantID := uuid.New()

	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}

	if err := s.Create(ctx, agent); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.ID == uuid.Nil {
		t.Fatal("expected agent ID to be set")
	}
}

func TestAgentService_Create_GeneratesExternalID(t *testing.T) {
	s := NewAgentService(newMockAgentStore())
	ctx := context.Background()

	agent := &domain.Agent{
		TenantID: uuid.New(),
		Name:     "Support Bot 3000!",
		// ExternalID intentionally omitted — service must derive a unique one.
	}
	if err := s.Create(ctx, agent); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.ExternalID == "" {
		t.Fatal("expected external_id to be generated")
	}
	if !strings.HasPrefix(agent.ExternalID, "support-bot-3000-") {
		t.Fatalf("expected slug-prefixed external_id, got %q", agent.ExternalID)
	}

	// Two unnamed-id agents with the same name must not collide.
	other := &domain.Agent{TenantID: agent.TenantID, Name: "Support Bot 3000!"}
	if err := s.Create(ctx, other); err != nil {
		t.Fatalf("expected no error on second create, got %v", err)
	}
	if other.ExternalID == agent.ExternalID {
		t.Fatalf("generated external_ids collided: %q", other.ExternalID)
	}
}

func TestAgentService_CreateDuplicate(t *testing.T) {
	s := NewAgentService(newMockAgentStore())
	ctx := context.Background()
	tenantID := uuid.New()

	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	if err := s.Create(ctx, agent); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	dup := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Duplicate Bot",
	}
	err := s.Create(ctx, dup)
	if err != ErrAgentConflict {
		t.Fatalf("expected ErrAgentConflict, got %v", err)
	}
}

func TestAgentService_GetByID(t *testing.T) {
	mockStore := newMockAgentStore()
	s := NewAgentService(mockStore)
	ctx := context.Background()
	tenantID := uuid.New()

	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = s.Create(ctx, agent)

	found, err := s.GetByID(ctx, agent.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.Name != "Test Bot" {
		t.Fatalf("expected name 'Test Bot', got %s", found.Name)
	}
}

func TestAgentService_GetByID_NotFound(t *testing.T) {
	s := NewAgentService(newMockAgentStore())
	ctx := context.Background()

	_, err := s.GetByID(ctx, uuid.New(), uuid.New())
	if err != ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestAgentService_GetByID_WrongTenant(t *testing.T) {
	mockStore := newMockAgentStore()
	s := NewAgentService(mockStore)
	ctx := context.Background()
	tenantID := uuid.New()

	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = s.Create(ctx, agent)

	_, err := s.GetByID(ctx, agent.ID, uuid.New())
	if err != ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound for wrong tenant, got %v", err)
	}
}
