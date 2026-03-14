-- +goose Up
CREATE TABLE squads (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 VARCHAR(100)  NOT NULL,
    slug                 VARCHAR(50)   NOT NULL,
    issue_prefix         VARCHAR(10)   NOT NULL,
    description          TEXT          NOT NULL DEFAULT '',
    status               VARCHAR(20)   NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'paused', 'archived')),
    settings             JSONB         NOT NULL DEFAULT '{"requireApprovalForNewAgents": false}',
    issue_counter        BIGINT        NOT NULL DEFAULT 0,
    budget_monthly_cents BIGINT        CHECK (budget_monthly_cents IS NULL OR budget_monthly_cents > 0),
    brand_color          VARCHAR(7),
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT squads_slug_unique       UNIQUE (slug),
    CONSTRAINT squads_issue_prefix_unique UNIQUE (issue_prefix)
);

CREATE INDEX idx_squads_status ON squads (status);

-- +goose Down
DROP TABLE IF EXISTS squads;
