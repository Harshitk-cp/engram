package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type RelationType string

const (
	RelationEntityLink  RelationType = "entity_link"
	RelationCausal      RelationType = "causal"
	RelationTemporal    RelationType = "temporal"
	RelationThematic    RelationType = "thematic"
	RelationContradicts RelationType = "contradicts"
	RelationSupports    RelationType = "supports"
	RelationDerivedFrom RelationType = "derived_from"
	RelationSupersedes  RelationType = "supersedes"
)

func ValidRelationType(r string) bool {
	switch RelationType(r) {
	case RelationEntityLink, RelationCausal, RelationTemporal, RelationThematic,
		RelationContradicts, RelationSupports, RelationDerivedFrom, RelationSupersedes:
		return true
	}
	return false
}

// SymmetricRelations indicates which relations are bidirectional
var SymmetricRelations = map[RelationType]bool{
	RelationEntityLink: true,
	RelationThematic:   true,
	RelationSupports:   true,
}

// AsymmetricRelations indicates which relations are directional
var AsymmetricRelations = map[RelationType]bool{
	RelationCausal:      true,
	RelationTemporal:    true,
	RelationDerivedFrom: true,
	RelationSupersedes:  true,
	RelationContradicts: true,
}

// RelationDecayMultipliers controls how fast activation decays when traversing each relation type
var RelationDecayMultipliers = map[RelationType]float64{
	RelationEntityLink:  0.7,  // Standard decay
	RelationCausal:      0.9,  // Slow decay - causality is stable
	RelationTemporal:    0.6,  // Fast decay - temporal proximity fades
	RelationThematic:    0.7,  // Standard
	RelationSupports:    0.85, // Slow decay - support relationships are valuable
	RelationContradicts: 0.95, // Very slow - contradictions are important to remember
	RelationDerivedFrom: 0.8,  // Moderate - derivation chains matter
	RelationSupersedes:  0.9,  // Slow - supersession is structural
}

type GraphEdge struct {
	ID             uuid.UUID    `json:"id"`
	SourceID       uuid.UUID    `json:"source_id"`
	TargetID       uuid.UUID    `json:"target_id"`
	RelationType   RelationType `json:"relation_type"`
	Strength       float32      `json:"strength"`
	CreatedAt      time.Time    `json:"created_at"`
	LastTraversedAt *time.Time  `json:"last_traversed_at,omitempty"`
	TraversalCount int          `json:"traversal_count"`
}

type EntityType string

const (
	EntityPerson       EntityType = "person"
	EntityOrganization EntityType = "organization"
	EntityTool         EntityType = "tool"
	EntityConcept      EntityType = "concept"
	EntityLocation     EntityType = "location"
	EntityEvent        EntityType = "event"
	EntityProduct      EntityType = "product"
	EntityOther        EntityType = "other"
)

func ValidEntityType(e string) bool {
	switch EntityType(e) {
	case EntityPerson, EntityOrganization, EntityTool, EntityConcept,
		EntityLocation, EntityEvent, EntityProduct, EntityOther:
		return true
	}
	return false
}

