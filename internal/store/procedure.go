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

type ProcedureStore struct {
	db *pgxpool.Pool
}

func NewProcedureStore(db *pgxpool.Pool) *ProcedureStore {
	return &ProcedureStore{db: db}
}

func (s *ProcedureStore) Create(ctx context.Context, p *domain.Procedure) error {
	var triggerEmbedding *pgvector.Vector
	if len(p.TriggerEmbedding) > 0 {
		v := pgvector.NewVector(p.TriggerEmbedding)
		triggerEmbedding = &v
	}

	triggerKeywordsJSON, err := json.Marshal(p.TriggerKeywords)
	if err != nil {
		return fmt.Errorf("marshal trigger_keywords: %w", err)
	}

	exampleExchangesJSON, err := json.Marshal(p.ExampleExchanges)
	if err != nil {
		return fmt.Errorf("marshal example_exchanges: %w", err)
	}

	// Set defaults
	if p.Confidence == 0 {
		p.Confidence = 0.5
	}
	if p.MemoryStrength == 0 {
		p.MemoryStrength = 1.0
	}
	if p.Version == 0 {
		p.Version = 1
	}

	return s.db.QueryRow(ctx,
		`INSERT INTO procedures (
			agent_id, tenant_id, trigger_pattern, trigger_keywords, trigger_embedding,
			action_template, action_type, use_count, success_count, failure_count,
			last_used_at, derived_from_episodes, example_exchanges,
			confidence, memory_strength, last_verified_at, version, previous_version_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13,
			$14, $15, $16, $17, $18
		) RETURNING id, created_at, updated_at`,
		p.AgentID, p.TenantID, p.TriggerPattern, triggerKeywordsJSON, triggerEmbedding,
		p.ActionTemplate, p.ActionType, p.UseCount, p.SuccessCount, p.FailureCount,
		p.LastUsedAt, p.DerivedFromEpisodes, exampleExchangesJSON,
		p.Confidence, p.MemoryStrength, p.LastVerifiedAt, p.Version, p.PreviousVersionID,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (s *ProcedureStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Procedure, error) {
	p := &domain.Procedure{}
	var triggerKeywordsJSON, exampleExchangesJSON []byte

	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, trigger_pattern, trigger_keywords,
			action_template, action_type, use_count, success_count, failure_count, success_rate,
			last_used_at, derived_from_episodes, example_exchanges,
			confidence, memory_strength, last_verified_at, version, previous_version_id,
			created_at, updated_at
		FROM procedures WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(
		&p.ID, &p.AgentID, &p.TenantID, &p.TriggerPattern, &triggerKeywordsJSON,
		&p.ActionTemplate, &p.ActionType, &p.UseCount, &p.SuccessCount, &p.FailureCount, &p.SuccessRate,
		&p.LastUsedAt, &p.DerivedFromEpisodes, &exampleExchangesJSON,
		&p.Confidence, &p.MemoryStrength, &p.LastVerifiedAt, &p.Version, &p.PreviousVersionID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(triggerKeywordsJSON) > 0 {
		if err := json.Unmarshal(triggerKeywordsJSON, &p.TriggerKeywords); err != nil {
			return nil, fmt.Errorf("unmarshal trigger_keywords: %w", err)
		}
	}
	if len(exampleExchangesJSON) > 0 {
		if err := json.Unmarshal(exampleExchangesJSON, &p.ExampleExchanges); err != nil {
			return nil, fmt.Errorf("unmarshal example_exchanges: %w", err)
		}
	}

	return p, nil
}

func (s *ProcedureStore) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Procedure, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, trigger_pattern, trigger_keywords,
			action_template, action_type, use_count, success_count, failure_count, success_rate,
			last_used_at, derived_from_episodes, example_exchanges,
			confidence, memory_strength, last_verified_at, version, previous_version_id,
			created_at, updated_at
		FROM procedures WHERE agent_id = $1 AND tenant_id = $2
		ORDER BY success_rate DESC, confidence DESC`,
		agentID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanProcedures(rows)
}

func (s *ProcedureStore) FindByTriggerSimilarity(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.ProcedureWithScore, error) {
	if limit <= 0 {
		limit = 10
	}

	vec := pgvector.NewVector(embedding)

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, trigger_pattern, trigger_keywords,
			action_template, action_type, use_count, success_count, failure_count, success_rate,
			last_used_at, derived_from_episodes, example_exchanges,
			confidence, memory_strength, last_verified_at, version, previous_version_id,
			created_at, updated_at,
			1 - (trigger_embedding <=> $1) AS score
		FROM procedures
		WHERE agent_id = $2 AND tenant_id = $3 AND trigger_embedding IS NOT NULL AND 1 - (trigger_embedding <=> $1) >= $4
		ORDER BY score DESC
		LIMIT $5`,
		vec, agentID, tenantID, threshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find similar procedures query: %w", err)
	}
	defer rows.Close()

	var results []domain.ProcedureWithScore
	for rows.Next() {
		var p domain.ProcedureWithScore
		var triggerKeywordsJSON, exampleExchangesJSON []byte

		err := rows.Scan(
			&p.ID, &p.AgentID, &p.TenantID, &p.TriggerPattern, &triggerKeywordsJSON,
			&p.ActionTemplate, &p.ActionType, &p.UseCount, &p.SuccessCount, &p.FailureCount, &p.SuccessRate,
			&p.LastUsedAt, &p.DerivedFromEpisodes, &exampleExchangesJSON,
			&p.Confidence, &p.MemoryStrength, &p.LastVerifiedAt, &p.Version, &p.PreviousVersionID,
			&p.CreatedAt, &p.UpdatedAt,
			&p.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan similar procedure row: %w", err)
		}

		if len(triggerKeywordsJSON) > 0 {
			_ = json.Unmarshal(triggerKeywordsJSON, &p.TriggerKeywords)
		}
		if len(exampleExchangesJSON) > 0 {
			_ = json.Unmarshal(exampleExchangesJSON, &p.ExampleExchanges)
		}

		results = append(results, p)
	}

	return results, rows.Err()
}

func (s *ProcedureStore) RecordUse(ctx context.Context, id uuid.UUID, success bool) error {
	var query string
	if success {
		query = `UPDATE procedures
			SET use_count = use_count + 1,
				success_count = success_count + 1,
				last_used_at = NOW(),
				updated_at = NOW()
			WHERE id = $1`
	} else {
		query = `UPDATE procedures
			SET use_count = use_count + 1,
				failure_count = failure_count + 1,
				last_used_at = NOW(),
				updated_at = NOW()
			WHERE id = $1`
	}

	tag, err := s.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ProcedureStore) Reinforce(ctx context.Context, id uuid.UUID, episodeID uuid.UUID, confidenceBoost float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE procedures
		SET confidence = LEAST(confidence + $1, 0.99),
			last_verified_at = NOW(),
			derived_from_episodes = array_append(derived_from_episodes, $2),
			updated_at = NOW()
		WHERE id = $3`,
		confidenceBoost, episodeID, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ProcedureStore) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE procedures SET confidence = $1, updated_at = NOW() WHERE id = $2`,
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

func (s *ProcedureStore) Archive(ctx context.Context, id uuid.UUID) error {
	// Soft delete by setting memory_strength to 0
	tag, err := s.db.Exec(ctx,
		`UPDATE procedures SET memory_strength = 0, updated_at = NOW() WHERE id = $1`,
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

func (s *ProcedureStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Procedure, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, trigger_pattern, trigger_keywords,
			action_template, action_type, use_count, success_count, failure_count, success_rate,
			last_used_at, derived_from_episodes, example_exchanges,
			confidence, memory_strength, last_verified_at, version, previous_version_id,
			created_at, updated_at
		FROM procedures
		WHERE agent_id = $1 AND memory_strength > 0
		ORDER BY last_used_at ASC NULLS FIRST`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanProcedures(rows)
}

func (s *ProcedureStore) CreateNewVersion(ctx context.Context, p *domain.Procedure) error {
	// Get current version to set as previous
	var currentID uuid.UUID
	var currentVersion int
	err := s.db.QueryRow(ctx,
		`SELECT id, version FROM procedures WHERE id = $1`,
		p.PreviousVersionID,
	).Scan(&currentID, &currentVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// Update the new procedure's version and previous version
	p.Version = currentVersion + 1
	p.PreviousVersionID = &currentID
	p.ID = uuid.Nil // Will be set by Create

	return s.Create(ctx, p)
}

func (s *ProcedureStore) scanProcedures(rows pgx.Rows) ([]domain.Procedure, error) {
	var procedures []domain.Procedure
	for rows.Next() {
		var p domain.Procedure
		var triggerKeywordsJSON, exampleExchangesJSON []byte

		err := rows.Scan(
			&p.ID, &p.AgentID, &p.TenantID, &p.TriggerPattern, &triggerKeywordsJSON,
			&p.ActionTemplate, &p.ActionType, &p.UseCount, &p.SuccessCount, &p.FailureCount, &p.SuccessRate,
			&p.LastUsedAt, &p.DerivedFromEpisodes, &exampleExchangesJSON,
			&p.Confidence, &p.MemoryStrength, &p.LastVerifiedAt, &p.Version, &p.PreviousVersionID,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(triggerKeywordsJSON) > 0 {
			_ = json.Unmarshal(triggerKeywordsJSON, &p.TriggerKeywords)
		}
		if len(exampleExchangesJSON) > 0 {
			_ = json.Unmarshal(exampleExchangesJSON, &p.ExampleExchanges)
		}

		procedures = append(procedures, p)
	}

	return procedures, rows.Err()
}

// Verify interface compliance at compile time
var _ domain.ProcedureStore = (*ProcedureStore)(nil)
