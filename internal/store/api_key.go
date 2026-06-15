package store

import (
	"context"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type APIKeyStore struct {
	db *pgxpool.Pool
}

func NewAPIKeyStore(db *pgxpool.Pool) *APIKeyStore {
	return &APIKeyStore{db: db}
}

func (s *APIKeyStore) Create(ctx context.Context, k *domain.APIKey) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO api_keys (tenant_id, name, key_hash, key_prefix, scopes, expires_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		k.TenantID, k.Name, k.KeyHash, k.KeyPrefix, k.Scopes, k.ExpiresAt, k.CreatedBy,
	).Scan(&k.ID, &k.CreatedAt)
}

// GetAuthByHash looks up a key and its owning tenant in a single query.
// Returns ErrNotFound when the key is missing, revoked, or expired.
func (s *APIKeyStore) GetAuthByHash(ctx context.Context, hash string) (*domain.APIKeyAuth, error) {
	auth := &domain.APIKeyAuth{
		Tenant: &domain.Tenant{},
	}
	err := s.db.QueryRow(ctx,
		`SELECT ak.id, ak.scopes, t.id, t.name, t.created_at, t.updated_at
		 FROM api_keys ak
		 JOIN tenants t ON t.id = ak.tenant_id
		 WHERE ak.key_hash = $1
		   AND ak.revoked_at IS NULL
		   AND (ak.expires_at IS NULL OR ak.expires_at > NOW())`,
		hash,
	).Scan(
		&auth.KeyID, &auth.Scopes,
		&auth.Tenant.ID, &auth.Tenant.Name, &auth.Tenant.CreatedAt, &auth.Tenant.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return auth, nil
}

// ListByTenantID returns all non-revoked keys for a tenant. Key hashes are never returned.
func (s *APIKeyStore) ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.APIKey, error) {
	rows, err := s.db.Query(ctx,
		`SELECT ak.id, ak.tenant_id, ak.name, ak.key_prefix, ak.scopes, ak.last_used_at, ak.expires_at, ak.revoked_at, ak.created_at, ak.created_by, u.email
		 FROM api_keys ak
		 LEFT JOIN users u ON u.id = ak.created_by
		 WHERE ak.tenant_id = $1 AND ak.revoked_at IS NULL
		 ORDER BY ak.created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.Scopes, &k.LastUsedAt, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt, &k.CreatedBy, &k.CreatedByEmail); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *APIKeyStore) Revoke(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`,
		id, tenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *APIKeyStore) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}
