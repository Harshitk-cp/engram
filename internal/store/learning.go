package store

import (
	"context"
	"errors"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MutationLogStore struct {
	db *pgxpool.Pool
}

func NewMutationLogStore(db *pgxpool.Pool) *MutationLogStore {
	return &MutationLogStore{db: db}
}

func (s *MutationLogStore) Create(ctx context.Context, m *domain.MutationLog) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO mutation_log (memory_id, agent_id, mutation_type, source_type, source_id, old_confidence, new_confidence, old_reinforcement_count, new_reinforcement_count, reason, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id, created_at`,
		m.MemoryID, m.AgentID, m.MutationType, m.SourceType, m.SourceID, m.OldConfidence, m.NewConfidence, m.OldReinforcementCount, m.NewReinforcementCount, m.Reason, m.Metadata,
	).Scan(&m.ID, &m.CreatedAt)
}

func (s *MutationLogStore) GetByMemoryID(ctx context.Context, memoryID uuid.UUID, limit int) ([]domain.MutationLog, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, memory_id, agent_id, mutation_type, source_type, source_id, old_confidence, new_confidence, old_reinforcement_count, new_reinforcement_count, reason, metadata, created_at
		 FROM mutation_log WHERE memory_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		memoryID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.MutationLog
	for rows.Next() {
		var m domain.MutationLog
		if err := rows.Scan(&m.ID, &m.MemoryID, &m.AgentID, &m.MutationType, &m.SourceType, &m.SourceID, &m.OldConfidence, &m.NewConfidence, &m.OldReinforcementCount, &m.NewReinforcementCount, &m.Reason, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, m)
	}
	return logs, rows.Err()
}

func (s *MutationLogStore) GetByAgentID(ctx context.Context, agentID uuid.UUID, since time.Time, limit int) ([]domain.MutationLog, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, memory_id, agent_id, mutation_type, source_type, source_id, old_confidence, new_confidence, old_reinforcement_count, new_reinforcement_count, reason, metadata, created_at
		 FROM mutation_log WHERE agent_id = $1 AND created_at >= $2
		 ORDER BY created_at DESC
		 LIMIT $3`,
		agentID, since, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.MutationLog
	for rows.Next() {
		var m domain.MutationLog
		if err := rows.Scan(&m.ID, &m.MemoryID, &m.AgentID, &m.MutationType, &m.SourceType, &m.SourceID, &m.OldConfidence, &m.NewConfidence, &m.OldReinforcementCount, &m.NewReinforcementCount, &m.Reason, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, m)
	}
	return logs, rows.Err()
}

type EpisodeMemoryUsageStore struct {
	db *pgxpool.Pool
}

func NewEpisodeMemoryUsageStore(db *pgxpool.Pool) *EpisodeMemoryUsageStore {
	return &EpisodeMemoryUsageStore{db: db}
}

func (s *EpisodeMemoryUsageStore) Create(ctx context.Context, u *domain.EpisodeMemoryUsage) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO episode_memory_usage (episode_id, memory_id, usage_type, relevance_score)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (episode_id, memory_id, usage_type) DO UPDATE SET relevance_score = EXCLUDED.relevance_score
		 RETURNING id, created_at`,
		u.EpisodeID, u.MemoryID, u.UsageType, u.RelevanceScore,
	).Scan(&u.ID, &u.CreatedAt)
}

func (s *EpisodeMemoryUsageStore) GetByEpisodeID(ctx context.Context, episodeID uuid.UUID) ([]domain.EpisodeMemoryUsage, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, episode_id, memory_id, usage_type, relevance_score, created_at
		 FROM episode_memory_usage WHERE episode_id = $1
		 ORDER BY created_at DESC`,
		episodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []domain.EpisodeMemoryUsage
	for rows.Next() {
		var u domain.EpisodeMemoryUsage
		if err := rows.Scan(&u.ID, &u.EpisodeID, &u.MemoryID, &u.UsageType, &u.RelevanceScore, &u.CreatedAt); err != nil {
			return nil, err
		}
		usages = append(usages, u)
	}
	return usages, rows.Err()
}

