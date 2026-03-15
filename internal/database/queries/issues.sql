-- name: CreateIssue :one
INSERT INTO issues (
    squad_id, identifier, type, title, description, status, priority,
    parent_id, project_id, goal_id, assignee_agent_id, assignee_user_id,
    billing_code, request_depth
) VALUES (
    @squad_id, @identifier, @type, @title, @description, @status, @priority,
    @parent_id, @project_id, @goal_id, @assignee_agent_id, @assignee_user_id,
    @billing_code, @request_depth
)
RETURNING *;

-- name: GetIssueByID :one
SELECT * FROM issues WHERE id = @id;

-- name: GetIssueByIdentifier :one
SELECT * FROM issues WHERE squad_id = @squad_id AND identifier = @identifier;

-- name: UpdateIssue :one
UPDATE issues
SET
    title             = COALESCE(sqlc.narg('title'), title),
    description       = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    type              = COALESCE(sqlc.narg('type'), type),
    status            = COALESCE(sqlc.narg('status'), status),
    priority          = COALESCE(sqlc.narg('priority'), priority),
    parent_id         = CASE WHEN sqlc.arg('set_parent')::boolean THEN sqlc.narg('parent_id') ELSE parent_id END,
    project_id        = CASE WHEN sqlc.arg('set_project')::boolean THEN sqlc.narg('project_id') ELSE project_id END,
    goal_id           = CASE WHEN sqlc.arg('set_goal')::boolean THEN sqlc.narg('goal_id') ELSE goal_id END,
    assignee_agent_id = CASE WHEN sqlc.arg('set_assignee_agent')::boolean THEN sqlc.narg('assignee_agent_id') ELSE assignee_agent_id END,
    assignee_user_id  = CASE WHEN sqlc.arg('set_assignee_user')::boolean THEN sqlc.narg('assignee_user_id') ELSE assignee_user_id END,
    billing_code      = CASE WHEN sqlc.arg('set_billing_code')::boolean THEN sqlc.narg('billing_code') ELSE billing_code END
WHERE id = @id
RETURNING *;

-- name: DeleteIssue :exec
DELETE FROM issues WHERE id = @id;

-- name: CountSubTasks :one
SELECT count(*) FROM issues WHERE parent_id = @parent_id;

-- name: ListIssuesBySquad :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL           OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_priority')::issue_priority IS NULL       OR priority = sqlc.narg('filter_priority'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL               OR type = sqlc.narg('filter_type'))
  AND (sqlc.narg('filter_assignee_agent_id')::UUID IS NULL        OR assignee_agent_id = sqlc.narg('filter_assignee_agent_id'))
  AND (sqlc.narg('filter_assignee_user_id')::UUID IS NULL         OR assignee_user_id = sqlc.narg('filter_assignee_user_id'))
  AND (sqlc.narg('filter_project_id')::UUID IS NULL               OR project_id = sqlc.narg('filter_project_id'))
  AND (sqlc.narg('filter_goal_id')::UUID IS NULL                  OR goal_id = sqlc.narg('filter_goal_id'))
  AND (sqlc.narg('filter_parent_id')::UUID IS NULL                OR parent_id = sqlc.narg('filter_parent_id'))
  AND (sqlc.narg('filter_pipeline_id')::UUID IS NULL              OR pipeline_id = sqlc.narg('filter_pipeline_id'))
ORDER BY
    CASE WHEN @sort_field::TEXT = 'created_at'  THEN created_at END DESC,
    CASE WHEN @sort_field::TEXT = 'updated_at'  THEN updated_at END DESC,
    CASE WHEN @sort_field::TEXT = 'priority'    THEN priority   END ASC,
    CASE WHEN @sort_field::TEXT = 'status'      THEN status     END ASC,
    created_at DESC
LIMIT  @page_limit
OFFSET @page_offset;

-- name: CountIssuesBySquad :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL           OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_priority')::issue_priority IS NULL       OR priority = sqlc.narg('filter_priority'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL               OR type = sqlc.narg('filter_type'))
  AND (sqlc.narg('filter_assignee_agent_id')::UUID IS NULL        OR assignee_agent_id = sqlc.narg('filter_assignee_agent_id'))
  AND (sqlc.narg('filter_assignee_user_id')::UUID IS NULL         OR assignee_user_id = sqlc.narg('filter_assignee_user_id'))
  AND (sqlc.narg('filter_project_id')::UUID IS NULL               OR project_id = sqlc.narg('filter_project_id'))
  AND (sqlc.narg('filter_goal_id')::UUID IS NULL                  OR goal_id = sqlc.narg('filter_goal_id'))
  AND (sqlc.narg('filter_parent_id')::UUID IS NULL                OR parent_id = sqlc.narg('filter_parent_id'))
  AND (sqlc.narg('filter_pipeline_id')::UUID IS NULL              OR pipeline_id = sqlc.narg('filter_pipeline_id'));

-- name: ListIssuesByAssigneeAgent :many
SELECT * FROM issues
WHERE assignee_agent_id = @agent_id
  AND type != 'conversation'
  AND status NOT IN ('done', 'cancelled')
ORDER BY created_at DESC;

-- name: ListConversationsByAgent :many
SELECT * FROM issues
WHERE type = 'conversation'
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
ORDER BY updated_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountConversationsByAgent :one
SELECT count(*) FROM issues
WHERE type = 'conversation'
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'));

-- name: UpdateIssuePipeline :one
UPDATE issues
SET
    pipeline_id       = sqlc.narg('pipeline_id'),
    current_stage_id  = sqlc.narg('current_stage_id'),
    assignee_agent_id = CASE WHEN sqlc.arg('set_assignee')::boolean THEN sqlc.narg('assignee_agent_id') ELSE assignee_agent_id END,
    status            = CASE WHEN sqlc.arg('set_status')::boolean THEN sqlc.narg('status')::issue_status ELSE status END
WHERE id = @id
RETURNING *;

-- name: AdvanceIssuePipelineStage :one
-- CAS guard: only advances if current_stage_id matches expected value.
-- Prevents concurrent double-advancement.
UPDATE issues
SET
    current_stage_id  = sqlc.narg('next_stage_id'),
    assignee_agent_id = CASE WHEN sqlc.arg('set_assignee')::boolean THEN sqlc.narg('assignee_agent_id') ELSE assignee_agent_id END,
    status            = CASE WHEN sqlc.arg('set_status')::boolean THEN sqlc.narg('status')::issue_status ELSE status END
WHERE id = @id
  AND current_stage_id = @expected_stage_id
RETURNING *;

-- name: ListAssignmentsByAgent :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAssignmentsByAgent :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'));

-- name: ListAssignmentsByAgentIDs :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = ANY(@agent_ids::UUID[])
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAssignmentsByAgentIDs :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = ANY(@agent_ids::UUID[])
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'));

-- name: IncrementSquadIssueCounter :one
UPDATE squads
SET issue_counter = issue_counter + 1,
    updated_at = now()
WHERE id = @id
RETURNING issue_prefix, issue_counter;
