-- 014_admin_overrides.down.sql
-- Reverses 014. Will FAIL if any row already uses a new mutation_type/source_type
-- value; purge such rows first if a rollback is truly intended.

BEGIN;

ALTER TABLE mutation_log DROP COLUMN IF EXISTS actor_id;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS actor_type;

ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_source_type_check;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_source_type_check
    CHECK (source_type IN ('explicit', 'implicit', 'system'));

ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_mutation_type_check;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_mutation_type_check
    CHECK (mutation_type IN ('feedback', 'outcome', 'decay', 'reinforcement', 'contradiction'));

COMMIT;
