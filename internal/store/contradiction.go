package store

import (
	"context"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ContradictionStore struct {
	db   DBTX
	pool *pgxpool.Pool
}

func NewContradictionStore(db *pgxpool.Pool) *ContradictionStore {
	return &ContradictionStore{db: db, pool: db}
}

// withTx returns a clone of the store that runs against the given transaction.
func (s *ContradictionStore) withTx(tx pgx.Tx) *ContradictionStore {
	return &ContradictionStore{db: tx, pool: s.pool}
}

func (s *ContradictionStore) Create(ctx context.Context, beliefID, contradictedByID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO belief_contradictions (belief_id, contradicted_by_id)
		 VALUES ($1, $2)
		 ON CONFLICT (belief_id, contradicted_by_id) DO NOTHING`,
		beliefID, contradictedByID,
	)
	return err
}

// CountByAgent counts contradiction edges whose belief belongs to the agent/tenant.
func (s *ContradictionStore) CountByAgent(ctx context.Context, agentID, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM belief_contradictions bc
		 JOIN memories m ON m.id = bc.belief_id
		 WHERE m.agent_id = $1 AND m.tenant_id = $2`,
		agentID, tenantID,
	).Scan(&n)
	return n, err
}

// ListByAgent returns contradiction pairs (with both beliefs' content) for an
// agent — the source of truth for the console's contradictions view.
func (s *ContradictionStore) ListByAgent(ctx context.Context, agentID, tenantID uuid.UUID, limit int) ([]domain.ContradictionPair, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT bc.belief_id, b.content, b.confidence, bc.contradicted_by_id, c.content, c.confidence, bc.detected_at
		 FROM belief_contradictions bc
		 JOIN memories b ON b.id = bc.belief_id
		 JOIN memories c ON c.id = bc.contradicted_by_id
		 WHERE b.agent_id = $1 AND b.tenant_id = $2
		 ORDER BY bc.detected_at DESC
		 LIMIT $3`,
		agentID, tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ContradictionPair
	for rows.Next() {
		var p domain.ContradictionPair
		if err := rows.Scan(&p.BeliefID, &p.BeliefContent, &p.BeliefConfidence, &p.OtherID, &p.OtherContent, &p.OtherConfidence, &p.DetectedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ContradictionStore) GetByBeliefID(ctx context.Context, beliefID uuid.UUID) ([]domain.BeliefContradiction, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, belief_id, contradicted_by_id, detected_at
		 FROM belief_contradictions WHERE belief_id = $1`,
		beliefID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.BeliefContradiction
	for rows.Next() {
		var c domain.BeliefContradiction
		var detectedAt time.Time
		if err := rows.Scan(&c.ID, &c.BeliefID, &c.ContradictedByID, &detectedAt); err != nil {
			return nil, err
		}
		c.DetectedAt = detectedAt
		results = append(results, c)
	}
	return results, rows.Err()
}

func (s *ContradictionStore) GetByContradictedByID(ctx context.Context, contradictedByID uuid.UUID) ([]domain.BeliefContradiction, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, belief_id, contradicted_by_id, detected_at
		 FROM belief_contradictions WHERE contradicted_by_id = $1`,
		contradictedByID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.BeliefContradiction
	for rows.Next() {
		var c domain.BeliefContradiction
		var detectedAt time.Time
		if err := rows.Scan(&c.ID, &c.BeliefID, &c.ContradictedByID, &detectedAt); err != nil {
			return nil, err
		}
		c.DetectedAt = detectedAt
		results = append(results, c)
	}
	return results, rows.Err()
}
