DROP INDEX IF EXISTS idx_memories_embedding;
DROP INDEX IF EXISTS idx_episodes_embedding;
DROP INDEX IF EXISTS idx_procedures_trigger_embedding;
DROP INDEX IF EXISTS idx_schemas_embedding;
DROP INDEX IF EXISTS idx_entity_embedding;

CREATE INDEX idx_memories_embedding           ON memories    USING ivfflat (embedding vector_cosine_ops)          WITH (lists = 100);
CREATE INDEX idx_episodes_embedding           ON episodes    USING ivfflat (embedding vector_cosine_ops)          WITH (lists = 100);
CREATE INDEX idx_procedures_trigger_embedding ON procedures  USING hnsw    (trigger_embedding vector_cosine_ops);
CREATE INDEX idx_schemas_embedding            ON schemas     USING ivfflat (embedding vector_cosine_ops)          WITH (lists = 100);
CREATE INDEX idx_entity_embedding             ON entities    USING ivfflat (embedding vector_cosine_ops)          WITH (lists = 100);
