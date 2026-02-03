package domain

import (
	"time"

	"github.com/google/uuid"
)

// ActionType represents the type of action a procedure performs.
type ActionType string

const (
	ActionTypeResponseStyle   ActionType = "response_style"
	ActionTypeProblemSolving  ActionType = "problem_solving"
	ActionTypeCommunication   ActionType = "communication"
	ActionTypeWorkflow        ActionType = "workflow"
)

// ValidActionTypes returns all valid action types.
func ValidActionTypes() []ActionType {
	return []ActionType{
		ActionTypeResponseStyle,
		ActionTypeProblemSolving,
		ActionTypeCommunication,
		ActionTypeWorkflow,
	}
}

// IsValid checks if the action type is valid.
func (at ActionType) IsValid() bool {
	switch at {
	case ActionTypeResponseStyle, ActionTypeProblemSolving, ActionTypeCommunication, ActionTypeWorkflow:
		return true
	default:
		return false
	}
}

// Procedure represents a learned skill or pattern extracted from successful episodes.
// It encodes "when X situation, do Y action" patterns.
type Procedure struct {
	ID       uuid.UUID `json:"id"`
	AgentID  uuid.UUID `json:"agent_id"`
	TenantID uuid.UUID `json:"tenant_id"`

	// Trigger pattern (when to use this)
	TriggerPattern   string    `json:"trigger_pattern"`
	TriggerKeywords  []string  `json:"trigger_keywords,omitempty"`
	TriggerEmbedding []float32 `json:"-"`

	// Action/response pattern (what to do)
	ActionTemplate string     `json:"action_template"`
	ActionType     ActionType `json:"action_type"`

	// Effectiveness tracking
	UseCount     int        `json:"use_count"`
	SuccessCount int        `json:"success_count"`
	FailureCount int        `json:"failure_count"`
	SuccessRate  float32    `json:"success_rate"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`

	// Learning source
	DerivedFromEpisodes []uuid.UUID       `json:"derived_from_episodes,omitempty"`
	ExampleExchanges    []ExampleExchange `json:"example_exchanges,omitempty"`

	// Confidence and decay
	Confidence     float32    `json:"confidence"`
	MemoryStrength float32    `json:"memory_strength"`
	LastVerifiedAt *time.Time `json:"last_verified_at,omitempty"`

	// Versioning
	Version           int        `json:"version"`
	PreviousVersionID *uuid.UUID `json:"previous_version_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExampleExchange represents a concrete example of the procedure in use.
type ExampleExchange struct {
	Trigger  string `json:"trigger"`
	Response string `json:"response"`
	Outcome  string `json:"outcome,omitempty"`
}

// ProcedureWithScore represents a procedure with its match score from similarity search.
type ProcedureWithScore struct {
	Procedure
	Score float32 `json:"score"`
}

// ProcedureExtraction represents a procedure pattern extracted by LLM from episode content.
type ProcedureExtraction struct {
	TriggerPattern  string     `json:"trigger_pattern"`
	TriggerKeywords []string   `json:"trigger_keywords"`
	ActionTemplate  string     `json:"action_template"`
	ActionType      ActionType `json:"action_type"`
}
