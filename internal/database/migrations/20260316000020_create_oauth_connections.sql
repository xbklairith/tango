-- +goose Up

CREATE TABLE oauth_connections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('google', 'github')),
    provider_user_id TEXT NOT NULL,
    provider_email  TEXT NOT NULL,
    access_token_encrypted  BYTEA,
    refresh_token_encrypted BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One connection per provider per user
CREATE UNIQUE INDEX uq_oauth_user_provider ON oauth_connections (user_id, provider);

-- One Ari link per provider identity
CREATE UNIQUE INDEX uq_oauth_provider_identity ON oauth_connections (provider, provider_user_id);

-- Lookup by provider email (for auto-link on first OAuth login)
CREATE INDEX idx_oauth_provider_email ON oauth_connections (provider_email);

-- Auto-update updated_at (per-table trigger function, matching existing pattern)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_oauth_connections_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_oauth_connections_updated_at
    BEFORE UPDATE ON oauth_connections
    FOR EACH ROW EXECUTE FUNCTION update_oauth_connections_updated_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_oauth_connections_updated_at ON oauth_connections;
DROP FUNCTION IF EXISTS update_oauth_connections_updated_at;
DROP TABLE IF EXISTS oauth_connections;
