package domain

import (
	"time"

	"github.com/google/uuid"
)

// SchemaType represents the type of mental model.
type SchemaType string

const (
	SchemaTypeUserArchetype     SchemaType = "user_archetype"
	SchemaTypeSituationTemplate SchemaType = "situation_template"
	SchemaTypeCausalModel       SchemaType = "causal_model"
)

// ValidSchemaTypes returns all valid schema types.
func ValidSchemaTypes() []SchemaType {
	return []SchemaType{
		SchemaTypeUserArchetype,
		SchemaTypeSituationTemplate,
		SchemaTypeCausalModel,
	}
}

// IsValid checks if the schema type is valid.
func (st SchemaType) IsValid() bool {
	switch st {
	case SchemaTypeUserArchetype, SchemaTypeSituationTemplate, SchemaTypeCausalModel:
		return true
	default:
		return false
	}
}

// Schema represents a mental model derived from patterns across semantic memories.
// Examples: "Night-owl power user", "Technical expert", "Impatient debugger"
type Schema struct {
	ID       uuid.UUID `json:"id"`
	AgentID  uuid.UUID `json:"agent_id"`
	TenantID uuid.UUID `json:"tenant_id"`

	// Schema identification
	SchemaType  SchemaType `json:"schema_type"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`

	// Schema attributes (the mental model)
	// Example: {"communication_style": "direct", "technical_level": "expert"}
	Attributes map[string]any `json:"attributes"`

	// Evidence tracking
	EvidenceMemories []uuid.UUID `json:"evidence_memories,omitempty"`
	EvidenceEpisodes []uuid.UUID `json:"evidence_episodes,omitempty"`
	EvidenceCount    int         `json:"evidence_count"`

	// Confidence and validation
	Confidence         float32    `json:"confidence"`
	LastValidatedAt    *time.Time `json:"last_validated_at,omitempty"`
	ContradictionCount int        `json:"contradiction_count"`

	// Applicable contexts (when this schema should activate)
	// Example: ["debugging", "late_night", "code_review"]
	ApplicableContexts []string `json:"applicable_contexts,omitempty"`

	// Embedding for similarity matching
	Embedding []float32 `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SchemaWithScore represents a schema with its match score.
type SchemaWithScore struct {
	Schema
	Score float32 `json:"score"`
}

// SchemaMatch represents a schema matched to a current situation.
type SchemaMatch struct {
	Schema      Schema  `json:"schema"`
	MatchScore  float32 `json:"match_score"`
	MatchReason string  `json:"match_reason,omitempty"`
}

// SchemaExtraction represents a schema pattern detected by LLM from memory clusters.
type SchemaExtraction struct {
	SchemaType         SchemaType     `json:"schema_type"`
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	Attributes         map[string]any `json:"attributes"`
	ApplicableContexts []string       `json:"applicable_contexts,omitempty"`
	Confidence         float32        `json:"confidence"`
}

// MemoryCluster represents a group of semantically related memories.
type MemoryCluster struct {
	Memories  []Memory    `json:"memories"`
	MemoryIDs []uuid.UUID `json:"memory_ids"`
	Theme     string      `json:"theme"`
	Centroid  []float32   `json:"-"`
}
