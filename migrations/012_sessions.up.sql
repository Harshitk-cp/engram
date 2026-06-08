-- 012_sessions.up.sql
-- Phase 2 of Engram's binding model (see engram_docs/ANCHOR_MODEL.md): the SESSION
-- axis — short-term memory tied to one conversation. A session optionally belongs
-- to an anchor (a returning guest's call) or is anonymous (a walk-in chat).
--
-- Session-bound traces decay/expire after the session ends, so "what happened on
-- the last call" stays available for days, then fades — without polluting the
-- anchor's durable profile.

BEGIN;

CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id)  ON DELETE CASCADE,
    anchor_id   UUID NULL REFERENCES entities(id)    ON DELETE SET NULL,
    external_id TEXT NULL,                 -- caller's own session/call id
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'ended', 'expired')),
    metadata    JSONB DEFAULT '{}',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at    TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,              -- session memory is swept after this
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sessions_anchor ON sessions (anchor_id) WHERE anchor_id IS NOT NULL;
CREATE INDEX idx_sessions_agent  ON sessions (agent_id);
CREATE INDEX idx_sessions_expiry ON sessions (expires_at)
    WHERE status <> 'expired' AND expires_at IS NOT NULL;

-- session_id on memories + FK.
ALTER TABLE memories
    ADD COLUMN session_id UUID NULL REFERENCES sessions(id) ON DELETE SET NULL;
CREATE INDEX idx_memories_session ON memories (session_id)
    WHERE session_id IS NOT NULL AND is_archived = FALSE;

-- Extend the binding/IDs consistency constraint to cover the session branch.
-- A session trace must carry a session_id; it may additionally be about an anchor.
ALTER TABLE memories DROP CONSTRAINT IF EXISTS chk_memory_binding_anchor;
ALTER TABLE memories ADD CONSTRAINT chk_memory_binding_ids CHECK (
    (binding IN ('canon', 'private') AND anchor_id IS NULL AND session_id IS NULL) OR
    (binding = 'anchored' AND anchor_id IS NOT NULL AND session_id IS NULL) OR
    (binding = 'session'  AND session_id IS NOT NULL)
);

COMMIT;
