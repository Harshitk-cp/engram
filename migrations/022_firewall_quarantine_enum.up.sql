-- 022_firewall_quarantine_enum.up.sql
-- Provenance Firewall, part 1: add the `quarantine` binding value.
--
-- A quarantined trace is untrusted memory held OUTSIDE active recall and belief
-- logic until a human/admin releases or rejects it — the defense against memory
-- poisoning (OWASP ASI06). It must be a committed enum value before any later
-- migration can reference it, so this runs alone (Postgres forbids using a new
-- enum value in the same transaction that adds it).
ALTER TYPE memory_binding ADD VALUE IF NOT EXISTS 'quarantine';
