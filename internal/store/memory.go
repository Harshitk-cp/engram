package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type MemoryStore struct {
	db   DBTX
	pool *pgxpool.Pool
}

func NewMemoryStore(db *pgxpool.Pool) *MemoryStore {
	return &MemoryStore{db: db, pool: db}
}

// withTx returns a clone of the store that runs against the given transaction.
func (s *MemoryStore) withTx(tx pgx.Tx) *MemoryStore {
	return &MemoryStore{db: tx, pool: s.pool}
}

func (s *MemoryStore) Create(ctx context.Context, m *domain.Memory) error {
	var embedding *pgvector.Vector
	if len(m.Embedding) > 0 {
		v := pgvector.NewVector(m.Embedding)
		embedding = &v
	}

	// Default embedding provider/model if not set
	if m.EmbeddingProvider == "" {
		m.EmbeddingProvider = "openai"
	}
	if m.EmbeddingModel == "" {
		m.EmbeddingModel = "text-embedding-3-small"
	}

	// Default reinforcement count
	if m.ReinforcementCount == 0 {
		m.ReinforcementCount = 1
	}

	// Decay rate is defaulted per-binding below (after binding is computed).

	// Default provenance
	if m.Provenance == "" {
		m.Provenance = domain.ProvenanceAgent
	}

	// Set initial confidence based on provenance if not explicitly set
	if m.Confidence == 0 {
		m.Confidence = m.Provenance.InitialConfidence()
	}

	// Binding is computed server-side from the ids present — never trusted from
	// the client — and pinned to them by a DB CHECK constraint. (Canon is set
	// explicitly by the caller.)
	if m.Binding == "" {
		m.Binding = domain.ComputeMemoryBinding(m.AnchorID, m.SessionID)
	}

	if m.DecayRate == 0 {
		m.DecayRate = domain.DefaultDecayRate(m.Binding)
	}

	var quarantineReason *string
	if m.QuarantineReason != "" {
		quarantineReason = &m.QuarantineReason
	}
	return s.db.QueryRow(ctx,
		`INSERT INTO memories (agent_id, tenant_id, type, content, embedding, embedding_provider, embedding_model, source, provenance, confidence, metadata, event_date, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, binding, anchor_id, session_id, quarantine_reason, quarantined_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $14, NOW(), $12, $13, NOW(), 0, $15, $16, $17, $18, $19)
		 RETURNING id, created_at, updated_at, last_verified_at, last_accessed_at`,
		m.AgentID, m.TenantID, m.Type, m.Content, embedding, m.EmbeddingProvider, m.EmbeddingModel, m.Source, m.Provenance, m.Confidence, m.Metadata, m.ReinforcementCount, m.DecayRate, m.EventDate, m.Binding, m.AnchorID, m.SessionID, quarantineReason, m.QuarantinedAt,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt, &m.LastVerifiedAt, &m.LastAccessedAt)
}

