-- 017_billing.down.sql

BEGIN;

DROP TABLE IF EXISTS usage_counters;

DROP INDEX IF EXISTS idx_tenants_stripe_customer;

ALTER TABLE tenants DROP COLUMN IF EXISTS plan_period_start;
ALTER TABLE tenants DROP COLUMN IF EXISTS stripe_subscription_id;
ALTER TABLE tenants DROP COLUMN IF EXISTS stripe_customer_id;
ALTER TABLE tenants DROP COLUMN IF EXISTS subscription_status;
ALTER TABLE tenants DROP COLUMN IF EXISTS plan;

COMMIT;
