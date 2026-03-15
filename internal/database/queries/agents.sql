-- name: CreateAgent :one
INSERT INTO agents (
    squad_id, name, short_name, role, status,
    parent_agent_id, adapter_type, adapter_config,
    system_prompt, model, budget_monthly_cents
) VALUES (
    @squad_id, @name, @short_name, @role, @status,
    @parent_agent_id, @adapter_type, @adapter_config,
    @system_prompt, @model, @budget_monthly_cents
)
RETURNING *;

-- name: GetAgentByID :one
SELECT * FROM agents
WHERE id = @id;

-- name: ListAgentsBySquad :many
SELECT * FROM agents
WHERE squad_id = @squad_id
ORDER BY created_at ASC;

-- name: UpdateAgent :one
UPDATE agents SET
    name = COALESCE(sqlc.narg('name'), name),
    short_name = COALESCE(sqlc.narg('short_name'), short_name),
    role = COALESCE(sqlc.narg('role'), role),
    status = COALESCE(sqlc.narg('status'), status),
    parent_agent_id = CASE
        WHEN sqlc.arg('set_parent')::boolean THEN sqlc.narg('parent_agent_id')
        ELSE parent_agent_id
    END,
    adapter_type = COALESCE(sqlc.narg('adapter_type'), adapter_type),
    adapter_config = COALESCE(sqlc.narg('adapter_config'), adapter_config),
    system_prompt = COALESCE(sqlc.narg('system_prompt'), system_prompt),
    model = COALESCE(sqlc.narg('model'), model),
    budget_monthly_cents = CASE
        WHEN sqlc.arg('set_budget')::boolean THEN sqlc.narg('budget_monthly_cents')
        ELSE budget_monthly_cents
    END
WHERE id = @id
RETURNING *;

-- name: GetSquadCaptain :one
SELECT * FROM agents
WHERE squad_id = @squad_id
  AND role = 'captain'
  AND status != 'terminated'
LIMIT 1;

-- name: GetAgentParent :one
SELECT id, squad_id, role FROM agents
WHERE id = @id;

-- NOTE: CheckCycleInHierarchy uses a recursive CTE which sqlc cannot parse.
-- It is implemented as a raw SQL query in the repository layer. The query is:
--
-- WITH RECURSIVE ancestors AS (
--     SELECT id, parent_agent_id, 1 AS depth
--     FROM agents WHERE id = $1
--     UNION ALL
--     SELECT a.id, a.parent_agent_id, anc.depth + 1
--     FROM agents a JOIN ancestors anc ON a.id = anc.parent_agent_id
--     WHERE anc.depth < 10
-- )
-- SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $2) AS would_cycle;

-- name: CountAgentsBySquad :one
SELECT COUNT(*) FROM agents
WHERE squad_id = @squad_id
  AND status != 'terminated';

-- name: ListAgentChildren :many
SELECT * FROM agents
WHERE parent_agent_id = @parent_agent_id
ORDER BY created_at ASC;

-- name: ListAgentChildrenBySquad :many
SELECT * FROM agents
WHERE squad_id = @squad_id
  AND parent_agent_id = @parent_agent_id
ORDER BY created_at ASC;
