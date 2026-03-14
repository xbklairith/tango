-- +goose Up

CREATE TABLE cost_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id        UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    amount_cents    BIGINT NOT NULL CHECK (amount_cents > 0),
    event_type      VARCHAR(50) NOT NULL DEFAULT 'llm_call',
    model           VARCHAR(100),
    input_tokens    BIGINT,
    output_tokens   BIGINT,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for agent monthly spend aggregation
CREATE INDEX idx_cost_events_agent_month
    ON cost_events(agent_id, created_at);

-- Index for squad monthly spend aggregation
CREATE INDEX idx_cost_events_squad_month
    ON cost_events(squad_id, created_at);

-- Index for event type filtering
CREATE INDEX idx_cost_events_event_type
    ON cost_events(event_type);

-- +goose Down

DROP TABLE IF EXISTS cost_events;
