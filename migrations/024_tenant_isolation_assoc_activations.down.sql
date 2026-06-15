-- 024_tenant_isolation_assoc_activations.down.sql
BEGIN;

DROP INDEX IF EXISTS idx_memory_assoc_tenant;
ALTER TABLE memory_associations DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_wm_activations_tenant;
ALTER TABLE working_memory_activations DROP COLUMN IF EXISTS tenant_id;

COMMIT;
