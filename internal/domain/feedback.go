package domain

import (
	"time"

	"github.com/google/uuid"
)

type FeedbackType string

const (
	FeedbackTypeUsed        FeedbackType = "used"
	FeedbackTypeIgnored     FeedbackType = "ignored"
	FeedbackTypeHelpful     FeedbackType = "helpful"
	FeedbackTypeUnhelpful   FeedbackType = "unhelpful"
	FeedbackTypeContradicted FeedbackType = "contradicted"
	FeedbackTypeOutdated    FeedbackType = "outdated"
)

func ValidFeedbackType(t string) bool {
	switch FeedbackType(t) {
	case FeedbackTypeUsed, FeedbackTypeIgnored, FeedbackTypeHelpful, FeedbackTypeUnhelpful,
		FeedbackTypeContradicted, FeedbackTypeOutdated:
		return true
	}
	return false
}

// FeedbackEffect defines how feedback mutates memory confidence and state.
type FeedbackEffect struct {
	ConfidenceDelta    float32
	ReinforcementDelta int
	TriggerReview      bool
	TriggerSummarize   bool
}

// FeedbackEffects maps feedback types to their mutation effects.
var FeedbackEffects = map[FeedbackType]FeedbackEffect{
	FeedbackTypeHelpful: {
		ConfidenceDelta:    +0.05,
		ReinforcementDelta: +1,
	},
	FeedbackTypeUnhelpful: {
		ConfidenceDelta:    -0.10,
		ReinforcementDelta: -1,
	},
	FeedbackTypeUsed: {
		ConfidenceDelta:    +0.02,
		ReinforcementDelta: 0,
	},
	FeedbackTypeIgnored: {
		ConfidenceDelta:    -0.02,
		ReinforcementDelta: 0,
	},
	FeedbackTypeContradicted: {
		ConfidenceDelta:    -0.20,
		ReinforcementDelta: -2,
		TriggerReview:      true,
	},
	FeedbackTypeOutdated: {
		ConfidenceDelta:    -0.15,
		ReinforcementDelta: -1,
		TriggerSummarize:   true,
	},
}

type Feedback struct {
	ID         uuid.UUID      `json:"id"`
	MemoryID   uuid.UUID      `json:"memory_id"`
	AgentID    uuid.UUID      `json:"agent_id"`
	SignalType FeedbackType   `json:"signal_type"`
	Context    map[string]any `json:"context,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}
