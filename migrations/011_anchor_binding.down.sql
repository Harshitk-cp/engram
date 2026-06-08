-- 011_anchor_binding.down.sql
-- Reverses 011. Re-adding NOT NULL on entities.agent_id would fail if any
-- tenant-level anchors (agent_id IS NULL) were created, so those are removed first.

BEGIN;

DROP INDEX IF EXISTS idx_memories_anchor;
DROP INDEX IF EXISTS idx_memories_tenant_anchor;

ALTER TABLE memories DROP CONSTRAINT IF EXISTS chk_memory_binding_anchor;
ALTER TABLE memories DROP COLUMN IF EXISTS anchor_id;
ALTER TABLE memories DROP COLUMN IF EXISTS binding;

DROP TYPE IF EXISTS memory_binding;

DROP INDEX IF EXISTS uq_entities_tenant_anchor;
DROP INDEX IF EXISTS idx_entities_tenant_anchor;

-- Remove anchors that have no owning agent before restoring NOT NULL.
DELETE FROM entities WHERE is_anchor = TRUE AND agent_id IS NULL;
ALTER TABLE entities ALTER COLUMN agent_id SET NOT NULL;

ALTER TABLE entities DROP COLUMN IF EXISTS external_id;
ALTER TABLE entities DROP COLUMN IF EXISTS is_anchor;
ALTER TABLE entities DROP COLUMN IF EXISTS tenant_id;

COMMIT;