func (s *MemoryStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	m := &domain.Memory{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id
		 FROM memories WHERE id = $1 AND tenant_id = $2 AND is_archived = FALSE`,
		id, tenantID,
	).Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt, &m.Binding, &m.AnchorID, &m.SessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (s *MemoryStore) GetByIDOnly(ctx context.Context, id uuid.UUID) (*domain.Memory, error) {
	m := &domain.Memory{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE id = $1 AND is_archived = FALSE LIMIT 1`,
		id,
	).Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

// snapshotMemoriesForRemoval writes one mutation_log row per memory matching
// whereSQL inside tx, BEFORE the caller removes/archives those rows, so the audit
// of a removal survives it. mutationType is 'deletion' or 'archive'; retainContent
// stores the original text (pass false for GDPR erasure, where only the hash is kept).
func snapshotMemoriesForRemoval(ctx context.Context, tx pgx.Tx, mutationType domain.MutationType, reason string, retainContent bool, whereSQL string, args ...any) error {
	rows, err := tx.Query(ctx,
		`SELECT id, agent_id, tenant_id, anchor_id, binding::text, content FROM memories WHERE `+whereSQL,
		args...,
	)
	if err != nil {
		return err
	}
	type snap struct {
		id, agentID, tenantID uuid.UUID
		anchorID              *uuid.UUID
		binding, content      string
	}
	var snaps []snap
	for rows.Next() {
		var sp snap
		if err := rows.Scan(&sp.id, &sp.agentID, &sp.tenantID, &sp.anchorID, &sp.binding, &sp.content); err != nil {
			rows.Close()
			return err
		}
		snaps = append(snaps, sp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range snaps {
		sp := &snaps[i]
		tid := sp.tenantID
		ml := &domain.MutationLog{
			MemoryID:     sp.id,
			AgentID:      sp.agentID,
			MutationType: mutationType,
			SourceType:   domain.MutationSourceSystem,
			Reason:       reason,
			TenantID:     &tid,
			AnchorID:     sp.anchorID,
			Binding:      sp.binding,
			ContentHash:  domain.HashContent(sp.content),
		}
		if retainContent {
			c := sp.content
			ml.ContentSnapshot = &c
		}
		if err := insertMutationLog(ctx, tx, ml); err != nil {
			return err
		}
	}
	return nil
}

func (s *MemoryStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := snapshotMemoriesForRemoval(ctx, tx, domain.MutationDeletion, "deletion: api delete", true,
			"id = $1 AND tenant_id = $2", id, tenantID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM memories WHERE id = $1 AND tenant_id = $2`, id, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *MemoryStore) ListByAnchor(ctx context.Context, anchorID, tenantID uuid.UUID, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id
		 FROM memories
		 WHERE anchor_id = $1 AND tenant_id = $2 AND is_archived = FALSE
		 ORDER BY confidence DESC, created_at DESC
		 LIMIT $3`,
		anchorID, tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt, &m.Binding, &m.AnchorID, &m.SessionID); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) PurgeByAnchor(ctx context.Context, anchorID, tenantID uuid.UUID) (int64, error) {
	var affected int64
	err := WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txs := s.withTx(tx)

		// Identify the subject's memories and everything transitively derived from
		// them BEFORE deleting, so the erasure also covers inferred beliefs.
		anchorMems, err := txs.ListByAnchor(ctx, anchorID, tenantID, 100000)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(anchorMems))
		for i := range anchorMems {
			ids = append(ids, anchorMems[i].ID)
		}
		var derivedIDs []uuid.UUID
		if len(ids) > 0 {
			derived, err := txs.DerivedMemoryClosure(ctx, ids, tenantID)
			if err != nil {
				return err
			}
			for i := range derived {
				derivedIDs = append(derivedIDs, derived[i].ID)
			}
		}

		// GDPR erasure: keep the hash as proof of what was erased, but do NOT
		// retain the original content (that would defeat the erasure). Deleting the
		// memory rows cascades entity_mentions and memory_graph (FK ON DELETE CASCADE).
		if err := snapshotMemoriesForRemoval(ctx, tx, domain.MutationDeletion, "deletion: anchor purge (erasure)", false,
			"anchor_id = $1 AND tenant_id = $2", anchorID, tenantID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM memories WHERE anchor_id = $1 AND tenant_id = $2`, anchorID, tenantID)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()

		// Inferred beliefs derived from the subject's data.
		if len(derivedIDs) > 0 {
			if err := snapshotMemoriesForRemoval(ctx, tx, domain.MutationDeletion, "deletion: derived from purged subject (erasure)", false,
				"id = ANY($1) AND tenant_id = $2", derivedIDs, tenantID); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `DELETE FROM memories WHERE id = ANY($1) AND tenant_id = $2`, derivedIDs, tenantID)
			if err != nil {
				return err
			}
			affected += tag.RowsAffected()
		}

		// memory_associations has no FK to memories, so its rows don't cascade —
		// remove any referencing an erased memory to avoid dangling links.
		allIDs := append(ids, derivedIDs...)
		if len(allIDs) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM memory_associations WHERE source_memory_id = ANY($1) OR target_memory_id = ANY($1)`, allIDs); err != nil {
				return err
			}
		}
		return nil
	})
	return affected, err
}

// DerivedMemoryClosure returns every memory transitively derived from the given
// root memories (following memory_graph 'derived_from' edges, where source_id was
// derived from target_id), tenant-scoped and excluding the roots and rows whose
// content is already crypto-shredded. GDPR erasure uses this so beliefs inferred
// from a subject's data are erased along with the source.
func (s *MemoryStore) DerivedMemoryClosure(ctx context.Context, rootIDs []uuid.UUID, tenantID uuid.UUID) ([]domain.Memory, error) {
	if len(rootIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE derived AS (
			SELECT mg.source_id AS id
			FROM memory_graph mg
			WHERE mg.relation_type = 'derived_from' AND mg.target_id = ANY($1)
			UNION
			SELECT mg.source_id
			FROM memory_graph mg
			JOIN derived d ON mg.target_id = d.id
			WHERE mg.relation_type = 'derived_from'
		)
		SELECT m.id, m.agent_id, m.tenant_id, m.content, m.anchor_id, m.binding
		FROM memories m
		JOIN derived d ON m.id = d.id
		WHERE m.tenant_id = $2
		  AND NOT (m.id = ANY($1))
		  AND m.is_archived = FALSE
		  AND m.content NOT LIKE 'enc:v1:%'`, // cryptoShredPrefix (service/crypto.go) — skip already-shredded
		rootIDs, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Content, &m.AnchorID, &m.Binding); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PurgeSubjectGraphLinks deletes the structural traces of the given (erased)
// memories: entity↔memory links, knowledge-graph edges, and spreading-activation
// associations. These tables cascade on memory DELETE, but crypto-shred redacts
// in place (the row survives), so GDPR erasure must remove them explicitly.
func (s *MemoryStore) PurgeSubjectGraphLinks(ctx context.Context, memoryIDs []uuid.UUID) (mentions, edges, assocs int64, err error) {
	if len(memoryIDs) == 0 {
		return 0, 0, 0, nil
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM entity_mentions WHERE memory_id = ANY($1)`, memoryIDs)
	if err != nil {
		return 0, 0, 0, err
	}
	mentions = tag.RowsAffected()

	tag, err = s.db.Exec(ctx, `DELETE FROM memory_graph WHERE source_id = ANY($1) OR target_id = ANY($1)`, memoryIDs)
	if err != nil {
		return mentions, 0, 0, err
	}
	edges = tag.RowsAffected()

	tag, err = s.db.Exec(ctx, `DELETE FROM memory_associations WHERE source_memory_id = ANY($1) OR target_memory_id = ANY($1)`, memoryIDs)
	if err != nil {
		return mentions, edges, 0, err
	}
	assocs = tag.RowsAffected()
	return mentions, edges, assocs, nil
}

