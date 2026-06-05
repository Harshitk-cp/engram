package domain

import (
	"time"

	"github.com/google/uuid"
)

type MemoryType string

const (
	MemoryTypePreference MemoryType = "preference"
	MemoryTypeFact       MemoryType = "fact"
	MemoryTypeDecision   MemoryType = "decision"
	MemoryTypeConstraint MemoryType = "constraint"
	MemoryTypeBelief MemoryType = "belief"
)

type EvidenceType string

const (
	EvidenceExplicit   EvidenceType = "explicit_statement"
	EvidenceImplicit   EvidenceType = "implicit_inference"
	EvidenceBehavioral EvidenceType = "behavioral_signal"
)

func (e EvidenceType) ConfidenceRange() (min, max float32) {
	switch e {
	case EvidenceExplicit:
		return 0.85, 0.95
	case EvidenceImplicit:
		return 0.50, 0.75
	case EvidenceBehavioral:
		return 0.30, 0.55
	default:
		return 0.40, 0.60
	}
}

func (e EvidenceType) InitialConfidence() float32 {
	min, max := e.ConfidenceRange()
	return (min + max) / 2
}

func ValidEvidenceType(e string) bool {
	switch EvidenceType(e) {
	case EvidenceExplicit, EvidenceImplicit, EvidenceBehavioral:
		return true
	}
	return false
}

type Provenance string

const (
	ProvenanceUser     Provenance = "user"
	ProvenanceAgent    Provenance = "agent"
	ProvenanceTool     Provenance = "tool"
	ProvenanceDerived  Provenance = "derived"
	ProvenanceInferred Provenance = "inferred"
)

func ValidProvenance(p string) bool {
	switch Provenance(p) {
	case ProvenanceUser, ProvenanceAgent, ProvenanceTool, ProvenanceDerived, ProvenanceInferred:
		return true
	}
	return false
}

func (p Provenance) InitialConfidence() float32 {
	switch p {
	case ProvenanceUser:
		return 0.9
	case ProvenanceTool:
		return 0.8
	case ProvenanceAgent:
		return 0.6
	case ProvenanceDerived:
		return 0.5
	case ProvenanceInferred:
		return 0.4
	default:
		return 0.5
	}
}

func ValidMemoryType(t string) bool {
	switch MemoryType(t) {
	case MemoryTypePreference, MemoryTypeFact, MemoryTypeDecision, MemoryTypeConstraint, MemoryTypeBelief:
		return true
	}
	return false
}

type Memory struct {
	ID                 uuid.UUID      `json:"id"`
	AgentID            uuid.UUID      `json:"agent_id"`
	TenantID           uuid.UUID      `json:"tenant_id,omitempty"`
	Type               MemoryType     `json:"type"`
	Content            string         `json:"content"`
	Embedding          []float32      `json:"-"`
	EmbeddingProvider  string         `json:"embedding_provider,omitempty"`
	EmbeddingModel     string         `json:"embedding_model,omitempty"`
	Source             string         `json:"source,omitempty"`
	Provenance         Provenance     `json:"provenance"`
	Confidence         float32        `json:"confidence"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	EventDate          *time.Time     `json:"event_date,omitempty"`
	ExpiresAt          *time.Time     `json:"expires_at,omitempty"`
	LastVerifiedAt     *time.Time     `json:"last_verified_at,omitempty"`
	ReinforcementCount int            `json:"reinforcement_count"`
	DecayRate          float32        `json:"decay_rate"`
	LastAccessedAt     *time.Time     `json:"last_accessed_at,omitempty"`
	AccessCount        int            `json:"access_count"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	SourceMemoryID *uuid.UUID `json:"source_memory_id,omitempty"`
	BeliefSubject  string     `json:"belief_subject,omitempty"`
	BeliefPredicate string    `json:"belief_predicate,omitempty"`
	BeliefObject   string     `json:"belief_object,omitempty"`
}

type ConversationIngestRequest struct {
	AgentID   uuid.UUID      `json:"agent_id"`
	TenantID  uuid.UUID      `json:"-"`
	Messages  []Message      `json:"messages"`
	EventDate *time.Time     `json:"event_date,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Sync 	  bool 		     `json:"sync"`
}

type IngestResult struct {
	Stored   []*Memory `json:"stored"`
	Skipped  int       `json:"skipped"`
	Duration int64     `json:"duration_ms"`
}

type ExtractedConversationMemory struct {
	Type         MemoryType   `json:"type"`
	Content      string       `json:"content"`
	Confidence   float32      `json:"confidence,omitempty"`
	EvidenceType EvidenceType `json:"evidence_type,omitempty"`
	Source string `json:"source"`
}
