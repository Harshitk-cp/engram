DROP INDEX IF EXISTS idx_memories_is_archived;
ALTER TABLE memories DROP COLUMN IF EXISTS is_archived;
ALTER TABLE memories DROP COLUMN IF EXISTS archived_at;

ALTER TABLE episodes DROP COLUMN IF EXISTS is_archived;
ALTER TABLE episodes DROP COLUMN IF EXISTS archived_at;

ALTER TABLE procedures DROP COLUMN IF EXISTS is_archived;
ALTER TABLE procedures DROP COLUMN IF EXISTS archived_at;
