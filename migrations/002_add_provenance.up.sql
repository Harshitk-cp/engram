-- Add provenance column to memories table
-- Provenance tracks the source/origin of a memory and affects initial confidence

ALTER TABLE memories ADD COLUMN IF NOT EXISTS
    provenance TEXT NOT NULL DEFAULT 'agent'
    CHECK (provenance IN ('user', 'agent', 'tool', 'derived', 'inferred'));

-- Create index for filtering by provenance
CREATE INDEX IF NOT EXISTS idx_memories_provenance ON memories(provenance);
