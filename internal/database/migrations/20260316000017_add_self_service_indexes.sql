-- +goose Up
CREATE INDEX idx_issues_squad_assignee ON issues(squad_id, assignee_agent_id, created_at DESC);
CREATE INDEX idx_cost_events_agent_period ON cost_events(agent_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_issues_squad_assignee;
DROP INDEX IF EXISTS idx_cost_events_agent_period;
