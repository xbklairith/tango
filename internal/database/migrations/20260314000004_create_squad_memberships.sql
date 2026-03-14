-- +goose Up
CREATE TABLE squad_memberships (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    squad_id   UUID        NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    role       VARCHAR(20) NOT NULL DEFAULT 'viewer'
               CHECK (role IN ('owner', 'admin', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT squad_memberships_user_squad_unique UNIQUE (user_id, squad_id)
);

CREATE INDEX idx_squad_memberships_squad_id ON squad_memberships (squad_id);
CREATE INDEX idx_squad_memberships_user_id  ON squad_memberships (user_id);

-- +goose Down
DROP TABLE IF EXISTS squad_memberships;
