-- Revert 018: allow NULL tenant_id again. Rows deleted by the up migration were
-- unreachable orphans and are not restored.
ALTER TABLE entities ALTER COLUMN tenant_id DROP NOT NULL;
