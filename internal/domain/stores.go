package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TenantStore interface {
	Create(ctx context.Context, t *Tenant) error
	GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*Tenant, error)
}

type AgentStore interface {
	Create(ctx context.Context, a *Agent) error
	GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Agent, error)
	GetByExternalID(ctx context.Context, externalID string, tenantID uuid.UUID) (*Agent, error)
}

type RecallOpts struct {
	TopK          int
	MemoryType    *MemoryType
	MinConfidence float32
}

type MemoryWithScore struct {
	Memory
	Score float32 `json:"score"`
}

type MemoryStore interface {
	Create(ctx context.Context, m *Memory) error
	GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Memory, error)
	Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
	Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts RecallOpts) ([]MemoryWithScore, error)
	CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType MemoryType) (int, error)
	ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType MemoryType, limit int) ([]Memory, error)
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType MemoryType, retentionDays int) (int64, error)
	// Belief system methods
	FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]MemoryWithScore, error)
	UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error
	UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error
	// Decay methods
	ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]Memory, error)
	Archive(ctx context.Context, id uuid.UUID) error
	IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error
}

type BeliefContradiction struct {
	ID               uuid.UUID
	BeliefID         uuid.UUID
	ContradictedByID uuid.UUID
	DetectedAt       any
}

type ContradictionStore interface {
	Create(ctx context.Context, beliefID, contradictedByID uuid.UUID) error
	GetByBeliefID(ctx context.Context, beliefID uuid.UUID) ([]BeliefContradiction, error)
	GetByContradictedByID(ctx context.Context, contradictedByID uuid.UUID) ([]BeliefContradiction, error)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ExtractedMemory struct {
	Type       MemoryType `json:"type"`
	Content    string     `json:"content"`
	Confidence float32    `json:"confidence"`
}

// EpisodeExtraction represents structured information extracted from an episode.
type EpisodeExtraction struct {
	Entities           []string     `json:"entities"`
	Topics             []string     `json:"topics"`
	CausalLinks        []CausalLink `json:"causal_links"`
	EmotionalValence   *float32     `json:"emotional_valence"`
	EmotionalIntensity *float32     `json:"emotional_intensity"`
	ImportanceScore    float32      `json:"importance_score"`
}

type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type LLMClient interface {
	Classify(ctx context.Context, content string) (MemoryType, error)
	Extract(ctx context.Context, conversation []Message) ([]ExtractedMemory, error)
	Summarize(ctx context.Context, memories []Memory) (string, error)
	CheckContradiction(ctx context.Context, stmtA, stmtB string) (bool, error)
	ExtractEpisodeStructure(ctx context.Context, content string) (*EpisodeExtraction, error)
	ExtractProcedure(ctx context.Context, content string) (*ProcedureExtraction, error)
	DetectSchemaPattern(ctx context.Context, memories []Memory) (*SchemaExtraction, error)
}

type PolicyStore interface {
	Upsert(ctx context.Context, p *Policy) error
	GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]Policy, error)
	GetByAgentIDAndType(ctx context.Context, agentID uuid.UUID, memType MemoryType) (*Policy, error)
}

type FeedbackStore interface {
	Create(ctx context.Context, f *Feedback) error
	GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]Feedback, error)
	GetByMemoryID(ctx context.Context, memoryID uuid.UUID) ([]Feedback, error)
	GetAggregatesByAgentID(ctx context.Context, agentID uuid.UUID) ([]FeedbackAggregate, error)
	CountByAgentID(ctx context.Context, agentID uuid.UUID) (int, error)
	ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error)
}

type FeedbackAggregate struct {
	AgentID    uuid.UUID
	MemoryType MemoryType
	SignalType FeedbackType
	Count      int
}

