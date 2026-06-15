package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrReasonRequired is returned when an audited admin operation is attempted
// without a reason. Every operator action must be attributable and explained.
var ErrReasonRequired = errors.New("reason is required")

const redactionTombstone = "[REDACTED]"

// AdminService performs operator-initiated memory changes as first-class, audited
// mutations (mutation_type=admin_override/redaction) with required reason and
// API-key actor attribution. It never edits state outside the audit pipeline.
type AdminService struct {
	memoryStore     domain.MemoryStore
	embeddingClient domain.EmbeddingClient
	uow             *store.UnitOfWork
	logger          *zap.Logger
}

func NewAdminService(ms domain.MemoryStore, ec domain.EmbeddingClient, uow *store.UnitOfWork, logger *zap.Logger) *AdminService {
	return &AdminService{memoryStore: ms, embeddingClient: ec, uow: uow, logger: logger}
}

// adminMutation builds an audit row for an operator action. ContentHash is the
// hash of the memory's content at the time of the action.
func adminMutation(mem *domain.Memory, mtype domain.MutationType, reason, actorType string, actorID uuid.UUID) *domain.MutationLog {
	tid := mem.TenantID
	aid := actorID
	return &domain.MutationLog{
		MemoryID:     mem.ID,
		AgentID:      mem.AgentID,
		MutationType: mtype,
		SourceType:   domain.MutationSourceAdmin,
		Reason:       reason,
		TenantID:     &tid,
		AnchorID:     mem.AnchorID,
		Binding:      string(mem.Binding),
		ContentHash:  domain.HashContent(mem.Content),
		ActorType:    actorType,
		ActorID:      &aid,
	}
}

func mapMemoryErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrMemoryNotFound
	}
	return err
}

// UpdateConfidence overrides a memory's confidence, recording the before/after.
func (s *AdminService) UpdateConfidence(ctx context.Context, memID, tenantID uuid.UUID, newConfidence float32, reason, actorType string, actorID uuid.UUID) (*domain.Memory, error) {
	if reason == "" {
		return nil, ErrReasonRequired
	}
	mem, err := s.memoryStore.GetByID(ctx, memID, tenantID)
	if err != nil {
		return nil, mapMemoryErr(err)
	}

	old := mem.Confidence
	mut := adminMutation(mem, domain.MutationAdminOverride, reason, actorType, actorID)
	mut.OldConfidence = &old
	mut.NewConfidence = &newConfidence

	if err := s.uow.Do(ctx, func(st *store.TxStores) error {
		if err := st.Memory.UpdateConfidence(ctx, memID, newConfidence); err != nil {
			return err
		}
		return st.MutationLog.Create(ctx, mut)
	}); err != nil {
		return nil, err
	}
	mem.Confidence = newConfidence
	return mem, nil
}

// UpdateContent corrects a memory's content, re-embedding when possible. The
// audit row keeps the old content hash; the new hash is recorded in metadata.
func (s *AdminService) UpdateContent(ctx context.Context, memID, tenantID uuid.UUID, content, reason, actorType string, actorID uuid.UUID) (*domain.Memory, error) {
	if reason == "" {
		return nil, ErrReasonRequired
	}
	if content == "" {
		return nil, errors.New("content is required")
	}
	mem, err := s.memoryStore.GetByID(ctx, memID, tenantID)
	if err != nil {
		return nil, mapMemoryErr(err)
	}

	var embedding []float32
	if s.embeddingClient != nil {
		if emb, embErr := s.embeddingClient.Embed(ctx, content); embErr != nil {
			s.logger.Warn("failed to re-embed corrected content; keeping prior embedding", zap.Error(embErr))
		} else {
			embedding = emb
		}
	}

	mut := adminMutation(mem, domain.MutationAdminOverride, reason, actorType, actorID)
	mut.Metadata = map[string]any{"new_content_hash": domain.HashContent(content)}

	if err := s.uow.Do(ctx, func(st *store.TxStores) error {
		if err := st.Memory.UpdateContent(ctx, memID, content, embedding); err != nil {
			return err
		}
		return st.MutationLog.Create(ctx, mut)
	}); err != nil {
		return nil, err
	}
	mem.Content = content
	return mem, nil
}

// RedactMemory replaces a memory's content with a tombstone and clears its
// embedding (GDPR redaction). The audit row keeps the original content hash as
// proof of what was redacted, without retaining the original text.
func (s *AdminService) RedactMemory(ctx context.Context, memID, tenantID uuid.UUID, reason, actorType string, actorID uuid.UUID) error {
	if reason == "" {
		return ErrReasonRequired
	}
	mem, err := s.memoryStore.GetByID(ctx, memID, tenantID)
	if err != nil {
		return mapMemoryErr(err)
	}

	mut := adminMutation(mem, domain.MutationRedaction, reason, actorType, actorID)
	return s.uow.Do(ctx, func(st *store.TxStores) error {
		if err := st.Memory.RedactContent(ctx, memID, redactionTombstone); err != nil {
			return err
		}
		return st.MutationLog.Create(ctx, mut)
	})
}

