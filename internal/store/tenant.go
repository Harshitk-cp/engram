package store

import (
	"context"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TenantStore struct {
	db *pgxpool.Pool
}

func NewTenantStore(db *pgxpool.Pool) *TenantStore {
	return &TenantStore{db: db}
}

func (s *TenantStore) Create(ctx context.Context, t *domain.Tenant) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO tenants (name, api_key_hash) VALUES ($1, $2)
		 RETURNING id, created_at, updated_at`,
		t.Name, t.APIKeyHash,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (s *TenantStore) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.Tenant, error) {
	t := &domain.Tenant{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, api_key_hash, created_at, updated_at
		 FROM tenants WHERE api_key_hash = $1`,
		apiKeyHash,
	).Scan(&t.ID, &t.Name, &t.APIKeyHash, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}
