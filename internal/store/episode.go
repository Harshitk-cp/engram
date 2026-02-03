package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type EpisodeStore struct {
	db *pgxpool.Pool
}

func NewEpisodeStore(db *pgxpool.Pool) *EpisodeStore {
	return &EpisodeStore{db: db}
}

func (s *EpisodeStore) Create(ctx context.Context, e *domain.Episode) error {
	var embedding *pgvector.Vector
	if len(e.Embedding) > 0 {
		v := pgvector.NewVector(e.Embedding)
		embedding = &v
	}

	// Encode JSON fields
	entitiesJSON, err := json.Marshal(e.Entities)
	if err != nil {
		return fmt.Errorf("marshal entities: %w", err)
	}

	causalLinksJSON, err := json.Marshal(e.CausalLinks)
	if err != nil {
		return fmt.Errorf("marshal causal_links: %w", err)
	}

	topicsJSON, err := json.Marshal(e.Topics)
	if err != nil {
		return fmt.Errorf("marshal topics: %w", err)
	}

	// Set defaults
	if e.ConsolidationStatus == "" {
		e.ConsolidationStatus = domain.ConsolidationRaw
	}
	if e.MemoryStrength == 0 {
		e.MemoryStrength = 1.0
	}
	if e.DecayRate == 0 {
		e.DecayRate = 0.1
	}
	if e.AccessCount == 0 {
		e.AccessCount = 1
	}
	if e.ImportanceScore == 0 {
		e.ImportanceScore = 0.5
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}

	var outcome *string
	if e.Outcome != "" {
		outcomeStr := string(e.Outcome)
		outcome = &outcomeStr
	}

	return s.db.QueryRow(ctx,
		`INSERT INTO episodes (
			agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, memory_strength, decay_rate, access_count,
			embedding
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18,
			$19, $20, $21, $22,
			$23
		) RETURNING id, last_accessed_at, created_at, updated_at`,
		e.AgentID, e.TenantID, e.RawContent, e.ConversationID, e.MessageSequence,
		e.OccurredAt, e.DurationSeconds, e.TimeOfDay, e.DayOfWeek,
		e.EmotionalValence, e.EmotionalIntensity, e.ImportanceScore,
		entitiesJSON, causalLinksJSON, topicsJSON,
		outcome, e.OutcomeDescription, e.OutcomeValence,
		e.ConsolidationStatus, e.MemoryStrength, e.DecayRate, e.AccessCount,
		embedding,
	).Scan(&e.ID, &e.LastAccessedAt, &e.CreatedAt, &e.UpdatedAt)
}

