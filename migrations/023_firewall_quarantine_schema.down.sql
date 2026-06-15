-- 023_firewall_quarantine_schema.down.sql
BEGIN;

-- Restore the pre-firewall mutation_type check.
ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_mutation_type_check;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_mutation_type_check
    CHECK (mutation_type IN ('feedback', 'outcome', 'decay', 'reinforcement', 'contradiction',
                             'deletion', 'archive', 'admin_override', 'redaction'));

DROP INDEX IF EXISTS idx_memories_quarantine;

-- Restore the pre-firewall constraint (drops the quarantine branch). Any rows
-- still bound 'quarantine' must be cleared first to satisfy it.
UPDATE memories SET binding = 'private', anchor_id = NULL, session_id = NULL
    WHERE binding = 'quarantine';

ALTER TABLE memories DROP CONSTRAINT IF EXISTS chk_memory_binding_ids;
ALTER TABLE memories ADD CONSTRAINT chk_memory_binding_ids CHECK (
    (binding IN ('canon', 'private') AND anchor_id IS NULL AND session_id IS NULL) OR
    (binding = 'anchored' AND anchor_id IS NOT NULL AND session_id IS NULL) OR
    (binding = 'session'  AND session_id IS NOT NULL)
);

ALTER TABLE memories
    DROP COLUMN IF EXISTS quarantine_reason,
    DROP COLUMN IF EXISTS quarantined_at;

COMMIT;
