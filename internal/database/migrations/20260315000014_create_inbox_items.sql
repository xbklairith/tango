-- +goose Up

CREATE TYPE inbox_category AS ENUM (
    'approval', 'question', 'decision', 'alert'
);

CREATE TYPE inbox_urgency AS ENUM (
    'critical', 'normal', 'low'
);

CREATE TYPE inbox_status AS ENUM (
    'pending', 'acknowledged', 'resolved', 'expired'
);

CREATE TYPE inbox_resolution AS ENUM (
    'approved', 'rejected', 'request_revision', 'answered', 'dismissed'
);

CREATE TABLE inbox_items (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id                UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    category                inbox_category NOT NULL,
    type                    TEXT NOT NULL CHECK(char_length(type) <= 100),
    status                  inbox_status NOT NULL DEFAULT 'pending',
    urgency                 inbox_urgency NOT NULL DEFAULT 'normal',
    title                   TEXT NOT NULL CHECK(char_length(title) >= 1 AND char_length(title) <= 500),
    body                    TEXT,
    payload                 JSONB NOT NULL DEFAULT '{}',

    -- Source references
    requested_by_agent_id   UUID REFERENCES agents(id) ON DELETE SET NULL,
    related_agent_id        UUID REFERENCES agents(id) ON DELETE SET NULL,
    related_issue_id        UUID REFERENCES issues(id) ON DELETE SET NULL,
    related_run_id          UUID REFERENCES heartbeat_runs(id) ON DELETE SET NULL,

    -- Resolution
    resolution              inbox_resolution,
    response_note           TEXT,
    response_payload        JSONB,
    resolved_by_user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_at             TIMESTAMPTZ,

    -- Acknowledgment
    acknowledged_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    acknowledged_at         TIMESTAMPTZ,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary listing query: squad-scoped, sorted by urgency then recency
CREATE INDEX idx_inbox_items_squad_list ON inbox_items(squad_id, urgency, created_at DESC);

-- Filter by status (for badge counts and filtered views)
CREATE INDEX idx_inbox_items_squad_status ON inbox_items(squad_id, status)
    WHERE status NOT IN ('resolved', 'expired');

-- Deduplication for auto-created alert items (one active alert per agent per type)
CREATE UNIQUE INDEX uq_inbox_active_alert_per_agent_type ON inbox_items(squad_id, related_agent_id, type)
    WHERE category = 'alert' AND status IN ('pending', 'acknowledged');

-- +goose Down
DROP TABLE IF EXISTS inbox_items;
DROP TYPE IF EXISTS inbox_resolution;
DROP TYPE IF EXISTS inbox_status;
DROP TYPE IF EXISTS inbox_urgency;
DROP TYPE IF EXISTS inbox_category;
