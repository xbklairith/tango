-- +goose Up

-- Wakeup requests queue
CREATE TYPE wakeup_invocation_source AS ENUM (
    'on_demand', 'timer', 'assignment', 'inbox_resolved', 'conversation_message'
);
CREATE TYPE wakeup_request_status AS ENUM ('pending', 'dispatched', 'discarded');

CREATE TABLE wakeup_requests (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id          UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id          UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    invocation_source wakeup_invocation_source NOT NULL,
    status            wakeup_request_status NOT NULL DEFAULT 'pending',
    context_json      JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at     TIMESTAMPTZ,
    discarded_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_wakeup_pending_per_agent ON wakeup_requests(agent_id)
    WHERE status = 'pending';
CREATE INDEX idx_wakeup_squad_pending ON wakeup_requests(squad_id, created_at)
    WHERE status = 'pending';

-- HeartbeatRun records
CREATE TYPE heartbeat_run_status AS ENUM (
    'queued', 'running', 'succeeded', 'failed', 'cancelled', 'timed_out'
);

CREATE TABLE heartbeat_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id          UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id          UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    wakeup_request_id UUID REFERENCES wakeup_requests(id),
    invocation_source wakeup_invocation_source NOT NULL,
    status            heartbeat_run_status NOT NULL DEFAULT 'queued',
    session_id_before TEXT,
    session_id_after  TEXT,
    exit_code         INTEGER,
    usage_json        JSONB,
    stdout_excerpt    TEXT,
    stderr_excerpt    TEXT,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_heartbeat_runs_agent  ON heartbeat_runs(agent_id, created_at DESC);
CREATE INDEX idx_heartbeat_runs_squad  ON heartbeat_runs(squad_id, created_at DESC);
CREATE INDEX idx_heartbeat_runs_active ON heartbeat_runs(squad_id)
    WHERE status IN ('queued', 'running');

-- Task checkout columns on issues
ALTER TABLE issues
    ADD COLUMN checkout_run_id     UUID REFERENCES heartbeat_runs(id) ON DELETE SET NULL,
    ADD COLUMN execution_locked_at TIMESTAMPTZ;
CREATE INDEX idx_issues_checkout_run ON issues(checkout_run_id)
    WHERE checkout_run_id IS NOT NULL;

-- Session persistence
CREATE TABLE agent_task_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id      UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_state TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_task_session UNIQUE (agent_id, issue_id)
);

CREATE TABLE agent_conversation_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id      UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_state TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_conversation_session UNIQUE (agent_id, issue_id)
);

-- +goose Down
ALTER TABLE issues DROP COLUMN IF EXISTS checkout_run_id;
ALTER TABLE issues DROP COLUMN IF EXISTS execution_locked_at;
DROP INDEX IF EXISTS idx_issues_checkout_run;
DROP TABLE IF EXISTS agent_conversation_sessions;
DROP TABLE IF EXISTS agent_task_sessions;
DROP TABLE IF EXISTS heartbeat_runs;
DROP TABLE IF EXISTS wakeup_requests;
DROP TYPE IF EXISTS heartbeat_run_status;
DROP TYPE IF EXISTS wakeup_request_status;
DROP TYPE IF EXISTS wakeup_invocation_source;
