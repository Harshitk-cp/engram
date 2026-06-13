-- 019_audit_chain_complete.up.sql
-- Closes the gaps that made the 016 chain weaker than "tamper-evident":
--
--   1. The v1 canonical encoding omitted content_snapshot, actor_type,
--      reinforcement counts, metadata, anchor_id, and binding — so the
--      "proof of what was erased" snapshot could be edited and system/api_key
--      attribution flipped without breaking the chain. It was also built with
--      concat_ws('|'), which is delimiter-ambiguous against free-text reason.
--   2. verify_audit_chain() never compared the walked chain against
--      audit_chain_heads, so deleting the most-recent N rows ("tail
--      truncation") still verified as valid.
--   3. audit_chain_heads had no append-only protection of its own.
--   4. (tenant_id, seq) had no UNIQUE guard beneath the advisory lock.
--
-- Existing rows keep their v1 hashes (rewriting them would itself break the
-- chain); a hash_version column records which canon each row was sealed with
-- and verification picks the matching one per row.

BEGIN;

-- ── hash versioning ──────────────────────────────────────────────────────────
-- Metadata-only default: existing rows read as version 1 without a rewrite.
ALTER TABLE mutation_log ADD COLUMN IF NOT EXISTS hash_version SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE mutation_log ALTER COLUMN hash_version SET DEFAULT 2;

-- ── v2 canonical encoding ────────────────────────────────────────────────────
-- jsonb_build_array gives an injective, delimiter-free encoding (strings are
-- escaped, NULL is distinguishable from ''), and now covers every evidentiary
-- column including content_snapshot and actor_type.
CREATE OR REPLACE FUNCTION audit_canon_v2(r mutation_log) RETURNS TEXT
LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_array(
        r.hash_version,
        r.seq, r.prev_hash,
        r.memory_id::text, r.agent_id::text, r.tenant_id::text,
        r.mutation_type, r.source_type, r.source_id::text,
        r.old_confidence, r.new_confidence,
        r.old_reinforcement_count, r.new_reinforcement_count,
        r.reason, r.metadata,
        r.content_hash, r.content_snapshot,
        r.actor_type, r.actor_id::text,
        r.anchor_id::text, r.binding,
        extract(epoch from r.created_at)::text
    )::text
$$;

-- ── protect the chain heads ──────────────────────────────────────────────────
-- Heads may only change from inside the chain trigger (signalled via a
-- transaction-local GUC). Deletes are never allowed.
CREATE OR REPLACE FUNCTION audit_chain_heads_guard() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'audit_chain_heads rows cannot be deleted (tamper-evident audit trail)';
    END IF;
    IF current_setting('engram.allow_head_write', true) IS DISTINCT FROM '1' THEN
        RAISE EXCEPTION 'audit_chain_heads is maintained only by the audit chain trigger';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_audit_chain_heads_guard ON audit_chain_heads;
CREATE TRIGGER trg_audit_chain_heads_guard
    BEFORE INSERT OR UPDATE OR DELETE ON audit_chain_heads
    FOR EACH ROW EXECUTE FUNCTION audit_chain_heads_guard();

-- ── chain trigger: seal new rows with v2 ─────────────────────────────────────
CREATE OR REPLACE FUNCTION mutation_log_chain() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    v_tenant UUID := COALESCE(NEW.tenant_id, '00000000-0000-0000-0000-000000000000'::uuid);
    v_seq BIGINT;
    v_prev TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext(v_tenant::text));
    SELECT last_seq, last_hash INTO v_seq, v_prev FROM audit_chain_heads WHERE tenant_id = v_tenant;
    IF NOT FOUND THEN v_seq := 0; v_prev := ''; END IF;
    NEW.seq := v_seq + 1;
    NEW.prev_hash := v_prev;
    NEW.hash_version := 2;
    NEW.row_hash := audit_hash(audit_canon_v2(NEW));

    PERFORM set_config('engram.allow_head_write', '1', true);
    INSERT INTO audit_chain_heads(tenant_id, last_seq, last_hash)
        VALUES (v_tenant, NEW.seq, NEW.row_hash)
        ON CONFLICT (tenant_id) DO UPDATE SET last_seq = EXCLUDED.last_seq, last_hash = EXCLUDED.last_hash;
    PERFORM set_config('engram.allow_head_write', '0', true);
    RETURN NEW;
