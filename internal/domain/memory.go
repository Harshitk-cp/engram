package domain

import (
	"time"

	"github.com/google/uuid"
)

type MemoryType string

const (
	MemoryTypePreference MemoryType = "preference"
	MemoryTypeFact       MemoryType = "fact"
	MemoryTypeDecision   MemoryType = "decision"
	MemoryTypeConstraint MemoryType = "constraint"
)

func ValidMemoryType(t string) bool {
	switch MemoryType(t) {
	case MemoryTypePreference, MemoryTypeFact, MemoryTypeDecision, MemoryTypeConstraint:
		return true
	}
	return false
}

type Memory struct {
	ID                 uuid.UUID      `json:"id"`
	AgentID            uuid.UUID      `json:"agent_id"`
	TenantID           uuid.UUID      `json:"tenant_id,omitempty"`
	Type               MemoryType     `json:"type"`
	Content            string         `json:"content"`
	Embedding          []float32      `json:"-"`
	EmbeddingProvider  string         `json:"embedding_provider,omitempty"`
	EmbeddingModel     string         `json:"embedding_model,omitempty"`
	Source             string         `json:"source,omitempty"`
	Confidence         float32        `json:"confidence"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	ExpiresAt          *time.Time     `json:"expires_at,omitempty"`
	LastVerifiedAt     *time.Time     `json:"last_verified_at,omitempty"`
	ReinforcementCount int            `json:"reinforcement_count"`
	DecayRate          float32        `json:"decay_rate"`
	LastAccessedAt     *time.Time     `json:"last_accessed_at,omitempty"`
	AccessCount        int            `json:"access_count"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}
