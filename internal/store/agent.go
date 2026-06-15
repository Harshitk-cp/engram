package store

import (
	"context"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentStore struct {
	db *pgxpool.Pool
}

func NewAgentStore(db *pgxpool.Pool) *AgentStore {
	return &AgentStore{db: db}
}

func (s *AgentStore) Create(ctx context.Context, a *domain.Agent) error {
	err := s.db.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, external_id, name, metadata)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, updated_at`,
		a.TenantID, a.ExternalID, a.Name, a.Metadata,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (s *AgentStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Agent, error) {
	a := &domain.Agent{}
	err := s.db.QueryRow(ctx,
		`SELECT id, tenant_id, external_id, name, metadata, created_at, updated_at
		 FROM agents WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(&a.ID, &a.TenantID, &a.ExternalID, &a.Name, &a.Metadata, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

func (s *AgentStore) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE tenant_id = $1`, tenantID).Scan(&n)
	return n, err
}

func (s *AgentStore) ListByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]domain.Agent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, tenant_id, external_id, name, metadata, created_at, updated_at
		 FROM agents WHERE tenant_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		var a domain.Agent
		if err := rows.Scan(&a.ID, &a.TenantID, &a.ExternalID, &a.Name, &a.Metadata, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *AgentStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM agents WHERE id = $1 AND tenant_id = $2`,
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

func (s *AgentStore) GetByExternalID(ctx context.Context, externalID string, tenantID uuid.UUID) (*domain.Agent, error) {
	a := &domain.Agent{}
	err := s.db.QueryRow(ctx,
		`SELECT id, tenant_id, external_id, name, metadata, created_at, updated_at
		 FROM agents WHERE external_id = $1 AND tenant_id = $2`,
		externalID, tenantID,
	).Scan(&a.ID, &a.TenantID, &a.ExternalID, &a.Name, &a.Metadata, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}
