-- Add new feedback types for the learning loop
ALTER TYPE feedback_type ADD VALUE IF NOT EXISTS 'contradicted';
ALTER TYPE feedback_type ADD VALUE IF NOT EXISTS 'outdated';

-- Mutation log: tracks all confidence/reinforcement changes for explainability
CREATE TABLE mutation_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    mutation_type TEXT NOT NULL CHECK (mutation_type IN ('feedback', 'outcome', 'decay', 'reinforcement', 'contradiction')),
    source_type TEXT NOT NULL CHECK (source_type IN ('explicit', 'implicit', 'system')),
    source_id UUID, -- feedback_id, episode_id, or NULL for system
    old_confidence REAL,
    new_confidence REAL,
    old_reinforcement_count INTEGER,
    new_reinforcement_count INTEGER,
    reason TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_mutation_log_memory_id ON mutation_log(memory_id);
CREATE INDEX idx_mutation_log_agent_id ON mutation_log(agent_id);
CREATE INDEX idx_mutation_log_created_at ON mutation_log(created_at DESC);

-- Memory usage tracking: which memories were used per episode
CREATE TABLE episode_memory_usage (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    usage_type TEXT NOT NULL CHECK (usage_type IN ('retrieved', 'used_in_response', 'influenced_decision')),
    relevance_score REAL CHECK (relevance_score >= 0 AND relevance_score <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(episode_id, memory_id, usage_type)
);

CREATE INDEX idx_episode_memory_usage_episode ON episode_memory_usage(episode_id);
CREATE INDEX idx_episode_memory_usage_memory ON episode_memory_usage(memory_id);

-- Learning analytics aggregates (materialized for performance)
CREATE TABLE learning_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,

    -- Feedback counts
    helpful_count INTEGER DEFAULT 0,
    unhelpful_count INTEGER DEFAULT 0,
    ignored_count INTEGER DEFAULT 0,
    contradicted_count INTEGER DEFAULT 0,
    outdated_count INTEGER DEFAULT 0,

    -- Outcome counts
    success_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    neutral_count INTEGER DEFAULT 0,

    -- Mutation counts
    confidence_increases INTEGER DEFAULT 0,
    confidence_decreases INTEGER DEFAULT 0,
    memories_reinforced INTEGER DEFAULT 0,
    memories_archived INTEGER DEFAULT 0,

    -- Computed metrics
    learning_velocity REAL, -- rate of positive changes
    stability_score REAL,   -- 1 - variance in confidence changes

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, period_start, period_end)
);

CREATE INDEX idx_learning_stats_agent_id ON learning_stats(agent_id);
CREATE INDEX idx_learning_stats_period ON learning_stats(period_start, period_end);

-- Add needs_review flag to memories for flagged contradictions
ALTER TABLE memories ADD COLUMN IF NOT EXISTS needs_review BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX idx_memories_needs_review ON memories(agent_id, needs_review) WHERE needs_review = true;
