package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type SchemaStore struct {
	db *pgxpool.Pool
}

func NewSchemaStore(db *pgxpool.Pool) *SchemaStore {
	return &SchemaStore{db: db}
}

func (s *SchemaStore) Create(ctx context.Context, schema *domain.Schema) error {
	var embedding *pgvector.Vector
	if len(schema.Embedding) > 0 {
		v := pgvector.NewVector(schema.Embedding)
		embedding = &v
	}

	attributesJSON, err := json.Marshal(schema.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	applicableContextsJSON, err := json.Marshal(schema.ApplicableContexts)
	if err != nil {
		return fmt.Errorf("marshal applicable_contexts: %w", err)
	}

	// Set defaults
	if schema.Confidence == 0 {
		schema.Confidence = 0.5
	}

	return s.db.QueryRow(ctx,
		`INSERT INTO schemas (
			agent_id, tenant_id, schema_type, name, description,
			attributes, evidence_memories, evidence_episodes, evidence_count,
			confidence, last_validated_at, contradiction_count, applicable_contexts, embedding
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12, $13, $14
		) RETURNING id, created_at, updated_at`,
		schema.AgentID, schema.TenantID, schema.SchemaType, schema.Name, schema.Description,
		attributesJSON, schema.EvidenceMemories, schema.EvidenceEpisodes, schema.EvidenceCount,
		schema.Confidence, schema.LastValidatedAt, schema.ContradictionCount, applicableContextsJSON, embedding,
	).Scan(&schema.ID, &schema.CreatedAt, &schema.UpdatedAt)
}

func (s *SchemaStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Schema, error) {
	schema := &domain.Schema{}
	var attributesJSON, applicableContextsJSON []byte

	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, schema_type, name, description,
			attributes, evidence_memories, evidence_episodes, evidence_count,
			confidence, last_validated_at, contradiction_count, applicable_contexts,
			created_at, updated_at
		FROM schemas WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(
		&schema.ID, &schema.AgentID, &schema.TenantID, &schema.SchemaType, &schema.Name, &schema.Description,
		&attributesJSON, &schema.EvidenceMemories, &schema.EvidenceEpisodes, &schema.EvidenceCount,
		&schema.Confidence, &schema.LastValidatedAt, &schema.ContradictionCount, &applicableContextsJSON,
		&schema.CreatedAt, &schema.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(attributesJSON) > 0 {
		if err := json.Unmarshal(attributesJSON, &schema.Attributes); err != nil {
			return nil, fmt.Errorf("unmarshal attributes: %w", err)
		}
	}
	if len(applicableContextsJSON) > 0 {
		if err := json.Unmarshal(applicableContextsJSON, &schema.ApplicableContexts); err != nil {
			return nil, fmt.Errorf("unmarshal applicable_contexts: %w", err)
		}
	}

	return schema, nil
}

func (s *SchemaStore) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Schema, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, schema_type, name, description,
			attributes, evidence_memories, evidence_episodes, evidence_count,
			confidence, last_validated_at, contradiction_count, applicable_contexts,
			created_at, updated_at
		FROM schemas WHERE agent_id = $1 AND tenant_id = $2
		ORDER BY confidence DESC`,
		agentID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSchemas(rows)
}

func (s *SchemaStore) GetByName(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, schemaType domain.SchemaType, name string) (*domain.Schema, error) {
	schema := &domain.Schema{}
	var attributesJSON, applicableContextsJSON []byte

	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, schema_type, name, description,
			attributes, evidence_memories, evidence_episodes, evidence_count,
			confidence, last_validated_at, contradiction_count, applicable_contexts,
			created_at, updated_at
		FROM schemas WHERE agent_id = $1 AND tenant_id = $2 AND schema_type = $3 AND name = $4`,
		agentID, tenantID, schemaType, name,
	).Scan(
		&schema.ID, &schema.AgentID, &schema.TenantID, &schema.SchemaType, &schema.Name, &schema.Description,
		&attributesJSON, &schema.EvidenceMemories, &schema.EvidenceEpisodes, &schema.EvidenceCount,
		&schema.Confidence, &schema.LastValidatedAt, &schema.ContradictionCount, &applicableContextsJSON,
		&schema.CreatedAt, &schema.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(attributesJSON) > 0 {
		if err := json.Unmarshal(attributesJSON, &schema.Attributes); err != nil {
			return nil, fmt.Errorf("unmarshal attributes: %w", err)
		}
	}
	if len(applicableContextsJSON) > 0 {
		if err := json.Unmarshal(applicableContextsJSON, &schema.ApplicableContexts); err != nil {
			return nil, fmt.Errorf("unmarshal applicable_contexts: %w", err)
		}
	}

	return schema, nil
}

func (s *SchemaStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM schemas WHERE id = $1 AND tenant_id = $2`,
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

func (s *SchemaStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.SchemaWithScore, error) {
	if limit <= 0 {
		limit = 10
	}

	vec := pgvector.NewVector(embedding)

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, schema_type, name, description,
			attributes, evidence_memories, evidence_episodes, evidence_count,
			confidence, last_validated_at, contradiction_count, applicable_contexts,
			created_at, updated_at,
			1 - (embedding <=> $1) AS score
		FROM schemas
		WHERE agent_id = $2 AND tenant_id = $3 AND embedding IS NOT NULL AND 1 - (embedding <=> $1) >= $4
		ORDER BY score DESC
		LIMIT $5`,
		vec, agentID, tenantID, threshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find similar schemas query: %w", err)
	}
	defer rows.Close()

	var results []domain.SchemaWithScore
	for rows.Next() {
		var s domain.SchemaWithScore
		var attributesJSON, applicableContextsJSON []byte

		err := rows.Scan(
			&s.ID, &s.AgentID, &s.TenantID, &s.SchemaType, &s.Name, &s.Description,
			&attributesJSON, &s.EvidenceMemories, &s.EvidenceEpisodes, &s.EvidenceCount,
			&s.Confidence, &s.LastValidatedAt, &s.ContradictionCount, &applicableContextsJSON,
			&s.CreatedAt, &s.UpdatedAt,
			&s.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan similar schema row: %w", err)
		}

		if len(attributesJSON) > 0 {
			_ = json.Unmarshal(attributesJSON, &s.Attributes)
		}
		if len(applicableContextsJSON) > 0 {
			_ = json.Unmarshal(applicableContextsJSON, &s.ApplicableContexts)
		}

		results = append(results, s)
	}

	return results, rows.Err()
}

