package service

import (
	"context"
	"errors"

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
	err := s.store.Create(ctx, a)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ErrAgentConflict
		}
		return err
	}
	return nil
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
