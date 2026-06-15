package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

type AgentService struct {
	store domain.AgentStore
}

func NewAgentService(s domain.AgentStore) *AgentService {
	return &AgentService{store: s}
}

var (
	ErrAgentNotFound = errors.New("agent not found")
	ErrAgentConflict = errors.New("agent with this external_id already exists")
)

func (s *AgentService) Create(ctx context.Context, a *domain.Agent) error {
	if a.ExternalID == "" {
		a.ExternalID = generateExternalID(a.Name)
	}
	err := s.store.Create(ctx, a)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ErrAgentConflict
		}
		return err
	}
	return nil
}

func generateExternalID(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range slug {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "agent"
	}
	return base + "-" + uuid.NewString()[:8]
}

func (s *AgentService) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Agent, error) {
	a, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return a, nil
}

func (s *AgentService) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]domain.Agent, error) {
	return s.store.ListByTenantID(ctx, tenantID, limit, offset)
}

func (s *AgentService) Count(ctx context.Context, tenantID uuid.UUID) (int, error) {
	return s.store.CountByTenant(ctx, tenantID)
}

func (s *AgentService) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := s.store.Delete(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrAgentNotFound
		}
		return err
	}
	return nil
}
