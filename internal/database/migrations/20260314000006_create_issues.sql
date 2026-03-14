-- +goose Up

CREATE TYPE issue_type AS ENUM ('task', 'conversation');
CREATE TYPE issue_status AS ENUM ('backlog', 'todo', 'in_progress', 'done', 'blocked', 'cancelled');
CREATE TYPE issue_priority AS ENUM ('critical', 'high', 'medium', 'low');

CREATE TABLE issues (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id          UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    identifier        TEXT NOT NULL,
    type              issue_type NOT NULL DEFAULT 'task',
    title             TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 500),
    description       TEXT,
    status            issue_status NOT NULL DEFAULT 'backlog',
    priority          issue_priority NOT NULL DEFAULT 'medium',
    parent_id         UUID REFERENCES issues(id) ON DELETE SET NULL,
    project_id        UUID,
    goal_id           UUID,
    assignee_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    assignee_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    billing_code      TEXT,
    request_depth     INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_issues_squad_identifier UNIQUE (squad_id, identifier),
    CONSTRAINT ck_issues_no_self_parent CHECK (parent_id IS NULL OR parent_id != id)
);

CREATE INDEX idx_issues_squad_id          ON issues (squad_id);
CREATE INDEX idx_issues_status            ON issues (squad_id, status);
CREATE INDEX idx_issues_priority          ON issues (squad_id, priority);
CREATE INDEX idx_issues_assignee_agent_id ON issues (assignee_agent_id) WHERE assignee_agent_id IS NOT NULL;
CREATE INDEX idx_issues_assignee_user_id  ON issues (assignee_user_id)  WHERE assignee_user_id IS NOT NULL;
CREATE INDEX idx_issues_project_id        ON issues (project_id)        WHERE project_id IS NOT NULL;
CREATE INDEX idx_issues_goal_id           ON issues (goal_id)           WHERE goal_id IS NOT NULL;
CREATE INDEX idx_issues_parent_id         ON issues (parent_id)         WHERE parent_id IS NOT NULL;
CREATE INDEX idx_issues_identifier        ON issues (identifier);
CREATE INDEX idx_issues_squad_created_at  ON issues (squad_id, created_at DESC);
CREATE INDEX idx_issues_squad_updated_at  ON issues (squad_id, updated_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_issues_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_issues_updated_at
    BEFORE UPDATE ON issues
    FOR EACH ROW
    EXECUTE FUNCTION update_issues_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_issues_updated_at ON issues;
DROP FUNCTION IF EXISTS update_issues_updated_at;
DROP TABLE IF EXISTS issues;
DROP TYPE IF EXISTS issue_priority;
DROP TYPE IF EXISTS issue_status;
DROP TYPE IF EXISTS issue_type;
