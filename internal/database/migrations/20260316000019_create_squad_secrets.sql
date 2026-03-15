-- +goose Up

-- Squad secrets vault (AES-256-GCM encrypted)
CREATE TABLE squad_secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id        UUID NOT NULL REFERENCES squads(id) ON DELETE RESTRICT,
    name            TEXT NOT NULL,
    encrypted_value BYTEA NOT NULL,
    nonce           BYTEA NOT NULL,
    masked_hint     VARCHAR(12) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_rotated_at TIMESTAMPTZ,

    CONSTRAINT uq_squad_secrets_squad_name UNIQUE (squad_id, name),
    CONSTRAINT chk_secret_name_format CHECK (name ~ '^[A-Z][A-Z0-9_]{0,127}$')
);

-- Index for listing secrets by squad
CREATE INDEX idx_squad_secrets_squad_id ON squad_secrets(squad_id);

-- Updated_at trigger
CREATE TRIGGER set_squad_secrets_updated_at
    BEFORE UPDATE ON squad_secrets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER IF EXISTS set_squad_secrets_updated_at ON squad_secrets;
DROP TABLE IF EXISTS squad_secrets;
