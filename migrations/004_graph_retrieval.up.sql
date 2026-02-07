-- Memory graph for hybrid retrieval
-- Enhanced relationship tracking between semantic memories

CREATE TABLE memory_graph (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,

    -- Relationship typing
    relation_type TEXT NOT NULL CHECK (relation_type IN (
        'entity_link',   -- Shared entity reference
        'causal',        -- Cause-effect relationship
        'temporal',      -- Temporal proximity/sequence
        'thematic',      -- Shared theme/topic (from semantic similarity)
        'contradicts',   -- Contradictory information
        'supports',      -- Supporting/reinforcing information
        'derived_from',  -- One memory derived from another
        'supersedes'     -- One memory replaces another
    )),

    -- Relationship strength (for weighted traversal)
    strength REAL NOT NULL DEFAULT 0.5 CHECK (strength >= 0 AND strength <= 1),

    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_traversed_at TIMESTAMPTZ,
    traversal_count INT DEFAULT 0,

    UNIQUE(source_id, target_id, relation_type)
);

CREATE INDEX idx_memory_graph_source ON memory_graph(source_id);
CREATE INDEX idx_memory_graph_target ON memory_graph(target_id);
CREATE INDEX idx_memory_graph_type ON memory_graph(relation_type);
CREATE INDEX idx_memory_graph_strength ON memory_graph(strength DESC);

-- Entities extracted from memories for graph-based retrieval
CREATE TABLE entities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    entity_type TEXT NOT NULL CHECK (entity_type IN (
        'person',
        'organization',
        'tool',
        'concept',
        'location',
        'event',
        'product',
        'other'
    )),
    aliases TEXT[] DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(agent_id, name, entity_type)
);

CREATE INDEX idx_entities_agent_id ON entities(agent_id);
CREATE INDEX idx_entities_name ON entities(name);
CREATE INDEX idx_entities_type ON entities(entity_type);

-- Entity-to-memory links
CREATE TABLE entity_mentions (
    entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    mention_type TEXT NOT NULL CHECK (mention_type IN ('subject', 'object', 'context')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (entity_id, memory_id)
);

CREATE INDEX idx_entity_mentions_entity ON entity_mentions(entity_id);
CREATE INDEX idx_entity_mentions_memory ON entity_mentions(memory_id);
