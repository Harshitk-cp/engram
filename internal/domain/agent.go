package domain

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID         uuid.UUID         `json:"id"`
	TenantID   uuid.UUID         `json:"tenant_id,omitempty"`
	ExternalID string            `json:"external_id"`
	Name       string            `json:"name"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}
