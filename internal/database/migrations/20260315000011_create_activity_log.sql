-- +goose Up

CREATE TYPE activity_actor_type AS ENUM ('agent', 'user', 'system');

CREATE TABLE activity_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID        NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    actor_type  activity_actor_type NOT NULL,
    actor_id    UUID        NOT NULL,
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   UUID        NOT NULL,
    metadata    JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_activity_log_squad_created_at
    ON activity_log (squad_id, created_at DESC);

CREATE INDEX idx_activity_log_squad_actor_type
    ON activity_log (squad_id, actor_type);

CREATE INDEX idx_activity_log_squad_entity_type
    ON activity_log (squad_id, entity_type);

-- +goose Down
DROP TABLE IF EXISTS activity_log;
DROP TYPE  IF EXISTS activity_actor_type;
