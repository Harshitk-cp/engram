CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";

-- tenants
CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        TEXT NOT NULL,
    api_key_hash TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenants_api_key_hash ON tenants(api_key_hash);

-- agents
CREATE TABLE agents (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, external_id)
);

CREATE INDEX idx_agents_tenant_id ON agents(tenant_id);

-- memory type enum
CREATE TYPE memory_type AS ENUM ('preference', 'fact', 'decision', 'constraint');

-- memories (beliefs)
CREATE TABLE memories (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id            UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type                memory_type NOT NULL,
    content             TEXT NOT NULL,
    embedding           vector(1536),
    embedding_provider  TEXT NOT NULL DEFAULT 'openai',
    embedding_model     TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    source              TEXT DEFAULT '',
    confidence          REAL NOT NULL DEFAULT 1.0 CHECK (confidence >= 0 AND confidence <= 1),
    metadata            JSONB DEFAULT '{}',
    expires_at          TIMESTAMPTZ,
    last_verified_at    TIMESTAMPTZ,
    reinforcement_count INTEGER NOT NULL DEFAULT 1,
    decay_rate          REAL NOT NULL DEFAULT 0.05,
    last_accessed_at    TIMESTAMPTZ DEFAULT NOW(),
    access_count        INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_memories_agent_id ON memories(agent_id);
CREATE INDEX idx_memories_tenant_id ON memories(tenant_id);
CREATE INDEX idx_memories_type ON memories(type);
CREATE INDEX idx_memories_expires_at ON memories(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_memories_embedding ON memories USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_memories_agent_type_confidence ON memories(agent_id, type, confidence);
CREATE INDEX idx_memories_last_accessed ON memories(last_accessed_at);
CREATE INDEX idx_memories_confidence_decay ON memories(agent_id, confidence, last_accessed_at);

-- belief contradictions
CREATE TABLE belief_contradictions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    belief_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    contradicted_by_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(belief_id, contradicted_by_id)
);

CREATE INDEX idx_belief_contradictions_belief_id ON belief_contradictions(belief_id);
CREATE INDEX idx_belief_contradictions_contradicted_by ON belief_contradictions(contradicted_by_id);

-- memory policies
CREATE TABLE memory_policies (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    memory_type     memory_type NOT NULL,
    max_memories    INT NOT NULL DEFAULT 100,
    retention_days  INT,
    priority_weight REAL NOT NULL DEFAULT 1.0,
    auto_summarize  BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, memory_type)
);

CREATE INDEX idx_memory_policies_agent_id ON memory_policies(agent_id);

-- feedback type enum
CREATE TYPE feedback_type AS ENUM ('used', 'ignored', 'helpful', 'unhelpful');

