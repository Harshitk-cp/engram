package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

type MutationType string

const (
	MutationFeedback     	  MutationType = "feedback"
	MutationOutcome       	  MutationType = "outcome"
	MutationDecay         	  MutationType = "decay"
	MutationReinforcement 	  MutationType = "reinforcement"
	MutationContradiction 	  MutationType = "contradiction"
	MutationDeletion      	  MutationType = "deletion"
	MutationArchive       	  MutationType = "archive"
	MutationAdminOverride 	  MutationType = "admin_override"
	MutationRedaction     	  MutationType = "redaction"
	MutationQuarantine        MutationType = "quarantine"
	MutationQuarantineRelease MutationType = "quarantine_release"
	MutationQuarantineReject  MutationType = "quarantine_reject"
)

type MutationSourceType string

const (
	MutationSourceExplicit MutationSourceType = "explicit"
	MutationSourceImplicit MutationSourceType = "implicit"
	MutationSourceSystem   MutationSourceType = "system"
	MutationSourceAdmin    MutationSourceType = "admin"
)

type MutationLog struct {
	ID                    uuid.UUID          `json:"id"`
	MemoryID              uuid.UUID          `json:"memory_id"`
	AgentID               uuid.UUID          `json:"agent_id"`
	MutationType          MutationType       `json:"mutation_type"`
	SourceType            MutationSourceType `json:"source_type"`
	SourceID              *uuid.UUID         `json:"source_id,omitempty"`
	OldConfidence         *float32           `json:"old_confidence,omitempty"`
	NewConfidence         *float32           `json:"new_confidence,omitempty"`
	OldReinforcementCount *int               `json:"old_reinforcement_count,omitempty"`
	NewReinforcementCount *int               `json:"new_reinforcement_count,omitempty"`
	Reason                string             `json:"reason"`
	Metadata              map[string]any     `json:"metadata,omitempty"`
	// Denormalized identity + content snapshot so a log row survives deletion of
	// its memory (memory_id is SET NULL on delete). content_snapshot is populated
	// only for deletion/redaction events.
	TenantID        *uuid.UUID `json:"tenant_id,omitempty"`
	AnchorID        *uuid.UUID `json:"anchor_id,omitempty"`
	Binding         string     `json:"binding,omitempty"`
	ContentHash     string     `json:"content_hash,omitempty"`
	ContentSnapshot *string    `json:"content_snapshot,omitempty"`
	// Actor attribution for operator (admin) actions; data-plane actor is an API key.
	ActorType string     `json:"actor_type,omitempty"`
	ActorID   *uuid.UUID `json:"actor_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	// Tamper-evident hash chain (assigned by a DB trigger, per tenant).
	Seq      int64  `json:"seq,omitempty"`
	PrevHash string `json:"prev_hash,omitempty"`
	RowHash  string `json:"row_hash,omitempty"`
}

// HashContent returns a hex sha256 of memory content, used to record what was
// deleted/redacted without retaining the original text.
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

type MemoryUsageType string

const (
	UsageRetrieved          MemoryUsageType = "retrieved"
	UsageUsedInResponse     MemoryUsageType = "used_in_response"
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
	ID          uuid.UUID `json:"id"`
	AgentID     uuid.UUID `json:"agent_id"`
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
	MemoryID   uuid.UUID    `json:"memory_id"`
	SignalType FeedbackType `json:"signal_type"`
	Confidence float32      `json:"confidence"`
	Evidence   string       `json:"evidence"`
}
