-- 024_tenant_isolation_assoc_activations.up.sql
-- Close the remaining tenant-isolation gaps: memory_associations and
-- working_memory_activations carried no tenant_id, so cross-memory links and
-- activations were filtered only by (globally-unique) memory/session ids. Add an
-- explicit tenant_id, backfilled from the owning row, NOT NULL going forward.
BEGIN;

-- ── working_memory_activations: tenant comes from its session ────────────────
ALTER TABLE working_memory_activations
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

UPDATE working_memory_activations a
    SET tenant_id = s.tenant_id
    FROM working_memory_sessions s
    WHERE a.session_id = s.id AND a.tenant_id IS NULL;

-- Any activation whose session is gone is dead state; drop it so NOT NULL holds.
DELETE FROM working_memory_activations WHERE tenant_id IS NULL;

ALTER TABLE working_memory_activations ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_wm_activations_tenant ON working_memory_activations(tenant_id);

-- ── memory_associations: tenant comes from the SOURCE memory (per type) ───────
ALTER TABLE memory_associations
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

UPDATE memory_associations a SET tenant_id = m.tenant_id
    FROM memories m
    WHERE a.source_memory_type = 'semantic' AND a.source_memory_id = m.id AND a.tenant_id IS NULL;
UPDATE memory_associations a SET tenant_id = e.tenant_id
    FROM episodes e
    WHERE a.source_memory_type = 'episodic' AND a.source_memory_id = e.id AND a.tenant_id IS NULL;
UPDATE memory_associations a SET tenant_id = p.tenant_id
    FROM procedures p
    WHERE a.source_memory_type = 'procedural' AND a.source_memory_id = p.id AND a.tenant_id IS NULL;
UPDATE memory_associations a SET tenant_id = sc.tenant_id
    FROM schemas sc
    WHERE a.source_memory_type = 'schema' AND a.source_memory_id = sc.id AND a.tenant_id IS NULL;

-- Dangling associations (source row already deleted) can't be attributed; drop.
DELETE FROM memory_associations WHERE tenant_id IS NULL;

ALTER TABLE memory_associations ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memory_assoc_tenant ON memory_associations(tenant_id);

COMMIT;
