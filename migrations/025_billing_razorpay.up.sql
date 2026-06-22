-- 025_billing_razorpay.up.sql
-- Migrate managed-cloud billing from Stripe to Razorpay. The billing entity is
-- still the org (tenants); only the provider-specific reference columns change.
-- RENAME COLUMN preserves the existing partial unique index's column reference, so
-- only the index name needs renaming for clarity.

BEGIN;

ALTER TABLE tenants RENAME COLUMN stripe_customer_id     TO razorpay_customer_id;
ALTER TABLE tenants RENAME COLUMN stripe_subscription_id TO razorpay_subscription_id;

ALTER INDEX idx_tenants_stripe_customer RENAME TO idx_tenants_razorpay_customer;

COMMIT;
