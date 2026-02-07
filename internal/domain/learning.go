package domain

import (
	"time"

	"github.com/google/uuid"
)

type MutationType string

const (
	MutationFeedback      MutationType = "feedback"
	MutationOutcome       MutationType = "outcome"
	MutationDecay         MutationType = "decay"
	MutationReinforcement MutationType = "reinforcement"
	MutationContradiction MutationType = "contradiction"
)

type MutationSourceType string

const (
	MutationSourceExplicit MutationSourceType = "explicit"
	MutationSourceImplicit MutationSourceType = "implicit"
	MutationSourceSystem   MutationSourceType = "system"
)

type MutationLog struct {
	ID                     uuid.UUID          `json:"id"`
	MemoryID               uuid.UUID          `json:"memory_id"`
	AgentID                uuid.UUID          `json:"agent_id"`
	MutationType           MutationType       `json:"mutation_type"`
	SourceType             MutationSourceType `json:"source_type"`
	SourceID               *uuid.UUID         `json:"source_id,omitempty"`
	OldConfidence          *float32           `json:"old_confidence,omitempty"`
	NewConfidence          *float32           `json:"new_confidence,omitempty"`
	OldReinforcementCount  *int               `json:"old_reinforcement_count,omitempty"`
	NewReinforcementCount  *int               `json:"new_reinforcement_count,omitempty"`
	Reason                 string             `json:"reason"`
	Metadata               map[string]any     `json:"metadata,omitempty"`
	CreatedAt              time.Time          `json:"created_at"`
}

type MemoryUsageType string

const (
	UsageRetrieved         MemoryUsageType = "retrieved"
	UsageUsedInResponse    MemoryUsageType = "used_in_response"
	UsageInfluencedDecision MemoryUsageType = "influenced_decision"
)

type EpisodeMemoryUsage struct {
	ID             uuid.UUID       `json:"id"`
	EpisodeID      uuid.UUID       `json:"episode_id"`
	MemoryID       uuid.UUID       `json:"memory_id"`
	UsageType      MemoryUsageType `json:"usage_type"`
	RelevanceScore *float32        `json:"relevance_score,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type LearningStats struct {
	ID        uuid.UUID `json:"id"`
	AgentID   uuid.UUID `json:"agent_id"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	// Feedback counts
	HelpfulCount      int `json:"helpful_count"`
	UnhelpfulCount    int `json:"unhelpful_count"`
	IgnoredCount      int `json:"ignored_count"`
	ContradictedCount int `json:"contradicted_count"`
	OutdatedCount     int `json:"outdated_count"`

	// Outcome counts
	SuccessCount int `json:"success_count"`
	FailureCount int `json:"failure_count"`
	NeutralCount int `json:"neutral_count"`

	// Mutation counts
	ConfidenceIncreases int `json:"confidence_increases"`
	ConfidenceDecreases int `json:"confidence_decreases"`
	MemoriesReinforced  int `json:"memories_reinforced"`
	MemoriesArchived    int `json:"memories_archived"`

	// Computed metrics
	LearningVelocity *float32 `json:"learning_velocity,omitempty"`
	StabilityScore   *float32 `json:"stability_score,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type OutcomeRecord struct {
	EpisodeID     uuid.UUID   `json:"episode_id"`
	MemoriesUsed  []uuid.UUID `json:"memories_used"`
	Outcome       OutcomeType `json:"outcome"`
	UserSatisfied *bool       `json:"user_satisfied,omitempty"`
	OccurredAt    time.Time   `json:"occurred_at"`
}

// ImplicitFeedback represents feedback detected from conversation patterns.
type ImplicitFeedback struct {
	MemoryID    uuid.UUID    `json:"memory_id"`
	SignalType  FeedbackType `json:"signal_type"`
	Confidence  float32      `json:"confidence"`
	Evidence    string       `json:"evidence"`
}
