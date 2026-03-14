# Design: Issue Tracking

**Created:** 2026-03-14
**Status:** Complete
**Feature:** 05-issue-tracking
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management, 04-agent-management

---

## 1. Architecture Overview

Issue Tracking is the core work-management layer of Ari. It sits between squad/agent management (organizational structure) and execution (Phase 2 checkout/pipeline). Every unit of work is an **Issue** -- a squad-scoped entity with an auto-generated human-readable identifier (e.g., "ARI-39"), a validated status lifecycle, optional sub-task hierarchy, and a threaded comment trail.

The feature introduces two database tables (`issues`, `issue_comments`), one domain package (`internal/domain/issue.go`), one handler file (`internal/server/handlers/issue_handler.go`), and corresponding sqlc queries. All endpoints are JWT-authenticated and enforce squad-scoped data isolation through middleware that verifies the caller's `SquadMembership`.

```
HTTP Request
     |
     v
AuthMiddleware (JWT)
     |
     v
SquadScopeMiddleware (verify membership)
     |
     v
IssueHandler (validate, call DB)
     |
     +---> sqlc Queries (issues, issue_comments)
     |         |
     |         v
     |    PostgreSQL (atomic counter, FK constraints)
     |
     v
JSON Response
```

---

## 2. System Context

- **Depends On:**
  - `01-go-scaffold` -- HTTP server, router, middleware chain, error response helpers
  - `02-user-auth` -- JWT authentication middleware, `UserFromContext(ctx)`
  - `03-squad-management` -- `squads` table (issuePrefix, issueCounter atomic increment), `squad_memberships` table (access checks)
  - `04-agent-management` -- `agents` table (assignee validation)

- **Used By:**
  - React UI dashboard (issue board, detail views, comments)
  - Agent runtime adapters (agents create sub-tasks, post comments via API)
  - Phase 2 features: checkout/lock, pipeline stages, conversations

- **External Dependencies:** None (all PostgreSQL-based)

---

## 3. Component Structure

### 3.1 Domain Types -- `internal/domain/issue.go`

```go
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// -------- Enums --------

type IssueType string

const (
	IssueTypeTask         IssueType = "task"
	IssueTypeConversation IssueType = "conversation"
)

func (t IssueType) Valid() bool {
	switch t {
	case IssueTypeTask, IssueTypeConversation:
		return true
	}
	return false
}

type IssueStatus string

const (
	IssueStatusBacklog    IssueStatus = "backlog"
	IssueStatusTodo       IssueStatus = "todo"
	IssueStatusInProgress IssueStatus = "in_progress"
	IssueStatusDone       IssueStatus = "done"
	IssueStatusBlocked    IssueStatus = "blocked"
	IssueStatusCancelled  IssueStatus = "cancelled"
)

func (s IssueStatus) Valid() bool {
	switch s {
	case IssueStatusBacklog, IssueStatusTodo, IssueStatusInProgress,
		IssueStatusDone, IssueStatusBlocked, IssueStatusCancelled:
		return true
	}
	return false
}

type IssuePriority string

const (
	IssuePriorityCritical IssuePriority = "critical"
	IssuePriorityHigh     IssuePriority = "high"
	IssuePriorityMedium   IssuePriority = "medium"
	IssuePriorityLow      IssuePriority = "low"
)

func (p IssuePriority) Valid() bool {
	switch p {
	case IssuePriorityCritical, IssuePriorityHigh, IssuePriorityMedium, IssuePriorityLow:
		return true
	}
	return false
}

type CommentAuthorType string

const (
	CommentAuthorAgent  CommentAuthorType = "agent"
	CommentAuthorUser   CommentAuthorType = "user"
	CommentAuthorSystem CommentAuthorType = "system"
)

func (a CommentAuthorType) Valid() bool {
	switch a {
	case CommentAuthorAgent, CommentAuthorUser, CommentAuthorSystem:
		return true
	}
	return false
}

// -------- Status Machine --------

// validTransitions defines every legal (from -> to) pair.
var validTransitions = map[IssueStatus][]IssueStatus{
	IssueStatusBacklog:    {IssueStatusTodo, IssueStatusInProgress, IssueStatusCancelled},
	IssueStatusTodo:       {IssueStatusInProgress, IssueStatusBacklog, IssueStatusBlocked, IssueStatusCancelled},
	IssueStatusInProgress: {IssueStatusDone, IssueStatusBlocked, IssueStatusCancelled},
	IssueStatusBlocked:    {IssueStatusInProgress, IssueStatusTodo, IssueStatusCancelled},
	IssueStatusDone:       {IssueStatusTodo},
	IssueStatusCancelled:  {IssueStatusTodo},
}

// ValidateTransition checks whether moving from `from` to `to` is allowed.
// Returns nil on success, a descriptive error on failure.
func ValidateTransition(from, to IssueStatus) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown current status %q", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("cannot transition from %q to %q", from, to)
}

// IsReopen returns true when the transition represents a reopen event
// (done->todo or cancelled->todo). Reopens generate a system comment.
func IsReopen(from, to IssueStatus) bool {
	return to == IssueStatusTodo &&
		(from == IssueStatusDone || from == IssueStatusCancelled)
}

// -------- Domain Models --------

type Issue struct {
	ID               uuid.UUID      `json:"id"`
	SquadID          uuid.UUID      `json:"squadId"`
	Identifier       string         `json:"identifier"`
	Type             IssueType      `json:"type"`
	Title            string         `json:"title"`
	Description      *string        `json:"description,omitempty"`
	Status           IssueStatus    `json:"status"`
	Priority         IssuePriority  `json:"priority"`
	ParentID         *uuid.UUID     `json:"parentId,omitempty"`
	ProjectID        *uuid.UUID     `json:"projectId,omitempty"`
	GoalID           *uuid.UUID     `json:"goalId,omitempty"`
	AssigneeAgentID  *uuid.UUID     `json:"assigneeAgentId,omitempty"`
	AssigneeUserID   *uuid.UUID     `json:"assigneeUserId,omitempty"`
	BillingCode      *string        `json:"billingCode,omitempty"`
	RequestDepth     int            `json:"requestDepth"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type IssueComment struct {
	ID         uuid.UUID         `json:"id"`
	IssueID    uuid.UUID         `json:"issueId"`
	AuthorType CommentAuthorType `json:"authorType"`
	AuthorID   uuid.UUID         `json:"authorId"`
	Body       string            `json:"body"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// -------- Request / Response DTOs --------

type CreateIssueRequest struct {
	Title           string         `json:"title"`
	Description     *string        `json:"description,omitempty"`
	Type            *IssueType     `json:"type,omitempty"`            // default: task
	Status          *IssueStatus   `json:"status,omitempty"`          // default: backlog
	Priority        *IssuePriority `json:"priority,omitempty"`        // default: medium
	ParentID        *uuid.UUID     `json:"parentId,omitempty"`
	ProjectID       *uuid.UUID     `json:"projectId,omitempty"`
	GoalID          *uuid.UUID     `json:"goalId,omitempty"`
	AssigneeAgentID *uuid.UUID     `json:"assigneeAgentId,omitempty"`
	AssigneeUserID  *uuid.UUID     `json:"assigneeUserId,omitempty"`
	BillingCode     *string        `json:"billingCode,omitempty"`
	RequestDepth    *int           `json:"requestDepth,omitempty"`    // default: 0
}

type UpdateIssueRequest struct {
	Title           *string        `json:"title,omitempty"`
	Description     *string        `json:"description,omitempty"`
	Type            *IssueType     `json:"type,omitempty"`
	Status          *IssueStatus   `json:"status,omitempty"`
	Priority        *IssuePriority `json:"priority,omitempty"`
	ParentID        *uuid.UUID     `json:"parentId,omitempty"`
	ProjectID       *uuid.UUID     `json:"projectId,omitempty"`
	GoalID          *uuid.UUID     `json:"goalId,omitempty"`
	AssigneeAgentID *uuid.UUID     `json:"assigneeAgentId,omitempty"`
	AssigneeUserID  *uuid.UUID     `json:"assigneeUserId,omitempty"`
	BillingCode     *string        `json:"billingCode,omitempty"`
}

type CreateCommentRequest struct {
	AuthorType CommentAuthorType `json:"authorType"`
	AuthorID   uuid.UUID         `json:"authorId"`
	Body       string            `json:"body"`
}

type IssueListParams struct {
	SquadID         uuid.UUID      `json:"-"`
	Status          *IssueStatus   `json:"status,omitempty"`
	Priority        *IssuePriority `json:"priority,omitempty"`
	Type            *IssueType     `json:"type,omitempty"`
	AssigneeAgentID *uuid.UUID     `json:"assigneeAgentId,omitempty"`
	AssigneeUserID  *uuid.UUID     `json:"assigneeUserId,omitempty"`
	ProjectID       *uuid.UUID     `json:"projectId,omitempty"`
	GoalID          *uuid.UUID     `json:"goalId,omitempty"`
	ParentID        *uuid.UUID     `json:"parentId,omitempty"`
	Sort            string         `json:"sort,omitempty"`   // created_at, updated_at, priority, status
	Limit           int            `json:"limit,omitempty"`  // default 50, max 200
	Offset          int            `json:"offset,omitempty"`
}
```

### 3.2 HTTP Handler -- `internal/server/handlers/issue_handler.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"ari/internal/database/db"
	"ari/internal/domain"
)

