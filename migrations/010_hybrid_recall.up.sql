-- 010_hybrid_recall.up.sql
-- Add BM25 full-text search support for hybrid recall mode.
-- Enables keyword-based retrieval to complement vector similarity,
-- combined via Reciprocal Rank Fusion (RRF) for best-of-both results.
--
-- Use cases:
--   - Named entity queries: "how many times did I meet John?"
--   - Exact product/brand names: "Kansas City Masterpiece BBQ sauce"
--   - Date/number queries where exact terms matter more than semantics

ALTER TABLE memories ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

-- GIN index for fast full-text search (O(log N) lookup)
CREATE INDEX idx_memories_tsv ON memories USING GIN(content_tsv)
    WHERE is_archived = FALSE;
