-- Remove provenance column from memories table
DROP INDEX IF EXISTS idx_memories_provenance;
ALTER TABLE memories DROP COLUMN IF EXISTS provenance;
