package domain

// Belief is a Memory with epistemic properties.
// This is a type alias for gradual transition - all memories are beliefs.
type Belief = Memory

// BeliefSource indicates where a belief originated.
type BeliefSource string

const (
	SourceUserStatement  BeliefSource = "user_statement"
	SourceAgentInference BeliefSource = "agent_inference"
	SourceToolOutput     BeliefSource = "tool_output"
	SourceExtraction     BeliefSource = "extraction"
)

// Contradiction represents a detected contradiction between two beliefs.
type Contradiction struct {
	ID               string `json:"id"`
	BeliefID         string `json:"belief_id"`
	ContradictedByID string `json:"contradicted_by_id"`
	DetectedAt       string `json:"detected_at"`
}
