-- +goose Up
CREATE TABLE goals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    parent_id   UUID REFERENCES goals(id) ON DELETE SET NULL,
    title       VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR parent_id != id)
);

CREATE INDEX idx_goals_squad_id  ON goals (squad_id);
CREATE INDEX idx_goals_parent_id ON goals (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_goals_status    ON goals (squad_id, status);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_goals_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_goals_updated_at
    BEFORE UPDATE ON goals
    FOR EACH ROW EXECUTE FUNCTION update_goals_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_goals_updated_at ON goals;
DROP FUNCTION IF EXISTS update_goals_updated_at();
DROP TABLE IF EXISTS goals;
