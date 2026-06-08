package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type MemoryStore struct {
	db *pgxpool.Pool
}

func NewMemoryStore(db *pgxpool.Pool) *MemoryStore {
	return &MemoryStore{db: db}
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

	return s.db.QueryRow(ctx,
		`INSERT INTO memories (agent_id, tenant_id, type, content, embedding, embedding_provider, embedding_model, source, provenance, confidence, metadata, event_date, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, binding, anchor_id, session_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $14, NOW(), $12, $13, NOW(), 0, $15, $16, $17)
		 RETURNING id, created_at, updated_at, last_verified_at, last_accessed_at`,
		m.AgentID, m.TenantID, m.Type, m.Content, embedding, m.EmbeddingProvider, m.EmbeddingModel, m.Source, m.Provenance, m.Confidence, m.Metadata, m.ReinforcementCount, m.DecayRate, m.EventDate, m.Binding, m.AnchorID, m.SessionID,
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

func (s *MemoryStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE id = $1 AND tenant_id = $2`,
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
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE anchor_id = $1 AND tenant_id = $2`,
		anchorID, tenantID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET is_archived = TRUE, archived_at = NOW(), updated_at = NOW()
		 WHERE binding = 'session' AND is_archived = FALSE
		   AND session_id IN (
		     SELECT id FROM sessions WHERE expires_at IS NOT NULL AND expires_at < NOW()
		   )`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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


	anchorClause := "AND anchor_id IS NULL"
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
			 WHERE agent_id = $1 AND tenant_id = $2 AND embedding IS NOT NULL AND is_archived = FALSE %s
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

	anchorCondition := "AND anchor_id IS NULL"
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
		WHERE m.is_archived = FALSE
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
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE expires_at IS NOT NULL AND expires_at < NOW()`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *MemoryStore) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE agent_id = $1 AND type = $2 AND created_at < NOW() - ($3 || ' days')::interval`,
		agentID, memType, fmt.Sprintf("%d", retentionDays),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *MemoryStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	vec := pgvector.NewVector(embedding)

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, provenance, confidence, metadata, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at, binding, anchor_id, session_id,
		        embedding::text,
		        1 - (embedding <=> $1) AS score
		 FROM memories
		 WHERE agent_id = $2 AND tenant_id = $3 AND embedding IS NOT NULL AND is_archived = FALSE AND 1 - (embedding <=> $1) >= $4
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

func (s *MemoryStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding, embedding_provider, embedding_model, source, provenance, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE agent_id = $1 AND is_archived = FALSE`,
		agentID,
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

func (s *MemoryStore) Restore(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memories SET is_archived = FALSE, archived_at = NULL, updated_at = NOW() WHERE id = $1 AND is_archived = TRUE`,
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
