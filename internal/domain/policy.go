package domain

import (
	"time"

	"github.com/google/uuid"
)

type Policy struct {
	ID             uuid.UUID  `json:"id"`
	AgentID        uuid.UUID  `json:"agent_id"`
	MemoryType     MemoryType `json:"memory_type"`
	MaxMemories    int        `json:"max_memories"`
	RetentionDays  *int       `json:"retention_days,omitempty"`
	PriorityWeight float64    `json:"priority_weight"`
	AutoSummarize  bool       `json:"auto_summarize"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
