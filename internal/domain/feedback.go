package domain

import (
	"time"

	"github.com/google/uuid"
)

type FeedbackType string

const (
	FeedbackTypeUsed      FeedbackType = "used"
	FeedbackTypeIgnored   FeedbackType = "ignored"
	FeedbackTypeHelpful   FeedbackType = "helpful"
	FeedbackTypeUnhelpful FeedbackType = "unhelpful"
)

func ValidFeedbackType(t string) bool {
	switch FeedbackType(t) {
	case FeedbackTypeUsed, FeedbackTypeIgnored, FeedbackTypeHelpful, FeedbackTypeUnhelpful:
		return true
	}
	return false
}

type Feedback struct {
	ID         uuid.UUID      `json:"id"`
	MemoryID   uuid.UUID      `json:"memory_id"`
	AgentID    uuid.UUID      `json:"agent_id"`
	SignalType FeedbackType   `json:"signal_type"`
	Context    map[string]any `json:"context,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}