// identifierPattern matches "PREFIX-123" format.
var identifierPattern = regexp.MustCompile(`^[A-Z]{2,10}-\d+$`)

type IssueHandler struct {
	queries *db.Queries
}

func NewIssueHandler(q *db.Queries) *IssueHandler {
	return &IssueHandler{queries: q}
}

// RegisterRoutes wires all issue and comment routes into the mux.
func (h *IssueHandler) RegisterRoutes(mux *http.ServeMux) {
	// Squad-scoped issue list + create
	mux.HandleFunc("POST /api/squads/{squadId}/issues", h.CreateIssue)
	mux.HandleFunc("GET /api/squads/{squadId}/issues", h.ListIssues)

	// Issue by ID or identifier
	mux.HandleFunc("GET /api/issues/{id}", h.GetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", h.UpdateIssue)
	mux.HandleFunc("DELETE /api/issues/{id}", h.DeleteIssue)

	// Comments
	mux.HandleFunc("POST /api/issues/{issueId}/comments", h.CreateComment)
	mux.HandleFunc("GET /api/issues/{issueId}/comments", h.ListComments)
}

// CreateIssue handles POST /api/squads/:squadId/issues.
//
// Flow:
//  1. Parse & validate request body
//  2. Verify squad membership (via middleware or inline)
//  3. Apply defaults (status=backlog, priority=medium, type=task, requestDepth=0)
//  4. Validate parentId same-squad + no circular refs
//  5. Validate assigneeAgentId / assigneeUserId belong to squad
//  6. Atomically increment squad.issueCounter, generate identifier
//  7. INSERT issue
//  8. Return 201 + issue JSON
func (h *IssueHandler) CreateIssue(w http.ResponseWriter, r *http.Request) {
	// Implementation follows the flow above.
	// See Section 5 for identifier generation detail.
}

// GetIssue handles GET /api/issues/:id.
// Supports both UUID and identifier lookup (see Section 8).
func (h *IssueHandler) GetIssue(w http.ResponseWriter, r *http.Request) {
	idParam := r.PathValue("id")

	if identifierPattern.MatchString(idParam) {
		// Identifier lookup: split on "-", query by squad prefix + number
		// See Section 8 for full logic.
		return
	}

	// UUID lookup
	issueID, err := uuid.Parse(idParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "id must be a UUID or identifier like ARI-39")
		return
	}
	_ = issueID
	// ... fetch by UUID, verify squad access, return JSON
}

// UpdateIssue handles PATCH /api/issues/:id.
//
// Flow:
//  1. Fetch existing issue (404 if missing)
//  2. Verify squad membership
//  3. If status changed -> validate transition (Section 6)
//  4. If parentId changed -> validate same-squad + no cycles (Section 7)
//  5. If assignee changed -> validate belongs to squad
//  6. UPDATE issue, set updatedAt = now()
//  7. If reopen -> insert system comment (REQ-ISS-022)
//  8. Return 200 + updated issue JSON
func (h *IssueHandler) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	// Implementation follows the flow above.
}

// DeleteIssue handles DELETE /api/issues/:id.
// Rejects deletion if the issue has active sub-tasks (REQ-ISS-032).
func (h *IssueHandler) DeleteIssue(w http.ResponseWriter, r *http.Request) {
	// 1. Fetch issue, verify access
	// 2. Check for sub-tasks: SELECT count(*) FROM issues WHERE parent_id = $1
	// 3. If count > 0 -> 409 CONFLICT "cannot delete issue with sub-tasks"
	// 4. DELETE FROM issues WHERE id = $1
	// 5. Return 200
}

// ListIssues handles GET /api/squads/:squadId/issues with filtering + pagination.
func (h *IssueHandler) ListIssues(w http.ResponseWriter, r *http.Request) {
	// Parse query params into IssueListParams (Section 9)
	// Execute filtered query
	// Return JSON array with pagination metadata
}

// CreateComment handles POST /api/issues/:issueId/comments.
func (h *IssueHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	// 1. Validate body non-empty (REQ-ISS-062)
	// 2. Validate authorType enum
	// 3. If authorType=agent -> validate agent exists
	//    If authorType=user  -> validate user exists
	// 4. INSERT comment
	// 5. Return 201
}

// ListComments handles GET /api/issues/:issueId/comments.
func (h *IssueHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	// Parse limit/offset, default sort by created_at ASC
	// Return JSON array
}