// ScrubAnchorEntity erases the subject entity's identity in place: name, aliases,
// metadata, external id and embedding are cleared (the row remains for audit/FK
// integrity, but no PII survives). Name is set to a unique tombstone to satisfy
// the (agent_id, name, entity_type) uniqueness constraint.
func (s *MemoryStore) ScrubAnchorEntity(ctx context.Context, anchorID, tenantID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE entities
		SET name = 'erased-' || id::text,
		    aliases = '{}',
		    metadata = '{}'::jsonb,
		    external_id = NULL,
		    embedding = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2`,
		anchorID, tenantID,
	)
	return err
}

func (s *MemoryStore) ListCanon(ctx context.Context, tenantID uuid.UUID, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id
		 FROM memories
		 WHERE tenant_id = $1 AND binding = 'canon' AND is_archived = FALSE
		 ORDER BY confidence DESC, created_at DESC
		 LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt, &m.Binding, &m.AnchorID, &m.SessionID); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) FindCanonByContent(ctx context.Context, tenantID uuid.UUID, content string) (*domain.Memory, error) {
	var m domain.Memory
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id
		 FROM memories
		 WHERE tenant_id = $1 AND binding = 'canon' AND is_archived = FALSE AND content = $2
		 ORDER BY created_at ASC
		 LIMIT 1`,
		tenantID, content,
	).Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt, &m.Binding, &m.AnchorID, &m.SessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (s *MemoryStore) ArchiveExpiredSessionMemories(ctx context.Context) (int64, error) {
	var affected int64
	err := WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		const where = `binding = 'session' AND is_archived = FALSE
		   AND session_id IN (
		     SELECT id FROM sessions WHERE expires_at IS NOT NULL AND expires_at < NOW()
		   )`
		// Soft archive: content is preserved in the row, so no content snapshot.
		if err := snapshotMemoriesForRemoval(ctx, tx, domain.MutationArchive, "archive: session expired", false, where); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE memories SET is_archived = TRUE, archived_at = NOW(), updated_at = NOW() WHERE `+where)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

func (s *MemoryStore) PromoteSessionToAnchor(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET binding = 'anchored', session_id = NULL,
		        decay_rate = $2, updated_at = NOW()
		 WHERE id = $1 AND binding = 'session' AND anchor_id IS NOT NULL`,
		id, domain.DefaultDecayRate(domain.BindingAnchored),
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *MemoryStore) Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	if opts.TopK <= 0 {
		opts.TopK = 10
	}

	vec := pgvector.NewVector(embedding)

	var conditions []string
	var args []any

	// agent_id is optional: when recalling by anchor across all of a tenant's
	if agentID != uuid.Nil {
		conditions = append(conditions, fmt.Sprintf("agent_id = $%d", len(args)+1))
		args = append(args, agentID)
	}

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", len(args)+1))
	args = append(args, tenantID)

	if opts.Binding != nil {
		conditions = append(conditions, fmt.Sprintf("binding = $%d::memory_binding", len(args)+1))
		args = append(args, string(*opts.Binding))
		switch *opts.Binding {
		case domain.BindingAnchored:
			if opts.AnchorID != nil {
				conditions = append(conditions, fmt.Sprintf("anchor_id = $%d", len(args)+1))
				args = append(args, *opts.AnchorID)
			}
		case domain.BindingSession:
			if opts.SessionID != nil {
				conditions = append(conditions, fmt.Sprintf("session_id = $%d", len(args)+1))
				args = append(args, *opts.SessionID)
			}
		}
	} else if opts.AnchorID != nil {
		conditions = append(conditions, fmt.Sprintf("anchor_id = $%d", len(args)+1))
		args = append(args, *opts.AnchorID)
	} else {
		conditions = append(conditions, "anchor_id IS NULL")
		conditions = append(conditions, "session_id IS NULL")
	}

	conditions = append(conditions, "embedding IS NOT NULL")
	conditions = append(conditions, "is_archived = FALSE")
	// Provenance Firewall: quarantined (untrusted) traces never surface in recall.
	conditions = append(conditions, "binding <> 'quarantine'")

	if opts.MemoryType != nil {
		conditions = append(conditions, fmt.Sprintf("type = $%d", len(args)+1))
		args = append(args, string(*opts.MemoryType))
	}

	if opts.MinConfidence > 0 {
		conditions = append(conditions, fmt.Sprintf("confidence >= $%d", len(args)+1))
		args = append(args, opts.MinConfidence)
	}

	if opts.EventDateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("COALESCE(event_date, created_at) >= $%d", len(args)+1))
		args = append(args, *opts.EventDateFrom)
	}
	if opts.EventDateTo != nil {
		conditions = append(conditions, fmt.Sprintf("COALESCE(event_date, created_at) <= $%d", len(args)+1))
		args = append(args, *opts.EventDateTo)
	}

	// Add the embedding parameter
	embeddingParam := len(args) + 1
	args = append(args, vec)

	// Add the limit parameter
	limitParam := len(args) + 1
	args = append(args, opts.TopK)

	var query string
	if opts.RecencyBoost > 0 {
		recencyParam := len(args) + 1
		args = append(args, opts.RecencyBoost)
		query = fmt.Sprintf(
			`WITH ranked AS (
			   SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model,
			          source, provenance, confidence, metadata, event_date, last_verified_at, reinforcement_count,
			          decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id,
			          (embedding <=> $%d) AS vec_dist,
			          COALESCE(
			            EXTRACT(EPOCH FROM (COALESCE(event_date, created_at)
			              - MIN(COALESCE(event_date, created_at)) OVER (PARTITION BY agent_id))) /
			            NULLIF(EXTRACT(EPOCH FROM (
			              MAX(COALESCE(event_date, created_at)) OVER (PARTITION BY agent_id)
			            - MIN(COALESCE(event_date, created_at)) OVER (PARTITION BY agent_id)
			            )), 0),
			            0.5
			          ) AS relative_recency
			   FROM memories
			   WHERE %s
			 )
			 SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model,
			        source, provenance, confidence, metadata, event_date, last_verified_at, reinforcement_count,
			        decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id,
			        (1 - vec_dist) + $%d * relative_recency AS score
			 FROM ranked
			 ORDER BY vec_dist - $%d * relative_recency ASC
			 LIMIT $%d`,
			embeddingParam,
			strings.Join(conditions, " AND "),
			recencyParam, recencyParam,
			limitParam,
		)
	} else {
		query = fmt.Sprintf(
			`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, event_date, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id,
			        1 - (embedding <=> $%d) AS score
			 FROM memories
			 WHERE %s
			 ORDER BY embedding <=> $%d ASC
			 LIMIT $%d`,
			embeddingParam,
			strings.Join(conditions, " AND "),
			embeddingParam,
			limitParam,
		)
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("recall query: %w", err)
	}
	defer rows.Close()

	var results []domain.MemoryWithScore
	for rows.Next() {
		var ms domain.MemoryWithScore
		err := rows.Scan(
			&ms.ID, &ms.AgentID, &ms.TenantID, &ms.Type, &ms.Content,
			&ms.EmbeddingProvider, &ms.EmbeddingModel,
			&ms.Source, &ms.Provenance, &ms.Confidence, &ms.Metadata, &ms.EventDate,
			&ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate, &ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
			&ms.Binding, &ms.AnchorID, &ms.SessionID,
			&ms.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recall row: %w", err)
		}
		results = append(results, ms)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recall rows: %w", err)
	}

	return results, nil
}

