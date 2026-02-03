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

	// Default decay rate
	if m.DecayRate == 0 {
		m.DecayRate = 0.05
	}

	return s.db.QueryRow(ctx,
		`INSERT INTO memories (agent_id, tenant_id, type, content, embedding, embedding_provider, embedding_model, source, confidence, metadata, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11, $12, NOW(), 0)
		 RETURNING id, created_at, updated_at, last_verified_at, last_accessed_at`,
		m.AgentID, m.TenantID, m.Type, m.Content, embedding, m.EmbeddingProvider, m.EmbeddingModel, m.Source, m.Confidence, m.Metadata, m.ReinforcementCount, m.DecayRate,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt, &m.LastVerifiedAt, &m.LastAccessedAt)
}

func (s *MemoryStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	m := &domain.Memory{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt)
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

func (s *MemoryStore) Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	if opts.TopK <= 0 {
		opts.TopK = 10
	}

	vec := pgvector.NewVector(embedding)

	var conditions []string
	var args []any

	conditions = append(conditions, fmt.Sprintf("agent_id = $%d", len(args)+1))
	args = append(args, agentID)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", len(args)+1))
	args = append(args, tenantID)

	conditions = append(conditions, "embedding IS NOT NULL")

	if opts.MemoryType != nil {
		conditions = append(conditions, fmt.Sprintf("type = $%d", len(args)+1))
		args = append(args, string(*opts.MemoryType))
	}

	if opts.MinConfidence > 0 {
		conditions = append(conditions, fmt.Sprintf("confidence >= $%d", len(args)+1))
		args = append(args, opts.MinConfidence)
	}

	// Add the embedding parameter
	embeddingParam := len(args) + 1
	args = append(args, vec)

	// Add the limit parameter
	limitParam := len(args) + 1
	args = append(args, opts.TopK)

	query := fmt.Sprintf(
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, confidence, metadata, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at,
		        1 - (embedding <=> $%d) AS score
		 FROM memories
		 WHERE %s
		 ORDER BY confidence DESC, created_at DESC
		 LIMIT $%d`,
		embeddingParam,
		strings.Join(conditions, " AND "),
		limitParam,
	)

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
			&ms.Source, &ms.Confidence, &ms.Metadata, &ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate, &ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
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

func (s *MemoryStore) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE agent_id = $1 AND type = $2`,
		agentID, memType,
	).Scan(&count)
	return count, err
}

func (s *MemoryStore) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE agent_id = $1 AND type = $2
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
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
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
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, confidence, metadata, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at,
		        1 - (embedding <=> $1) AS score
		 FROM memories
		 WHERE agent_id = $2 AND tenant_id = $3 AND embedding IS NOT NULL AND 1 - (embedding <=> $1) >= $4
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
		err := rows.Scan(
			&ms.ID, &ms.AgentID, &ms.TenantID, &ms.Type, &ms.Content,
			&ms.EmbeddingProvider, &ms.EmbeddingModel,
			&ms.Source, &ms.Confidence, &ms.Metadata, &ms.LastVerifiedAt, &ms.ReinforcementCount, &ms.DecayRate, &ms.LastAccessedAt, &ms.AccessCount, &ms.CreatedAt, &ms.UpdatedAt,
			&ms.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan find similar row: %w", err)
		}
		results = append(results, ms)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find similar rows: %w", err)
	}

	return results, nil
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
	rows, err := s.db.Query(ctx, `SELECT DISTINCT agent_id FROM memories`)
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
		`SELECT id, agent_id, tenant_id, type, content, embedding_provider, embedding_model, source, confidence, metadata, expires_at, last_verified_at, reinforcement_count, decay_rate, last_accessed_at, access_count, created_at, updated_at
		 FROM memories WHERE agent_id = $1`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider, &m.EmbeddingModel, &m.Source, &m.Confidence, &m.Metadata, &m.ExpiresAt, &m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) Archive(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE id = $1`,
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
