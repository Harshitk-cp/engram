-- Add embedding column to entities for fuzzy matching / co-reference resolution

ALTER TABLE entities ADD COLUMN IF NOT EXISTS embedding vector(1536);

CREATE INDEX IF NOT EXISTS idx_entities_embedding ON entities
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