func (s *MemoryStore) RecallExhaustive(ctx context.Context, queryEmbedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	minSim := opts.MinSimilarity
	if minSim <= 0 {
		minSim = 0.25
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 500
	}

	var allResults []domain.MemoryWithScore
	pageSize := 1000
	offset := 0

	// No-anchor recall must exclude session-bound rows too: anonymous-session
	// memories have anchor_id NULL but belong to one conversation only
	// (mirrors the default branch in Recall).
	anchorClause := "AND anchor_id IS NULL AND session_id IS NULL"
	var anchorArg []any
	if opts.AnchorID != nil {
		anchorClause = "AND anchor_id = $5"
		anchorArg = []any{*opts.AnchorID}
	}

	for {
		queryArgs := append([]any{agentID, tenantID, pageSize, offset}, anchorArg...)
		rows, err := s.db.Query(ctx,
			fmt.Sprintf(`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model,
			        source, provenance, confidence, metadata, event_date, last_verified_at,
			        reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at,
			        embedding
			 FROM memories
			 WHERE agent_id = $1 AND tenant_id = $2 AND embedding IS NOT NULL AND is_archived = FALSE AND binding <> 'quarantine' %s
			 ORDER BY created_at
			 LIMIT $3 OFFSET $4`, anchorClause),
			queryArgs...,
		)
		if err != nil {
			return nil, fmt.Errorf("exhaustive recall page: %w", err)
		}

		var pageResults []domain.MemoryWithScore
		for rows.Next() {
			var ms domain.MemoryWithScore
			var embVec pgvector.Vector
			err := rows.Scan(
				&ms.ID, &ms.AgentID, &ms.TenantID, &ms.Type, &ms.Content,
				&ms.EmbeddingProvider, &ms.EmbeddingModel,
				&ms.Source, &ms.Provenance, &ms.Confidence, &ms.Metadata, &ms.EventDate,
				&ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate,
				&ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
				&embVec,
			)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("exhaustive scan: %w", err)
			}
			sim := cosineSimilarity(queryEmbedding, embVec.Slice())
			if sim >= minSim {
				ms.Score = sim
				pageResults = append(pageResults, ms)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("exhaustive rows: %w", err)
		}

		allResults = append(allResults, pageResults...)
		if len(pageResults) < pageSize {
			break
		}
		offset += pageSize
		if len(allResults) >= maxResults {
			break
		}
	}

	sortByScore(allResults)
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}
	return allResults, nil
}