// CryptoShredAnchor cryptographically erases every memory bound to a subject
// (anchor): each content is replaced with AES-GCM ciphertext under a key that is
// immediately discarded, the embedding is cleared, and a redaction is recorded in
// the immutable audit chain. The rows and audit history remain (provable that data
// existed and was erased) but the content is permanently unrecoverable — GDPR
// right-to-erasure that's compatible with an append-only audit log.
func (s *AdminService) CryptoShredAnchor(ctx context.Context, anchorID, tenantID uuid.UUID, reason, actorType string, actorID uuid.UUID) (int, error) {
	if reason == "" {
		return 0, ErrReasonRequired
	}
	count := 0
	err := s.uow.Do(ctx, func(st *store.TxStores) error {
		// shred crypto-shreds one memory's content and audits it.
		shred := func(m *domain.Memory) error {
			ct, err := cryptoShred(m.Content)
			if err != nil {
				return err
			}
			if err := st.Memory.RedactContent(ctx, m.ID, ct); err != nil {
				return err
			}
			if err := st.MutationLog.Create(ctx, adminMutation(m, domain.MutationRedaction, reason, actorType, actorID)); err != nil {
				return err
			}
			count++
			return nil
		}

		// 1. Memories directly bound to the subject.
		mems, err := st.Memory.ListByAnchor(ctx, anchorID, tenantID, 100000)
		if err != nil {
			return err
		}
		erasedIDs := make([]uuid.UUID, 0, len(mems))
		for i := range mems {
			if err := shred(&mems[i]); err != nil {
				return err
			}
			erasedIDs = append(erasedIDs, mems[i].ID)
		}

		// 2. Beliefs transitively DERIVED from the subject's memories — these may
		//    encode the subject's data even though they aren't anchor-bound.
		if len(erasedIDs) > 0 {
			derived, err := st.Memory.DerivedMemoryClosure(ctx, erasedIDs, tenantID)
			if err != nil {
				return err
			}
			for i := range derived {
				if err := shred(&derived[i]); err != nil {
					return err
				}
				erasedIDs = append(erasedIDs, derived[i].ID)
			}
		}

		// 3. Structural traces of every erased memory: entity↔memory links,
		//    knowledge-graph edges, spreading-activation associations.
		if _, _, _, err := st.Memory.PurgeSubjectGraphLinks(ctx, erasedIDs); err != nil {
			return err
		}

		// 4. The subject entity itself — its name/aliases/metadata/embedding are PII.
		if err := st.Memory.ScrubAnchorEntity(ctx, anchorID, tenantID); err != nil {
			return err
		}

		// 5. Record the subject-level erasure in the tamper-evident audit chain.
		tid := tenantID
		aid := actorID
		return st.MutationLog.Create(ctx, &domain.MutationLog{
			MemoryID:     anchorID, // the subject (entity) id
			MutationType: domain.MutationRedaction,
			SourceType:   domain.MutationSourceAdmin,
			Reason:       "subject erasure (GDPR): " + reason,
			TenantID:     &tid,
			AnchorID:     &anchorID,
			ActorType:    actorType,
			ActorID:      &aid,
		})
	})
	return count, err
}

// ErrReembedUnavailable is returned when re-embedding is requested without an
// embedding client configured.
var ErrReembedUnavailable = errors.New("no embedding client configured")

// ReembedAgent recomputes the vector for every memory of an agent using the
// currently configured embedding model. Use it after switching to a different
// embedding provider/model of the SAME dimension (a different dimension needs a
// fresh database). Returns the number of memories re-embedded.
func (s *AdminService) ReembedAgent(ctx context.Context, agentID, tenantID uuid.UUID, expectedDim int) (int, error) {
	if s.embeddingClient == nil {
		return 0, ErrReembedUnavailable
	}
	mems, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range mems {
		m := &mems[i]
		if m.TenantID != tenantID || m.Content == "" {
			continue
		}
		vec, err := s.embeddingClient.Embed(ctx, m.Content)
		if err != nil {
			return count, fmt.Errorf("re-embed memory %s: %w", m.ID, err)
		}
		if expectedDim > 0 && len(vec) != expectedDim {
			return count, fmt.Errorf("re-embed produced dimension %d, expected %d — the new model's width must match the schema; a different width needs a fresh database", len(vec), expectedDim)
		}
		if err := s.memoryStore.UpdateContent(ctx, m.ID, m.Content, vec); err != nil {
			return count, fmt.Errorf("update embedding for %s: %w", m.ID, err)
		}
		count++
	}
	s.logger.Info("re-embedded agent memories",
		zap.String("agent_id", agentID.String()), zap.Int("count", count))
	return count, nil
}

// ResolveContradiction manually settles a contradiction: the demoted belief is
// archived, the kept belief's review flag is cleared, and the action is audited.
func (s *AdminService) ResolveContradiction(ctx context.Context, tenantID, keepID, demoteID uuid.UUID, reason, actorType string, actorID uuid.UUID) error {
	if reason == "" {
		return ErrReasonRequired
	}
	demote, err := s.memoryStore.GetByID(ctx, demoteID, tenantID)
	if err != nil {
		return mapMemoryErr(err)
	}
	if _, err := s.memoryStore.GetByID(ctx, keepID, tenantID); err != nil {
		return mapMemoryErr(err)
	}

	mut := adminMutation(demote, domain.MutationAdminOverride, reason, actorType, actorID)
	return s.uow.Do(ctx, func(st *store.TxStores) error {
		if err := st.Memory.Archive(ctx, demoteID); err != nil {
			return err
		}
		if err := st.Memory.SetNeedsReview(ctx, keepID, false); err != nil {
			return err
		}
		return st.MutationLog.Create(ctx, mut)
	})
}
