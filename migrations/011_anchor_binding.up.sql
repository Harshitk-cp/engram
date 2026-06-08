-- 011_anchor_binding.up.sql
-- Phase 1 of Engram's binding model (see engram_docs/ANCHOR_MODEL.md): gives every
-- memory trace something to hold onto — an ANCHOR: who/what the trace is about.
--
-- Until now a trace recorded only which agent FORMED it, never who it was ABOUT,
-- so one agent forming traces about thousands of people had no way to keep them
-- apart on recall. An anchor is reused from the existing `entities` table
-- (is_anchor = true), promoted from agent scope to tenant scope so one
-- guest/lead/patient is shared across all of a tenant's agents.
--
-- A trace's `binding` records what it is bound to:
--   canon    — tenant-shared, authoritative knowledge (policies, catalog)
--   private  — the forming agent's own memory (default; pre-binding behavior)
--   anchored — bound to a specific anchor (a person/account/case)
--   session  — bound to one conversation (Phase 2; enum value reserved here)

BEGIN;

-- What a trace is bound to. 'canon' and 'private' are deliberately
-- indistinguishable by IDs (neither carries an anchor) — the difference is set
-- explicitly at write time, never inferred from "no anchor".
CREATE TYPE memory_binding AS ENUM ('canon', 'private', 'anchored', 'session');

-- ── memories: the anchor axis ───────────────────────────────────────────────
ALTER TABLE memories
    ADD COLUMN binding   memory_binding NOT NULL DEFAULT 'private',
    -- ON DELETE SET NULL is the soft "unlink" case only (e.g. an anchor merge
    -- that re-points history). Hard GDPR/HIPAA erasure is a separate purge path
    -- that DELETEs the rows (see DELETE /v1/anchors/{id}?purge=true).
    ADD COLUMN anchor_id UUID NULL REFERENCES entities(id) ON DELETE SET NULL;

-- Consistency: the stored binding must agree with whether an anchor is present,
-- so it can never drift from the data. (session branch added in Phase 2.)
ALTER TABLE memories ADD CONSTRAINT chk_memory_binding_anchor CHECK (
    (binding IN ('canon', 'private') AND anchor_id IS NULL) OR
    (binding = 'anchored' AND anchor_id IS NOT NULL)
);

-- Index every binding key we filter on (don't leave a filter key in metadata only).
CREATE INDEX idx_memories_anchor ON memories (agent_id, anchor_id, binding)
    WHERE is_archived = FALSE;
CREATE INDEX idx_memories_tenant_anchor ON memories (tenant_id, anchor_id)
    WHERE anchor_id IS NOT NULL AND is_archived = FALSE;

-- ── entities: promote to tenant scope, mark anchors ─────────────────────────
ALTER TABLE entities
    ADD COLUMN tenant_id   UUID NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ADD COLUMN is_anchor   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN external_id TEXT NULL;

-- An anchor may exist independently of any single agent.
ALTER TABLE entities ALTER COLUMN agent_id DROP NOT NULL;

-- Backfill tenant_id for existing entities from their owning agent.
UPDATE entities e SET tenant_id = a.tenant_id
    FROM agents a WHERE e.agent_id = a.id AND e.tenant_id IS NULL;

-- Anchors are unique per tenant, keyed by the caller's own external_id.
CREATE UNIQUE INDEX uq_entities_tenant_anchor
    ON entities (tenant_id, external_id)
    WHERE is_anchor = TRUE AND external_id IS NOT NULL;

CREATE INDEX idx_entities_tenant_anchor
    ON entities (tenant_id) WHERE is_anchor = TRUE;

COMMIT;
