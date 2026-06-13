-- 018: entities.tenant_id NOT NULL
--
-- Migration 011 added tenant_id as nullable and only backfilled rows that had
-- an agent_id at migration time; EntityStore.Create kept inserting without a
-- tenant afterwards. A NULL tenant_id row escapes every `tenant_id = $1`
-- filter: it is invisible to listing, undeletable through the API, and excluded
-- from the partial unique anchor index (so duplicate anchors can accumulate).
-- The store now always writes tenant_id; this migration repairs old rows and
-- locks the invariant in at the schema level.

-- Backfill from the owning agent (covers non-anchor entities created since 011).
UPDATE entities e SET tenant_id = a.tenant_id
    FROM agents a WHERE e.agent_id = a.id AND e.tenant_id IS NULL;

-- Anything still NULL has no owning agent and no tenant: it is unreachable by
-- every tenant-scoped query, so there is no owner to guess. Remove it.
-- (entity_mentions cascades on entity delete.)
DELETE FROM entities WHERE tenant_id IS NULL;

ALTER TABLE entities ALTER COLUMN tenant_id SET NOT NULL;
