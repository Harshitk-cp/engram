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
)

type WorkingMemoryStore struct {
	db *pgxpool.Pool
}

func NewWorkingMemoryStore(db *pgxpool.Pool) *WorkingMemoryStore {
	return &WorkingMemoryStore{db: db}
}

// CreateSession creates a new working memory session.
func (s *WorkingMemoryStore) CreateSession(ctx context.Context, sess *domain.WorkingMemorySession) error {
	activeContextJSON, err := json.Marshal(sess.ActiveContext)
	if err != nil {
		return fmt.Errorf("marshal active_context: %w", err)
	}

	reasoningStateJSON, err := json.Marshal(sess.ReasoningState)
	if err != nil {
		return fmt.Errorf("marshal reasoning_state: %w", err)
	}

	if sess.MaxSlots == 0 {
		sess.MaxSlots = 7 // Miller's Law default
	}

	return s.db.QueryRow(ctx,
		`INSERT INTO working_memory_sessions (
			agent_id, tenant_id, current_goal, active_context, reasoning_state,
			max_slots, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (agent_id) DO UPDATE SET
			current_goal = EXCLUDED.current_goal,
			active_context = EXCLUDED.active_context,
			reasoning_state = EXCLUDED.reasoning_state,
			max_slots = EXCLUDED.max_slots,
			expires_at = EXCLUDED.expires_at,
			last_activity_at = NOW(),
			updated_at = NOW()
		RETURNING id, started_at, last_activity_at, created_at, updated_at`,
		sess.AgentID, sess.TenantID, sess.CurrentGoal, activeContextJSON, reasoningStateJSON,
		sess.MaxSlots, sess.ExpiresAt,
	).Scan(&sess.ID, &sess.StartedAt, &sess.LastActivityAt, &sess.CreatedAt, &sess.UpdatedAt)
}

