-- name: CreateGoal :one
INSERT INTO goals (squad_id, parent_id, title, description)
VALUES (@squad_id, @parent_id, @title, @description)
RETURNING *;

-- name: GetGoalByID :one
SELECT * FROM goals WHERE id = @id;

-- name: ListGoalsBySquad :many
SELECT * FROM goals WHERE squad_id = @squad_id ORDER BY created_at DESC;

-- name: ListTopLevelGoalsBySquad :many
SELECT * FROM goals WHERE squad_id = @squad_id AND parent_id IS NULL ORDER BY created_at DESC;

-- name: ListGoalsBySquadAndParent :many
SELECT * FROM goals WHERE squad_id = @squad_id AND parent_id = @parent_id ORDER BY created_at DESC;

-- name: UpdateGoal :one
UPDATE goals
SET
    title       = COALESCE(sqlc.narg('title'), title),
    description = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    parent_id   = CASE WHEN sqlc.arg('set_parent')::boolean THEN sqlc.narg('parent_id') ELSE parent_id END,
    status      = COALESCE(sqlc.narg('status'), status)
WHERE id = @id
RETURNING *;

-- name: GetGoalAncestors :many
WITH RECURSIVE ancestors AS (
    SELECT parent_id, 1 AS depth
    FROM goals
    WHERE goals.id = @id
    UNION ALL
    SELECT g.parent_id, a.depth + 1
    FROM goals g
    JOIN ancestors a ON g.id = a.parent_id
    WHERE a.depth < 6 AND a.parent_id IS NOT NULL
)
SELECT parent_id::UUID AS ancestor_id FROM ancestors WHERE parent_id IS NOT NULL;

-- name: CountGoalChildren :one
SELECT count(*) FROM goals WHERE parent_id = @parent_id;

-- name: GetGoalMaxSubtreeDepth :one
WITH RECURSIVE subtree AS (
    SELECT goals.id, 1 AS depth
    FROM goals
    WHERE goals.parent_id = @goal_id
    UNION ALL
    SELECT g.id, s.depth + 1
    FROM goals g
    JOIN subtree s ON g.parent_id = s.id
    WHERE s.depth < 10
)
SELECT COALESCE(MAX(depth), 0)::bigint AS max_depth FROM subtree;