func (s *EpisodeStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Episode, error) {
	e := &domain.Episode{}
	var entitiesJSON, causalLinksJSON, topicsJSON []byte
	var outcome *string

	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(
		&e.ID, &e.AgentID, &e.TenantID, &e.RawContent, &e.ConversationID, &e.MessageSequence,
		&e.OccurredAt, &e.DurationSeconds, &e.TimeOfDay, &e.DayOfWeek,
		&e.EmotionalValence, &e.EmotionalIntensity, &e.ImportanceScore,
		&entitiesJSON, &causalLinksJSON, &topicsJSON,
		&outcome, &e.OutcomeDescription, &e.OutcomeValence,
		&e.ConsolidationStatus, &e.LastConsolidatedAt, &e.AbstractionCount,
		&e.DerivedSemanticIDs, &e.DerivedProceduralIDs,
		&e.MemoryStrength, &e.LastAccessedAt, &e.AccessCount, &e.DecayRate,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Parse JSON fields
	if len(entitiesJSON) > 0 {
		if err := json.Unmarshal(entitiesJSON, &e.Entities); err != nil {
			return nil, fmt.Errorf("unmarshal entities: %w", err)
		}
	}
	if len(causalLinksJSON) > 0 {
		if err := json.Unmarshal(causalLinksJSON, &e.CausalLinks); err != nil {
			return nil, fmt.Errorf("unmarshal causal_links: %w", err)
		}
	}
	if len(topicsJSON) > 0 {
		if err := json.Unmarshal(topicsJSON, &e.Topics); err != nil {
			return nil, fmt.Errorf("unmarshal topics: %w", err)
		}
	}

	if outcome != nil {
		e.Outcome = domain.OutcomeType(*outcome)
	}

	return e, nil
}

func (s *EpisodeStore) GetByConversationID(ctx context.Context, conversationID uuid.UUID, tenantID uuid.UUID) ([]domain.Episode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE conversation_id = $1 AND tenant_id = $2
		ORDER BY message_sequence, occurred_at`,
		conversationID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) GetByTimeRange(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, start, end time.Time) ([]domain.Episode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE agent_id = $1 AND tenant_id = $2 AND occurred_at >= $3 AND occurred_at <= $4
		ORDER BY occurred_at DESC`,
		agentID, tenantID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) GetByImportance(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, minImportance float32, limit int) ([]domain.Episode, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE agent_id = $1 AND tenant_id = $2 AND importance_score >= $3
		ORDER BY importance_score DESC, occurred_at DESC
		LIMIT $4`,
		agentID, tenantID, minImportance, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.EpisodeWithScore, error) {
	if limit <= 0 {
		limit = 10
	}

	vec := pgvector.NewVector(embedding)

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at,
			1 - (embedding <=> $1) AS score
		FROM episodes
		WHERE agent_id = $2 AND tenant_id = $3 AND embedding IS NOT NULL AND 1 - (embedding <=> $1) >= $4
		ORDER BY score DESC
		LIMIT $5`,
		vec, agentID, tenantID, threshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find similar episodes query: %w", err)
	}
	defer rows.Close()

	var results []domain.EpisodeWithScore
	for rows.Next() {
		var e domain.EpisodeWithScore
		var entitiesJSON, causalLinksJSON, topicsJSON []byte
		var outcome *string

		err := rows.Scan(
			&e.ID, &e.AgentID, &e.TenantID, &e.RawContent, &e.ConversationID, &e.MessageSequence,
			&e.OccurredAt, &e.DurationSeconds, &e.TimeOfDay, &e.DayOfWeek,
			&e.EmotionalValence, &e.EmotionalIntensity, &e.ImportanceScore,
			&entitiesJSON, &causalLinksJSON, &topicsJSON,
			&outcome, &e.OutcomeDescription, &e.OutcomeValence,
			&e.ConsolidationStatus, &e.LastConsolidatedAt, &e.AbstractionCount,
			&e.DerivedSemanticIDs, &e.DerivedProceduralIDs,
			&e.MemoryStrength, &e.LastAccessedAt, &e.AccessCount, &e.DecayRate,
			&e.CreatedAt, &e.UpdatedAt,
			&e.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan similar episode row: %w", err)
		}

		if len(entitiesJSON) > 0 {
			_ = json.Unmarshal(entitiesJSON, &e.Entities)
		}
		if len(causalLinksJSON) > 0 {
			_ = json.Unmarshal(causalLinksJSON, &e.CausalLinks)
		}
		if len(topicsJSON) > 0 {
			_ = json.Unmarshal(topicsJSON, &e.Topics)
		}
		if outcome != nil {
			e.Outcome = domain.OutcomeType(*outcome)
		}

		results = append(results, e)
	}

	return results, rows.Err()
}