func (s *MemoryStore) RecallHybrid(ctx context.Context, query string, queryEmbedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}

	vec := pgvector.NewVector(queryEmbedding)

	var typeCondition string
	var args []any
	args = append(args, agentID, tenantID, query, vec, topK)

	if opts.MemoryType != nil {
		args = append(args, string(*opts.MemoryType))
		typeCondition = fmt.Sprintf("AND type = $%d", len(args))
	}

	// No-anchor recall must exclude session-bound rows too: anonymous-session
	// memories have anchor_id NULL but belong to one conversation only
	anchorCondition := "AND anchor_id IS NULL AND session_id IS NULL"
	if opts.AnchorID != nil {
		args = append(args, *opts.AnchorID)
		anchorCondition = fmt.Sprintf("AND anchor_id = $%d", len(args))
	}

	hybridQuery := fmt.Sprintf(`
		WITH bm25_ranked AS (
		  SELECT id,
		         ts_rank(content_tsv, plainto_tsquery('english', $3)) AS bm25_score,
		         ROW_NUMBER() OVER (ORDER BY ts_rank(content_tsv, plainto_tsquery('english', $3)) DESC) AS bm25_rank
		  FROM memories
		  WHERE agent_id = $1 AND tenant_id = $2 AND is_archived = FALSE
		    AND content_tsv @@ plainto_tsquery('english', $3) %s %s
		),
		vec_ranked AS (
		  SELECT id,
		         1 - (embedding <=> $4) AS vec_score,
		         ROW_NUMBER() OVER (ORDER BY embedding <=> $4 ASC) AS vec_rank
		  FROM memories
		  WHERE agent_id = $1 AND tenant_id = $2 AND embedding IS NOT NULL AND is_archived = FALSE %s %s
		  LIMIT 100
		),
		rrf AS (
		  SELECT
		    COALESCE(b.id, v.id) AS id,
		    (1.0 / (60 + COALESCE(b.bm25_rank, 1000))) + (1.0 / (60 + COALESCE(v.vec_rank, 1000))) AS rrf_score
		  FROM bm25_ranked b
		  FULL OUTER JOIN vec_ranked v ON b.id = v.id
		)
		SELECT m.id, m.agent_id, m.tenant_id, m.type, m.content, m.embedding_provider, m.embedding_model,
		       m.source, m.provenance, m.confidence, m.metadata, m.event_date, m.last_verified_at,
		       m.reinforcement_count, m.decay_rate, m.last_accessed_at, m.access_count,
		       m.created_at, m.updated_at, r.rrf_score AS score
		FROM rrf r JOIN memories m ON m.id = r.id
		WHERE m.is_archived = FALSE AND m.binding <> 'quarantine'
		ORDER BY r.rrf_score DESC
		LIMIT $5
	`, typeCondition, anchorCondition, typeCondition, anchorCondition)

	rows, err := s.db.Query(ctx, hybridQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("hybrid recall: %w", err)
	}
	defer rows.Close()

	var results []domain.MemoryWithScore
	for rows.Next() {
		var ms domain.MemoryWithScore
		err := rows.Scan(
			&ms.ID, &ms.AgentID, &ms.TenantID, &ms.Type, &ms.Content,
			&ms.EmbeddingProvider, &ms.EmbeddingModel,
			&ms.Source, &ms.Provenance, &ms.Confidence, &ms.Metadata, &ms.EventDate,
			&ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate,
			&ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
			&ms.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("hybrid scan: %w", err)
		}
		results = append(results, ms)
	}
	return results, rows.Err()
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