// --- helpers ---

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  code,
	})
}
```

---

## 4. Database Schema

### 4.1 Migration: Create Issues Table

**File:** `internal/database/migrations/XXXXXX_create_issues.sql`

```sql
-- +goose Up

CREATE TYPE issue_type AS ENUM ('task', 'conversation');
CREATE TYPE issue_status AS ENUM ('backlog', 'todo', 'in_progress', 'done', 'blocked', 'cancelled');
CREATE TYPE issue_priority AS ENUM ('critical', 'high', 'medium', 'low');

CREATE TABLE issues (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id          UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    identifier        TEXT NOT NULL,
    type              issue_type NOT NULL DEFAULT 'task',
    title             TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 500),
    description       TEXT,
    status            issue_status NOT NULL DEFAULT 'backlog',
    priority          issue_priority NOT NULL DEFAULT 'medium',
    parent_id         UUID REFERENCES issues(id) ON DELETE SET NULL,
    project_id        UUID,  -- FK added when projects table exists
    goal_id           UUID,  -- FK added when goals table exists
    assignee_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    assignee_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    billing_code      TEXT,
    request_depth     INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Identifier must be unique within a squad
    CONSTRAINT uq_issues_squad_identifier UNIQUE (squad_id, identifier),

    -- An issue cannot be its own parent
    CONSTRAINT ck_issues_no_self_parent CHECK (parent_id IS NULL OR parent_id != id)
);

-- Performance indexes (REQ-ISS-NF-003)
CREATE INDEX idx_issues_squad_id          ON issues (squad_id);
CREATE INDEX idx_issues_status            ON issues (squad_id, status);
CREATE INDEX idx_issues_priority          ON issues (squad_id, priority);
CREATE INDEX idx_issues_assignee_agent_id ON issues (assignee_agent_id) WHERE assignee_agent_id IS NOT NULL;
CREATE INDEX idx_issues_assignee_user_id  ON issues (assignee_user_id)  WHERE assignee_user_id IS NOT NULL;
CREATE INDEX idx_issues_project_id        ON issues (project_id)        WHERE project_id IS NOT NULL;
CREATE INDEX idx_issues_goal_id           ON issues (goal_id)           WHERE goal_id IS NOT NULL;
CREATE INDEX idx_issues_parent_id         ON issues (parent_id)         WHERE parent_id IS NOT NULL;
CREATE INDEX idx_issues_identifier        ON issues (identifier);
CREATE INDEX idx_issues_squad_created_at  ON issues (squad_id, created_at DESC);
CREATE INDEX idx_issues_squad_updated_at  ON issues (squad_id, updated_at DESC);

-- Trigger: auto-update updated_at on row change
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_issues_updated_at
    BEFORE UPDATE ON issues
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER IF EXISTS trg_issues_updated_at ON issues;
DROP TABLE IF EXISTS issues;
DROP TYPE IF EXISTS issue_priority;
DROP TYPE IF EXISTS issue_status;
DROP TYPE IF EXISTS issue_type;
```

### 4.2 Migration: Create Issue Comments Table

**File:** `internal/database/migrations/XXXXXX_create_issue_comments.sql`

```sql
-- +goose Up

CREATE TYPE comment_author_type AS ENUM ('agent', 'user', 'system');

CREATE TABLE issue_comments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id    UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author_type comment_author_type NOT NULL,
    author_id   UUID NOT NULL,
    body        TEXT NOT NULL CHECK (char_length(body) >= 1),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_issue_comments_issue_id ON issue_comments (issue_id, created_at ASC);

