package store

import (
	"context"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ContradictionStore struct {
	db *pgxpool.Pool
}

func NewContradictionStore(db *pgxpool.Pool) *ContradictionStore {
	return &ContradictionStore{db: db}
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
