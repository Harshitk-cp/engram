-- Remove embedding column from entities

DROP INDEX IF EXISTS idx_entities_embedding;
ALTER TABLE entities DROP COLUMN IF EXISTS embedding;