func (s *SchemaStore) AddEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error {
	var query string
	var args []any

	if memoryID != nil && episodeID != nil {
		query = `UPDATE schemas
			SET evidence_memories = array_append(evidence_memories, $1),
				evidence_episodes = array_append(evidence_episodes, $2),
				evidence_count = evidence_count + 2,
				updated_at = NOW()
			WHERE id = $3`
		args = []any{memoryID, episodeID, id}
	} else if memoryID != nil {
		query = `UPDATE schemas
			SET evidence_memories = array_append(evidence_memories, $1),
				evidence_count = evidence_count + 1,
				updated_at = NOW()
			WHERE id = $2`
		args = []any{memoryID, id}
	} else if episodeID != nil {
		query = `UPDATE schemas
			SET evidence_episodes = array_append(evidence_episodes, $1),
				evidence_count = evidence_count + 1,
				updated_at = NOW()
			WHERE id = $2`
		args = []any{episodeID, id}
	} else {
		return nil // Nothing to add
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

func (s *SchemaStore) RemoveEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error {
	var query string
	var args []any

	if memoryID != nil && episodeID != nil {
		query = `UPDATE schemas
			SET evidence_memories = array_remove(evidence_memories, $1),
				evidence_episodes = array_remove(evidence_episodes, $2),
				evidence_count = GREATEST(evidence_count - 2, 0),
				updated_at = NOW()
			WHERE id = $3`
		args = []any{memoryID, episodeID, id}
	} else if memoryID != nil {
		query = `UPDATE schemas
			SET evidence_memories = array_remove(evidence_memories, $1),
				evidence_count = GREATEST(evidence_count - 1, 0),
				updated_at = NOW()
			WHERE id = $2`
		args = []any{memoryID, id}
	} else if episodeID != nil {
		query = `UPDATE schemas
			SET evidence_episodes = array_remove(evidence_episodes, $1),
				evidence_count = GREATEST(evidence_count - 1, 0),
				updated_at = NOW()
			WHERE id = $2`
		args = []any{episodeID, id}
	} else {
		return nil // Nothing to remove
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

func (s *SchemaStore) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE schemas SET confidence = $1, updated_at = NOW() WHERE id = $2`,
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

func (s *SchemaStore) IncrementContradiction(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE schemas SET contradiction_count = contradiction_count + 1, updated_at = NOW() WHERE id = $1`,
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

func (s *SchemaStore) UpdateValidation(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE schemas SET last_validated_at = NOW(), updated_at = NOW() WHERE id = $1`,
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

func (s *SchemaStore) Update(ctx context.Context, schema *domain.Schema) error {
	var embedding *pgvector.Vector
	if len(schema.Embedding) > 0 {
		v := pgvector.NewVector(schema.Embedding)
		embedding = &v
	}

	attributesJSON, err := json.Marshal(schema.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	applicableContextsJSON, err := json.Marshal(schema.ApplicableContexts)
	if err != nil {
		return fmt.Errorf("marshal applicable_contexts: %w", err)
	}

	tag, err := s.db.Exec(ctx,
		`UPDATE schemas SET
			schema_type = $1, name = $2, description = $3,
			attributes = $4, evidence_memories = $5, evidence_episodes = $6, evidence_count = $7,
			confidence = $8, last_validated_at = $9, contradiction_count = $10,
			applicable_contexts = $11, embedding = $12, updated_at = NOW()
		WHERE id = $13 AND tenant_id = $14`,
		schema.SchemaType, schema.Name, schema.Description,
		attributesJSON, schema.EvidenceMemories, schema.EvidenceEpisodes, schema.EvidenceCount,
		schema.Confidence, schema.LastValidatedAt, schema.ContradictionCount,
		applicableContextsJSON, embedding, schema.ID, schema.TenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SchemaStore) scanSchemas(rows pgx.Rows) ([]domain.Schema, error) {
	var schemas []domain.Schema
	for rows.Next() {
		var schema domain.Schema
		var attributesJSON, applicableContextsJSON []byte

		err := rows.Scan(
			&schema.ID, &schema.AgentID, &schema.TenantID, &schema.SchemaType, &schema.Name, &schema.Description,
			&attributesJSON, &schema.EvidenceMemories, &schema.EvidenceEpisodes, &schema.EvidenceCount,
			&schema.Confidence, &schema.LastValidatedAt, &schema.ContradictionCount, &applicableContextsJSON,
			&schema.CreatedAt, &schema.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(attributesJSON) > 0 {
			_ = json.Unmarshal(attributesJSON, &schema.Attributes)
		}
		if len(applicableContextsJSON) > 0 {
			_ = json.Unmarshal(applicableContextsJSON, &schema.ApplicableContexts)
		}

		schemas = append(schemas, schema)
	}

	return schemas, rows.Err()
}

// Verify interface compliance at compile time
var _ domain.SchemaStore = (*SchemaStore)(nil)
