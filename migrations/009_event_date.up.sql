-- 009_event_date.up.sql
-- Add event_date column to memories to track WHEN the event happened (vs created_at = when stored).
-- This is the critical fix for recency ordering: recency should rank by event date, not store date.
-- Example: "Back in 2019 I loved Python, by 2022 I switched to Go" — both stored today but 2022 > 2019.
-- When event_date is NULL, falls back to created_at (backward compatible).

ALTER TABLE memories ADD COLUMN event_date TIMESTAMPTZ NULL;

-- Index for efficient recency ordering and date-range queries per agent
CREATE INDEX idx_memories_event_date ON memories(agent_id, event_date DESC NULLS LAST)
    WHERE is_archived = FALSE;

-- Composite index for date-range + tenant isolation queries
CREATE INDEX idx_memories_event_date_tenant ON memories(tenant_id, agent_id, event_date DESC NULLS LAST)
    WHERE is_archived = FALSE;
