-- 020_tenant_settings.up.sql
-- Per-tenant cognitive-engine tuning (decay rate, confidence deltas, etc.).
-- Stored as JSONB so new tunables can be added without a schema change; an
-- absent row means the tenant uses the engine's built-in defaults.
BEGIN;

CREATE TABLE IF NOT EXISTS tenant_settings (
    tenant_id  UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    settings   JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