-- +goose Down
DROP TABLE IF EXISTS issue_comments;
DROP TYPE IF EXISTS comment_author_type;
```

### 4.3 sqlc Queries

**File:** `internal/database/queries/issues.sql`

```sql
-- name: CreateIssue :one
INSERT INTO issues (
    squad_id, identifier, type, title, description, status, priority,
    parent_id, project_id, goal_id, assignee_agent_id, assignee_user_id,
    billing_code, request_depth
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetIssueByID :one
SELECT * FROM issues WHERE id = $1;

-- name: GetIssueByIdentifier :one
SELECT * FROM issues WHERE squad_id = $1 AND identifier = $2;

-- name: UpdateIssue :one
UPDATE issues
SET
    title             = COALESCE(sqlc.narg('title'), title),
    description       = COALESCE(sqlc.narg('description'), description),
    type              = COALESCE(sqlc.narg('type'), type),
    status            = COALESCE(sqlc.narg('status'), status),
    priority          = COALESCE(sqlc.narg('priority'), priority),
    parent_id         = sqlc.narg('parent_id'),
    project_id        = sqlc.narg('project_id'),
    goal_id           = sqlc.narg('goal_id'),
    assignee_agent_id = sqlc.narg('assignee_agent_id'),
    assignee_user_id  = sqlc.narg('assignee_user_id'),
    billing_code      = sqlc.narg('billing_code')
WHERE id = $1
RETURNING *;

-- **Design note on COALESCE/sqlc.narg pattern:**
-- For non-nullable fields (title, type, status, priority), COALESCE(sqlc.narg('field'), field)
-- means passing NULL leaves the field unchanged ("not provided"). This works correctly.
-- For nullable fields (parent_id, project_id, goal_id, assignee_agent_id, assignee_user_id,
-- billing_code), we use bare sqlc.narg('field') instead of COALESCE so that callers CAN
-- explicitly set the field to NULL. However, this means the handler layer CANNOT distinguish
-- "field not provided" from "set to NULL" -- both arrive as sql.NullXxx with Valid=false.
-- The handler MUST therefore require the client to always send the full set of nullable fields
-- on PATCH, or implement a separate mechanism (e.g., a field mask or a sentinel value) to
-- distinguish the two cases. In Phase 1, the convention is: if a nullable field is omitted
-- from the JSON body, the handler passes the field's CURRENT value (read-before-write),
-- and only passes NULL when the client explicitly sends `"field": null`.

-- name: DeleteIssue :exec
DELETE FROM issues WHERE id = $1;

-- name: CountSubTasks :one
SELECT count(*) FROM issues WHERE parent_id = $1;

-- name: ListIssuesBySquad :many
SELECT * FROM issues
WHERE squad_id = $1
  AND (sqlc.narg('status')::issue_status IS NULL           OR status = sqlc.narg('status'))
  AND (sqlc.narg('priority')::issue_priority IS NULL       OR priority = sqlc.narg('priority'))
  AND (sqlc.narg('type')::issue_type IS NULL               OR type = sqlc.narg('type'))
  AND (sqlc.narg('assignee_agent_id')::UUID IS NULL        OR assignee_agent_id = sqlc.narg('assignee_agent_id'))
  AND (sqlc.narg('assignee_user_id')::UUID IS NULL         OR assignee_user_id = sqlc.narg('assignee_user_id'))
  AND (sqlc.narg('project_id')::UUID IS NULL               OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('goal_id')::UUID IS NULL                  OR goal_id = sqlc.narg('goal_id'))
  AND (sqlc.narg('parent_id')::UUID IS NULL                OR parent_id = sqlc.narg('parent_id'))
ORDER BY
    CASE WHEN @sort_field::TEXT = 'created_at'  THEN created_at END DESC,
    CASE WHEN @sort_field::TEXT = 'updated_at'  THEN updated_at END DESC,
    CASE WHEN @sort_field::TEXT = 'priority'    THEN priority   END ASC,
    CASE WHEN @sort_field::TEXT = 'status'      THEN status     END ASC,
    created_at DESC
LIMIT  @page_limit
OFFSET @page_offset;

-- name: IncrementSquadIssueCounter :one
-- Atomically increments the squad's issue_counter and returns the new value + prefix.
UPDATE squads
SET issue_counter = issue_counter + 1,
    updated_at = now()
WHERE id = $1
RETURNING issue_prefix, issue_counter;

-- name: GetIssueAncestors :many
-- Recursive CTE to fetch all ancestors of an issue (for cycle detection).
WITH RECURSIVE ancestors AS (
    SELECT id, parent_id, 1 AS depth
    FROM issues
    WHERE id = $1
    UNION ALL
    SELECT i.id, i.parent_id, a.depth + 1
    FROM issues i
    INNER JOIN ancestors a ON a.parent_id = i.id
    WHERE a.depth < 100  -- safety limit
)
SELECT id FROM ancestors;
```

**File:** `internal/database/queries/issue_comments.sql`

```sql
-- name: CreateIssueComment :one
INSERT INTO issue_comments (issue_id, author_type, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListIssueComments :many
SELECT * FROM issue_comments
WHERE issue_id = $1
ORDER BY created_at ASC
LIMIT  @page_limit
OFFSET @page_offset;

-- name: CountIssueComments :one
SELECT count(*) FROM issue_comments WHERE issue_id = $1;
```

---

## 5. Issue Identifier Generation

Identifiers are generated atomically using the squad's `issue_counter` column. The operation uses a single `UPDATE ... RETURNING` statement that both increments the counter and returns the new value, guaranteeing uniqueness under concurrent requests (REQ-ISS-003, REQ-SM-042, REQ-ISS-NF-004).

### Algorithm

```go
// generateIdentifier atomically increments the squad counter and returns
// the new identifier string (e.g., "ARI-39").
// IMPORTANT: Must be called within a transaction (using qtx) so the counter
// increment and issue INSERT are atomic. See createIssueInTx below.
func (h *IssueHandler) generateIdentifier(ctx context.Context, qtx *db.Queries, squadID uuid.UUID) (string, error) {
	// Single atomic query: UPDATE squads SET issue_counter = issue_counter + 1
	// WHERE id = $1 RETURNING issue_prefix, issue_counter;
	// The UPDATE acquires a row-level lock, serializing concurrent callers.
	row, err := qtx.IncrementSquadIssueCounter(ctx, squadID)
	if err != nil {
		return "", fmt.Errorf("increment issue counter: %w", err)
	}
	return fmt.Sprintf("%s-%d", row.IssuePrefix, row.IssueCounter), nil
}
```

### Concurrency guarantee

- PostgreSQL's `UPDATE ... RETURNING` acquires a row-level lock on the squad row.
- Concurrent transactions will serialize on this lock -- one completes and increments, then the next one increments from the new value.
- No application-level locking or distributed locks are required.
- Gaps in the sequence are acceptable after failed/rolled-back transactions.

### Transaction boundary

The identifier generation and issue INSERT must be in the **same database transaction**. If the INSERT fails after the counter increment, the transaction rolls back, and the counter is not incremented:

```go
func (h *IssueHandler) createIssueInTx(ctx context.Context, squadID uuid.UUID, req domain.CreateIssueRequest) (*domain.Issue, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	qtx := h.queries.WithTx(tx)

	// Step 1: Atomic counter increment
	row, err := qtx.IncrementSquadIssueCounter(ctx, squadID)
	if err != nil {
		return nil, fmt.Errorf("generate identifier: %w", err)
	}
	identifier := fmt.Sprintf("%s-%d", row.IssuePrefix, row.IssueCounter)

	// Step 2: Insert issue with generated identifier
	issue, err := qtx.CreateIssue(ctx, db.CreateIssueParams{
		SquadID:    squadID,
		Identifier: identifier,
		// ... remaining fields from req with defaults applied
	})
	if err != nil {
		return nil, fmt.Errorf("insert issue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return mapIssue(issue), nil
}
```

---

## 6. Status Machine

### 6.1 Transition Table

```
         +----------+     +-----------+     +-------------+
         | backlog  |---->|   todo    |---->| in_progress |
         +----------+     +-----------+     +-------------+
              |  ^             |  ^  ^            |  |
              |  |             |  |  |            |  |
              |  +-- (reopen)--+  |  +-- (unblock)+  |
              |                |  |               |  |
              v                v  |               v  v
         +-----------+    +-----------+     +-----------+
         | cancelled |    | blocked   |     |   done    |
         +-----------+    +-----------+     +-----------+
              ^                ^                  ^
              |                |                  |
              +--- from any ---+--- except done --+
                   open state       & cancelled
```

### 6.2 Valid Transitions Map

| From         | Allowed To                                  |
|--------------|---------------------------------------------|
| `backlog`    | `todo`, `in_progress`, `cancelled`          |
| `todo`       | `in_progress`, `backlog`, `blocked`, `cancelled` |
| `in_progress`| `done`, `blocked`, `cancelled`              |
| `blocked`    | `in_progress`, `todo`, `cancelled`          |
| `done`       | `todo` (reopen)                             |
| `cancelled`  | `todo` (reopen)                             |

### 6.3 Reopen Behavior

When an issue transitions from `done` -> `todo` or `cancelled` -> `todo`, the system automatically inserts a system-generated comment:

```go
func (h *IssueHandler) handleStatusChange(ctx context.Context, qtx *db.Queries, issue db.Issue, newStatus domain.IssueStatus) error {
	if err := domain.ValidateTransition(domain.IssueStatus(issue.Status), newStatus); err != nil {
		return &AppError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "INVALID_STATUS_TRANSITION",
			Message: err.Error(),
		}
	}

	if domain.IsReopen(domain.IssueStatus(issue.Status), newStatus) {
		_, err := qtx.CreateIssueComment(ctx, db.CreateIssueCommentParams{
			IssueID:    issue.ID,
			AuthorType: "system",
			AuthorID:   uuid.Nil, // system-generated, no real author
			Body:       fmt.Sprintf("Issue reopened: status changed from %s to %s", issue.Status, newStatus),
		})
		if err != nil {
			return fmt.Errorf("create reopen comment: %w", err)
		}
	}

	return nil
}
```

### 6.4 Error Response for Invalid Transition

```json
{
  "error": "cannot transition from \"done\" to \"in_progress\"",
  "code": "INVALID_STATUS_TRANSITION"
}
```

HTTP status: **422 Unprocessable Entity**

---

## 7. Sub-task Hierarchy

### 7.1 Self-referencing FK

The `parent_id` column is a nullable self-referencing foreign key on `issues`. A sub-task is simply an issue whose `parent_id` points to another issue.

### 7.2 Same-squad Validation

Before setting `parent_id`, the handler verifies the parent issue exists and belongs to the same squad:

```go
func (h *IssueHandler) validateParent(ctx context.Context, squadID uuid.UUID, parentID uuid.UUID) error {
	parent, err := h.queries.GetIssueByID(ctx, parentID)
	if err != nil {
		return &AppError{Status: 404, Code: "NOT_FOUND", Message: "parent issue not found"}
	}
	if parent.SquadID != squadID {
		return &AppError{Status: 400, Code: "VALIDATION_ERROR", Message: "parent issue must belong to the same squad"}
	}
	return nil
}
```

### 7.3 Circular Reference Prevention

On create or update, if `parent_id` is set, the handler walks the ancestor chain using a recursive CTE and checks whether the current issue's ID appears:

```go
func (h *IssueHandler) detectCycle(ctx context.Context, issueID, newParentID uuid.UUID) error {
	// Walk up from newParentID to root. If we encounter issueID, it is a cycle.
	ancestors, err := h.queries.GetIssueAncestors(ctx, newParentID)
	if err != nil {
		return fmt.Errorf("fetch ancestors: %w", err)
	}
	for _, a := range ancestors {
		if a.ID == issueID {
			return &AppError{
				Status:  400,
				Code:    "VALIDATION_ERROR",
				Message: "circular parent reference detected",
			}
		}
	}
	return nil
}
```

The recursive CTE has a depth safety limit of 100 to prevent runaway queries on degenerate data.

### 7.4 Deletion Guard

An issue with sub-tasks cannot be deleted (REQ-ISS-032):

```go
count, err := h.queries.CountSubTasks(ctx, issueID)
if err != nil {
    return err
}
if count > 0 {
    return &AppError{
        Status:  409,
        Code:    "CONFLICT",
        Message: "cannot delete issue with active sub-tasks",
    }
}
```

---

## 8. Identifier Lookup

`GET /api/issues/:id` accepts both UUID and human-readable identifier formats.

### Detection Logic

```go
var identifierPattern = regexp.MustCompile(`^[A-Z]{2,10}-\d+$`)

func resolveIssueID(idParam string) (isIdentifier bool, id uuid.UUID, identifier string) {
	if identifierPattern.MatchString(idParam) {
		return true, uuid.Nil, idParam
	}
	parsed, err := uuid.Parse(idParam)
	if err != nil {
		return false, uuid.Nil, "" // will result in 400
	}
	return false, parsed, ""
}
```

### Identifier Query

When the parameter matches the identifier pattern (e.g., `ARI-39`), the handler queries by identifier **scoped to squad_id** to enforce data isolation. Although identifiers are practically unique across squads (each squad has a unique prefix), the query must include `squad_id` to guarantee correctness and prevent cross-squad leakage if prefixes were ever reused:

```go
if isIdentifier {
    // squad_id must be resolved from context (route param or user's accessible squads)
    issue, err = h.queries.GetIssueByIdentifier(ctx, squadID, identifier)
} else {
    issue, err = h.queries.GetIssueByID(ctx, id)
}
```

When the caller does not provide an explicit squad context (e.g., `GET /api/issues/:id`), the handler should iterate the user's accessible squads and call `GetIssueByIdentifier(ctx, squadID, identifier)` for each until found, or return 404. After retrieval, the handler verifies the authenticated user has squad membership for the issue's squad, returning 403 if access is denied.

---

## 9. Filtering and Pagination

### 9.1 Query Parameters for `GET /api/squads/:squadId/issues`

| Parameter         | Type   | Description                                    | Default      |
|-------------------|--------|------------------------------------------------|--------------|
| `status`          | string | Filter by issue_status enum                    | (all)        |
| `priority`        | string | Filter by issue_priority enum                  | (all)        |
| `type`            | string | Filter by issue_type enum                      | (all)        |
| `assigneeAgentId` | UUID   | Filter by assigned agent                       | (all)        |
| `assigneeUserId`  | UUID   | Filter by assigned user                        | (all)        |
| `projectId`       | UUID   | Filter by project                              | (all)        |
| `goalId`          | UUID   | Filter by goal                                 | (all)        |
| `parentId`        | UUID   | Filter by parent issue (list sub-tasks)        | (all)        |
| `sort`            | string | `created_at`, `updated_at`, `priority`, `status` | `created_at` |
| `limit`           | int    | Page size (1-200)                              | 50           |
| `offset`          | int    | Offset for pagination                          | 0            |

### 9.2 Parsing Helper

```go
func parseIssueListParams(r *http.Request, squadID uuid.UUID) (domain.IssueListParams, error) {
	q := r.URL.Query()
	params := domain.IssueListParams{
		SquadID: squadID,
		Limit:   50,
		Offset:  0,
		Sort:    "created_at",
	}

	if v := q.Get("status"); v != "" {
		s := domain.IssueStatus(v)
		if !s.Valid() {
			return params, fmt.Errorf("invalid status: %s", v)
		}
		params.Status = &s
	}
	if v := q.Get("priority"); v != "" {
		p := domain.IssuePriority(v)
		if !p.Valid() {
			return params, fmt.Errorf("invalid priority: %s", v)
		}
		params.Priority = &p
	}
	if v := q.Get("type"); v != "" {
		t := domain.IssueType(v)
		if !t.Valid() {
			return params, fmt.Errorf("invalid type: %s", v)
		}
		params.Type = &t
	}
	if v := q.Get("assigneeAgentId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid assigneeAgentId")
		}
		params.AssigneeAgentID = &id
	}
	if v := q.Get("assigneeUserId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid assigneeUserId")
		}
		params.AssigneeUserID = &id
	}
	if v := q.Get("projectId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid projectId")
		}
		params.ProjectID = &id
	}
	if v := q.Get("goalId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid goalId")
		}
		params.GoalID = &id
	}
	if v := q.Get("parentId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid parentId")
		}
		params.ParentID = &id
	}
	if v := q.Get("sort"); v != "" {
		switch v {
		case "created_at", "updated_at", "priority", "status":
			params.Sort = v
		default:
			return params, fmt.Errorf("invalid sort field: %s", v)
		}
	}
	if v := q.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 {
			return params, fmt.Errorf("invalid limit")
		}
		if limit > 200 {
			limit = 200
		}
		params.Limit = limit
	}
	if v := q.Get("offset"); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil || offset < 0 {
			return params, fmt.Errorf("invalid offset")
		}
		params.Offset = offset
	}

	return params, nil
}
```

### 9.3 Comment Pagination

`GET /api/issues/:issueId/comments` supports `limit` (default 50, max 200) and `offset`. Comments are always ordered by `created_at ASC` (chronological).

---

## 10. API Contracts

### 10.1 `POST /api/squads/:squadId/issues` -- Create Issue

**Request:**

```json
{
  "title": "Implement login page",
  "description": "Build the login form with email/password fields",
  "type": "task",
  "priority": "high",
  "parentId": "550e8400-e29b-41d4-a716-446655440000",
  "assigneeAgentId": "660e8400-e29b-41d4-a716-446655440000",
  "billingCode": "PROJECT-ALPHA"
}
```

**Response (201 Created):**

```json
{
  "id": "770e8400-e29b-41d4-a716-446655440000",
  "squadId": "880e8400-e29b-41d4-a716-446655440000",
  "identifier": "ARI-39",
  "type": "task",
  "title": "Implement login page",
  "description": "Build the login form with email/password fields",
  "status": "backlog",
  "priority": "high",
  "parentId": "550e8400-e29b-41d4-a716-446655440000",
  "assigneeAgentId": "660e8400-e29b-41d4-a716-446655440000",
  "billingCode": "PROJECT-ALPHA",
  "requestDepth": 0,
  "createdAt": "2026-03-14T10:30:00Z",
  "updatedAt": "2026-03-14T10:30:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` -- missing title, title too long, invalid enum, invalid UUID
- `404 NOT_FOUND` -- squad, parent issue, agent, or user not found
- `403 FORBIDDEN` -- user not a member of the squad

---

### 10.2 `GET /api/squads/:squadId/issues` -- List Issues

**Request:** `GET /api/squads/:squadId/issues?status=todo&priority=high&sort=updated_at&limit=20&offset=0`

**Response (200 OK):**

```json
{
  "data": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440000",
      "squadId": "880e8400-e29b-41d4-a716-446655440000",
      "identifier": "ARI-39",
      "type": "task",
      "title": "Implement login page",
      "status": "todo",
      "priority": "high",
      "assigneeAgentId": "660e8400-e29b-41d4-a716-446655440000",
      "requestDepth": 0,
      "createdAt": "2026-03-14T10:30:00Z",
      "updatedAt": "2026-03-14T11:00:00Z"
    }
  ],
  "pagination": {
    "limit": 20,
    "offset": 0,
    "total": 1
  }
}
```

**Errors:**
- `400 VALIDATION_ERROR` -- invalid filter value, invalid sort field
- `403 FORBIDDEN` -- user not a member of the squad

---

### 10.3 `GET /api/issues/:id` -- Get Issue

Accepts UUID (`770e8400-...`) or identifier (`ARI-39`).

**Response (200 OK):**

```json
{
  "id": "770e8400-e29b-41d4-a716-446655440000",
  "squadId": "880e8400-e29b-41d4-a716-446655440000",
  "identifier": "ARI-39",
  "type": "task",
  "title": "Implement login page",
  "description": "Build the login form with email/password fields",
  "status": "todo",
  "priority": "high",
  "parentId": "550e8400-e29b-41d4-a716-446655440000",
  "assigneeAgentId": "660e8400-e29b-41d4-a716-446655440000",
  "billingCode": "PROJECT-ALPHA",
  "requestDepth": 0,
  "createdAt": "2026-03-14T10:30:00Z",
  "updatedAt": "2026-03-14T11:00:00Z"
}
```

**Errors:**
- `400 INVALID_ID` -- not a valid UUID or identifier format
- `403 FORBIDDEN` -- user not a member of the issue's squad
- `404 NOT_FOUND` -- issue does not exist

---

### 10.4 `PATCH /api/issues/:id` -- Update Issue

**Request (partial update):**

```json
{
  "status": "in_progress",
  "assigneeAgentId": "660e8400-e29b-41d4-a716-446655440000"
}
```

**Response (200 OK):** Full issue object (same shape as GET response).

**Errors:**
- `400 VALIDATION_ERROR` -- invalid field values, circular parent reference
- `403 FORBIDDEN` -- not a squad member
- `404 NOT_FOUND` -- issue, agent, or parent not found
- `422 INVALID_STATUS_TRANSITION` -- invalid status transition

---

### 10.5 `DELETE /api/issues/:id` -- Delete Issue

**Response (200 OK):**

```json
{
  "message": "issue deleted"
}
```

**Errors:**
- `403 FORBIDDEN` -- not a squad member
- `404 NOT_FOUND` -- issue not found
- `409 CONFLICT` -- issue has active sub-tasks

---

### 10.6 `POST /api/issues/:issueId/comments` -- Create Comment

**Request:**

```json
{
  "authorType": "user",
  "authorId": "990e8400-e29b-41d4-a716-446655440000",
  "body": "This looks ready for review."
}
```

**Response (201 Created):**

```json
{
  "id": "aa0e8400-e29b-41d4-a716-446655440000",
  "issueId": "770e8400-e29b-41d4-a716-446655440000",
  "authorType": "user",
  "authorId": "990e8400-e29b-41d4-a716-446655440000",
  "body": "This looks ready for review.",
  "createdAt": "2026-03-14T12:00:00Z",
  "updatedAt": "2026-03-14T12:00:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` -- empty body, invalid authorType, invalid authorId
- `403 FORBIDDEN` -- not a squad member
- `404 NOT_FOUND` -- issue not found, or referenced agent/user not found

---

### 10.7 `GET /api/issues/:issueId/comments` -- List Comments

**Request:** `GET /api/issues/:issueId/comments?limit=50&offset=0`

**Response (200 OK):**

```json
{
  "data": [
    {
      "id": "aa0e8400-e29b-41d4-a716-446655440000",
      "issueId": "770e8400-e29b-41d4-a716-446655440000",
      "authorType": "user",
      "authorId": "990e8400-e29b-41d4-a716-446655440000",
      "body": "This looks ready for review.",
      "createdAt": "2026-03-14T12:00:00Z",
      "updatedAt": "2026-03-14T12:00:00Z"
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 1
  }
}
```

---

## 11. Error Handling

All errors follow the standard format: `{"error": "message", "code": "CODE"}`.

| HTTP Status | Code                         | When                                              |
|-------------|------------------------------|---------------------------------------------------|
| 400         | `VALIDATION_ERROR`           | Missing required field, invalid enum, title > 500 chars, invalid UUID, circular parent reference |
| 400         | `INVALID_ID`                 | Path parameter is neither a valid UUID nor a valid identifier format |
| 401         | `UNAUTHORIZED`               | Missing or invalid JWT                            |
| 403         | `FORBIDDEN`                  | User is not a member of the issue's squad         |
| 404         | `NOT_FOUND`                  | Issue, squad, agent, user, project, goal, or parent issue does not exist |
| 409         | `CONFLICT`                   | Deleting issue with sub-tasks                     |
| 422         | `INVALID_STATUS_TRANSITION`  | Status transition not in allowed set              |
| 500         | `INTERNAL_ERROR`             | Unexpected database or server error               |

---

## 12. Security Considerations

### Authentication
- All endpoints require a valid JWT in the `Authorization: Bearer <token>` header.
- The `AuthMiddleware` extracts the user from the token and adds it to the request context.

### Squad-Scoped Isolation
- `POST /api/squads/:squadId/issues` and `GET /api/squads/:squadId/issues` verify the caller has a `SquadMembership` for the given squad.
- `GET/PATCH/DELETE /api/issues/:id` fetch the issue first, then verify the caller's membership in `issue.squad_id`.
- Comment endpoints inherit the same squad-scoping check from the parent issue.

### Input Validation
- All enum fields are validated against known values before database insertion.
- `title` length is enforced at both the application layer (handler) and database layer (CHECK constraint).
- UUID parameters are parsed with `uuid.Parse()` -- invalid formats return 400.
- SQL injection is prevented by sqlc's parameterized queries (no string interpolation).

### Data Integrity
- Foreign key constraints enforce referential integrity at the database level.
- The `CHECK (parent_id != id)` constraint prevents direct self-referencing.
- The recursive CTE prevents transitive cycles at the application level.

---

## 13. Performance Considerations

### Database Indexes

The schema includes targeted indexes (Section 4.1) designed for the common query patterns:

| Query Pattern                    | Index Used                            |
|----------------------------------|---------------------------------------|
| List issues by squad             | `idx_issues_squad_id`                 |
| Filter by status within squad    | `idx_issues_status` (composite)       |
| Filter by priority within squad  | `idx_issues_priority` (composite)     |
| Filter by assignee agent         | `idx_issues_assignee_agent_id` (partial) |
| Filter by assignee user          | `idx_issues_assignee_user_id` (partial)  |
| Sort by created_at desc          | `idx_issues_squad_created_at`         |
| Sort by updated_at desc          | `idx_issues_squad_updated_at`         |
| Lookup by identifier             | `idx_issues_identifier`               |
| List sub-tasks                   | `idx_issues_parent_id` (partial)      |
| List comments by issue           | `idx_issue_comments_issue_id`         |

Partial indexes (with `WHERE col IS NOT NULL`) reduce index size for sparse nullable columns.

### Performance Targets

- Issue list queries: < 200ms (p95) for squads with up to 10,000 issues (REQ-ISS-NF-001)
- Issue creation with identifier generation: safe under concurrent access (REQ-ISS-NF-002)
- Cycle detection CTE limited to depth 100 to prevent runaway recursion

---

## 14. Testing Strategy

### 14.1 Unit Tests -- `internal/domain/issue_test.go`

Test the pure domain logic with no database dependency:

```go
package domain_test

import (
	"testing"

	"ari/internal/domain"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    domain.IssueStatus
		to      domain.IssueStatus
		wantErr bool
	}{
		{"backlog to todo", domain.IssueStatusBacklog, domain.IssueStatusTodo, false},
		{"backlog to in_progress", domain.IssueStatusBacklog, domain.IssueStatusInProgress, false},
		{"backlog to cancelled", domain.IssueStatusBacklog, domain.IssueStatusCancelled, false},
		{"backlog to done (invalid)", domain.IssueStatusBacklog, domain.IssueStatusDone, true},
		{"backlog to blocked (invalid)", domain.IssueStatusBacklog, domain.IssueStatusBlocked, true},
		{"todo to in_progress", domain.IssueStatusTodo, domain.IssueStatusInProgress, false},
		{"todo to backlog", domain.IssueStatusTodo, domain.IssueStatusBacklog, false},
		{"todo to blocked", domain.IssueStatusTodo, domain.IssueStatusBlocked, false},
		{"todo to cancelled", domain.IssueStatusTodo, domain.IssueStatusCancelled, false},
		{"todo to done (invalid)", domain.IssueStatusTodo, domain.IssueStatusDone, true},
		{"in_progress to done", domain.IssueStatusInProgress, domain.IssueStatusDone, false},
		{"in_progress to blocked", domain.IssueStatusInProgress, domain.IssueStatusBlocked, false},
		{"in_progress to cancelled", domain.IssueStatusInProgress, domain.IssueStatusCancelled, false},
		{"in_progress to backlog (invalid)", domain.IssueStatusInProgress, domain.IssueStatusBacklog, true},
		{"blocked to in_progress", domain.IssueStatusBlocked, domain.IssueStatusInProgress, false},
		{"blocked to todo", domain.IssueStatusBlocked, domain.IssueStatusTodo, false},
		{"blocked to cancelled", domain.IssueStatusBlocked, domain.IssueStatusCancelled, false},
		{"done to todo (reopen)", domain.IssueStatusDone, domain.IssueStatusTodo, false},
		{"done to in_progress (invalid)", domain.IssueStatusDone, domain.IssueStatusInProgress, true},
		{"cancelled to todo (reopen)", domain.IssueStatusCancelled, domain.IssueStatusTodo, false},
		{"cancelled to in_progress (invalid)", domain.IssueStatusCancelled, domain.IssueStatusInProgress, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition(%s, %s) error = %v, wantErr %v",
					tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestIsReopen(t *testing.T) {
	tests := []struct {
		from domain.IssueStatus
		to   domain.IssueStatus
		want bool
	}{
		{domain.IssueStatusDone, domain.IssueStatusTodo, true},
		{domain.IssueStatusCancelled, domain.IssueStatusTodo, true},
		{domain.IssueStatusBacklog, domain.IssueStatusTodo, false},
		{domain.IssueStatusBlocked, domain.IssueStatusTodo, false},
	}
	for _, tt := range tests {
		got := domain.IsReopen(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("IsReopen(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestIssueTypeValid(t *testing.T) {
	if !domain.IssueTypeTask.Valid() {
		t.Error("task should be valid")
	}
	if domain.IssueType("invalid").Valid() {
		t.Error("invalid should not be valid")
	}
}

func TestIssuePriorityValid(t *testing.T) {
	if !domain.IssuePriorityCritical.Valid() {
		t.Error("critical should be valid")
	}
	if domain.IssuePriority("urgent").Valid() {
		t.Error("urgent should not be valid")
	}
}
```

### 14.2 Integration Tests -- `internal/server/handlers/issue_handler_test.go`

Tests that exercise the full HTTP handler + database layer using an embedded PostgreSQL instance:

| Test Case | Description | Key Assertions |
|-----------|-------------|----------------|
| `TestCreateIssue_Success` | Create issue with valid fields | 201, identifier matches pattern, defaults applied |
| `TestCreateIssue_AutoIdentifier` | Create multiple issues in same squad | Sequential identifiers (ARI-1, ARI-2, ...) |
| `TestCreateIssue_ConcurrentIdentifiers` | Parallel goroutines creating issues | No duplicate identifiers |
| `TestCreateIssue_MissingTitle` | Empty title | 400 VALIDATION_ERROR |
| `TestCreateIssue_TitleTooLong` | Title > 500 chars | 400 VALIDATION_ERROR |
| `TestCreateIssue_InvalidType` | Unknown type value | 400 VALIDATION_ERROR |
| `TestCreateIssue_InvalidPriority` | Unknown priority value | 400 VALIDATION_ERROR |
| `TestCreateIssue_ParentDifferentSquad` | Parent in another squad | 400 VALIDATION_ERROR |
| `TestCreateIssue_AssigneeNotInSquad` | Agent from different squad | 404 NOT_FOUND |
| `TestGetIssue_ByUUID` | Fetch by UUID | 200, full issue returned |
| `TestGetIssue_ByIdentifier` | Fetch by "ARI-1" | 200, same issue as UUID lookup |
| `TestGetIssue_NotFound` | Non-existent UUID | 404 NOT_FOUND |
| `TestGetIssue_WrongSquad` | User lacks membership | 403 FORBIDDEN |
| `TestUpdateIssue_StatusTransition` | Valid transition todo->in_progress | 200, status updated |
| `TestUpdateIssue_InvalidTransition` | backlog->done | 422 INVALID_STATUS_TRANSITION |
| `TestUpdateIssue_ReopenCreatesComment` | done->todo | 200, system comment created |
| `TestUpdateIssue_CircularParent` | A->B->A cycle | 400 VALIDATION_ERROR |
| `TestDeleteIssue_Success` | Delete leaf issue | 200 |
| `TestDeleteIssue_HasSubTasks` | Delete parent | 409 CONFLICT |
| `TestListIssues_FilterByStatus` | Filter with ?status=todo | Only matching issues |
| `TestListIssues_Pagination` | ?limit=2&offset=2 | Correct page |
| `TestListIssues_SortByPriority` | ?sort=priority | Ordered correctly |
| `TestCreateComment_Success` | Valid comment | 201, body preserved |
| `TestCreateComment_EmptyBody` | Empty body | 400 VALIDATION_ERROR |
| `TestListComments_Chronological` | Multiple comments | Ordered by createdAt ASC |

### 14.3 Test Helpers

```go
// testutil/issue.go

func CreateTestSquad(t *testing.T, q *db.Queries, name, prefix string) db.Squad {
	t.Helper()
	squad, err := q.CreateSquad(context.Background(), db.CreateSquadParams{
		Name:        name,
		Slug:        strings.ToLower(name),
		IssuePrefix: prefix,
	})
	require.NoError(t, err)
	return squad
}

func CreateTestIssue(t *testing.T, q *db.Queries, squadID uuid.UUID, title string) db.Issue {
	t.Helper()
	// Uses the same transactional identifier generation as production code
	issue, err := createIssueInTx(context.Background(), q, squadID, domain.CreateIssueRequest{
		Title: title,
	})
	require.NoError(t, err)
	return issue
}
```

---

## 15. Data Flow

### Issue Creation Flow

```
Client
  |
  | POST /api/squads/:squadId/issues
  v
AuthMiddleware
  |-- extract user from JWT
  v
SquadScopeMiddleware
  |-- verify user has SquadMembership for squadId
  v
IssueHandler.CreateIssue
  |-- parse & validate request body
  |-- apply defaults (status=backlog, priority=medium, type=task, depth=0)
  |-- validate parentId (same squad, no cycle)
  |-- validate assigneeAgentId (belongs to squad)
  |-- validate assigneeUserId (member of squad)
  |
  | BEGIN TRANSACTION
  |-- IncrementSquadIssueCounter -> "ARI-39"
  |-- INSERT INTO issues (...)
  | COMMIT
  |
  v
201 Created + Issue JSON
```

### Issue Update (Status Change) Flow

```
Client
  |
  | PATCH /api/issues/:id  {"status": "in_progress"}
  v
AuthMiddleware -> SquadScopeMiddleware (via issue.squad_id)
  v
IssueHandler.UpdateIssue
  |-- fetch existing issue (404 if missing)
  |-- detect status field in request
  |-- ValidateTransition(current, new) -> 422 if invalid
  |-- IsReopen(current, new)?
  |     |-- yes: INSERT system comment
  |-- UPDATE issues SET status = $1 WHERE id = $2
  v
200 OK + Updated Issue JSON
```

---

## 16. Open Questions

All resolved for Phase 1 scope:

- [x] **Soft vs hard delete for issues?** Hard delete for Phase 1 (REQ-ISS-054 allows either). Activity log will record deletion events in a future feature.
- [x] **project_id and goal_id FKs?** Defined as plain UUID columns now; FK constraints will be added when the projects/goals tables are created in feature 06.
- [x] **System comment authorId for reopens?** Uses `uuid.Nil` (all zeros) to indicate system-generated content with no human or agent author.

---

## 17. Alternatives Considered

### Alternative 1: Application-level Sequence for Identifiers

**Description:** Use a Redis INCR or an in-memory counter instead of PostgreSQL's row-level UPDATE ... RETURNING.

**Pros:**
- Lower contention under extreme concurrency (thousands of issues/second)

**Cons:**
- Adds external dependency (Redis) or process-level state (in-memory counter breaks with multiple instances)
- Gaps/duplicates possible on crash without careful two-phase logic

**Rejected Because:** PostgreSQL row-level locking is sufficient for Ari's expected workload (hundreds of issues per squad, not millions). Keeping the counter in the same transaction as the INSERT guarantees atomicity with zero additional infrastructure.

### Alternative 2: Separate Status Transition Table

**Description:** Store every status change as a row in an `issue_status_history` table instead of validating transitions in application code.

**Pros:**
- Full audit trail of status changes
- Could enforce transitions with triggers

**Cons:**
- Additional table and query complexity for a simple state machine
- Current status would need to be derived from the latest history row (slower reads)

**Rejected Because:** The status machine is small (6 states, ~14 transitions). Application-level validation with a simple map is clear, testable, and fast. An audit trail can be added via the activity log feature without complicating the issue table.

---

## 18. Timeline Estimate

- Requirements: 1 day -- Complete
- Design: 1 day -- Complete
- Implementation: 3 days
  - Day 1: Migrations, sqlc queries, domain types
  - Day 2: Issue CRUD handlers, identifier generation, status machine
  - Day 3: Comment handlers, filtering, pagination
- Testing: 1 day
- Total: 6 days

---

## 19. References

- [Requirements](./requirements.md)
- [Squad Management Requirements](../03-squad-management/requirements.md) -- issueCounter, issuePrefix
- [Agent Management Requirements](../04-agent-management/requirements.md) -- assignee validation
- [PRD Section 4.2 -- Issue Entity](../../core/01-PRODUCT.md)
- [PRD Section 4.2 -- IssueComment Entity](../../core/01-PRODUCT.md)
