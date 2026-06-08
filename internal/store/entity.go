package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EntityStore struct {
	db *pgxpool.Pool
}

func NewEntityStore(db *pgxpool.Pool) *EntityStore {
	return &EntityStore{db: db}
}

func (s *EntityStore) Create(ctx context.Context, e *domain.Entity) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO entities (agent_id, name, entity_type, aliases, metadata)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (agent_id, name, entity_type) DO UPDATE
		 SET aliases = ARRAY(SELECT DISTINCT unnest(entities.aliases || EXCLUDED.aliases)),
		     updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		e.AgentID, e.Name, e.EntityType, e.Aliases, e.Metadata,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

func (s *EntityStore) CreateAnchor(ctx context.Context, a *domain.Entity, agentID *uuid.UUID) error {
	if a.EntityType == "" {
		a.EntityType = domain.EntityPerson
	}
	var externalID *string
	if a.ExternalID != "" {
		externalID = &a.ExternalID
	}
	a.IsAnchor = true
	return s.db.QueryRow(ctx,
		`INSERT INTO entities (tenant_id, agent_id, name, entity_type, aliases, metadata, is_anchor, external_id)
		 VALUES ($1, $2, $3, $4, $5, $6, TRUE, $7)
		 ON CONFLICT (tenant_id, external_id) WHERE is_anchor = TRUE AND external_id IS NOT NULL
		 DO UPDATE SET name = EXCLUDED.name,
		               aliases = ARRAY(SELECT DISTINCT unnest(entities.aliases || EXCLUDED.aliases)),
		               updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		a.TenantID, agentID, a.Name, a.EntityType, a.Aliases, a.Metadata, externalID,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func scanAnchor(row pgx.Row) (*domain.Entity, error) {
	e := &domain.Entity{}
	err := row.Scan(&e.ID, &e.TenantID, &e.AgentID,
		&e.Name, &e.EntityType, &e.Aliases, &e.Metadata, &e.IsAnchor, &e.ExternalID,
		&e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

const anchorCols = `id, tenant_id, COALESCE(agent_id, '00000000-0000-0000-0000-000000000000'::uuid),
	name, entity_type, aliases, metadata, is_anchor, COALESCE(external_id, ''), created_at, updated_at`

func (s *EntityStore) GetAnchor(ctx context.Context, id, tenantID uuid.UUID) (*domain.Entity, error) {
	return scanAnchor(s.db.QueryRow(ctx,
		`SELECT `+anchorCols+` FROM entities
		 WHERE id = $1 AND tenant_id = $2 AND is_anchor = TRUE`,
		id, tenantID))
}

func (s *EntityStore) FindAnchorByExternalID(ctx context.Context, tenantID uuid.UUID, externalID string) (*domain.Entity, error) {
	return scanAnchor(s.db.QueryRow(ctx,
		`SELECT `+anchorCols+` FROM entities
		 WHERE tenant_id = $1 AND external_id = $2 AND is_anchor = TRUE`,
		tenantID, externalID))
}

func (s *EntityStore) ListAnchors(ctx context.Context, tenantID uuid.UUID, entityType domain.EntityType, limit int) ([]domain.Entity, error) {
	if limit <= 0 {
		limit = 100
	}
	args := []any{tenantID}
	typeClause := ""
	if entityType != "" {
		args = append(args, string(entityType))
		typeClause = "AND entity_type = $2"
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx,
		fmt.Sprintf(`SELECT `+anchorCols+` FROM entities
		 WHERE tenant_id = $1 AND is_anchor = TRUE `+typeClause+`
		 ORDER BY name LIMIT $%d`, len(args)),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var anchors []domain.Entity
	for rows.Next() {
		e, err := scanAnchor(rows)
		if err != nil {
			return nil, err
		}
		anchors = append(anchors, *e)
	}
	return anchors, rows.Err()
}

func (s *EntityStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Entity, error) {
	e := &domain.Entity{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, name, entity_type, aliases, metadata, created_at, updated_at
		 FROM entities WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Aliases, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

func (s *EntityStore) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM entities WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *EntityStore) FindByName(ctx context.Context, agentID uuid.UUID, name string) (*domain.Entity, error) {
	e := &domain.Entity{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, name, entity_type, aliases, metadata, created_at, updated_at
		 FROM entities WHERE agent_id = $1 AND LOWER(name) = LOWER($2)`,
		agentID, name,
	).Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Aliases, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

func (s *EntityStore) FindByNameOrAlias(ctx context.Context, agentID uuid.UUID, name string) (*domain.Entity, error) {
	e := &domain.Entity{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, name, entity_type, aliases, metadata, created_at, updated_at
		 FROM entities
		 WHERE agent_id = $1 AND (LOWER(name) = LOWER($2) OR LOWER($2) = ANY(SELECT LOWER(unnest(aliases))))`,
		agentID, name,
	).Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Aliases, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

func (s *EntityStore) GetByAgent(ctx context.Context, agentID uuid.UUID) ([]domain.Entity, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, name, entity_type, aliases, metadata, created_at, updated_at
		 FROM entities WHERE agent_id = $1 ORDER BY name`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []domain.Entity
	for rows.Next() {
		var e domain.Entity
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Aliases, &e.Metadata, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

func (s *EntityStore) AddAlias(ctx context.Context, id uuid.UUID, alias string) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE entities
		 SET aliases = ARRAY(SELECT DISTINCT unnest(aliases || ARRAY[$2])),
		     updated_at = NOW()
		 WHERE id = $1`,
		id, alias,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *EntityStore) CreateMention(ctx context.Context, m *domain.EntityMention) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO entity_mentions (entity_id, memory_id, mention_type)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (entity_id, memory_id) DO NOTHING`,
		m.EntityID, m.MemoryID, m.MentionType,
	)
	return err
}

func (s *EntityStore) GetMentionsByEntity(ctx context.Context, entityID uuid.UUID) ([]domain.EntityMention, error) {
	rows, err := s.db.Query(ctx,
		`SELECT entity_id, memory_id, mention_type, created_at
		 FROM entity_mentions WHERE entity_id = $1`,
		entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mentions []domain.EntityMention
	for rows.Next() {
		var m domain.EntityMention
		if err := rows.Scan(&m.EntityID, &m.MemoryID, &m.MentionType, &m.CreatedAt); err != nil {
			return nil, err
		}
		mentions = append(mentions, m)
	}
	return mentions, rows.Err()
}

func (s *EntityStore) GetMentionsByMemory(ctx context.Context, memoryID uuid.UUID) ([]domain.EntityMention, error) {
	rows, err := s.db.Query(ctx,
		`SELECT entity_id, memory_id, mention_type, created_at
		 FROM entity_mentions WHERE memory_id = $1`,
		memoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mentions []domain.EntityMention
	for rows.Next() {
		var m domain.EntityMention
		if err := rows.Scan(&m.EntityID, &m.MemoryID, &m.MentionType, &m.CreatedAt); err != nil {
			return nil, err
		}
		mentions = append(mentions, m)
	}
	return mentions, rows.Err()
}

func (s *EntityStore) DeleteMentionsByMemory(ctx context.Context, memoryID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM entity_mentions WHERE memory_id = $1`, memoryID)
	return err
}

func (s *EntityStore) GetMemoriesForEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT m.id, m.agent_id, m.tenant_id, m.type, m.content, m.embedding_provider, m.embedding_model,
		        m.source, m.provenance, m.confidence, m.metadata, m.expires_at, m.last_verified_at,
		        m.reinforcement_count, m.decay_rate, m.last_accessed_at, m.access_count, m.created_at, m.updated_at
		 FROM memories m
		 INNER JOIN entity_mentions em ON em.memory_id = m.id
		 WHERE em.entity_id = $1
		 ORDER BY m.confidence DESC, m.created_at DESC
		 LIMIT $2`,
		entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Type, &m.Content, &m.EmbeddingProvider,
			&m.EmbeddingModel, &m.Source, &m.Provenance, &m.Confidence, &m.Metadata, &m.ExpiresAt,
			&m.LastVerifiedAt, &m.ReinforcementCount, &m.DecayRate, &m.LastAccessedAt, &m.AccessCount,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *EntityStore) GetEntitiesForMemory(ctx context.Context, memoryID uuid.UUID) ([]domain.Entity, error) {
	rows, err := s.db.Query(ctx,
		`SELECT e.id, e.agent_id, e.name, e.entity_type, e.aliases, e.metadata, e.created_at, e.updated_at
		 FROM entities e
		 INNER JOIN entity_mentions em ON em.entity_id = e.id
		 WHERE em.memory_id = $1`,
		memoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []domain.Entity
	for rows.Next() {
		var e domain.Entity
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Aliases, &e.Metadata, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

func (s *EntityStore) FindByEmbeddingSimilarity(ctx context.Context, agentID uuid.UUID, entityType domain.EntityType, embedding []float32, threshold float32, limit int) ([]domain.Entity, error) {
	if len(embedding) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, name, entity_type, aliases, embedding, metadata, created_at, updated_at,
		        1 - (embedding <=> $3::vector) as similarity
		 FROM entities
		 WHERE agent_id = $1
		   AND entity_type = $2
		   AND embedding IS NOT NULL
		   AND 1 - (embedding <=> $3::vector) >= $4
		 ORDER BY similarity DESC
		 LIMIT $5`,
		agentID, entityType, embedding, threshold, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []domain.Entity
	for rows.Next() {
		var e domain.Entity
		var similarity float32
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Aliases, &e.Embedding, &e.Metadata, &e.CreatedAt, &e.UpdatedAt, &similarity); err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

func (s *EntityStore) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE entities SET embedding = $2, updated_at = NOW() WHERE id = $1`,
		id, embedding,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
