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

type SessionStore struct {
	db *pgxpool.Pool
}

func NewSessionStore(db *pgxpool.Pool) *SessionStore {
	return &SessionStore{db: db}
}

const sessionCols = `id, tenant_id, agent_id, anchor_id, COALESCE(external_id, ''),
	status, metadata, started_at, ended_at, expires_at, created_at`

func scanSession(row pgx.Row) (*domain.Session, error) {
	s := &domain.Session{}
	err := row.Scan(&s.ID, &s.TenantID, &s.AgentID, &s.AnchorID, &s.ExternalID,
		&s.Status, &s.Metadata, &s.StartedAt, &s.EndedAt, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (s *SessionStore) Create(ctx context.Context, sess *domain.Session) error {
	if sess.Status == "" {
		sess.Status = domain.SessionActive
	}
	var externalID *string
	if sess.ExternalID != "" {
		externalID = &sess.ExternalID
	}
	return s.db.QueryRow(ctx,
		`INSERT INTO sessions (tenant_id, agent_id, anchor_id, external_id, status, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, started_at, created_at`,
		sess.TenantID, sess.AgentID, sess.AnchorID, externalID, sess.Status, sess.Metadata,
	).Scan(&sess.ID, &sess.StartedAt, &sess.CreatedAt)
}

func (s *SessionStore) GetByID(ctx context.Context, id, tenantID uuid.UUID) (*domain.Session, error) {
	return scanSession(s.db.QueryRow(ctx,
		`SELECT `+sessionCols+` FROM sessions WHERE id = $1 AND tenant_id = $2`,
		id, tenantID))
}

func (s *SessionStore) FindByExternalID(ctx context.Context, tenantID uuid.UUID, externalID string) (*domain.Session, error) {
	return scanSession(s.db.QueryRow(ctx,
		`SELECT `+sessionCols+` FROM sessions WHERE tenant_id = $1 AND external_id = $2
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, externalID))
}

func (s *SessionStore) End(ctx context.Context, id, tenantID uuid.UUID, expiresAt time.Time) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE sessions SET status = 'ended', ended_at = NOW(), expires_at = $3
		 WHERE id = $1 AND tenant_id = $2 AND status = 'active'`,
		id, tenantID, expiresAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SessionStore) ListExpired(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(ctx,
		`SELECT id FROM sessions
		 WHERE status <> 'expired' AND expires_at IS NOT NULL AND expires_at < NOW()
		 LIMIT $1`,
		limit,
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

func (s *SessionStore) MarkExpired(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE sessions SET status = 'expired' WHERE id = $1`, id)
	return err
}
