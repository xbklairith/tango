-- +goose Up

CREATE TYPE agent_role AS ENUM ('captain', 'lead', 'member');

CREATE TYPE agent_status AS ENUM (
    'pending_approval',
    'active',
    'idle',
    'running',
    'error',
    'paused',
    'terminated'
);

CREATE TYPE adapter_type AS ENUM (
    'claude_local',
    'codex_local',
    'cursor',
    'process',
    'http',
    'openclaw_gateway'
);

CREATE TABLE agents (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id            UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    short_name          VARCHAR(50) NOT NULL,
    role                agent_role NOT NULL,
    status              agent_status NOT NULL DEFAULT 'active',
    parent_agent_id     UUID REFERENCES agents(id) ON DELETE SET NULL,
    adapter_type        adapter_type,
    adapter_config      JSONB DEFAULT '{}',
    system_prompt       TEXT,
    model               VARCHAR(100),
    budget_monthly_cents BIGINT CHECK (budget_monthly_cents IS NULL OR budget_monthly_cents >= 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- REQ-AGT-NF-003: Unique short_name within a squad.
CREATE UNIQUE INDEX idx_agents_squad_short_name ON agents(squad_id, short_name);

-- REQ-AGT-NF-002: Index on squad_id for list queries.
CREATE INDEX idx_agents_squad_id ON agents(squad_id);

-- REQ-AGT-NF-002: Index on parent_agent_id for hierarchy queries.
CREATE INDEX idx_agents_parent_agent_id ON agents(parent_agent_id);

-- REQ-AGT-014: Only one captain per squad (partial unique index).
CREATE UNIQUE INDEX idx_agents_one_captain_per_squad
    ON agents(squad_id)
    WHERE role = 'captain' AND status != 'terminated';

-- Auto-update updated_at on row modification.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_agents_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_agents_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE FUNCTION update_agents_updated_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_agents_updated_at ON agents;
DROP FUNCTION IF EXISTS update_agents_updated_at;
DROP TABLE IF EXISTS agents;
DROP TYPE IF EXISTS adapter_type;
DROP TYPE IF EXISTS agent_status;
DROP TYPE IF EXISTS agent_role;
