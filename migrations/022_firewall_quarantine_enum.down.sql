-- 022_firewall_quarantine_enum.down.sql
-- Postgres cannot drop a value from an enum type, so removing 'quarantine' is a
-- no-op. (The dependent schema is removed by 023's down migration.)
SELECT 1;
