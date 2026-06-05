DROP INDEX IF EXISTS idx_memories_event_date_tenant;
DROP INDEX IF EXISTS idx_memories_event_date;
ALTER TABLE memories DROP COLUMN IF EXISTS event_date;
