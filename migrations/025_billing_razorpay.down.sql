-- 025_billing_razorpay.down.sql
-- Revert the Razorpay column/index renames back to the Stripe names.

BEGIN;

ALTER INDEX idx_tenants_razorpay_customer RENAME TO idx_tenants_stripe_customer;

ALTER TABLE tenants RENAME COLUMN razorpay_customer_id     TO stripe_customer_id;
ALTER TABLE tenants RENAME COLUMN razorpay_subscription_id TO stripe_subscription_id;

COMMIT;
