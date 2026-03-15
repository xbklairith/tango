-- name: CreatePipelineStage :one
INSERT INTO pipeline_stages (pipeline_id, name, description, position, assigned_agent_id)
VALUES (@pipeline_id, @name, @description, @position, @assigned_agent_id)
RETURNING *;

-- name: GetPipelineStageByID :one
SELECT * FROM pipeline_stages WHERE id = @id;

-- name: ListStagesByPipeline :many
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
ORDER BY position ASC;

-- name: UpdatePipelineStage :one
UPDATE pipeline_stages
SET
    name              = COALESCE(sqlc.narg('name'), name),
    description       = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    position          = COALESCE(sqlc.narg('position'), position),
    assigned_agent_id = CASE WHEN sqlc.arg('set_agent')::boolean THEN sqlc.narg('assigned_agent_id') ELSE assigned_agent_id END
WHERE id = @id
RETURNING *;

-- name: DeletePipelineStage :exec
DELETE FROM pipeline_stages WHERE id = @id;

-- name: CountStagesByPipeline :one
SELECT count(*) FROM pipeline_stages WHERE pipeline_id = @pipeline_id;

-- name: CountIssuesAtStage :one
SELECT count(*) FROM issues WHERE current_stage_id = @stage_id;

-- name: GetFirstStage :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
ORDER BY position ASC
LIMIT 1;

-- name: GetNextStage :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
  AND position > @current_position
ORDER BY position ASC
LIMIT 1;

-- name: GetPreviousStage :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
  AND position < @current_position
ORDER BY position DESC
LIMIT 1;

-- name: ReorderPipelineStages :exec
UPDATE pipeline_stages
SET position = @new_position
WHERE id = @id AND pipeline_id = @pipeline_id;