func sortByScore(ms []domain.MemoryWithScore) {
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && ms[j].Score > ms[j-1].Score; j-- {
			ms[j], ms[j-1] = ms[j-1], ms[j]
		}
	}
}

func (s *MemoryStore) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE agent_id = $1 AND type = $2 AND is_archived = FALSE`,
		agentID, memType,
	).Scan(&count)
	return count, err
}

func (s *MemoryStore) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE agent_id = $1 AND type = $2 AND is_archived = FALSE
		 ORDER BY created_at ASC
		 LIMIT $3`,
		agentID, memType, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) DeleteExpired(ctx context.Context) (int64, error) {
	var affected int64
	err := WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		const where = "expires_at IS NOT NULL AND expires_at < NOW()"
		if err := snapshotMemoriesForRemoval(ctx, tx, domain.MutationDeletion, "deletion: ttl expired", true, where); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM memories WHERE `+where)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

func (s *MemoryStore) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	var affected int64
	err := WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		const where = "agent_id = $1 AND type = $2 AND created_at < NOW() - ($3 || ' days')::interval"
		days := fmt.Sprintf("%d", retentionDays)
		if err := snapshotMemoriesForRemoval(ctx, tx, domain.MutationDeletion, "deletion: retention policy", true, where, agentID, memType, days); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM memories WHERE `+where, agentID, memType, days)
		if err != nil {
			return err
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

func (s *MemoryStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	vec := pgvector.NewVector(embedding)

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id,
		        embedding::text,
		        1 - (embedding <=> $1) AS score
		 FROM memories
		 WHERE agent_id = $2 AND tenant_id = $3 AND embedding IS NOT NULL AND is_archived = FALSE AND binding <> 'quarantine' AND 1 - (embedding <=> $1) >= $4
		 ORDER BY score DESC`,
		vec, agentID, tenantID, threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("find similar query: %w", err)
	}
	defer rows.Close()

	var results []domain.MemoryWithScore
	for rows.Next() {
		var ms domain.MemoryWithScore
		var embVec pgvector.Vector
		err := rows.Scan(
			&ms.ID, &ms.AgentID, &ms.TenantID, &ms.Type, &ms.Content,
			&ms.EmbeddingProvider, &ms.EmbeddingModel,
			&ms.Source, &ms.Provenance, &ms.Confidence, &ms.Metadata, &ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate, &ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
			&ms.Binding, &ms.AnchorID, &ms.SessionID,
			&embVec,
			&ms.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan find similar row: %w", err)
		}
		ms.Embedding = embVec.Slice()
		results = append(results, ms)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find similar rows: %w", err)
	}

	return results, nil
}

func (s *MemoryStore) GetRecentByType(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.MemoryWithScore, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id,
		        embedding::text,
		        1.0::float4 AS score
		 FROM memories
		 WHERE agent_id = $1 AND tenant_id = $2 AND type = $3 AND is_archived = FALSE
		 ORDER BY created_at DESC
		 LIMIT $4`,
		agentID, tenantID, string(memType), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent by type: %w", err)
	}
	defer rows.Close()

	var results []domain.MemoryWithScore
	for rows.Next() {
		var ms domain.MemoryWithScore
		var embVec pgvector.Vector
		if err := rows.Scan(
			&ms.ID, &ms.AgentID, &ms.TenantID, &ms.Type, &ms.Content,
			&ms.EmbeddingProvider, &ms.EmbeddingModel,
			&ms.Source, &ms.Provenance, &ms.Confidence, &ms.Metadata, &ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate, &ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
			&ms.Binding, &ms.AnchorID, &ms.SessionID,
			&embVec,
			&ms.Score,
		); err != nil {
			return nil, fmt.Errorf("scan get recent by type: %w", err)
		}
		ms.Embedding = embVec.Slice()
		results = append(results, ms)
	}
	return results, rows.Err()
}

// UpdateReinforcement atomically updates confidence, reinforcement_count, and last_verified_at.
func (s *MemoryStore) UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET confidence = $1, reinforcement_count = $2, last_verified_at = NOW(), updated_at = NOW() WHERE id = $3`,
		confidence, reinforcementCount, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET confidence = $1, updated_at = NOW() WHERE id = $2`,
		confidence, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ApplyConfidenceDelta atomically adjusts confidence by delta, clamped to