// GetSession retrieves the active working memory session for an agent.
func (s *WorkingMemoryStore) GetSession(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (*domain.WorkingMemorySession, error) {
	sess := &domain.WorkingMemorySession{}
	var activeContextJSON, reasoningStateJSON []byte

	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, current_goal, active_context, reasoning_state,
			max_slots, started_at, last_activity_at, expires_at, created_at, updated_at
		FROM working_memory_sessions
		WHERE agent_id = $1 AND tenant_id = $2`,
		agentID, tenantID,
	).Scan(
		&sess.ID, &sess.AgentID, &sess.TenantID, &sess.CurrentGoal, &activeContextJSON, &reasoningStateJSON,
		&sess.MaxSlots, &sess.StartedAt, &sess.LastActivityAt, &sess.ExpiresAt, &sess.CreatedAt, &sess.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(activeContextJSON) > 0 {
		if err := json.Unmarshal(activeContextJSON, &sess.ActiveContext); err != nil {
			return nil, fmt.Errorf("unmarshal active_context: %w", err)
		}
	}
	if len(reasoningStateJSON) > 0 {
		if err := json.Unmarshal(reasoningStateJSON, &sess.ReasoningState); err != nil {
			return nil, fmt.Errorf("unmarshal reasoning_state: %w", err)
		}
	}

	return sess, nil
}

// GetSessionByID retrieves a working memory session by ID.
func (s *WorkingMemoryStore) GetSessionByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.WorkingMemorySession, error) {
	sess := &domain.WorkingMemorySession{}
	var activeContextJSON, reasoningStateJSON []byte

	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, tenant_id, current_goal, active_context, reasoning_state,
			max_slots, started_at, last_activity_at, expires_at, created_at, updated_at
		FROM working_memory_sessions
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(
		&sess.ID, &sess.AgentID, &sess.TenantID, &sess.CurrentGoal, &activeContextJSON, &reasoningStateJSON,
		&sess.MaxSlots, &sess.StartedAt, &sess.LastActivityAt, &sess.ExpiresAt, &sess.CreatedAt, &sess.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(activeContextJSON) > 0 {
		_ = json.Unmarshal(activeContextJSON, &sess.ActiveContext)
	}
	if len(reasoningStateJSON) > 0 {
		_ = json.Unmarshal(reasoningStateJSON, &sess.ReasoningState)
	}

	return sess, nil
}

// UpdateSession updates an existing working memory session.
func (s *WorkingMemoryStore) UpdateSession(ctx context.Context, sess *domain.WorkingMemorySession) error {
	activeContextJSON, err := json.Marshal(sess.ActiveContext)
	if err != nil {
		return fmt.Errorf("marshal active_context: %w", err)
	}

	reasoningStateJSON, err := json.Marshal(sess.ReasoningState)
	if err != nil {
		return fmt.Errorf("marshal reasoning_state: %w", err)
	}

	tag, err := s.db.Exec(ctx,
		`UPDATE working_memory_sessions SET
			current_goal = $1, active_context = $2, reasoning_state = $3,
			max_slots = $4, expires_at = $5, last_activity_at = NOW(), updated_at = NOW()
		WHERE id = $6 AND tenant_id = $7`,
		sess.CurrentGoal, activeContextJSON, reasoningStateJSON,
		sess.MaxSlots, sess.ExpiresAt, sess.ID, sess.TenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSession deletes a working memory session.
func (s *WorkingMemoryStore) DeleteSession(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM working_memory_sessions WHERE agent_id = $1 AND tenant_id = $2`,
		agentID, tenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLastActivity updates the last activity timestamp for a session.
func (s *WorkingMemoryStore) UpdateLastActivity(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE working_memory_sessions SET last_activity_at = NOW(), updated_at = NOW() WHERE id = $1`,
		sessionID,
	)
	return err
}

// CreateActivation creates a memory activation in a session.
func (s *WorkingMemoryStore) CreateActivation(ctx context.Context, a *domain.WorkingMemoryActivation) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO working_memory_activations (
			session_id, memory_type, memory_id, activation_level, activation_source,
			activation_cue, slot_position
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (session_id, memory_type, memory_id) DO UPDATE SET
			activation_level = EXCLUDED.activation_level,
			activation_source = EXCLUDED.activation_source,
			activation_cue = EXCLUDED.activation_cue,
			slot_position = EXCLUDED.slot_position,
			activated_at = NOW()
		RETURNING id, activated_at`,
		a.SessionID, a.MemoryType, a.MemoryID, a.ActivationLevel, a.ActivationSource,
		a.ActivationCue, a.SlotPosition,
	).Scan(&a.ID, &a.ActivatedAt)
}

// GetActivations retrieves all activations for a session.
func (s *WorkingMemoryStore) GetActivations(ctx context.Context, sessionID uuid.UUID) ([]domain.WorkingMemoryActivation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, session_id, memory_type, memory_id, activation_level, activation_source,
			activation_cue, slot_position, activated_at
		FROM working_memory_activations
		WHERE session_id = $1
		ORDER BY activation_level DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activations []domain.WorkingMemoryActivation
	for rows.Next() {
		var a domain.WorkingMemoryActivation
		err := rows.Scan(
			&a.ID, &a.SessionID, &a.MemoryType, &a.MemoryID, &a.ActivationLevel, &a.ActivationSource,
			&a.ActivationCue, &a.SlotPosition, &a.ActivatedAt,
		)
		if err != nil {
			return nil, err
		}
		activations = append(activations, a)
	}

	return activations, rows.Err()
}

// ClearActivations removes all activations for a session.
func (s *WorkingMemoryStore) ClearActivations(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM working_memory_activations WHERE session_id = $1`,
		sessionID,
	)
	return err
}

// DeleteActivation removes a specific activation.
func (s *WorkingMemoryStore) DeleteActivation(ctx context.Context, sessionID uuid.UUID, memoryType domain.ActivatedMemoryType, memoryID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM working_memory_activations
		WHERE session_id = $1 AND memory_type = $2 AND memory_id = $3`,
		sessionID, memoryType, memoryID,
	)
	return err
}

