-- 013_mutation_log_audit.down.sql
-- Reverses 013. LOSSY: rows that reference now-deleted memories/agents (the very
-- audit history this migration preserves) must be removed to restore the CASCADE
-- FKs and NOT NULL constraints.

BEGIN;

DROP INDEX IF EXISTS idx_mutation_log_tenant;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS content_snapshot;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS content_hash;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS binding;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS anchor_id;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS tenant_id;

DELETE FROM mutation_log
WHERE memory_id IS NULL OR agent_id IS NULL
   OR memory_id NOT IN (SELECT id FROM memories)
   OR agent_id NOT IN (SELECT id FROM agents);

ALTER TABLE mutation_log ALTER COLUMN memory_id SET NOT NULL;
ALTER TABLE mutation_log ALTER COLUMN agent_id SET NOT NULL;

ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_memory_id_fkey
    FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

COMMIT;