type Entity struct {
	ID         uuid.UUID      `json:"id"`
	AgentID    uuid.UUID      `json:"agent_id"`
	Name       string         `json:"name"`
	EntityType EntityType     `json:"entity_type"`
	Aliases    []string       `json:"aliases,omitempty"`
	Embedding  []float32      `json:"embedding,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type MentionType string

const (
	MentionSubject MentionType = "subject"
	MentionObject  MentionType = "object"
	MentionContext MentionType = "context"
)

// MentionTypeWeights controls edge strength based on how central the entity is to the memory
var MentionTypeWeights = map[MentionType]float64{
	MentionSubject: 0.9, // Entity is central to the memory
	MentionObject:  0.7, // Entity is involved but not central
	MentionContext: 0.4, // Entity is peripheral/mentioned in passing
}

type EntityMention struct {
	EntityID    uuid.UUID   `json:"entity_id"`
	MemoryID    uuid.UUID   `json:"memory_id"`
	MentionType MentionType `json:"mention_type"`
	CreatedAt   time.Time   `json:"created_at"`
}

type GraphNeighbor struct {
	Edge   GraphEdge `json:"edge"`
	Memory *Memory   `json:"memory,omitempty"`
}

// TraversalConstraints controls graph traversal behavior
type TraversalConstraints struct {
	RespectTemporalOrder bool            // Only traverse to newer memories for causal edges
	MaxAge               time.Duration   // Only include memories from last N duration
	RelationFilter       []RelationType  // Only traverse these relation types (empty = all)
	MinEdgeStrength      float32         // Only traverse edges above this strength
}

// EdgeDecayResult tracks the outcome of edge decay operations
type EdgeDecayResult struct {
	Processed int
	Decayed   int
	Pruned    int
}

// PruningRules controls graph pruning behavior
type PruningRules struct {
	StrengthThreshold float32       // Delete edges below this strength (default 0.05)
	StaleThreshold    time.Duration // Delete edges not traversed in this duration
	MaxEdgesPerMemory int           // Keep only strongest N edges per memory
}

func DefaultPruningRules() PruningRules {
	return PruningRules{
		StrengthThreshold: 0.05,
		StaleThreshold:    90 * 24 * time.Hour,
		MaxEdgesPerMemory: 50,
	}
}

type GraphStore interface {
	CreateEdge(ctx context.Context, edge *GraphEdge) error
	GetEdge(ctx context.Context, sourceID, targetID uuid.UUID, relationType RelationType) (*GraphEdge, error)
	DeleteEdge(ctx context.Context, id uuid.UUID) error
	GetNeighbors(ctx context.Context, memoryID uuid.UUID, direction string, relationTypes []RelationType) ([]GraphEdge, error)
	UpdateTraversalStats(ctx context.Context, id uuid.UUID) error
	GetEdgesBySource(ctx context.Context, sourceID uuid.UUID) ([]GraphEdge, error)
	GetEdgesByTarget(ctx context.Context, targetID uuid.UUID) ([]GraphEdge, error)
	DeleteEdgesByMemory(ctx context.Context, memoryID uuid.UUID) error

	RecordTraversal(ctx context.Context, edgeID uuid.UUID, strengthBoost float32) error

	ApplyEdgeDecay(ctx context.Context, agentID uuid.UUID, decayRate float64) (*EdgeDecayResult, error)

	PruneGraph(ctx context.Context, agentID uuid.UUID, rules PruningRules) (*EdgeDecayResult, error)
}

type EntityStore interface {
	Create(ctx context.Context, e *Entity) error
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)
	Delete(ctx context.Context, id uuid.UUID) error
	FindByName(ctx context.Context, agentID uuid.UUID, name string) (*Entity, error)
	FindByNameOrAlias(ctx context.Context, agentID uuid.UUID, name string) (*Entity, error)
	FindByEmbeddingSimilarity(ctx context.Context, agentID uuid.UUID, entityType EntityType, embedding []float32, threshold float32, limit int) ([]Entity, error)
	GetByAgent(ctx context.Context, agentID uuid.UUID) ([]Entity, error)
	AddAlias(ctx context.Context, id uuid.UUID, alias string) error
	UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error

	CreateMention(ctx context.Context, m *EntityMention) error
	GetMentionsByEntity(ctx context.Context, entityID uuid.UUID) ([]EntityMention, error)
	GetMentionsByMemory(ctx context.Context, memoryID uuid.UUID) ([]EntityMention, error)
	DeleteMentionsByMemory(ctx context.Context, memoryID uuid.UUID) error
	GetMemoriesForEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]Memory, error)
	GetEntitiesForMemory(ctx context.Context, memoryID uuid.UUID) ([]Entity, error)
}

type ExtractedEntity struct {
	Name       string     `json:"name"`
	EntityType EntityType `json:"entity_type"`
	Role       MentionType `json:"role"`
}

type DetectedRelationship struct {
	SourceID     uuid.UUID    `json:"source_id"`
	TargetID     uuid.UUID    `json:"target_id"`
	RelationType RelationType `json:"relation_type"`
	Strength     float32      `json:"strength"`
	Reason       string       `json:"reason,omitempty"`
}

type HybridRecallRequest struct {
	Query        string    `json:"query"`
	AgentID      uuid.UUID `json:"agent_id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	TopK         int       `json:"top_k"`
	VectorWeight float64   `json:"vector_weight"`
	GraphWeight  float64   `json:"graph_weight"`
	MaxGraphHops int       `json:"max_graph_hops"`
	UseGraph     bool      `json:"use_graph"`
	MinConfidence float32  `json:"min_confidence,omitempty"`
	MemoryType   *MemoryType `json:"memory_type,omitempty"`
}

type ScoredMemory struct {
	Memory
	VectorScore   float32 `json:"vector_score"`
	GraphScore    float32 `json:"graph_score"`
	FinalScore    float32 `json:"score"`
	GraphPath     []uuid.UUID `json:"graph_path,omitempty"`
	PathLength    int     `json:"path_length,omitempty"`
}

type GraphTraversalResult struct {
	MemoryID       uuid.UUID    `json:"memory_id"`
	GraphRelevance float32      `json:"graph_relevance"`
	PathLength     int          `json:"path_length"`
	RelationType   RelationType `json:"relation_type"`
	Path           []uuid.UUID  `json:"path,omitempty"`
}
