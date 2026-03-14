-- +goose Up
ALTER TABLE issues
    ADD CONSTRAINT fk_issues_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL,
    ADD CONSTRAINT fk_issues_goal_id    FOREIGN KEY (goal_id)    REFERENCES goals(id)    ON DELETE SET NULL;

-- +goose Down
ALTER TABLE issues
    DROP CONSTRAINT IF EXISTS fk_issues_project_id,
    DROP CONSTRAINT IF EXISTS fk_issues_goal_id;
