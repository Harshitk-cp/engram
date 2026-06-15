-- Revert 019. PARTIALLY LOSSY for verification: rows sealed after 019 carry
-- v2 hashes; the restored v1 verifier cannot validate them and will report the
-- chain broken at the first v2 row. The rows themselves are untouched.

BEGIN;

ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS chk_mutation_log_tenant_required;
DROP INDEX IF EXISTS uq_mutation_log_chain_seq;

-- Restore the 016 verifier signature and logic.
DROP FUNCTION IF EXISTS verify_audit_chain(UUID);
CREATE FUNCTION verify_audit_chain(p_tenant UUID)
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

-- Restore the 016 chain trigger (v1 canon).
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

DROP TRIGGER IF EXISTS trg_audit_chain_heads_guard ON audit_chain_heads;
DROP FUNCTION IF EXISTS audit_chain_heads_guard();
DROP FUNCTION IF EXISTS audit_canon_v2(mutation_log);
ALTER TABLE mutation_log DROP COLUMN IF EXISTS hash_version;

COMMIT;
