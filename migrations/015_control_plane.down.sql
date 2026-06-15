-- 015_control_plane.down.sql
BEGIN;

ALTER TABLE api_keys DROP COLUMN IF EXISTS created_by;
DROP TABLE IF EXISTS console_sessions;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS oauth_accounts;
DROP TABLE IF EXISTS users;

COMMIT;
