-- 016_audit_chain.up.sql
-- Tamper-evidence for the audit trail: per-tenant hash chaining of mutation_log,
-- append-only enforcement, and a verify function. Tampering with any row (or
-- inserting/deleting/reordering) breaks the chain and is detectable.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS seq       BIGINT;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS prev_hash TEXT;
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS row_hash  TEXT;

-- One running chain head per tenant (the zero UUID holds untenanted rows).
CREATE TABLE IF NOT EXISTS audit_chain_heads (
    tenant_id UUID PRIMARY KEY,
    last_seq  BIGINT NOT NULL DEFAULT 0,
    last_hash TEXT   NOT NULL DEFAULT ''
);

-- Single source of truth for the canonical, timezone-independent row encoding...
CREATE OR REPLACE FUNCTION audit_canon(
    p_seq BIGINT, p_prev TEXT, p_memory UUID, p_agent UUID, p_tenant UUID,
    p_mtype TEXT, p_stype TEXT, p_source UUID, p_oldc REAL, p_newc REAL,
    p_reason TEXT, p_chash TEXT, p_actor UUID, p_created TIMESTAMPTZ
) RETURNS TEXT LANGUAGE sql STABLE AS $$
    SELECT concat_ws('|',
        p_seq, p_prev,
        COALESCE(p_memory::text, ''), COALESCE(p_agent::text, ''), COALESCE(p_tenant::text, ''),
        p_mtype, p_stype, COALESCE(p_source::text, ''),
        COALESCE(p_oldc::text, ''), COALESCE(p_newc::text, ''),
        COALESCE(p_reason, ''), COALESCE(p_chash, ''), COALESCE(p_actor::text, ''),
        COALESCE(extract(epoch from p_created)::text, ''))
$$;

CREATE OR REPLACE FUNCTION audit_hash(p_canon TEXT) RETURNS TEXT
LANGUAGE sql STABLE AS $$ SELECT encode(digest(p_canon, 'sha256'), 'hex') $$;

-- BEFORE INSERT: assign seq/prev_hash/row_hash and advance the tenant's chain head.
CREATE OR REPLACE FUNCTION mutation_log_chain() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    v_tenant UUID := COALESCE(NEW.tenant_id, '00000000-0000-0000-0000-000000000000'::uuid);
    v_seq BIGINT;
    v_prev TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext(v_tenant::text));
    SELECT last_seq, last_hash INTO v_seq, v_prev FROM audit_chain_heads WHERE tenant_id = v_tenant;
    IF NOT FOUND THEN v_seq := 0; v_prev := ''; END IF;
    v_seq := v_seq + 1;
    NEW.seq := v_seq;
    NEW.prev_hash := v_prev;
    NEW.row_hash := audit_hash(audit_canon(NEW.seq, NEW.prev_hash, NEW.memory_id, NEW.agent_id,
        NEW.tenant_id, NEW.mutation_type, NEW.source_type, NEW.source_id, NEW.old_confidence,
        NEW.new_confidence, NEW.reason, NEW.content_hash, NEW.actor_id, NEW.created_at));
    INSERT INTO audit_chain_heads(tenant_id, last_seq, last_hash)
        VALUES (v_tenant, v_seq, NEW.row_hash)
        ON CONFLICT (tenant_id) DO UPDATE SET last_seq = EXCLUDED.last_seq, last_hash = EXCLUDED.last_hash;
    RETURN NEW;
END $$;

CREATE TRIGGER trg_mutation_log_chain BEFORE INSERT ON mutation_log
    FOR EACH ROW EXECUTE FUNCTION mutation_log_chain();

-- Append-only: the audit trail cannot be updated or deleted by the application.
CREATE OR REPLACE FUNCTION mutation_log_immutable() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'mutation_log is append-only (tamper-evident audit trail)';
END $$;

CREATE TRIGGER trg_mutation_log_immutable BEFORE UPDATE OR DELETE ON mutation_log
    FOR EACH ROW EXECUTE FUNCTION mutation_log_immutable();

-- Backfill existing rows into per-tenant chains (stable order).
DO $$
DECLARE r RECORD; v_tenant UUID; v_seq BIGINT; v_prev TEXT; v_hash TEXT;
BEGIN
    ALTER TABLE mutation_log DISABLE TRIGGER trg_mutation_log_immutable;
    FOR r IN SELECT * FROM mutation_log
             ORDER BY COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), created_at, id LOOP
        v_tenant := COALESCE(r.tenant_id, '00000000-0000-0000-0000-000000000000'::uuid);
        SELECT last_seq, last_hash INTO v_seq, v_prev FROM audit_chain_heads WHERE tenant_id = v_tenant;
        IF NOT FOUND THEN v_seq := 0; v_prev := ''; END IF;
        v_seq := v_seq + 1;
        v_hash := audit_hash(audit_canon(v_seq, v_prev, r.memory_id, r.agent_id, r.tenant_id,
            r.mutation_type, r.source_type, r.source_id, r.old_confidence, r.new_confidence,
            r.reason, r.content_hash, r.actor_id, r.created_at));
        UPDATE mutation_log SET seq = v_seq, prev_hash = v_prev, row_hash = v_hash WHERE id = r.id;
        INSERT INTO audit_chain_heads(tenant_id, last_seq, last_hash) VALUES (v_tenant, v_seq, v_hash)
            ON CONFLICT (tenant_id) DO UPDATE SET last_seq = EXCLUDED.last_seq, last_hash = EXCLUDED.last_hash;
    END LOOP;
    ALTER TABLE mutation_log ENABLE TRIGGER trg_mutation_log_immutable;
END $$;

CREATE INDEX IF NOT EXISTS idx_mutation_log_tenant_seq ON mutation_log(tenant_id, seq);

-- Walk a tenant's chain; returns whether it is intact, how many rows verified,
-- and the seq of the first break (NULL when intact).
CREATE OR REPLACE FUNCTION verify_audit_chain(p_tenant UUID)
RETURNS TABLE(valid BOOLEAN, checked BIGINT, break_seq BIGINT) LANGUAGE plpgsql AS $$
DECLARE r RECORD; v_prev TEXT := ''; v_expect BIGINT := 0; v_count BIGINT := 0; v_calc TEXT;
BEGIN
    FOR r IN SELECT * FROM mutation_log WHERE tenant_id = p_tenant ORDER BY seq LOOP
        v_expect := v_expect + 1;
        IF r.seq <> v_expect OR COALESCE(r.prev_hash, '') <> v_prev THEN
            RETURN QUERY SELECT false, v_count, r.seq; RETURN;
        END IF;
        v_calc := audit_hash(audit_canon(r.seq, r.prev_hash, r.memory_id, r.agent_id, r.tenant_id,
            r.mutation_type, r.source_type, r.source_id, r.old_confidence, r.new_confidence,
            r.reason, r.content_hash, r.actor_id, r.created_at));
        IF v_calc <> COALESCE(r.row_hash, '') THEN
            RETURN QUERY SELECT false, v_count, r.seq; RETURN;
        END IF;
        v_prev := r.row_hash;
        v_count := v_count + 1;
    END LOOP;
    RETURN QUERY SELECT true, v_count, NULL::BIGINT;
END $$;

COMMIT;
