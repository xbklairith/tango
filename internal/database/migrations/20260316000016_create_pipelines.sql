-- +goose Up

-- Pipeline definitions (workflow templates)
CREATE TABLE pipelines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name        TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    description TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_pipelines_squad_name UNIQUE (squad_id, name)
);

-- [M-2] Removed redundant idx_pipelines_squad_id — uq_pipelines_squad_name already covers squad_id lookups.
CREATE INDEX idx_pipelines_squad_active ON pipelines (squad_id) WHERE is_active = true;

-- Pipeline stages (ordered steps within a pipeline)
CREATE TABLE pipeline_stages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id       UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    description       TEXT,
    position          INTEGER NOT NULL CHECK (position >= 1),
    assigned_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    gate_id           UUID DEFAULT NULL,  -- [XC-1] v2: FK to approval_gates table; NULL = no gate
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_pipeline_stages_position UNIQUE (pipeline_id, position)
);

CREATE INDEX idx_pipeline_stages_pipeline_id ON pipeline_stages (pipeline_id);
CREATE INDEX idx_pipeline_stages_agent ON pipeline_stages (assigned_agent_id)
    WHERE assigned_agent_id IS NOT NULL;

-- Add pipeline tracking columns to issues
ALTER TABLE issues
    ADD COLUMN pipeline_id       UUID REFERENCES pipelines(id) ON DELETE SET NULL,
    ADD COLUMN current_stage_id  UUID REFERENCES pipeline_stages(id) ON DELETE SET NULL;

-- [DB] CHECK constraint: pipeline_id and current_stage_id must be consistent.
-- current_stage_id can only be set when pipeline_id is set.
ALTER TABLE issues
    ADD CONSTRAINT chk_issues_pipeline_stage_consistency
    CHECK ((pipeline_id IS NULL AND current_stage_id IS NULL) OR (pipeline_id IS NOT NULL));

CREATE INDEX idx_issues_pipeline_id ON issues (pipeline_id)
    WHERE pipeline_id IS NOT NULL;
CREATE INDEX idx_issues_current_stage_id ON issues (current_stage_id)
    WHERE current_stage_id IS NOT NULL;

-- Auto-update updated_at triggers
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_pipelines_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_pipelines_updated_at
    BEFORE UPDATE ON pipelines
    FOR EACH ROW
    EXECUTE FUNCTION update_pipelines_updated_at();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_pipeline_stages_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_pipeline_stages_updated_at
    BEFORE UPDATE ON pipeline_stages
    FOR EACH ROW
    EXECUTE FUNCTION update_pipeline_stages_updated_at();

-- +goose Down
-- [H-4] Drop indexes first, then columns, then tables.
DROP INDEX IF EXISTS idx_issues_current_stage_id;
DROP INDEX IF EXISTS idx_issues_pipeline_id;
ALTER TABLE issues DROP CONSTRAINT IF EXISTS chk_issues_pipeline_stage_consistency;
ALTER TABLE issues DROP COLUMN IF EXISTS current_stage_id;
ALTER TABLE issues DROP COLUMN IF EXISTS pipeline_id;
DROP TRIGGER IF EXISTS trg_pipeline_stages_updated_at ON pipeline_stages;
DROP FUNCTION IF EXISTS update_pipeline_stages_updated_at;
DROP TRIGGER IF EXISTS trg_pipelines_updated_at ON pipelines;
DROP FUNCTION IF EXISTS update_pipelines_updated_at;
DROP TABLE IF EXISTS pipeline_stages;
DROP TABLE IF EXISTS pipelines;
