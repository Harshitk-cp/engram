package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantSettingsStore persists per-tenant engine settings as JSONB.
type TenantSettingsStore struct {
	db *pgxpool.Pool
}

func NewTenantSettingsStore(db *pgxpool.Pool) *TenantSettingsStore {
	return &TenantSettingsStore{db: db}
}

// Get returns the tenant's settings, defaults-merged and sanitized. Unmarshaling
// onto a defaults-initialized struct means any field absent from the stored JSON
// keeps its default (forward-compatible as new tunables are added).
func (s *TenantSettingsStore) Get(ctx context.Context, tenantID uuid.UUID) (domain.EngineSettings, error) {
	es := domain.DefaultEngineSettings()
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT settings FROM tenant_settings WHERE tenant_id = $1`, tenantID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return es, nil
	}
	if err != nil {
		return es, err
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &es) // best-effort overlay; bad JSON falls back to defaults
	}
	return es.Sanitize(), nil
}

func (s *TenantSettingsStore) Upsert(ctx context.Context, tenantID uuid.UUID, settings domain.EngineSettings) error {
	data, err := json.Marshal(settings.Sanitize())
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO tenant_settings (tenant_id, settings, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (tenant_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()`,
		tenantID, data,
	)
	return err
}
