-- name: CreatePipeline :one
INSERT INTO pipelines (squad_id, name, description)
VALUES (@squad_id, @name, @description)
RETURNING *;

-- name: GetPipelineByID :one
SELECT * FROM pipelines WHERE id = @id;

-- name: ListPipelinesBySquad :many
SELECT * FROM pipelines
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_is_active')::boolean IS NULL OR is_active = sqlc.narg('filter_is_active'))
ORDER BY name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPipelinesBySquad :one
SELECT count(*) FROM pipelines
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_is_active')::boolean IS NULL OR is_active = sqlc.narg('filter_is_active'));

-- name: UpdatePipeline :one
UPDATE pipelines
SET
    name        = COALESCE(sqlc.narg('name'), name),
    description = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    is_active   = COALESCE(sqlc.narg('is_active'), is_active)
WHERE id = @id
RETURNING *;

-- name: DeletePipeline :exec
DELETE FROM pipelines WHERE id = @id;

-- name: CountIssuesInPipeline :one
SELECT count(*) FROM issues WHERE pipeline_id = @pipeline_id;
