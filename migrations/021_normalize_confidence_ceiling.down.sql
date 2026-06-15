-- 021_normalize_confidence_ceiling.down.sql
-- Irreversible data normalization: the original (>0.99) confidence values are not
-- recoverable, so the down migration is a no-op.
BEGIN;
SELECT 1;
COMMIT;
