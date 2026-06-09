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
	db   DBTX
	pool *pgxpool.Pool
}

func NewMutationLogStore(db *pgxpool.Pool) *MutationLogStore {
	return &MutationLogStore{db: db, pool: db}
}

// withTx returns a clone of the store that runs against the given transaction.
func (s *MutationLogStore) withTx(tx pgx.Tx) *MutationLogStore {
	return &MutationLogStore{db: tx, pool: s.pool}
}

func (s *MutationLogStore) Create(ctx context.Context, m *domain.MutationLog) error {
	return insertMutationLog(ctx, s.db, m)
}

// insertMutationLog writes one audit row against any DBTX (pool or tx), so
// deletion/archive paths can log atomically inside their own transaction.
func insertMutationLog(ctx context.Context, db DBTX, m *domain.MutationLog) error {
	return db.QueryRow(ctx,
		`INSERT INTO mutation_log (memory_id, agent_id, mutation_type, source_type, source_id, old_confidence, new_confidence, old_reinforcement_count, new_reinforcement_count, reason, metadata, tenant_id, anchor_id, binding, content_hash, content_snapshot, actor_type, actor_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 RETURNING id, created_at`,
		m.MemoryID, m.AgentID, m.MutationType, m.SourceType, m.SourceID, m.OldConfidence, m.NewConfidence, m.OldReinforcementCount, m.NewReinforcementCount, m.Reason, m.Metadata, m.TenantID, m.AnchorID, nullIfEmpty(m.Binding), nullIfEmpty(m.ContentHash), m.ContentSnapshot, nullIfEmpty(m.ActorType), m.ActorID,
	).Scan(&m.ID, &m.CreatedAt)
}

// nullIfEmpty maps "" to a SQL NULL so optional text columns stay null rather
// than storing empty strings.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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

// VerifyChain walks the tenant's audit hash chain in the DB and reports whether
// it is intact, how many rows verified, and the first broken seq (nil if intact).
func (s *MutationLogStore) VerifyChain(ctx context.Context, tenantID uuid.UUID) (valid bool, checked int64, breakSeq *int64, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT valid, checked, break_seq FROM verify_audit_chain($1)`, tenantID,
	).Scan(&valid, &checked, &breakSeq)
	return valid, checked, breakSeq, err
}

// ChainHead returns the tenant's current chain length and head hash (the value an
// auditor anchors against). Returns (0, "") for an empty chain.
func (s *MutationLogStore) ChainHead(ctx context.Context, tenantID uuid.UUID) (int64, string, error) {
	var seq int64
	var hash string
	err := s.db.QueryRow(ctx,
		`SELECT last_seq, last_hash FROM audit_chain_heads WHERE tenant_id = $1`, tenantID,
	).Scan(&seq, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	return seq, hash, err
}

// ExportByTenant returns the full audit trail for a tenant ordered by chain seq,
// including the hash-chain fields, for signed export.
func (s *MutationLogStore) ExportByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.MutationLog, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, memory_id, agent_id, mutation_type, source_type, source_id, old_confidence, new_confidence, reason, metadata, tenant_id, anchor_id, COALESCE(binding,''), COALESCE(content_hash,''), content_snapshot, COALESCE(actor_type,''), actor_id, created_at, COALESCE(seq,0), COALESCE(prev_hash,''), COALESCE(row_hash,'')
		 FROM mutation_log WHERE tenant_id = $1 ORDER BY seq`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MutationLog
	for rows.Next() {
		var m domain.MutationLog
		if err := rows.Scan(&m.ID, &m.MemoryID, &m.AgentID, &m.MutationType, &m.SourceType, &m.SourceID, &m.OldConfidence, &m.NewConfidence, &m.Reason, &m.Metadata, &m.TenantID, &m.AnchorID, &m.Binding, &m.ContentHash, &m.ContentSnapshot, &m.ActorType, &m.ActorID, &m.CreatedAt, &m.Seq, &m.PrevHash, &m.RowHash); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