// EpisodeStore handles storage and retrieval of episodic memories.
type EpisodeStore interface {
	// Core CRUD
	Create(ctx context.Context, e *Episode) error
	GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Episode, error)

	// Retrieval
	GetByConversationID(ctx context.Context, conversationID uuid.UUID, tenantID uuid.UUID) ([]Episode, error)
	GetByTimeRange(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, start, end time.Time) ([]Episode, error)
	GetByImportance(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, minImportance float32, limit int) ([]Episode, error)
	FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]EpisodeWithScore, error)

	// Consolidation
	GetUnconsolidated(ctx context.Context, agentID uuid.UUID, limit int) ([]Episode, error)
	GetByConsolidationStatus(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, status ConsolidationStatus, limit int) ([]Episode, error)
	UpdateConsolidationStatus(ctx context.Context, id uuid.UUID, status ConsolidationStatus) error
	LinkDerivedMemory(ctx context.Context, episodeID uuid.UUID, memoryID uuid.UUID, memoryType string) error

	// Decay
	ApplyDecay(ctx context.Context, agentID uuid.UUID) (int64, error)
	GetWeakMemories(ctx context.Context, agentID uuid.UUID, threshold float32) ([]Episode, error)
	Archive(ctx context.Context, id uuid.UUID) error
	UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error

	// Access tracking
	RecordAccess(ctx context.Context, id uuid.UUID) error

	// Outcome
	UpdateOutcome(ctx context.Context, id uuid.UUID, outcome OutcomeType, description string) error

	// Associations
	CreateAssociation(ctx context.Context, a *EpisodeAssociation) error
	GetAssociations(ctx context.Context, episodeID uuid.UUID) ([]EpisodeAssociation, error)

	// For decay operations
	GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]Episode, error)
}

// ProcedureStore handles storage and retrieval of procedural memories (skills & patterns).
type ProcedureStore interface {
	// Core CRUD
	Create(ctx context.Context, p *Procedure) error
	GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Procedure, error)
	GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]Procedure, error)

	// Similarity search
	FindByTriggerSimilarity(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]ProcedureWithScore, error)

	// Effectiveness tracking
	RecordUse(ctx context.Context, id uuid.UUID, success bool) error
	Reinforce(ctx context.Context, id uuid.UUID, episodeID uuid.UUID, confidenceBoost float32) error

	// Decay and archival
	UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error
	Archive(ctx context.Context, id uuid.UUID) error
	GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]Procedure, error)

	// Versioning
	CreateNewVersion(ctx context.Context, p *Procedure) error
}

// SchemaStore handles storage and retrieval of schemas (mental models).
type SchemaStore interface {
	// Core CRUD
	Create(ctx context.Context, s *Schema) error
	GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Schema, error)
	GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]Schema, error)
	GetByName(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, schemaType SchemaType, name string) (*Schema, error)
	Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error

	// Similarity search
	FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]SchemaWithScore, error)

	// Evidence tracking
	AddEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error
	RemoveEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error

	// Confidence and validation
	UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error
	IncrementContradiction(ctx context.Context, id uuid.UUID) error
	UpdateValidation(ctx context.Context, id uuid.UUID) error

	// Updates
	Update(ctx context.Context, s *Schema) error
}

// WorkingMemoryStore handles storage of working memory sessions and activations.
type WorkingMemoryStore interface {
	// Session management
	CreateSession(ctx context.Context, s *WorkingMemorySession) error
	GetSession(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (*WorkingMemorySession, error)
	GetSessionByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*WorkingMemorySession, error)
	UpdateSession(ctx context.Context, s *WorkingMemorySession) error
	DeleteSession(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) error
	UpdateLastActivity(ctx context.Context, sessionID uuid.UUID) error

	// Memory activations
	CreateActivation(ctx context.Context, a *WorkingMemoryActivation) error
	GetActivations(ctx context.Context, sessionID uuid.UUID) ([]WorkingMemoryActivation, error)
	ClearActivations(ctx context.Context, sessionID uuid.UUID) error
	DeleteActivation(ctx context.Context, sessionID uuid.UUID, memoryType ActivatedMemoryType, memoryID uuid.UUID) error

	// Schema activations
	CreateSchemaActivation(ctx context.Context, a *SchemaActivation) error
	GetSchemaActivations(ctx context.Context, sessionID uuid.UUID) ([]SchemaActivation, error)
	ClearSchemaActivations(ctx context.Context, sessionID uuid.UUID) error

	// Session expiration
	DeleteExpiredSessions(ctx context.Context) (int64, error)
}

// MemoryAssociationStore handles cross-memory associations for spreading activation.
type MemoryAssociationStore interface {
	Create(ctx context.Context, a *MemoryAssociation) error
	GetBySource(ctx context.Context, sourceType ActivatedMemoryType, sourceID uuid.UUID) ([]MemoryAssociation, error)
	GetByTarget(ctx context.Context, targetType ActivatedMemoryType, targetID uuid.UUID) ([]MemoryAssociation, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error
}