// [0, 0.99]. Applying decay as a relative delta (rather than an absolute SET
// from a stale snapshot) lets it compose with concurrent recall boosts
// (IncrementAccessAndBoost) without either write clobbering the other.
func (s *MemoryStore) ApplyConfidenceDelta(ctx context.Context, id uuid.UUID, delta float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories
		 SET confidence = GREATEST(0, LEAST(confidence + $2, 0.99)),
		     updated_at = NOW()
		 WHERE id = $1`,
		id, delta,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateContent replaces a memory's content (admin correction). When a new
// embedding is provided it is updated too; otherwise the existing embedding is
// left in place.
func (s *MemoryStore) UpdateContent(ctx context.Context, id uuid.UUID, content string, embedding []float32) error {
	query := `UPDATE memories SET content = $1, updated_at = NOW() WHERE id = $2`
	args := []any{content, id}
	if len(embedding) > 0 {
		v := pgvector.NewVector(embedding)
		query = `UPDATE memories SET content = $1, embedding = $2, updated_at = NOW() WHERE id = $3`
		args = []any{content, v, id}
	}
	tag, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RedactContent overwrites content with a tombstone and clears the embedding, so
// neither the original text nor its vector remains recoverable (GDPR redaction).
func (s *MemoryStore) RedactContent(ctx context.Context, id uuid.UUID, tombstone string) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET content = $1, embedding = NULL, updated_at = NOW() WHERE id = $2`,
		tombstone, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT agent_id FROM memories WHERE is_archived = FALSE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agentIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		agentIDs = append(agentIDs, id)
	}
	return agentIDs, rows.Err()
}

// decayBatchLimit bounds how many rows a single background pass materializes
// (rows include full embedding vectors); without it one very large agent can
// OOM the whole process. Agents above the cap are processed partially per tick.
const decayBatchLimit = 10000

func (s *MemoryStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE agent_id = $1 AND is_archived = FALSE AND binding <> 'quarantine'
		 ORDER BY last_accessed_at ASC NULLS FIRST
		 LIMIT $2`,
		agentID, decayBatchLimit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		var emb pgvector.Vector
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &emb, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Embedding = emb.Slice()
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

// ListQuarantined returns the firewall review queue for an agent, newest first,
// with the quarantine reason/time populated.
func (s *MemoryStore) ListQuarantined(ctx context.Context, agentID, tenantID uuid.UUID, limit, offset int) ([]domain.Memory, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE agent_id = $1 AND tenant_id = $2 AND binding = 'quarantine' AND is_archived = FALSE`,
		agentID, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, provenance, confidence, source, metadata,
		        anchor_id, session_id, quarantine_reason, quarantined_at, created_at
		 FROM memories
		 WHERE agent_id = $1 AND tenant_id = $2 AND binding = 'quarantine' AND is_archived = FALSE
		 ORDER BY created_at DESC
		 LIMIT $3 OFFSET $4`,
		agentID, tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []domain.Memory
	for rows.Next() {
		var m domain.Memory
		var reason *string
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.Provenance, &m.Confidence, &m.Source, &m.Metadata,
			&m.AnchorID, &m.SessionID, &reason, &m.QuarantinedAt, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		if reason != nil {
			m.QuarantineReason = *reason
		}
		m.Binding = domain.BindingQuarantine
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// ReleaseQuarantine promotes a quarantined trace to its computed real binding and
// clears the quarantine metadata.
func (s *MemoryStore) ReleaseQuarantine(ctx context.Context, id, tenantID uuid.UUID, newBinding domain.MemoryBinding) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories
		 SET binding = $3::memory_binding, quarantine_reason = NULL, quarantined_at = NULL, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND binding = 'quarantine'`,
		id, tenantID, string(newBinding),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) Archive(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET is_archived = TRUE, archived_at = NOW(), updated_at = NOW() WHERE id = $1 AND is_archived = FALSE`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) Restore(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET is_archived = FALSE, archived_at = NULL, updated_at = NOW() WHERE id = $1 AND tenant_id = $2 AND is_archived = TRUE`,
		id, tenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories
		 SET access_count = access_count + 1,
		     last_accessed_at = NOW(),
		     confidence = LEAST(confidence + $2, 0.99),
		     updated_at = NOW()
		 WHERE id = $1`,
		id, boost,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) GetByTier(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, tier domain.MemoryTier, limit int) ([]domain.Memory, error) {
	thresholds := domain.TierConfidenceThresholds[tier]
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories
		 WHERE agent_id = $1 AND tenant_id = $2 AND confidence > $3 AND confidence <= $4 AND is_archived = FALSE
		 ORDER BY confidence DESC
		 LIMIT $5`,
		agentID, tenantID, thresholds.Min, thresholds.Max, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) GetTierCounts(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (map[domain.MemoryTier]int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT
			CASE
				WHEN confidence > 0.85 THEN 'hot'
				WHEN confidence > 0.70 THEN 'warm'
				WHEN confidence > 0.40 THEN 'cold'
				ELSE 'archive'
			END as tier,
			COUNT(*) as count
		 FROM memories
		 WHERE agent_id = $1 AND tenant_id = $2 AND is_archived = FALSE
		 GROUP BY tier`,
		agentID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[domain.MemoryTier]int)
	for rows.Next() {
		var tier string
		var count int
		if err := rows.Scan(&tier, &count); err != nil {
			return nil, err
		}
		counts[domain.MemoryTier(tier)] = count
	}
	return counts, rows.Err()
}

func (s *MemoryStore) SetNeedsReview(ctx context.Context, id uuid.UUID, needsReview bool) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET needs_review = $1, updated_at = NOW() WHERE id = $2`,
		needsReview, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStore) GetNeedsReview(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories
		 WHERE agent_id = $1 AND tenant_id = $2 AND needs_review = true AND is_archived = FALSE
		 ORDER BY updated_at DESC
		 LIMIT $3`,
		agentID, tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

// ListByAgentFiltered lists an agent's memories with optional tier (confidence
// band), type, and provenance (source) filters, newest-confidence first, plus the
// total match count for pagination. tier ∈ {hot,warm,cold,archive,""}; memType is a
// memory_type or ""; provenance ∈ {user,agent,tool,derived,inferred,""}.
func (s *MemoryStore) ListByAgentFiltered(ctx context.Context, agentID, tenantID uuid.UUID, f domain.MemoryFilter, limit, offset int) ([]domain.Memory, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := "agent_id = $1 AND tenant_id = $2"
	args := []any{agentID, tenantID}
	switch f.Tier {
	case "hot":
		where += " AND confidence > 0.85 AND is_archived = FALSE"
	case "warm":
		where += " AND confidence > 0.70 AND confidence <= 0.85 AND is_archived = FALSE"
	case "cold":
		where += " AND confidence > 0.40 AND confidence <= 0.70 AND is_archived = FALSE"
	case "archive":
		where += " AND (confidence <= 0.40 OR is_archived = TRUE)"
	default:
		where += " AND is_archived = FALSE"
	}
	if f.Type != "" {
		args = append(args, f.Type)
		where += fmt.Sprintf(" AND type = $%d", len(args))
	}
	if f.Provenance != "" {
		args = append(args, f.Provenance)
		where += fmt.Sprintf(" AND provenance = $%d", len(args))
	}
	if f.Binding != "" {
		args = append(args, f.Binding)
		where += fmt.Sprintf(" AND binding = $%d::memory_binding", len(args))
	}

	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM memories WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id
		 FROM memories WHERE `+where+
			fmt.Sprintf(" ORDER BY confidence DESC, created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt, &m.Binding, &m.AnchorID, &m.SessionID); err != nil {
			return nil, 0, err
		}
		memories = append(memories, m)
	}
	return memories, total, rows.Err()
}

