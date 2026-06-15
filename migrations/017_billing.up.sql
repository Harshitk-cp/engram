-- 017_billing.up.sql
-- Managed-cloud monetization: per-org plan + monthly usage counters.
-- The billing entity is the org (tenants). Quota enforcement is only active when
-- the server is configured with Stripe (managed cloud); self-hosted/OSS leaves
-- STRIPE_SECRET_KEY unset and runs unmetered, so these columns stay inert there.

BEGIN;

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS plan                   TEXT        NOT NULL DEFAULT 'free';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS subscription_status    TEXT        NOT NULL DEFAULT 'active';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS stripe_customer_id     TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS stripe_subscription_id TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS plan_period_start      TIMESTAMPTZ;

-- One Stripe customer maps to at most one org.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_stripe_customer
    ON tenants(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;

-- Monthly usage rollup. period_month is the first day of the UTC month.
-- Incremented with an atomic upsert on the write path; read back for quota checks
-- and the console usage display.
CREATE TABLE IF NOT EXISTS usage_counters (
    tenant_id        UUID   NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period_month     DATE   NOT NULL,
    memories_written BIGINT NOT NULL DEFAULT 0,
    recalls          BIGINT NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, period_month)
);

COMMIT;
