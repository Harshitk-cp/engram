package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionEnded   SessionStatus = "ended"
	SessionExpired SessionStatus = "expired"
)

type Session struct {
	ID         uuid.UUID      `json:"id"`
	TenantID   uuid.UUID      `json:"tenant_id,omitempty"`
	AgentID    uuid.UUID      `json:"agent_id"`
	AnchorID   *uuid.UUID     `json:"anchor_id,omitempty"`
	ExternalID string         `json:"external_id,omitempty"`
	Status     SessionStatus  `json:"status"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	EndedAt    *time.Time     `json:"ended_at,omitempty"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// SessionStore persists sessions.
type SessionStore interface {
	Create(ctx context.Context, s *Session) error
	GetByID(ctx context.Context, id, tenantID uuid.UUID) (*Session, error)
	FindByExternalID(ctx context.Context, tenantID uuid.UUID, externalID string) (*Session, error)
	End(ctx context.Context, id, tenantID uuid.UUID, expiresAt time.Time) error
	ListExpired(ctx context.Context, limit int) ([]uuid.UUID, error)
	MarkExpired(ctx context.Context, id uuid.UUID) error
}
