-- 023_firewall_quarantine_schema.up.sql
-- Provenance Firewall, part 2: columns, constraint and index for quarantine.
BEGIN;

-- Why a trace was quarantined and when (shown in the review queue; null for
-- normal traces).
ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS quarantine_reason TEXT NULL,
    ADD COLUMN IF NOT EXISTS quarantined_at    TIMESTAMPTZ NULL;

-- Relax the binding/IDs consistency constraint: a quarantined trace preserves
-- whatever anchor_id/session_id it would have had, so on release we can recompute
-- its real binding. Other branches are unchanged.
ALTER TABLE memories DROP CONSTRAINT IF EXISTS chk_memory_binding_ids;
ALTER TABLE memories ADD CONSTRAINT chk_memory_binding_ids CHECK (
    (binding IN ('canon', 'private') AND anchor_id IS NULL AND session_id IS NULL) OR
    (binding = 'anchored'   AND anchor_id IS NOT NULL AND session_id IS NULL) OR
    (binding = 'session'    AND session_id IS NOT NULL) OR
    (binding = 'quarantine')
);

-- Fast listing of the per-tenant/agent quarantine queue, newest first.
CREATE INDEX IF NOT EXISTS idx_memories_quarantine
    ON memories (tenant_id, agent_id, created_at DESC)
    WHERE binding = 'quarantine' AND is_archived = FALSE;

-- Firewall decisions are tamper-evident: record quarantine / release / reject in
-- the audit chain alongside the existing mutation types.
ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS mutation_log_mutation_type_check;
ALTER TABLE mutation_log ADD CONSTRAINT mutation_log_mutation_type_check
    CHECK (mutation_type IN ('feedback', 'outcome', 'decay', 'reinforcement', 'contradiction',
                             'deletion', 'archive', 'admin_override', 'redaction',
                             'quarantine', 'quarantine_release', 'quarantine_reject'));

COMMIT;