-- feedback signals
CREATE TABLE feedback_signals (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    memory_id   UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    signal_type feedback_type NOT NULL,
    context     JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_feedback_signals_memory_id ON feedback_signals(memory_id);
CREATE INDEX idx_feedback_signals_agent_id ON feedback_signals(agent_id);

-- Episodic memory: Rich experience storage
-- Stores raw experiences with full context (emotion, entities, causal links)
-- as opposed to extracted semantic memories (beliefs/facts)

CREATE TABLE episodes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Raw experience
    raw_content TEXT NOT NULL,
    conversation_id UUID,
    message_sequence INTEGER,

    -- Temporal context
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_seconds INTEGER,
    time_of_day TEXT, -- 'morning', 'afternoon', 'evening', 'night'
    day_of_week TEXT, -- 'monday', 'tuesday', etc.

    -- Emotional/importance markers
    emotional_valence REAL CHECK (emotional_valence >= -1 AND emotional_valence <= 1),
    emotional_intensity REAL CHECK (emotional_intensity >= 0 AND emotional_intensity <= 1),
    importance_score REAL NOT NULL DEFAULT 0.5 CHECK (importance_score >= 0 AND importance_score <= 1),

    -- Extracted structure (populated by LLM)
    entities JSONB DEFAULT '[]',
    causal_links JSONB DEFAULT '[]',
    topics JSONB DEFAULT '[]',

    -- Outcome tracking
    outcome TEXT CHECK (outcome IN ('success', 'failure', 'neutral', 'unknown')),
    outcome_description TEXT,
    outcome_valence REAL CHECK (outcome_valence >= -1 AND outcome_valence <= 1),

    -- Consolidation tracking
    consolidation_status TEXT NOT NULL DEFAULT 'raw' CHECK (consolidation_status IN ('raw', 'processed', 'abstracted', 'archived')),
    last_consolidated_at TIMESTAMPTZ,
    abstraction_count INTEGER DEFAULT 0,
    derived_semantic_ids UUID[] DEFAULT '{}',
    derived_procedural_ids UUID[] DEFAULT '{}',

    -- Memory strength (for forgetting curves)
    memory_strength REAL NOT NULL DEFAULT 1.0 CHECK (memory_strength >= 0 AND memory_strength <= 1),
    last_accessed_at TIMESTAMPTZ DEFAULT NOW(),
    access_count INTEGER DEFAULT 1,
    decay_rate REAL NOT NULL DEFAULT 0.1,

    -- Embedding for similarity
    embedding vector(1536),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_episodes_agent_id ON episodes(agent_id);
CREATE INDEX idx_episodes_tenant_id ON episodes(tenant_id);
CREATE INDEX idx_episodes_occurred_at ON episodes(occurred_at DESC);
CREATE INDEX idx_episodes_consolidation_status ON episodes(consolidation_status);
CREATE INDEX idx_episodes_memory_strength ON episodes(memory_strength);
CREATE INDEX idx_episodes_conversation_id ON episodes(conversation_id);
CREATE INDEX idx_episodes_embedding ON episodes USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Episode associations (links between related episodes)
CREATE TABLE episode_associations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    episode_a_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    episode_b_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    association_type TEXT NOT NULL CHECK (association_type IN ('temporal', 'causal', 'thematic', 'entity')),
    association_strength REAL NOT NULL DEFAULT 0.5 CHECK (association_strength >= 0 AND association_strength <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(episode_a_id, episode_b_id, association_type)
);

CREATE INDEX idx_episode_associations_a ON episode_associations(episode_a_id);
CREATE INDEX idx_episode_associations_b ON episode_associations(episode_b_id);

-- Procedural memory: Skills & patterns learned from successful episodes
-- Stores trigger-action patterns: "When X situation, do Y action"
CREATE TABLE procedures (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Trigger pattern (when to use this)
    trigger_pattern TEXT NOT NULL,
    trigger_keywords JSONB DEFAULT '[]',
    trigger_embedding vector(1536),

    -- Action/response pattern (what to do)
    action_template TEXT NOT NULL,
    action_type TEXT NOT NULL CHECK (action_type IN ('response_style', 'problem_solving', 'communication', 'workflow')),

    -- Effectiveness tracking
    use_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    success_rate REAL GENERATED ALWAYS AS (
        CASE WHEN use_count > 0 THEN success_count::REAL / use_count ELSE 0 END
    ) STORED,
    last_used_at TIMESTAMPTZ,

    -- Learning source
    derived_from_episodes UUID[] DEFAULT '{}',
    example_exchanges JSONB DEFAULT '[]',

    -- Confidence and decay
    confidence REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    memory_strength REAL NOT NULL DEFAULT 1.0 CHECK (memory_strength >= 0 AND memory_strength <= 1),
    last_verified_at TIMESTAMPTZ DEFAULT NOW(),

    -- Versioning (procedures can evolve)
    version INTEGER NOT NULL DEFAULT 1,
    previous_version_id UUID REFERENCES procedures(id),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_procedures_agent ON procedures(agent_id);
CREATE INDEX idx_procedures_tenant ON procedures(tenant_id);
CREATE INDEX idx_procedures_trigger_embedding ON procedures USING ivfflat (trigger_embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_procedures_success_rate ON procedures(success_rate DESC);
CREATE INDEX idx_procedures_action_type ON procedures(action_type);
CREATE INDEX idx_procedures_confidence ON procedures(confidence DESC);

-- Schema Memory: Mental models and user archetypes
-- Schemas represent higher-order patterns derived from clusters of semantic memories
-- Examples: "Night-owl power user", "Technical expert", "Impatient debugger"

CREATE TABLE schemas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Schema identification
    schema_type TEXT NOT NULL CHECK (schema_type IN ('user_archetype', 'situation_template', 'causal_model')),
    name TEXT NOT NULL,
    description TEXT,

    -- Schema attributes (the mental model)
    -- Example: {"communication_style": "direct", "technical_level": "expert", "patience_level": "low"}
    attributes JSONB NOT NULL DEFAULT '{}',

    -- Evidence tracking
    evidence_memories UUID[] DEFAULT '{}', -- Semantic memories supporting this schema
    evidence_episodes UUID[] DEFAULT '{}', -- Episodes supporting this schema
    evidence_count INTEGER NOT NULL DEFAULT 0,

    -- Confidence and validation
    confidence REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    last_validated_at TIMESTAMPTZ DEFAULT NOW(),
    contradiction_count INTEGER NOT NULL DEFAULT 0,

    -- Applicable contexts (when this schema should activate)
    -- Example: ["debugging", "late_night", "code_review"]
    applicable_contexts JSONB DEFAULT '[]',

    -- Embedding for similarity matching
    embedding vector(1536),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(agent_id, schema_type, name)
);

CREATE INDEX idx_schemas_agent ON schemas(agent_id);
CREATE INDEX idx_schemas_tenant ON schemas(tenant_id);
CREATE INDEX idx_schemas_type ON schemas(schema_type);
CREATE INDEX idx_schemas_confidence ON schemas(confidence DESC);
CREATE INDEX idx_schemas_embedding ON schemas USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Working Memory: Agent's mental workspace with limited capacity
-- Holds current goals, active context, and activated memories for the current task

CREATE TABLE working_memory_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Current state
    current_goal TEXT,
    active_context JSONB DEFAULT '[]', -- Recent messages
    reasoning_state JSONB DEFAULT '{}', -- Partial conclusions

    -- Capacity tracking
    max_slots INTEGER NOT NULL DEFAULT 7,

    -- Session lifecycle
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(agent_id) -- One active session per agent
);

CREATE INDEX idx_wm_sessions_agent ON working_memory_sessions(agent_id);
CREATE INDEX idx_wm_sessions_tenant ON working_memory_sessions(tenant_id);
CREATE INDEX idx_wm_sessions_last_activity ON working_memory_sessions(last_activity_at);

-- Activated memories within working memory
-- Tracks which memories (of any type) are currently "active" in the agent's attention

CREATE TABLE working_memory_activations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES working_memory_sessions(id) ON DELETE CASCADE,

    -- What's activated (can be any memory type)
    memory_type TEXT NOT NULL CHECK (memory_type IN ('episodic', 'semantic', 'procedural', 'schema')),
    memory_id UUID NOT NULL,

    -- Activation details
    activation_level REAL NOT NULL CHECK (activation_level >= 0 AND activation_level <= 1),
    activation_source TEXT NOT NULL CHECK (activation_source IN ('direct', 'spread', 'goal', 'temporal', 'recency', 'schema')),
    activation_cue TEXT, -- What triggered this activation

    -- Competition
    slot_position INTEGER CHECK (slot_position >= 1),

    activated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(session_id, memory_type, memory_id)
);