func (s *EpisodeMemoryUsageStore) GetByMemoryID(ctx context.Context, memoryID uuid.UUID) ([]domain.EpisodeMemoryUsage, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, episode_id, memory_id, usage_type, relevance_score, created_at
		 FROM episode_memory_usage WHERE memory_id = $1
		 ORDER BY created_at DESC`,
		memoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []domain.EpisodeMemoryUsage
	for rows.Next() {
		var u domain.EpisodeMemoryUsage
		if err := rows.Scan(&u.ID, &u.EpisodeID, &u.MemoryID, &u.UsageType, &u.RelevanceScore, &u.CreatedAt); err != nil {
			return nil, err
		}
		usages = append(usages, u)
	}
	return usages, rows.Err()
}

type LearningStatsStore struct {
	db *pgxpool.Pool
}

func NewLearningStatsStore(db *pgxpool.Pool) *LearningStatsStore {
	return &LearningStatsStore{db: db}
}

func (s *LearningStatsStore) Upsert(ctx context.Context, st *domain.LearningStats) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO learning_stats (agent_id, period_start, period_end, helpful_count, unhelpful_count, ignored_count, contradicted_count, outdated_count, success_count, failure_count, neutral_count, confidence_increases, confidence_decreases, memories_reinforced, memories_archived, learning_velocity, stability_score)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 ON CONFLICT (agent_id, period_start, period_end) DO UPDATE SET
		    helpful_count = EXCLUDED.helpful_count,
		    unhelpful_count = EXCLUDED.unhelpful_count,
		    ignored_count = EXCLUDED.ignored_count,
		    contradicted_count = EXCLUDED.contradicted_count,
		    outdated_count = EXCLUDED.outdated_count,
		    success_count = EXCLUDED.success_count,
		    failure_count = EXCLUDED.failure_count,
		    neutral_count = EXCLUDED.neutral_count,
		    confidence_increases = EXCLUDED.confidence_increases,
		    confidence_decreases = EXCLUDED.confidence_decreases,
		    memories_reinforced = EXCLUDED.memories_reinforced,
		    memories_archived = EXCLUDED.memories_archived,
		    learning_velocity = EXCLUDED.learning_velocity,
		    stability_score = EXCLUDED.stability_score
		 RETURNING id, created_at`,
		st.AgentID, st.PeriodStart, st.PeriodEnd, st.HelpfulCount, st.UnhelpfulCount, st.IgnoredCount, st.ContradictedCount, st.OutdatedCount, st.SuccessCount, st.FailureCount, st.NeutralCount, st.ConfidenceIncreases, st.ConfidenceDecreases, st.MemoriesReinforced, st.MemoriesArchived, st.LearningVelocity, st.StabilityScore,
	).Scan(&st.ID, &st.CreatedAt)
}

func (s *LearningStatsStore) GetByAgentID(ctx context.Context, agentID uuid.UUID, limit int) ([]domain.LearningStats, error) {
	if limit <= 0 {
		limit = 30
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, period_start, period_end, helpful_count, unhelpful_count, ignored_count, contradicted_count, outdated_count, success_count, failure_count, neutral_count, confidence_increases, confidence_decreases, memories_reinforced, memories_archived, learning_velocity, stability_score, created_at
		 FROM learning_stats WHERE agent_id = $1
		 ORDER BY period_end DESC
		 LIMIT $2`,
		agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []domain.LearningStats
	for rows.Next() {
		var st domain.LearningStats
		if err := rows.Scan(&st.ID, &st.AgentID, &st.PeriodStart, &st.PeriodEnd, &st.HelpfulCount, &st.UnhelpfulCount, &st.IgnoredCount, &st.ContradictedCount, &st.OutdatedCount, &st.SuccessCount, &st.FailureCount, &st.NeutralCount, &st.ConfidenceIncreases, &st.ConfidenceDecreases, &st.MemoriesReinforced, &st.MemoriesArchived, &st.LearningVelocity, &st.StabilityScore, &st.CreatedAt); err != nil {
			return nil, err
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

func (s *LearningStatsStore) GetLatest(ctx context.Context, agentID uuid.UUID) (*domain.LearningStats, error) {
	st := &domain.LearningStats{}
	err := s.db.QueryRow(ctx,
		`SELECT id, agent_id, period_start, period_end, helpful_count, unhelpful_count, ignored_count, contradicted_count, outdated_count, success_count, failure_count, neutral_count, confidence_increases, confidence_decreases, memories_reinforced, memories_archived, learning_velocity, stability_score, created_at
		 FROM learning_stats WHERE agent_id = $1
		 ORDER BY period_end DESC
		 LIMIT 1`,
		agentID,
	).Scan(&st.ID, &st.AgentID, &st.PeriodStart, &st.PeriodEnd, &st.HelpfulCount, &st.UnhelpfulCount, &st.IgnoredCount, &st.ContradictedCount, &st.OutdatedCount, &st.SuccessCount, &st.FailureCount, &st.NeutralCount, &st.ConfidenceIncreases, &st.ConfidenceDecreases, &st.MemoriesReinforced, &st.MemoriesArchived, &st.LearningVelocity, &st.StabilityScore, &st.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return st, nil
}
