-- 021_normalize_confidence_ceiling.up.sql
-- Normalize legacy memories stored above the engine's confidence ceiling.
-- The log-odds dynamics can never represent more than 0.99 (1.0 is +∞ log-odds),
-- so any row persisted at >0.99 (older default was 1.0) would appear to *lose*
-- confidence on its first reinforcement. Clamp them once so the stored value
-- matches what the engine can actually produce.
BEGIN;

-- confidence is REAL (float32). Compare against 0.99::real so the clamped value
-- (float32 0.99) doesn't re-match on a float64 literal — keeps this idempotent.
UPDATE memories SET confidence = 0.99 WHERE confidence > 0.99::real;

COMMIT;