// BeliefsAsOf reconstructs what the agent believed at instant `at`: for every
// memory that existed then (created_at <= at), confidence is folded from the
// audit log — the most recent mutation's new_confidence at/before `at`, else the
// earliest mutation's old_confidence (the value at creation), else the current
// confidence (never changed). Returns the top beliefs by reconstructed confidence
// plus the total count that existed at `at`.
func (s *MemoryStore) BeliefsAsOf(ctx context.Context, agentID, tenantID uuid.UUID, at time.Time, limit int) ([]domain.BeliefAtTime, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var total int
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE agent_id = $1 AND tenant_id = $2 AND created_at <= $3`,
		agentID, tenantID, at,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT m.id, m.content, m.type,
		    COALESCE(
		      (SELECT ml.new_confidence FROM mutation_log ml
		         WHERE ml.memory_id = m.id AND ml.created_at <= $3 AND ml.new_confidence IS NOT NULL
		         ORDER BY ml.created_at DESC, ml.seq DESC LIMIT 1),
		      (SELECT ml.old_confidence FROM mutation_log ml
		         WHERE ml.memory_id = m.id AND ml.old_confidence IS NOT NULL
		         ORDER BY ml.created_at ASC, ml.seq ASC LIMIT 1),
		      m.confidence
		    )::real AS conf_at_t,
		    m.created_at
		 FROM memories m
		 WHERE m.agent_id = $1 AND m.tenant_id = $2 AND m.created_at <= $3
		 ORDER BY conf_at_t DESC, m.created_at DESC
		 LIMIT $4`,
		agentID, tenantID, at, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.BeliefAtTime
	for rows.Next() {
		var b domain.BeliefAtTime
		if err := rows.Scan(&b.ID, &b.Content, &b.Type, &b.Confidence, &b.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// CountNeedsReview returns the number of active memories flagged for review.
func (s *MemoryStore) CountNeedsReview(ctx context.Context, agentID, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories
		 WHERE agent_id = $1 AND tenant_id = $2 AND needs_review = true AND is_archived = FALSE`,
		agentID, tenantID,
	).Scan(&n)
	return n, err
}