CREATE INDEX idx_wm_activations_session ON working_memory_activations(session_id);
CREATE INDEX idx_wm_activations_level ON working_memory_activations(activation_level DESC);
CREATE INDEX idx_wm_activations_type ON working_memory_activations(memory_type);

-- Schema activations (which schemas are currently active in a session)

CREATE TABLE schema_activations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES working_memory_sessions(id) ON DELETE CASCADE,
    schema_id UUID NOT NULL REFERENCES schemas(id) ON DELETE CASCADE,

    match_score REAL NOT NULL CHECK (match_score >= 0 AND match_score <= 1),
    activated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(session_id, schema_id)
);

CREATE INDEX idx_schema_activations_session ON schema_activations(session_id);
CREATE INDEX idx_schema_activations_schema ON schema_activations(schema_id);

-- Cross-memory associations for spreading activation
-- Links memories of different types to enable spreading activation

CREATE TABLE memory_associations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_memory_type TEXT NOT NULL CHECK (source_memory_type IN ('episodic', 'semantic', 'procedural', 'schema')),
    source_memory_id UUID NOT NULL,
    target_memory_type TEXT NOT NULL CHECK (target_memory_type IN ('episodic', 'semantic', 'procedural', 'schema')),
    target_memory_id UUID NOT NULL,
    association_type TEXT NOT NULL CHECK (association_type IN ('derived', 'thematic', 'causal', 'temporal', 'entity')),
    association_strength REAL NOT NULL DEFAULT 0.5 CHECK (association_strength >= 0 AND association_strength <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_memory_type, source_memory_id, target_memory_type, target_memory_id, association_type)
);

CREATE INDEX idx_memory_assoc_source ON memory_associations(source_memory_type, source_memory_id);
CREATE INDEX idx_memory_assoc_target ON memory_associations(target_memory_type, target_memory_id);
CREATE INDEX idx_memory_assoc_type ON memory_associations(association_type);
