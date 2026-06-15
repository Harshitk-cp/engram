-- 016_audit_chain.down.sql
BEGIN;

DROP TRIGGER IF EXISTS trg_mutation_log_immutable ON mutation_log;
DROP TRIGGER IF EXISTS trg_mutation_log_chain ON mutation_log;
DROP FUNCTION IF EXISTS verify_audit_chain(UUID);
DROP FUNCTION IF EXISTS mutation_log_immutable();
DROP FUNCTION IF EXISTS mutation_log_chain();
DROP FUNCTION IF EXISTS audit_hash(TEXT);
DROP FUNCTION IF EXISTS audit_canon(BIGINT, TEXT, UUID, UUID, UUID, TEXT, TEXT, UUID, REAL, REAL, TEXT, TEXT, UUID, TIMESTAMPTZ);
DROP INDEX IF EXISTS idx_mutation_log_tenant_seq;
DROP TABLE IF EXISTS audit_chain_heads;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS row_hash;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS prev_hash;
ALTER TABLE mutation_log DROP COLUMN IF EXISTS seq;

COMMIT;
