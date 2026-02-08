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

type FeedbackEffect struct {
	LogOddsDelta       float64
	ReinforcementDelta int
	TriggerReview      bool
	TriggerSummarize   bool
}

var FeedbackEffects = map[FeedbackType]FeedbackEffect{
	FeedbackTypeHelpful: {
		LogOddsDelta:       +0.3,
		ReinforcementDelta: +1,
	},
	FeedbackTypeUnhelpful: {
		LogOddsDelta:       -0.5,
		ReinforcementDelta: -1,
	},
	FeedbackTypeUsed: {
		LogOddsDelta:       +0.1,
		ReinforcementDelta: 0,
	},
	FeedbackTypeIgnored: {
		LogOddsDelta:       -0.1,
		ReinforcementDelta: 0,
	},
	FeedbackTypeContradicted: {
		LogOddsDelta:       -1.0,
		ReinforcementDelta: -2,
		TriggerReview:      true,
	},
	FeedbackTypeOutdated: {
		LogOddsDelta:       -0.8,
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
