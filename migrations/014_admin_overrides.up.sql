-- 014_admin_overrides.up.sql
-- Extend mutation_log to record deletion/archive events and audited admin
-- operations, with actor attribution (data plane actor = api key).

BEGIN;

ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_mutation_type_check;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_mutation_type_check
    CHECK (mutation_type IN ('feedback', 'outcome', 'decay', 'reinforcement', 'contradiction',
                             'deletion', 'archive', 'admin_override', 'redaction'));

ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_source_type_check;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_source_type_check
    CHECK (source_type IN ('explicit', 'implicit', 'system', 'admin'));

-- Actor attribution. No FK to api_keys: a revoked/deleted key must not erase
-- the audit record of what it did.
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS actor_type TEXT;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS actor_id UUID;

COMMIT;
