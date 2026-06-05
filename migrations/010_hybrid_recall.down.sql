DROP INDEX IF EXISTS idx_memories_tsv;
ALTER TABLE memories DROP COLUMN IF EXISTS content_tsv;
