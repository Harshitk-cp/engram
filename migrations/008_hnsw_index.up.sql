-- Switch vector indexes from IVFFlat to HNSW.
--
-- IVFFlat requires a minimum number of rows to work correctly
-- (lists=100 means probes=1 default scans only 1 cluster, missing everything
-- in collections < ~1000 rows). HNSW has no minimum data requirement,
-- works correctly at all scales, and doesn't need probes tuning.

DROP INDEX IF EXISTS idx_memories_embedding;
DROP INDEX IF EXISTS idx_episodes_embedding;
DROP INDEX IF EXISTS idx_procedures_trigger_embedding;
DROP INDEX IF EXISTS idx_schemas_embedding;
DROP INDEX IF EXISTS idx_entity_embedding;

CREATE INDEX idx_memories_embedding           ON memories    USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_episodes_embedding           ON episodes    USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_procedures_trigger_embedding ON procedures  USING hnsw (trigger_embedding vector_cosine_ops);
CREATE INDEX idx_schemas_embedding            ON schemas     USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_entity_embedding             ON entities    USING hnsw (embedding vector_cosine_ops);
