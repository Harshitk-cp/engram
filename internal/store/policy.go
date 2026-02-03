package store

import (
	"context"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PolicyStore struct {
	db *pgxpool.Pool
}

func NewPolicyStore(db *pgxpool.Pool) *PolicyStore {
	return &PolicyStore{db: db}
}

func (s *PolicyStore) Upsert(ctx context.Context, p *domain.Policy) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO memory_policies (agent_id, memory_type, max_memories, retention_days, priority_weight, auto_summarize)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (agent_id, memory_type)
		 DO UPDATE SET max_memories = EXCLUDED.max_memories,
		               retention_days = EXCLUDED.retention_days,
		               priority_weight = EXCLUDED.priority_weight,
		               auto_summarize = EXCLUDED.auto_summarize,
		               updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		p.AgentID, p.MemoryType, p.MaxMemories, p.RetentionDays, p.PriorityWeight, p.AutoSummarize,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (s *PolicyStore) GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.Policy, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, memory_type, max_memories, retention_days, priority_weight, auto_summarize, created_at, updated_at
		 FROM memory_policies WHERE agent_id = $1
		 ORDER BY memory_type`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []domain.Policy
	for rows.Next() {
		var p domain.Policy
		if err := rows.Scan(&p.ID, &p.AgentID, &p.MemoryType, &p.MaxMemories, &p.RetentionDays, &p.PriorityWeight, &p.AutoSummarize, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *PolicyStore) GetByAgentIDAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (*domain.Policy, error) {
	p := &domain.Policy{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, memory_type, max_memories, retention_days, priority_weight, auto_summarize, created_at, updated_at
		 FROM memory_policies WHERE agent_id = $1 AND memory_type = $2`,
		agentID, memType,
	).Scan(&p.ID, &p.AgentID, &p.MemoryType, &p.MaxMemories, &p.RetentionDays, &p.PriorityWeight, &p.AutoSummarize, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}
