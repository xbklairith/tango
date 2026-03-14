-- +goose Up
CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (squad_id, name)
);

CREATE INDEX idx_projects_squad_id ON projects (squad_id);
CREATE INDEX idx_projects_status   ON projects (squad_id, status);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_projects_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_projects_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_projects_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
DROP FUNCTION IF EXISTS update_projects_updated_at();
DROP TABLE IF EXISTS projects;
