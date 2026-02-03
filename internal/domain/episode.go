package domain

import (
	"time"

	"github.com/google/uuid"
)

// ConsolidationStatus represents the processing state of an episode.
type ConsolidationStatus string

const (
	ConsolidationRaw       ConsolidationStatus = "raw"
	ConsolidationProcessed ConsolidationStatus = "processed"
	ConsolidationAbstracted ConsolidationStatus = "abstracted"
	ConsolidationArchived  ConsolidationStatus = "archived"
)

func ValidConsolidationStatus(s string) bool {
	switch ConsolidationStatus(s) {
	case ConsolidationRaw, ConsolidationProcessed, ConsolidationAbstracted, ConsolidationArchived:
		return true
	}
	return false
}

// OutcomeType represents the result of an episode/interaction.
type OutcomeType string

const (
	OutcomeSuccess OutcomeType = "success"
	OutcomeFailure OutcomeType = "failure"
	OutcomeNeutral OutcomeType = "neutral"
	OutcomeUnknown OutcomeType = "unknown"
)

func ValidOutcomeType(s string) bool {
	switch OutcomeType(s) {
	case OutcomeSuccess, OutcomeFailure, OutcomeNeutral, OutcomeUnknown:
		return true
	}
	return false
}

// CausalLink represents a cause-effect relationship extracted from an episode.
type CausalLink struct {
	Cause      string  `json:"cause"`
	Effect     string  `json:"effect"`
	Confidence float32 `json:"confidence"`
}

// Episode represents a rich experience with full context.
// Unlike semantic memories (beliefs/facts), episodes preserve the raw experience
// along with emotional context, entities, causal links, and outcome tracking.
type Episode struct {
	ID       uuid.UUID `json:"id"`
	AgentID  uuid.UUID `json:"agent_id"`
	TenantID uuid.UUID `json:"tenant_id,omitempty"`

	// Raw experience
	RawContent      string     `json:"raw_content"`
	ConversationID  *uuid.UUID `json:"conversation_id,omitempty"`
	MessageSequence *int       `json:"message_sequence,omitempty"`

	// Temporal context
	OccurredAt      time.Time `json:"occurred_at"`
	DurationSeconds *int      `json:"duration_seconds,omitempty"`
	TimeOfDay       string    `json:"time_of_day,omitempty"`  // morning, afternoon, evening, night
	DayOfWeek       string    `json:"day_of_week,omitempty"`  // monday, tuesday, etc.

	// Emotional markers
	EmotionalValence   *float32 `json:"emotional_valence,omitempty"`   // -1 to 1
	EmotionalIntensity *float32 `json:"emotional_intensity,omitempty"` // 0 to 1
	ImportanceScore    float32  `json:"importance_score"`

	// Extracted structure (populated by LLM)
	Entities    []string     `json:"entities,omitempty"`
	CausalLinks []CausalLink `json:"causal_links,omitempty"`
	Topics      []string     `json:"topics,omitempty"`

	// Outcome
	Outcome            OutcomeType `json:"outcome,omitempty"`
	OutcomeDescription string      `json:"outcome_description,omitempty"`
	OutcomeValence     *float32    `json:"outcome_valence,omitempty"`

	// Consolidation
	ConsolidationStatus  ConsolidationStatus `json:"consolidation_status"`
	LastConsolidatedAt   *time.Time          `json:"last_consolidated_at,omitempty"`
	AbstractionCount     int                 `json:"abstraction_count"`
	DerivedSemanticIDs   []uuid.UUID         `json:"derived_semantic_ids,omitempty"`
	DerivedProceduralIDs []uuid.UUID         `json:"derived_procedural_ids,omitempty"`

	// Memory strength (forgetting)
	MemoryStrength float32   `json:"memory_strength"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	AccessCount    int       `json:"access_count"`
	DecayRate      float32   `json:"decay_rate"`

	// Embedding
	Embedding []float32 `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AssociationType represents the type of link between episodes.
type AssociationType string

const (
	AssociationTemporal AssociationType = "temporal"
	AssociationCausal   AssociationType = "causal"
	AssociationThematic AssociationType = "thematic"
	AssociationEntity   AssociationType = "entity"
)

func ValidAssociationType(s string) bool {
	switch AssociationType(s) {
	case AssociationTemporal, AssociationCausal, AssociationThematic, AssociationEntity:
		return true
	}
	return false
}

// EpisodeAssociation represents a link between two related episodes.
type EpisodeAssociation struct {
	ID                  uuid.UUID       `json:"id"`
	EpisodeAID          uuid.UUID       `json:"episode_a_id"`
	EpisodeBID          uuid.UUID       `json:"episode_b_id"`
	AssociationType     AssociationType `json:"association_type"`
	AssociationStrength float32         `json:"association_strength"`
	CreatedAt           time.Time       `json:"created_at"`
}

// EpisodeWithScore is an Episode with a similarity score from recall.
type EpisodeWithScore struct {
	Episode
	Score float32 `json:"score"`
}