// CreateSchemaActivation creates a schema activation in a session.
func (s *WorkingMemoryStore) CreateSchemaActivation(ctx context.Context, a *domain.SchemaActivation) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO schema_activations (session_id, schema_id, match_score)
		VALUES ($1, $2, $3)
		ON CONFLICT (session_id, schema_id) DO UPDATE SET
			match_score = EXCLUDED.match_score,
			activated_at = NOW()
		RETURNING id, activated_at`,
		a.SessionID, a.SchemaID, a.MatchScore,
	).Scan(&a.ID, &a.ActivatedAt)
}

// GetSchemaActivations retrieves all schema activations for a session.
func (s *WorkingMemoryStore) GetSchemaActivations(ctx context.Context, sessionID uuid.UUID) ([]domain.SchemaActivation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, session_id, schema_id, match_score, activated_at
		FROM schema_activations
		WHERE session_id = $1
		ORDER BY match_score DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activations []domain.SchemaActivation
	for rows.Next() {
		var a domain.SchemaActivation
		err := rows.Scan(&a.ID, &a.SessionID, &a.SchemaID, &a.MatchScore, &a.ActivatedAt)
		if err != nil {
			return nil, err
		}
		activations = append(activations, a)
	}

	return activations, rows.Err()
}

// ClearSchemaActivations removes all schema activations for a session.
func (s *WorkingMemoryStore) ClearSchemaActivations(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM schema_activations WHERE session_id = $1`,
		sessionID,
	)
	return err
}

// DeleteExpiredSessions removes sessions that have expired.
func (s *WorkingMemoryStore) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM working_memory_sessions WHERE expires_at IS NOT NULL AND expires_at < NOW()`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Verify interface compliance at compile time
var _ domain.WorkingMemoryStore = (*WorkingMemoryStore)(nil)

// MemoryAssociationStore handles cross-memory associations.
type MemoryAssociationStore struct {
	db *pgxpool.Pool
}

func NewMemoryAssociationStore(db *pgxpool.Pool) *MemoryAssociationStore {
	return &MemoryAssociationStore{db: db}
}

// Create creates a new memory association.
func (s *MemoryAssociationStore) Create(ctx context.Context, a *domain.MemoryAssociation) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO memory_associations (
			source_memory_type, source_memory_id, target_memory_type, target_memory_id,
			association_type, association_strength
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (source_memory_type, source_memory_id, target_memory_type, target_memory_id, association_type)
		DO UPDATE SET association_strength = EXCLUDED.association_strength
		RETURNING id, created_at`,
		a.SourceMemoryType, a.SourceMemoryID, a.TargetMemoryType, a.TargetMemoryID,
		a.AssociationType, a.AssociationStrength,
	).Scan(&a.ID, &a.CreatedAt)
}

// GetBySource retrieves all associations where the given memory is the source.
func (s *MemoryAssociationStore) GetBySource(ctx context.Context, sourceType domain.ActivatedMemoryType, sourceID uuid.UUID) ([]domain.MemoryAssociation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, source_memory_type, source_memory_id, target_memory_type, target_memory_id,
			association_type, association_strength, created_at
		FROM memory_associations
		WHERE source_memory_type = $1 AND source_memory_id = $2
		ORDER BY association_strength DESC`,
		sourceType, sourceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanAssociations(rows)
}

// GetByTarget retrieves all associations where the given memory is the target.
func (s *MemoryAssociationStore) GetByTarget(ctx context.Context, targetType domain.ActivatedMemoryType, targetID uuid.UUID) ([]domain.MemoryAssociation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, source_memory_type, source_memory_id, target_memory_type, target_memory_id,
			association_type, association_strength, created_at
		FROM memory_associations
		WHERE target_memory_type = $1 AND target_memory_id = $2
		ORDER BY association_strength DESC`,
		targetType, targetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanAssociations(rows)
}

// Delete removes a memory association.
func (s *MemoryAssociationStore) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM memory_associations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStrength updates the strength of an association.
func (s *MemoryAssociationStore) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memory_associations SET association_strength = $1 WHERE id = $2`,
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

func (s *MemoryAssociationStore) scanAssociations(rows pgx.Rows) ([]domain.MemoryAssociation, error) {
	var associations []domain.MemoryAssociation
	for rows.Next() {
		var a domain.MemoryAssociation
		err := rows.Scan(
			&a.ID, &a.SourceMemoryType, &a.SourceMemoryID, &a.TargetMemoryType, &a.TargetMemoryID,
			&a.AssociationType, &a.AssociationStrength, &a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		associations = append(associations, a)
	}
	return associations, rows.Err()
}

// Verify interface compliance at compile time
var _ domain.MemoryAssociationStore = (*MemoryAssociationStore)(nil)
