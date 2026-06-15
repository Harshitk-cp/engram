-- 015_control_plane.up.sql
-- Control plane: human users, social identities, org memberships, and sessions.
-- The data-plane API-key auth (api_keys) is unchanged; this adds a session-based
-- auth path for the console. Existing api_keys remain tenant-scoped; created_by is
-- nullable so legacy/system keys (created_by = NULL) keep working.

BEGIN;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT,                 -- NULL for social-only accounts
    name          TEXT NOT NULL DEFAULT '',
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE oauth_accounts (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,     -- 'google' | 'github'
    provider_user_id TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);
CREATE INDEX idx_oauth_accounts_user ON oauth_accounts(user_id);

CREATE TABLE memberships (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, tenant_id)
);
CREATE INDEX idx_memberships_user ON memberships(user_id);
CREATE INDEX idx_memberships_tenant ON memberships(tenant_id);

-- console_sessions: human login sessions (distinct from conversation `sessions`).
CREATE TABLE console_sessions (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    token_hash       TEXT NOT NULL UNIQUE,   -- sha256 of the opaque cookie token
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    active_tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_console_sessions_token ON console_sessions(token_hash);
CREATE INDEX idx_console_sessions_user ON console_sessions(user_id);

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL;

COMMIT;
