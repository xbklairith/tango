-- +goose Up
CREATE INDEX IF NOT EXISTS idx_issues_conversations_by_agent
    ON issues (assignee_agent_id, updated_at DESC)
    WHERE type = 'conversation' AND assignee_agent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_heartbeat_runs_active_agent
    ON heartbeat_runs (agent_id)
    WHERE status IN ('queued', 'running');

-- +goose Down
DROP INDEX IF EXISTS idx_heartbeat_runs_active_agent;
DROP INDEX IF EXISTS idx_issues_conversations_by_agent;
