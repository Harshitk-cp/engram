package domain

import (
	"time"

	"github.com/google/uuid"
)

// ActivationSource indicates how a memory was activated.
type ActivationSource string

const (
	ActivationSourceDirect   ActivationSource = "direct"   // Direct semantic match
	ActivationSourceSpread   ActivationSource = "spread"   // Spreading activation from associated memories
	ActivationSourceGoal     ActivationSource = "goal"     // Goal-directed activation
	ActivationSourceTemporal ActivationSource = "temporal" // Recent temporal activation
	ActivationSourceRecency  ActivationSource = "recency"  // Recently accessed memory
	ActivationSourceSchema   ActivationSource = "schema"   // Activated by matching schema
)

// ActivatedMemoryType indicates the type of memory in an activation.
type ActivatedMemoryType string

const (
	ActivatedMemoryTypeEpisodic   ActivatedMemoryType = "episodic"
	ActivatedMemoryTypeSemantic   ActivatedMemoryType = "semantic"
	ActivatedMemoryTypeProcedural ActivatedMemoryType = "procedural"
	ActivatedMemoryTypeSchema     ActivatedMemoryType = "schema"
)

// WorkingMemorySession represents an active working memory session for an agent.
// Working memory is the agent's "mental workspace" with limited capacity.
type WorkingMemorySession struct {
	ID       uuid.UUID `json:"id"`
	AgentID  uuid.UUID `json:"agent_id"`
	TenantID uuid.UUID `json:"tenant_id"`

	// Current state
	CurrentGoal    string           `json:"current_goal,omitempty"`
	ActiveContext  []Message        `json:"active_context,omitempty"`  // Recent messages
	ReasoningState map[string]any   `json:"reasoning_state,omitempty"` // Partial conclusions

	// Capacity
	MaxSlots int `json:"max_slots"` // Default: 7 (Miller's Law)

	// Session lifecycle
	StartedAt      time.Time  `json:"started_at"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`

	// Loaded activations (populated by service)
	Activations     []WorkingMemoryActivation `json:"activations,omitempty"`
	ActiveSchemas   []SchemaActivation        `json:"active_schemas,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkingMemoryActivation represents a memory activated in working memory.
type WorkingMemoryActivation struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`

	// What's activated
	MemoryType ActivatedMemoryType `json:"memory_type"`
	MemoryID   uuid.UUID           `json:"memory_id"`

	// Activation details
	ActivationLevel  float32          `json:"activation_level"`  // 0-1 activation strength
	ActivationSource ActivationSource `json:"activation_source"` // How it was activated
	ActivationCue    string           `json:"activation_cue,omitempty"` // What triggered it

	// Competition
	SlotPosition *int `json:"slot_position,omitempty"` // Position in limited slots

	ActivatedAt time.Time `json:"activated_at"`

	// Populated by service (not stored)
	Content         string  `json:"content,omitempty"`
	MemoryConfidence float32 `json:"memory_confidence,omitempty"`
}

// SchemaActivation represents an active schema in a working memory session.
type SchemaActivation struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	SchemaID  uuid.UUID `json:"schema_id"`

	MatchScore  float32   `json:"match_score"`
	ActivatedAt time.Time `json:"activated_at"`

	// Populated by service (not stored)
	Schema *Schema `json:"schema,omitempty"`
}

// MemoryAssociation links memories of different types for spreading activation.
type MemoryAssociation struct {
	ID                uuid.UUID           `json:"id"`
	SourceMemoryType  ActivatedMemoryType `json:"source_memory_type"`
	SourceMemoryID    uuid.UUID           `json:"source_memory_id"`
	TargetMemoryType  ActivatedMemoryType `json:"target_memory_type"`
	TargetMemoryID    uuid.UUID           `json:"target_memory_id"`
	AssociationType   string              `json:"association_type"` // derived, thematic, causal, temporal, entity
	AssociationStrength float32           `json:"association_strength"`
	CreatedAt         time.Time           `json:"created_at"`
}

// AssociationType constants
const (
	AssociationTypeDerived  = "derived"  // Target was derived from source (e.g., belief from episode)
	AssociationTypeThematic = "thematic" // Shared theme/topic
	AssociationTypeCausal   = "causal"   // Causal relationship
	AssociationTypeTemporal = "temporal" // Temporal proximity
	AssociationTypeEntity   = "entity"   // Shared entities
)

// ActivationInput contains input for memory activation.
type ActivationInput struct {
	AgentID  uuid.UUID `json:"agent_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Goal     string    `json:"goal,omitempty"`     // Current task goal
	Cues     []string  `json:"cues"`               // Activation cues (query, keywords)
	Context  []Message `json:"context,omitempty"`  // Recent conversation
}

// ActivatedContent holds full content for an activated memory.
type ActivatedContent struct {
	Type       ActivatedMemoryType `json:"type"`
	ID         uuid.UUID           `json:"id"`
	Content    string              `json:"content"`
	Confidence float32             `json:"confidence"`
	Score      float32             `json:"score"` // Combined activation score
}

// WorkingMemoryResult is the result of memory activation.
type WorkingMemoryResult struct {
	Session           *WorkingMemorySession `json:"session"`
	Activations       []ActivatedContent    `json:"activations"`
	ActiveSchemas     []SchemaMatch         `json:"active_schemas,omitempty"`
	SlotUsage         int                   `json:"slot_usage"`
	MaxSlots          int                   `json:"max_slots"`
	AssembledContext  string                `json:"assembled_context"` // Ready-to-use context for LLM
}
