package store

import (
	"context"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
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
		`INSERT INTO tenants (name) VALUES ($1) RETURNING id, created_at, updated_at`,
		t.Name,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (s *TenantStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	t := &domain.Tenant{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM tenants WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}
