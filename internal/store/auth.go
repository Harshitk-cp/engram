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

// ---- Users ----

type UserStore struct{ db *pgxpool.Pool }

func NewUserStore(db *pgxpool.Pool) *UserStore { return &UserStore{db: db} }

func (s *UserStore) Create(ctx context.Context, u *domain.User) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, avatar_url)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at, updated_at`,
		u.Email, u.PasswordHash, u.Name, u.AvatarURL,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.scanUser(s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, avatar_url, created_at, updated_at
		 FROM users WHERE lower(email) = lower($1)`, email))
}

func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.scanUser(s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, avatar_url, created_at, updated_at
		 FROM users WHERE id = $1`, id))
}

func (s *UserStore) scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// ---- OAuth accounts ----

type OAuthAccountStore struct{ db *pgxpool.Pool }

func NewOAuthAccountStore(db *pgxpool.Pool) *OAuthAccountStore { return &OAuthAccountStore{db: db} }

func (s *OAuthAccountStore) Create(ctx context.Context, a *domain.OAuthAccount) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO oauth_accounts (user_id, provider, provider_user_id)
		 VALUES ($1, $2, $3) RETURNING id, created_at`,
		a.UserID, a.Provider, a.ProviderUserID,
	).Scan(&a.ID, &a.CreatedAt)
}

func (s *OAuthAccountStore) GetByProvider(ctx context.Context, provider, providerUserID string) (*domain.OAuthAccount, error) {
	var a domain.OAuthAccount
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, provider, provider_user_id, created_at
		 FROM oauth_accounts WHERE provider = $1 AND provider_user_id = $2`,
		provider, providerUserID,
	).Scan(&a.ID, &a.UserID, &a.Provider, &a.ProviderUserID, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// ---- Memberships ----

type MembershipStore struct{ db *pgxpool.Pool }

func NewMembershipStore(db *pgxpool.Pool) *MembershipStore { return &MembershipStore{db: db} }

func (s *MembershipStore) Create(ctx context.Context, m *domain.Membership) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = EXCLUDED.role
		 RETURNING id, created_at`,
		m.UserID, m.TenantID, m.Role,
	).Scan(&m.ID, &m.CreatedAt)
}

func (s *MembershipStore) Get(ctx context.Context, userID, tenantID uuid.UUID) (*domain.Membership, error) {
	var m domain.Membership
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, tenant_id, role, created_at
		 FROM memberships WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID,
	).Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *MembershipStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.MembershipWithTenant, error) {
	rows, err := s.db.Query(ctx,
		`SELECT m.id, m.user_id, m.tenant_id, m.role, m.created_at, t.name
		 FROM memberships m JOIN tenants t ON t.id = m.tenant_id
		 WHERE m.user_id = $1 ORDER BY m.created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MembershipWithTenant
	for rows.Next() {
		var m domain.MembershipWithTenant
		if err := rows.Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.CreatedAt, &m.TenantName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- Sessions ----

type ConsoleSessionStore struct{ db *pgxpool.Pool }

func NewConsoleSessionStore(db *pgxpool.Pool) *ConsoleSessionStore {
	return &ConsoleSessionStore{db: db}
}

func (s *ConsoleSessionStore) Create(ctx context.Context, sess *domain.ConsoleSession) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO console_sessions (token_hash, user_id, active_tenant_id, expires_at)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		sess.TokenHash, sess.UserID, sess.ActiveTenantID, sess.ExpiresAt,
	).Scan(&sess.ID, &sess.CreatedAt)
}

func (s *ConsoleSessionStore) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ConsoleSession, error) {
	var sess domain.ConsoleSession
	err := s.db.QueryRow(ctx,
		`SELECT id, token_hash, user_id, active_tenant_id, expires_at, created_at
		 FROM console_sessions WHERE token_hash = $1`, tokenHash,
	).Scan(&sess.ID, &sess.TokenHash, &sess.UserID, &sess.ActiveTenantID, &sess.ExpiresAt, &sess.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sess, nil
}

func (s *ConsoleSessionStore) UpdateActiveTenant(ctx context.Context, sessionID, tenantID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE console_sessions SET active_tenant_id = $2 WHERE id = $1`, sessionID, tenantID)
	return err
}

func (s *ConsoleSessionStore) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM console_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *ConsoleSessionStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM console_sessions WHERE expires_at < $1`, time.Now())
	return err
}

// APIKeyStore extension: list keys including created_by attribution is handled in
// the existing api_key.go ListByTenantID query; created_by is additive.
