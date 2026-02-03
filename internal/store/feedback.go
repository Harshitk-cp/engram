package store

import (
	"context"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedbackStore struct {
	db *pgxpool.Pool
}

func NewFeedbackStore(db *pgxpool.Pool) *FeedbackStore {
	return &FeedbackStore{db: db}
}

func (s *FeedbackStore) Create(ctx context.Context, f *domain.Feedback) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO feedback_signals (memory_id, agent_id, signal_type, context)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		f.MemoryID, f.AgentID, f.SignalType, f.Context,
	).Scan(&f.ID, &f.CreatedAt)
}

func (s *FeedbackStore) GetByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.Feedback, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, memory_id, agent_id, signal_type, context, created_at
		 FROM feedback_signals WHERE agent_id = $1
		 ORDER BY created_at DESC`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedbacks []domain.Feedback
	for rows.Next() {
		var f domain.Feedback
		if err := rows.Scan(&f.ID, &f.MemoryID, &f.AgentID, &f.SignalType, &f.Context, &f.CreatedAt); err != nil {
			return nil, err
		}
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, rows.Err()
}

func (s *FeedbackStore) GetByMemoryID(ctx context.Context, memoryID uuid.UUID) ([]domain.Feedback, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, memory_id, agent_id, signal_type, context, created_at
		 FROM feedback_signals WHERE memory_id = $1
		 ORDER BY created_at DESC`,
		memoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedbacks []domain.Feedback
	for rows.Next() {
		var f domain.Feedback
		if err := rows.Scan(&f.ID, &f.MemoryID, &f.AgentID, &f.SignalType, &f.Context, &f.CreatedAt); err != nil {
			return nil, err
		}
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, rows.Err()
}

func (s *FeedbackStore) GetAggregatesByAgentID(ctx context.Context, agentID uuid.UUID) ([]domain.FeedbackAggregate, error) {
	rows, err := s.db.Query(ctx,
		`SELECT fs.agent_id, m.type, fs.signal_type, COUNT(*)
		 FROM feedback_signals fs
		 JOIN memories m ON fs.memory_id = m.id
		 WHERE fs.agent_id = $1
		 GROUP BY fs.agent_id, m.type, fs.signal_type
		 ORDER BY m.type, fs.signal_type`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aggregates []domain.FeedbackAggregate
	for rows.Next() {
		var a domain.FeedbackAggregate
		if err := rows.Scan(&a.AgentID, &a.MemoryType, &a.SignalType, &a.Count); err != nil {
			return nil, err
		}
		aggregates = append(aggregates, a)
	}
	return aggregates, rows.Err()
}

func (s *FeedbackStore) CountByAgentID(ctx context.Context, agentID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM feedback_signals WHERE agent_id = $1`,
		agentID,
	).Scan(&count)
	return count, err
}

func (s *FeedbackStore) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT agent_id FROM feedback_signals`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
