-- Remove needs_review flag
DROP INDEX IF EXISTS idx_memories_needs_review;
ALTER TABLE memories DROP COLUMN IF EXISTS needs_review;

-- Drop learning_stats
DROP TABLE IF EXISTS learning_stats;

-- Drop episode_memory_usage
DROP TABLE IF EXISTS episode_memory_usage;

-- Drop mutation_log
DROP TABLE IF EXISTS mutation_log;

-- Note: Cannot remove enum values in PostgreSQL without recreating the type
-- The 'contradicted' and 'outdated' values will remain but be unused
