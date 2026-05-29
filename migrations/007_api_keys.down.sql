ALTER TABLE tenants ADD COLUMN IF NOT EXISTS api_key_hash TEXT NOT NULL DEFAULT '';
DROP INDEX IF EXISTS idx_api_keys_key_hash;
DROP INDEX IF EXISTS idx_api_keys_tenant_id;
DROP TABLE IF EXISTS api_keys;