func (s *EpisodeStore) GetUnconsolidated(ctx context.Context, agentID uuid.UUID, limit int) ([]domain.Episode, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE agent_id = $1 AND consolidation_status = 'raw'
		ORDER BY occurred_at ASC
		LIMIT $2`,
		agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) GetByConsolidationStatus(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, status domain.ConsolidationStatus, limit int) ([]domain.Episode, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE agent_id = $1 AND tenant_id = $2 AND consolidation_status = $3
		ORDER BY occurred_at ASC
		LIMIT $4`,
		agentID, tenantID, status, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) UpdateConsolidationStatus(ctx context.Context, id uuid.UUID, status domain.ConsolidationStatus) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE episodes SET consolidation_status = $1, last_consolidated_at = NOW(), updated_at = NOW() WHERE id = $2`,
		status, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *EpisodeStore) LinkDerivedMemory(ctx context.Context, episodeID uuid.UUID, memoryID uuid.UUID, memoryType string) error {
	var query string
	switch memoryType {
	case "semantic":
		query = `UPDATE episodes SET derived_semantic_ids = array_append(derived_semantic_ids, $1), abstraction_count = abstraction_count + 1, updated_at = NOW() WHERE id = $2`
	case "procedural":
		query = `UPDATE episodes SET derived_procedural_ids = array_append(derived_procedural_ids, $1), abstraction_count = abstraction_count + 1, updated_at = NOW() WHERE id = $2`
	default:
		return fmt.Errorf("unknown memory type: %s", memoryType)
	}

	tag, err := s.db.Exec(ctx, query, memoryID, episodeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *EpisodeStore) ApplyDecay(ctx context.Context, agentID uuid.UUID) (int64, error) {
	// Apply exponential decay based on time since last access
	// new_strength = memory_strength * exp(-decay_rate * hours_since_access / 24)
	tag, err := s.db.Exec(ctx,
		`UPDATE episodes
		SET memory_strength = GREATEST(0.0, memory_strength * EXP(-decay_rate * EXTRACT(EPOCH FROM (NOW() - last_accessed_at)) / 86400)),
			updated_at = NOW()
		WHERE agent_id = $1 AND consolidation_status != 'archived'`,
		agentID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *EpisodeStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Episode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE agent_id = $1 AND consolidation_status != 'archived'
		ORDER BY last_accessed_at ASC`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) GetWeakMemories(ctx context.Context, agentID uuid.UUID, threshold float32) ([]domain.Episode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, tenant_id, raw_content, conversation_id, message_sequence,
			occurred_at, duration_seconds, time_of_day, day_of_week,
			emotional_valence, emotional_intensity, importance_score,
			entities, causal_links, topics,
			outcome, outcome_description, outcome_valence,
			consolidation_status, last_consolidated_at, abstraction_count,
			derived_semantic_ids, derived_procedural_ids,
			memory_strength, last_accessed_at, access_count, decay_rate,
			created_at, updated_at
		FROM episodes WHERE agent_id = $1 AND memory_strength < $2 AND consolidation_status != 'archived'
		ORDER BY memory_strength ASC`,
		agentID, threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *EpisodeStore) Archive(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE episodes SET consolidation_status = 'archived', updated_at = NOW() WHERE id = $1`,
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

func (s *EpisodeStore) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE episodes SET memory_strength = $1, updated_at = NOW() WHERE id = $2`,
		strength, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *EpisodeStore) RecordAccess(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE episodes SET last_accessed_at = NOW(), access_count = access_count + 1, updated_at = NOW() WHERE id = $1`,
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

func (s *EpisodeStore) UpdateOutcome(ctx context.Context, id uuid.UUID, outcome domain.OutcomeType, description string) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE episodes SET outcome = $1, outcome_description = $2, updated_at = NOW() WHERE id = $3`,
		outcome, description, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *EpisodeStore) CreateAssociation(ctx context.Context, a *domain.EpisodeAssociation) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO episode_associations (episode_a_id, episode_b_id, association_type, association_strength)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (episode_a_id, episode_b_id, association_type) DO UPDATE SET association_strength = $4
		RETURNING id, created_at`,
		a.EpisodeAID, a.EpisodeBID, a.AssociationType, a.AssociationStrength,
	).Scan(&a.ID, &a.CreatedAt)
}

func (s *EpisodeStore) GetAssociations(ctx context.Context, episodeID uuid.UUID) ([]domain.EpisodeAssociation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, episode_a_id, episode_b_id, association_type, association_strength, created_at
		FROM episode_associations
		WHERE episode_a_id = $1 OR episode_b_id = $1
		ORDER BY association_strength DESC`,
		episodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var associations []domain.EpisodeAssociation
	for rows.Next() {
		var a domain.EpisodeAssociation
		err := rows.Scan(&a.ID, &a.EpisodeAID, &a.EpisodeBID, &a.AssociationType, &a.AssociationStrength, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		associations = append(associations, a)
	}

	return associations, rows.Err()
}

// scanEpisodes is a helper to scan multiple episode rows
func (s *EpisodeStore) scanEpisodes(rows pgx.Rows) ([]domain.Episode, error) {
	var episodes []domain.Episode
	for rows.Next() {
		var e domain.Episode
		var entitiesJSON, causalLinksJSON, topicsJSON []byte
		var outcome *string

		err := rows.Scan(
			&e.ID, &e.AgentID, &e.TenantID, &e.RawContent, &e.ConversationID, &e.MessageSequence,
			&e.OccurredAt, &e.DurationSeconds, &e.TimeOfDay, &e.DayOfWeek,
			&e.EmotionalValence, &e.EmotionalIntensity, &e.ImportanceScore,
			&entitiesJSON, &causalLinksJSON, &topicsJSON,
			&outcome, &e.OutcomeDescription, &e.OutcomeValence,
			&e.ConsolidationStatus, &e.LastConsolidatedAt, &e.AbstractionCount,
			&e.DerivedSemanticIDs, &e.DerivedProceduralIDs,
			&e.MemoryStrength, &e.LastAccessedAt, &e.AccessCount, &e.DecayRate,
			&e.CreatedAt, &e.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(entitiesJSON) > 0 {
			_ = json.Unmarshal(entitiesJSON, &e.Entities)
		}
		if len(causalLinksJSON) > 0 {
			_ = json.Unmarshal(causalLinksJSON, &e.CausalLinks)
		}
		if len(topicsJSON) > 0 {
			_ = json.Unmarshal(topicsJSON, &e.Topics)
		}
		if outcome != nil {
			e.Outcome = domain.OutcomeType(*outcome)
		}

		episodes = append(episodes, e)
	}

	return episodes, rows.Err()
}
