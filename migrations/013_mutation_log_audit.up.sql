-- 013_mutation_log_audit.up.sql
-- Make mutation_log a durable, self-contained audit trail: rows must survive the
-- hard-deletion of the memory/agent they describe (otherwise deleting a memory
-- erases the very record of the deletion event).

BEGIN;

-- Drop the cascading FKs and keep memory_id/agent_id as plain (non-FK) references.
-- This is the standard audit-log pattern: the log outlives the rows it references,
-- and a deletion event's own audit row is not cascade-deleted with the memory.
ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_memory_id_fkey;
ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_agent_id_fkey;
ALTER TABLE mutation_log ALTER COLUMN memory_id DROP NOT NULL;
ALTER TABLE mutation_log ALTER COLUMN agent_id DROP NOT NULL;

-- Denormalize identity + a content snapshot so a row stays meaningful after its
-- memory is gone. content_snapshot is populated only for deletion/redaction events.
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS anchor_id UUID;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS binding TEXT;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS content_hash TEXT;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS content_snapshot TEXT;

UPDATE mutation_log ml
SET tenant_id = m.tenant_id,
    anchor_id = m.anchor_id,
    binding   = m.binding::text
FROM memories m
WHERE ml.memory_id = m.id AND ml.tenant_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_mutation_log_tenant ON mutation_log(tenant_id);

COMMIT;
