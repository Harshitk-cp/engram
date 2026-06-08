-- 012_sessions.down.sql
BEGIN;

ALTER TABLE memories DROP CONSTRAINT IF EXISTS chk_memory_binding_ids;

DROP INDEX IF EXISTS idx_memories_session;
ALTER TABLE memories DROP COLUMN IF EXISTS session_id;

-- Restore the Phase 1 constraint (no session branch).
ALTER TABLE memories ADD CONSTRAINT chk_memory_binding_anchor CHECK (
    (binding IN ('canon', 'private') AND anchor_id IS NULL) OR
    (binding = 'anchored' AND anchor_id IS NOT NULL)
);

DROP TABLE IF EXISTS sessions;

COMMIT;