END $$;

-- ── verification: per-row canon version, snapshot binding, head cross-check ──
DROP FUNCTION IF EXISTS verify_audit_chain(UUID);
CREATE FUNCTION verify_audit_chain(p_tenant UUID)
RETURNS TABLE(valid BOOLEAN, checked BIGINT, break_seq BIGINT, reason TEXT)
LANGUAGE plpgsql AS $$
DECLARE
    r mutation_log%ROWTYPE;
    v_prev TEXT := '';
    v_expect BIGINT := 0;
    v_count BIGINT := 0;
    v_calc TEXT;
    v_head_seq BIGINT;
    v_head_hash TEXT;
BEGIN
    FOR r IN SELECT * FROM mutation_log
             WHERE COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid) = p_tenant
             ORDER BY seq LOOP
        v_expect := v_expect + 1;
        IF r.seq <> v_expect OR COALESCE(r.prev_hash, '') <> v_prev THEN
            RETURN QUERY SELECT false, v_count, r.seq, 'chain link broken (seq or prev_hash mismatch)'::text; RETURN;
        END IF;
        IF r.hash_version >= 2 THEN
            v_calc := audit_hash(audit_canon_v2(r));
        ELSE
            v_calc := audit_hash(audit_canon(r.seq, r.prev_hash, r.memory_id, r.agent_id, r.tenant_id,
                r.mutation_type, r.source_type, r.source_id, r.old_confidence, r.new_confidence,
                r.reason, r.content_hash, r.actor_id, r.created_at));
        END IF;
        IF v_calc <> COALESCE(r.row_hash, '') THEN
            RETURN QUERY SELECT false, v_count, r.seq, 'row hash mismatch (row edited)'::text; RETURN;
        END IF;
        -- The snapshot is the proof of what was erased; it must still hash to
        -- the content_hash the chain sealed.
        IF r.content_snapshot IS NOT NULL AND COALESCE(r.content_hash, '') <> ''
           AND audit_hash(r.content_snapshot) <> r.content_hash THEN
            RETURN QUERY SELECT false, v_count, r.seq, 'content_snapshot does not match sealed content_hash'::text; RETURN;
        END IF;
        v_prev := r.row_hash;
        v_count := v_count + 1;
    END LOOP;

    -- Cross-check against the recorded head: a chain that walks cleanly but is
    -- shorter than the head has had its tail truncated.
    SELECT last_seq, last_hash INTO v_head_seq, v_head_hash FROM audit_chain_heads WHERE tenant_id = p_tenant;
    IF NOT FOUND THEN
        IF v_count > 0 THEN
            RETURN QUERY SELECT false, v_count, NULL::BIGINT, 'chain head row missing'::text; RETURN;
        END IF;
        RETURN QUERY SELECT true, v_count, NULL::BIGINT, NULL::text; RETURN;
    END IF;
    IF v_head_seq <> v_count OR COALESCE(v_head_hash, '') <> v_prev THEN
        RETURN QUERY SELECT false, v_count, NULL::BIGINT,
            format('head mismatch: head_seq=%s checked=%s (tail truncated or head edited)', v_head_seq, v_count)::text;
        RETURN;
    END IF;
    RETURN QUERY SELECT true, v_count, NULL::BIGINT, NULL::text;
END $$;

-- ── structural guards ────────────────────────────────────────────────────────
-- Defense-in-depth under the advisory lock: duplicate seqs in a chain are
-- impossible even if the trigger is ever bypassed.
CREATE UNIQUE INDEX IF NOT EXISTS uq_mutation_log_chain_seq
    ON mutation_log ((COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid)), seq)
    WHERE seq IS NOT NULL;

-- New audit rows must carry their tenant: a NULL-tenant row would fall into
-- the shared zero-UUID chain that no real tenant's verification inspects.
-- NOT VALID: existing rows keep their (already-chained) NULLs untouched.
ALTER TABLE mutation_log DROP CONSTRAINT IF EXISTS chk_mutation_log_tenant_required;
ALTER TABLE mutation_log ADD CONSTRAINT chk_mutation_log_tenant_required
    CHECK (tenant_id IS NOT NULL) NOT VALID;

COMMIT;
