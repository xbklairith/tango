# Design: Projects & Goals

**Created:** 2026-03-14
**Status:** Complete
**Feature:** 06-projects-goals
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management

---

## 1. Architecture Overview

Projects and Goals form the strategic layer of Ari's work management system. Projects group related issues under a shared scope within a squad. Goals define hierarchical strategic objectives that issues and projects align to. Both entities are strictly squad-scoped, following the same data isolation pattern established by Squad Management (feature 03).

The implementation adds two new database tables (`projects`, `goals`), two domain types, two HTTP handler files, and corresponding sqlc queries. Goal hierarchy uses a self-referential `parent_id` foreign key with application-level cycle detection and a maximum nesting depth of 5. Issue linkage is achieved by adding optional `project_id` and `goal_id` foreign key columns to the existing `issues` table via a migration.

```
Squad
├── Projects (flat list, unique name per squad)
│   └── Issues (many-to-one via projectId)
└── Goals (tree hierarchy, max depth 5)
    ├── Sub-Goals (via parentId self-reference)
    └── Issues (many-to-one via goalId)
```

---

## 2. System Context

- **Depends On:**
  - `01-go-scaffold` — HTTP server, router, middleware, error helpers
  - `02-user-auth` — JWT authentication middleware, user context extraction
  - `03-squad-management` — Squad entity, SquadMembership authorization, squad-scoped query patterns
- **Used By:**
  - `05-issue-tracking` — Issues reference `projectId` and `goalId` as optional foreign keys
  - `07-react-ui-foundation` — React UI renders project and goal lists, goal tree views
  - Future cost attribution — CostEvents will reference `projectId` and `goalId` (Phase 2)
- **External Dependencies:** None

---

## 3. Component Structure

### 3.1 Domain Types

#### `internal/domain/project.go`

```go
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ProjectStatus represents the lifecycle state of a project.
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

// ValidProjectStatuses is the set of all valid project statuses.
var ValidProjectStatuses = map[ProjectStatus]bool{
	ProjectStatusActive:    true,
	ProjectStatusCompleted: true,
	ProjectStatusArchived:  true,
}

// validProjectTransitions defines allowed from -> to status changes.
var validProjectTransitions = map[ProjectStatus]map[ProjectStatus]bool{
	ProjectStatusActive: {
		ProjectStatusCompleted: true,
		ProjectStatusArchived:  true,
	},
	ProjectStatusCompleted: {
		ProjectStatusActive:   true,
		ProjectStatusArchived: true,
	},
	ProjectStatusArchived: {
		ProjectStatusActive: true,
	},
}

// ValidateProjectTransition checks whether a status transition is allowed.
// Returns an error describing the invalid transition if not permitted.
func ValidateProjectTransition(from, to ProjectStatus) error {
	if from == to {
		return nil // no-op transition is always valid
	}
	if targets, ok := validProjectTransitions[from]; ok {
		if targets[to] {
			return nil
		}
	}
	return fmt.Errorf("invalid project status transition from %q to %q", from, to)
}

// Project represents a grouping of related issues within a squad.
type Project struct {
	ID          uuid.UUID     `json:"id"`
	SquadID     uuid.UUID     `json:"squadId"`
	Name        string        `json:"name"`
	Description *string       `json:"description,omitempty"`
	Status      ProjectStatus `json:"status"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// CreateProjectInput holds validated input for creating a project.
type CreateProjectInput struct {
	SquadID     uuid.UUID
	Name        string
	Description *string
}

// UpdateProjectInput holds validated input for updating a project.
type UpdateProjectInput struct {
	Name        *string
	Description *string
	Status      *ProjectStatus
}
```

#### `internal/domain/goal.go`

```go
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MaxGoalDepth is the maximum allowed nesting depth for goal hierarchies.
const MaxGoalDepth = 5

// GoalStatus represents the lifecycle state of a goal.
type GoalStatus string

const (
	GoalStatusActive    GoalStatus = "active"
	GoalStatusCompleted GoalStatus = "completed"
	GoalStatusArchived  GoalStatus = "archived"
)

// ValidGoalStatuses is the set of all valid goal statuses.
var ValidGoalStatuses = map[GoalStatus]bool{
	GoalStatusActive:    true,
	GoalStatusCompleted: true,
	GoalStatusArchived:  true,
}

// validGoalTransitions defines allowed from -> to status changes.
var validGoalTransitions = map[GoalStatus]map[GoalStatus]bool{
	GoalStatusActive: {
		GoalStatusCompleted: true,
		GoalStatusArchived:  true,
	},
	GoalStatusCompleted: {
		GoalStatusActive:   true,
		GoalStatusArchived: true,
	},
	GoalStatusArchived: {
		GoalStatusActive: true,
	},
}

// ValidateGoalTransition checks whether a status transition is allowed.
func ValidateGoalTransition(from, to GoalStatus) error {
	if from == to {
		return nil
	}
	if targets, ok := validGoalTransitions[from]; ok {
		if targets[to] {
			return nil
		}
	}
	return fmt.Errorf("invalid goal status transition from %q to %q", from, to)
}

// Goal represents a strategic objective within a squad.
type Goal struct {
	ID          uuid.UUID  `json:"id"`
	SquadID     uuid.UUID  `json:"squadId"`
	ParentID    *uuid.UUID `json:"parentId,omitempty"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      GoalStatus `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// CreateGoalInput holds validated input for creating a goal.
type CreateGoalInput struct {
	SquadID     uuid.UUID
	ParentID    *uuid.UUID
	Title       string
	Description *string
}

// UpdateGoalInput holds validated input for updating a goal.
type UpdateGoalInput struct {
	Title       *string
	Description *string
	ParentID    *uuid.UUID // set to non-nil to change parent; use uuid.Nil to unset
	Status      *GoalStatus
}

// GoalAncestryChain represents the chain of parent IDs from a goal up to the root.
// Used for cycle detection and depth validation.
type GoalAncestryChain []uuid.UUID

// ContainsCycle returns true if the given goalID appears anywhere in the ancestry chain.
func (chain GoalAncestryChain) ContainsCycle(goalID uuid.UUID) bool {
	for _, id := range chain {
		if id == goalID {
			return true
		}
	}
	return false
}

// Depth returns the nesting depth (1-indexed). A top-level goal has depth 1.
func (chain GoalAncestryChain) Depth() int {
	return len(chain) + 1
}
```

### 3.2 HTTP Handlers

#### `internal/server/handlers/project_handler.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"ari/internal/domain"
)

// ProjectHandler handles HTTP requests for project CRUD operations.
type ProjectHandler struct {
	queries ProjectQueries
}

// ProjectQueries defines the database interface required by ProjectHandler.
// Satisfied by the sqlc-generated Queries struct.
type ProjectQueries interface {
	CreateProject(ctx context.Context, arg db.CreateProjectParams) (db.Project, error)
	GetProjectByID(ctx context.Context, id uuid.UUID) (db.Project, error)
	ListProjectsBySquad(ctx context.Context, squadID uuid.UUID) ([]db.Project, error)
	UpdateProject(ctx context.Context, arg db.UpdateProjectParams) (db.Project, error)
	ProjectExistsByName(ctx context.Context, arg db.ProjectExistsByNameParams) (bool, error)
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(q ProjectQueries) *ProjectHandler {
	return &ProjectHandler{queries: q}
}

// createProjectRequest is the JSON body for POST /api/squads/:id/projects.
type createProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// updateProjectRequest is the JSON body for PATCH /api/projects/:id.
type updateProjectRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Status      *domain.ProjectStatus  `json:"status,omitempty"`
}

// List handles GET /api/squads/:id/projects
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	// 1. Extract squadID from URL path
	// 2. Verify squad membership via middleware context
	// 3. Query ListProjectsBySquad(ctx, squadID)
	// 4. Map db rows to domain.Project slice
	// 5. Return JSON array (empty array if none)
}

// GetByID handles GET /api/squads/:squadId/projects/:id
func (h *ProjectHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	// 1. Extract squadID and projectID from URL path
	// 2. Verify squad membership via middleware context
	// 3. Fetch project via GetProjectByID(ctx, projectID)
	//    - If not found, return 404
	// 4. Verify project.SquadID matches squadID (404 if mismatch for isolation)
	// 5. Return 200 with project JSON
}

// Create handles POST /api/squads/:id/projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	// 1. Extract squadID from URL path
	// 2. Verify squad membership via middleware context
	// 3. Decode and validate createProjectRequest
	//    - Name required, 1-255 chars
	// 4. Check uniqueness: ProjectExistsByName(ctx, {SquadID, Name})
	//    - If exists, return 409 CONFLICT
	// 5. Insert via CreateProject(ctx, params)
	// 6. Return 201 with project JSON
}

// Update handles PATCH /api/projects/:id
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	// 1. Extract projectID from URL path
	// 2. Fetch existing project via GetProjectByID
	//    - If not found, return 404
	// 3. Verify squad membership for project.SquadID
	// 4. Decode and validate updateProjectRequest
	// 5. If status change requested, validate transition
	//    - If invalid, return 422 with current/attempted status
	// 6. If name change requested, check uniqueness within squad
	//    - If duplicate, return 409
	// 7. Apply updates via UpdateProject(ctx, params)
	// 8. Return 200 with updated project JSON
}
```

#### `internal/server/handlers/goal_handler.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"ari/internal/domain"
)

// GoalHandler handles HTTP requests for goal CRUD operations.
type GoalHandler struct {
	queries GoalQueries
}

// GoalQueries defines the database interface required by GoalHandler.
type GoalQueries interface {
	CreateGoal(ctx context.Context, arg db.CreateGoalParams) (db.Goal, error)
	GetGoalByID(ctx context.Context, id uuid.UUID) (db.Goal, error)
	ListGoalsBySquad(ctx context.Context, squadID uuid.UUID) ([]db.Goal, error)
	ListGoalsBySquadAndParent(ctx context.Context, arg db.ListGoalsBySquadAndParentParams) ([]db.Goal, error)
	ListTopLevelGoalsBySquad(ctx context.Context, squadID uuid.UUID) ([]db.Goal, error)
	UpdateGoal(ctx context.Context, arg db.UpdateGoalParams) (db.Goal, error)
	GetGoalAncestors(ctx context.Context, goalID uuid.UUID) ([]uuid.UUID, error)
}

// NewGoalHandler creates a new GoalHandler.
func NewGoalHandler(q GoalQueries) *GoalHandler {
	return &GoalHandler{queries: q}
}

// createGoalRequest is the JSON body for POST /api/squads/:id/goals.
type createGoalRequest struct {
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parentId,omitempty"`
}

// updateGoalRequest is the JSON body for PATCH /api/goals/:id.
type updateGoalRequest struct {
	Title       *string            `json:"title,omitempty"`
	Description *string            `json:"description,omitempty"`
	ParentID    *uuid.UUID         `json:"parentId,omitempty"`
	Status      *domain.GoalStatus `json:"status,omitempty"`
}

// GetByID handles GET /api/squads/:squadId/goals/:id
func (h *GoalHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	// 1. Extract squadID and goalID from URL path
	// 2. Verify squad membership via middleware context
	// 3. Fetch goal via GetGoalByID(ctx, goalID)
	//    - If not found, return 404
	// 4. Verify goal.SquadID matches squadID (404 if mismatch for isolation)
	// 5. Return 200 with goal JSON
}

// List handles GET /api/squads/:id/goals
func (h *GoalHandler) List(w http.ResponseWriter, r *http.Request) {
	// 1. Extract squadID from URL path
	// 2. Verify squad membership
	// 3. Check for ?parentId query param:
	//    - If parentId=null -> ListTopLevelGoalsBySquad
	//    - If parentId=<uuid> -> ListGoalsBySquadAndParent
	//    - If absent -> ListGoalsBySquad (all goals)
	// 4. Return JSON array
}

// Create handles POST /api/squads/:id/goals
func (h *GoalHandler) Create(w http.ResponseWriter, r *http.Request) {
	// 1. Extract squadID from URL path
	// 2. Verify squad membership
	// 3. Decode and validate createGoalRequest
	//    - Title required, 1-255 chars
	// 4. If parentID provided:
	//    a. Fetch parent goal, verify it exists
	//    b. Verify parent belongs to same squad
	//    c. Walk ancestor chain via GetGoalAncestors
	//    d. Check depth <= MaxGoalDepth (5) — if chain length + 1 >= 5, reject
	// 5. Insert via CreateGoal
	// 6. Return 201 with goal JSON
}

// Update handles PATCH /api/goals/:id
func (h *GoalHandler) Update(w http.ResponseWriter, r *http.Request) {
	// 1. Extract goalID from URL path
	// 2. Fetch existing goal via GetGoalByID
	//    - If not found, return 404
	// 3. Verify squad membership for goal.SquadID
	// 4. Decode and validate updateGoalRequest
	// 5. If status change requested, validate transition
	//    - If invalid, return 422
	// 6. If parentID change requested:
	//    a. Validate new parent exists and belongs to same squad
	//    b. Run cycle detection (check if goalID appears in new parent's ancestry)
	//    c. Check resulting depth <= MaxGoalDepth
	//    - If cycle detected, return 422 CIRCULAR_REFERENCE
	//    - If depth exceeded, return 422 MAX_DEPTH_EXCEEDED
	// 7. Apply updates via UpdateGoal
	// 8. Return 200 with updated goal JSON
}
```

### 3.3 Router Registration

In `internal/server/router.go`, add route registrations:

```go
// Projects (squad-scoped)
mux.Handle("GET /api/squads/{squadID}/projects", authMiddleware(projectHandler.List))
mux.Handle("GET /api/squads/{squadID}/projects/{projectID}", authMiddleware(projectHandler.GetByID))
mux.Handle("POST /api/squads/{squadID}/projects", authMiddleware(projectHandler.Create))
mux.Handle("PATCH /api/projects/{projectID}", authMiddleware(projectHandler.Update))

// Goals (squad-scoped)
mux.Handle("GET /api/squads/{squadID}/goals", authMiddleware(goalHandler.List))
mux.Handle("GET /api/squads/{squadID}/goals/{goalID}", authMiddleware(goalHandler.GetByID))
mux.Handle("POST /api/squads/{squadID}/goals", authMiddleware(goalHandler.Create))
mux.Handle("PATCH /api/goals/{goalID}", authMiddleware(goalHandler.Update))
```

---

## 4. Database Schema

### 4.1 Migration: Create Projects Table

**File:** `internal/database/migrations/XXXXXX_create_projects.sql`

```sql
-- +goose Up
CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'completed', 'archived')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_projects_squad_name UNIQUE (squad_id, name)
);

CREATE INDEX idx_projects_squad_id ON projects(squad_id);
CREATE INDEX idx_projects_status ON projects(squad_id, status);

-- +goose Down
DROP TABLE IF EXISTS projects;
```

### 4.2 Migration: Create Goals Table

**File:** `internal/database/migrations/XXXXXX_create_goals.sql`

```sql
-- +goose Up
CREATE TABLE goals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    parent_id   UUID REFERENCES goals(id) ON DELETE SET NULL,
    title       VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'completed', 'archived')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_goals_squad_id ON goals(squad_id);
CREATE INDEX idx_goals_parent_id ON goals(parent_id);
CREATE INDEX idx_goals_status ON goals(squad_id, status);

-- +goose Down
DROP TABLE IF EXISTS goals;
```

### 4.3 Migration: Add Issue Linkage Columns

**File:** `internal/database/migrations/XXXXXX_add_issue_project_goal_fks.sql`

```sql
-- +goose Up
ALTER TABLE issues
    ADD COLUMN project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    ADD COLUMN goal_id    UUID REFERENCES goals(id) ON DELETE SET NULL;

CREATE INDEX idx_issues_project_id ON issues(project_id);
CREATE INDEX idx_issues_goal_id ON issues(goal_id);

-- +goose Down
ALTER TABLE issues
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS goal_id;
```

### 4.4 sqlc Queries

#### `internal/database/queries/projects.sql`

```sql
-- name: CreateProject :one
INSERT INTO projects (squad_id, name, description, status)
VALUES ($1, $2, $3, 'active')
RETURNING id, squad_id, name, description, status, created_at, updated_at;

-- name: GetProjectByID :one
SELECT id, squad_id, name, description, status, created_at, updated_at
FROM projects
WHERE id = $1;

-- name: ListProjectsBySquad :many
SELECT id, squad_id, name, description, status, created_at, updated_at
FROM projects
WHERE squad_id = $1
ORDER BY created_at DESC;

-- name: UpdateProject :one
UPDATE projects
SET
    name        = COALESCE(sqlc.narg('name'), name),
    description = COALESCE(sqlc.narg('description'), description),
    status      = COALESCE(sqlc.narg('status'), status),
    updated_at  = now()
WHERE id = $1
RETURNING id, squad_id, name, description, status, created_at, updated_at;

-- name: ProjectExistsByName :one
SELECT EXISTS(
    SELECT 1 FROM projects
    WHERE squad_id = $1 AND name = $2
) AS exists;

-- name: ProjectExistsByNameExcluding :one
-- Used when updating a project to check name uniqueness excluding the current project.
SELECT EXISTS(
    SELECT 1 FROM projects
    WHERE squad_id = $1 AND name = $2 AND id != $3
) AS exists;
```

#### `internal/database/queries/goals.sql`

```sql
-- name: CreateGoal :one
INSERT INTO goals (squad_id, parent_id, title, description, status)
VALUES ($1, $2, $3, $4, 'active')
RETURNING id, squad_id, parent_id, title, description, status, created_at, updated_at;

-- name: GetGoalByID :one
SELECT id, squad_id, parent_id, title, description, status, created_at, updated_at
FROM goals
WHERE id = $1;

-- name: ListGoalsBySquad :many
SELECT id, squad_id, parent_id, title, description, status, created_at, updated_at
FROM goals
WHERE squad_id = $1
ORDER BY created_at DESC;

-- name: ListTopLevelGoalsBySquad :many
SELECT id, squad_id, parent_id, title, description, status, created_at, updated_at
FROM goals
WHERE squad_id = $1 AND parent_id IS NULL
ORDER BY created_at DESC;

-- name: ListGoalsBySquadAndParent :many
SELECT id, squad_id, parent_id, title, description, status, created_at, updated_at
FROM goals
WHERE squad_id = $1 AND parent_id = $2
ORDER BY created_at DESC;

-- name: UpdateGoal :one
UPDATE goals
SET
    title       = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    parent_id   = COALESCE(sqlc.narg('parent_id'), parent_id),
    status      = COALESCE(sqlc.narg('status'), status),
    updated_at  = now()
WHERE id = $1
RETURNING id, squad_id, parent_id, title, description, status, created_at, updated_at;

-- name: GetGoalAncestors :many
-- Recursive CTE to walk the goal hierarchy upward. Returns ancestor IDs
-- from immediate parent to root. Used for cycle detection and depth checks.
WITH RECURSIVE ancestors AS (
    SELECT parent_id, 1 AS depth
    FROM goals
    WHERE id = $1
    UNION ALL
    SELECT g.parent_id, a.depth + 1
    FROM goals g
    JOIN ancestors a ON g.id = a.parent_id
    WHERE a.parent_id IS NOT NULL AND a.depth < 6
)
SELECT parent_id
FROM ancestors
WHERE parent_id IS NOT NULL;
```

#### Design Note: COALESCE/sqlc.narg Partial Update Limitation

The `UpdateProject` and `UpdateGoal` queries use the `COALESCE(sqlc.narg('field'), field)` pattern for partial updates. This pattern treats a NULL parameter as "not provided" and preserves the existing column value. However, this means it is **not possible to explicitly set a nullable field to NULL** via the update query (e.g., clearing a project's `description` or unsetting a goal's `parent_id`).

For Phase 1 this is an accepted limitation because:
- `description` is informational and rarely needs to be cleared.
- `parent_id` on goals can be unset by passing `uuid.Nil` (the zero UUID) and handling it in the application layer before the query, or by using a separate dedicated query.

If the need arises to distinguish "not provided" from "set to NULL" for nullable fields, the recommended approach is to either (a) add dedicated "clear field" queries (e.g., `ClearGoalParent`) or (b) switch to a two-query read-modify-write pattern for updates that need NULL semantics.

---

## 5. API Contracts

### 5.1 List Projects

**`GET /api/squads/{squadID}/projects`**

**Headers:**
```
Authorization: Bearer <jwt-token>
```

**Response (200 OK):**
```json
[
  {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
    "name": "Agent Onboarding Redesign",
    "description": "Redesign the agent onboarding flow for v2",
    "status": "active",
    "createdAt": "2026-03-14T10:00:00Z",
    "updatedAt": "2026-03-14T10:00:00Z"
  }
]
```

**Errors:**
- `401 Unauthorized` — missing or invalid JWT
- `403 Forbidden` — user is not a member of the squad
- `404 Not Found` — squad does not exist

### 5.2 Get Project by ID

**`GET /api/squads/{squadID}/projects/{projectID}`**

**Headers:**
```
Authorization: Bearer <jwt-token>
```

**Response (200 OK):**
```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
  "name": "Agent Onboarding Redesign",
  "description": "Redesign the agent onboarding flow for v2",
  "status": "active",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T10:00:00Z"
}
```

**Errors:**
- `401 Unauthorized` — missing or invalid JWT
- `403 Forbidden` — user is not a member of the squad
- `404 Not Found` — squad or project does not exist (also returned if the project belongs to a different squad)

### 5.3 Create Project

**`POST /api/squads/{squadID}/projects`**

**Request Body:**
```json
{
  "name": "Agent Onboarding Redesign",
  "description": "Redesign the agent onboarding flow for v2"
}
```

**Validation:**
- `name`: required, 1-255 characters
- `description`: optional, text

**Response (201 Created):**
```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
  "name": "Agent Onboarding Redesign",
  "description": "Redesign the agent onboarding flow for v2",
  "status": "active",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T10:00:00Z"
}
```

**Errors:**
- `400 Bad Request` — `{"error": "name is required", "code": "VALIDATION_ERROR"}`
- `401 Unauthorized`
- `403 Forbidden`
- `404 Not Found` — squad does not exist
- `409 Conflict` — `{"error": "project name already exists in this squad", "code": "PROJECT_NAME_TAKEN"}`

### 5.4 Update Project

**`PATCH /api/projects/{projectID}`**

**Request Body (all fields optional):**
```json
{
  "name": "Agent Onboarding v2",
  "description": "Updated scope for v2 launch",
  "status": "completed"
}
```

**Response (200 OK):**
```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
  "name": "Agent Onboarding v2",
  "description": "Updated scope for v2 launch",
  "status": "completed",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T12:30:00Z"
}
```

**Errors:**
- `400 Bad Request` — invalid input
- `401 Unauthorized`
- `403 Forbidden` — user not a member of the project's squad
- `404 Not Found` — project does not exist
- `409 Conflict` — name collision within squad
- `422 Unprocessable Entity` — `{"error": "invalid project status transition from \"archived\" to \"completed\"", "code": "INVALID_STATUS_TRANSITION"}`

### 5.5 List Goals

**`GET /api/squads/{squadID}/goals`**

**Query Parameters:**
- `parentId` (optional) — filter by parent goal ID; use `null` for top-level goals only

**Examples:**
- `GET /api/squads/{id}/goals` — all goals in the squad
- `GET /api/squads/{id}/goals?parentId=null` — top-level goals only
- `GET /api/squads/{id}/goals?parentId=<uuid>` — direct children of a specific goal

**Response (200 OK):**
```json
[
  {
    "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
    "parentId": null,
    "title": "Reduce agent error rate to < 1%",
    "description": "Strategic goal for Q2 reliability push",
    "status": "active",
    "createdAt": "2026-03-14T10:00:00Z",
    "updatedAt": "2026-03-14T10:00:00Z"
  },
  {
    "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
    "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
    "parentId": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "title": "Implement retry logic for transient failures",
    "description": null,
    "status": "active",
    "createdAt": "2026-03-14T11:00:00Z",
    "updatedAt": "2026-03-14T11:00:00Z"
  }
]
```

**Errors:**
- `401 Unauthorized`
- `403 Forbidden`
- `404 Not Found` — squad does not exist

### 5.6 Get Goal by ID

**`GET /api/squads/{squadID}/goals/{goalID}`**

**Headers:**
```
Authorization: Bearer <jwt-token>
```

**Response (200 OK):**
```json
{
  "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
  "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
  "parentId": null,
  "title": "Reduce agent error rate to < 1%",
  "description": "Strategic goal for Q2 reliability push",
  "status": "active",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T10:00:00Z"
}
```

**Errors:**
- `401 Unauthorized` — missing or invalid JWT
- `403 Forbidden` — user is not a member of the squad
- `404 Not Found` — squad or goal does not exist (also returned if the goal belongs to a different squad)

### 5.7 Create Goal

**`POST /api/squads/{squadID}/goals`**

**Request Body:**
```json
{
  "title": "Reduce agent error rate to < 1%",
  "description": "Strategic goal for Q2 reliability push",
  "parentId": null
}
```

**Validation:**
- `title`: required, 1-255 characters
- `description`: optional, text
- `parentId`: optional UUID; if provided, must reference an existing goal in the same squad

**Response (201 Created):**
```json
{
  "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
  "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
  "parentId": null,
  "title": "Reduce agent error rate to < 1%",
  "description": "Strategic goal for Q2 reliability push",
  "status": "active",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T10:00:00Z"
}
```

**Errors:**
- `400 Bad Request` — `{"error": "title is required", "code": "VALIDATION_ERROR"}`
- `401 Unauthorized`
- `403 Forbidden`
- `404 Not Found` — squad or parent goal does not exist
- `422 Unprocessable Entity` — `{"error": "parent goal does not belong to the same squad", "code": "CROSS_SQUAD_REFERENCE"}`
- `422 Unprocessable Entity` — `{"error": "maximum goal nesting depth of 5 exceeded", "code": "MAX_DEPTH_EXCEEDED"}`

### 5.8 Update Goal

**`PATCH /api/goals/{goalID}`**

**Request Body (all fields optional):**
```json
{
  "title": "Reduce agent error rate to < 0.5%",
  "parentId": "a1b2c3d4-0000-0000-0000-000000000000",
  "status": "completed"
}
```

**Response (200 OK):**
```json
{
  "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
  "squadId": "f0e1d2c3-b4a5-6789-abcd-ef1234567890",
  "parentId": "a1b2c3d4-0000-0000-0000-000000000000",
  "title": "Reduce agent error rate to < 0.5%",
  "description": "Strategic goal for Q2 reliability push",
  "status": "completed",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T14:00:00Z"
}
```

**Errors:**
- `400 Bad Request` — invalid input
- `401 Unauthorized`
- `403 Forbidden`
- `404 Not Found` — goal does not exist
- `422 Unprocessable Entity` — `{"error": "circular reference detected: goal cannot be its own ancestor", "code": "CIRCULAR_REFERENCE"}`
- `422 Unprocessable Entity` — `{"error": "maximum goal nesting depth of 5 exceeded", "code": "MAX_DEPTH_EXCEEDED"}`
- `422 Unprocessable Entity` — `{"error": "invalid goal status transition from \"archived\" to \"completed\"", "code": "INVALID_STATUS_TRANSITION"}`

---

## 6. Goal Hierarchy

### 6.1 Self-Referential Parent

Goals support a tree hierarchy via the nullable `parent_id` column. A `NULL` parent_id indicates a top-level (root) goal. The foreign key `REFERENCES goals(id) ON DELETE SET NULL` ensures that if a parent goal is somehow removed, children become top-level rather than being orphaned or cascade-deleted.

### 6.2 Cycle Detection Algorithm

Cycle detection runs in the application layer before any write that changes `parent_id`. The algorithm uses the recursive CTE `GetGoalAncestors` to fetch the full ancestor chain of the proposed parent, then checks whether the current goal's ID appears anywhere in that chain.

```go
// validateGoalHierarchy checks for cycles and max depth when setting a goal's parent.
// goalID is the goal being modified; newParentID is the proposed parent.
func validateGoalHierarchy(ctx context.Context, q GoalQueries, goalID, newParentID uuid.UUID) error {
	// Self-reference check
	if goalID == newParentID {
		return &AppError{
			Code:    "CIRCULAR_REFERENCE",
			Message: "circular reference detected: goal cannot be its own ancestor",
			Status:  http.StatusUnprocessableEntity,
		}
	}

	// Fetch ancestor chain of the proposed parent
	ancestors, err := q.GetGoalAncestors(ctx, newParentID)
	if err != nil {
		return fmt.Errorf("fetching goal ancestors: %w", err)
	}

	// Check if goalID appears in the parent's ancestry (would create a cycle)
	chain := domain.GoalAncestryChain(ancestors)
	if chain.ContainsCycle(goalID) {
		return &AppError{
			Code:    "CIRCULAR_REFERENCE",
			Message: "circular reference detected: goal cannot be its own ancestor",
			Status:  http.StatusUnprocessableEntity,
		}
	}

	// Depth check: ancestors of parent + parent itself + the goal = total depth
	// The new goal would be at depth = len(ancestors) + 2
	// (ancestors count = depth of parent - 1, plus parent itself, plus this goal)
	newDepth := len(ancestors) + 2 // +1 for parent, +1 for the goal itself
	if newDepth > domain.MaxGoalDepth {
		return &AppError{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("maximum goal nesting depth of %d exceeded", domain.MaxGoalDepth),
			Status:  http.StatusUnprocessableEntity,
		}
	}

	return nil
}
```

### 6.3 Max Depth Enforcement

The maximum depth is 5 levels. Depth is computed as the length of the ancestor chain plus one (for the goal itself). The recursive CTE in `GetGoalAncestors` has a safety limit of `depth < 6` to prevent runaway recursion even if a cycle somehow exists in the database.

**Depth examples:**
```
Level 1: Company OKR          (parentId = null, depth = 1)
Level 2: Team Objective        (parentId = L1,   depth = 2)
Level 3: Sprint Goal           (parentId = L2,   depth = 3)
Level 4: Sub-task Goal         (parentId = L3,   depth = 4)
Level 5: Atomic Deliverable    (parentId = L4,   depth = 5)
Level 6: REJECTED              (would exceed max depth)
```

### 6.4 Cross-Squad Validation

When a `parentId` is provided, the handler verifies that the parent goal's `squad_id` matches the target squad. This prevents cross-squad goal references, which would violate data isolation.

---

## 7. Status Transitions

Both projects and goals share the same status lifecycle with identical valid transitions:

```
         ┌──────────────┐
    ┌───>│    active     │<───┐
    │    └──────┬───────┘    │
    │           │            │
    │     ┌─────┴─────┐     │
    │     │           │     │
    │     v           v     │
┌───┴─────────┐ ┌───────────┴───┐
│  completed  │ │   archived    │
└──────┬──────┘ └───────────────┘
       │                ^
       └────────────────┘
```

**Valid transitions:**

| From | To | Allowed |
|------|----|---------|
| active | completed | Yes |
| active | archived | Yes |
| completed | active | Yes (reopen) |
| completed | archived | Yes |
| archived | active | Yes (restore) |
| archived | completed | **No** |

**Implementation:** The `ValidateProjectTransition` and `ValidateGoalTransition` functions in the domain package enforce these rules. The handler checks the transition before issuing the database update. If invalid, it returns:

```json
{
  "error": "invalid project status transition from \"archived\" to \"completed\"",
  "code": "INVALID_STATUS_TRANSITION"
}
```

HTTP status: `422 Unprocessable Entity`

---

## 8. Issue Linkage

### 8.1 Schema

Issues gain two optional FK columns: `project_id` and `goal_id`. Both use `ON DELETE SET NULL` so that archiving or removing a project/goal does not cascade-delete issues.

### 8.2 Validation on Issue Create/Update

When an issue is created or updated with a `projectId` or `goalId`:

1. Fetch the referenced project/goal by ID.
2. Verify it exists (return 404 if not).
3. Verify `project.squad_id == issue.squad_id` (return 422 `CROSS_SQUAD_REFERENCE` if mismatched).
4. Same check for goal.

This validation lives in the issue handler, not in the project/goal handlers. The project and goal entities themselves are unaware of issues.

### 8.3 No Cascade Blocking

Per REQ-PG-044, changing a project or goal's status (including archiving) does not require checking whether issues reference it. Issues can continue to reference archived projects and goals. The UI may display a visual indicator that the linked project/goal is archived.

### 8.4 Cost Attribution (Forward-Looking)

The `project_id` and `goal_id` columns will also appear on the `cost_events` table in Phase 2, enabling cost tracking per project and per goal. No implementation is needed now; the schema just needs to be compatible.

---

## 9. Error Handling

### Error Codes

| Code | HTTP Status | When |
|------|-------------|------|
| `VALIDATION_ERROR` | 400 | Missing/invalid required fields |
| `UNAUTHORIZED` | 401 | Missing or invalid JWT |
| `FORBIDDEN` | 403 | User lacks squad membership |
| `NOT_FOUND` | 404 | Squad, project, or goal not found |
| `PROJECT_NAME_TAKEN` | 409 | Duplicate project name within squad |
| `INVALID_STATUS_TRANSITION` | 422 | Status change violates transition rules |
| `CIRCULAR_REFERENCE` | 422 | Goal parent change would create cycle |
| `MAX_DEPTH_EXCEEDED` | 422 | Goal hierarchy exceeds 5 levels |
| `CROSS_SQUAD_REFERENCE` | 422 | Parent goal or linked entity in different squad |
| `INTERNAL_ERROR` | 500 | Unexpected database or server error |

### Error Response Format

All errors follow the standard Ari format:

```json
{
  "error": "human-readable message describing the problem",
  "code": "ERROR_CODE"
}
```

---

## 10. Security Considerations

### Authentication

All project and goal endpoints require a valid JWT in the `Authorization: Bearer` header. The auth middleware extracts the user ID from the token and attaches it to the request context.

### Authorization (Squad Membership)

Before any operation, the handler verifies the authenticated user has an active `SquadMembership` for the relevant squad. For list/create endpoints, the squad ID comes from the URL path. For update endpoints, the squad ID is read from the existing project/goal record.

If the user lacks membership, the response is `403 Forbidden` (not 404) because the squad ID is already known from the URL path. For the update endpoints where the project/goal ID is in the path, a 404 is returned if the entity does not exist, preventing information leakage.

### Input Validation

- All string inputs are trimmed of leading/trailing whitespace before validation.
- Name and title fields are validated for length (1-255 chars).
- UUID path parameters are parsed and validated before any database query.
- Status values are validated against the allowed enum set.
- SQL injection is prevented by sqlc's parameterized queries.

### Squad-Scoped Data Isolation

Every database query includes a `WHERE squad_id = $1` clause. No unscoped queries exist. The `ListProjectsBySquad` and `ListGoalsBySquad` queries enforce this at the SQL level.

---

## 11. Performance Considerations

### Database Indexes

| Table | Index | Purpose |
|-------|-------|---------|
| `projects` | `idx_projects_squad_id` | Fast lookup by squad |
| `projects` | `uq_projects_squad_name` (unique) | Name uniqueness + lookup |
| `projects` | `idx_projects_status` | Filter by status within squad |
| `goals` | `idx_goals_squad_id` | Fast lookup by squad |
| `goals` | `idx_goals_parent_id` | Efficient hierarchy traversal |
| `goals` | `idx_goals_status` | Filter by status within squad |
| `issues` | `idx_issues_project_id` | Find issues by project |
| `issues` | `idx_issues_goal_id` | Find issues by goal |

### Performance Targets

- List endpoints: < 200ms (p95) for squads with up to 1,000 projects or goals.
- The recursive CTE for ancestor lookup is bounded to depth 6, ensuring it terminates quickly even for pathological data.
- `COALESCE`-based partial updates avoid separate read-modify-write cycles.

---

## 12. Testing Strategy

### Unit Tests

#### `internal/domain/project_test.go`

```go
func TestValidateProjectTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    domain.ProjectStatus
		to      domain.ProjectStatus
		wantErr bool
	}{
		{"active to completed", domain.ProjectStatusActive, domain.ProjectStatusCompleted, false},
		{"active to archived", domain.ProjectStatusActive, domain.ProjectStatusArchived, false},
		{"completed to active", domain.ProjectStatusCompleted, domain.ProjectStatusActive, false},
		{"completed to archived", domain.ProjectStatusCompleted, domain.ProjectStatusArchived, false},
		{"archived to active", domain.ProjectStatusArchived, domain.ProjectStatusActive, false},
		{"archived to completed (invalid)", domain.ProjectStatusArchived, domain.ProjectStatusCompleted, true},
		{"same status no-op", domain.ProjectStatusActive, domain.ProjectStatusActive, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateProjectTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectTransition(%s, %s) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}
```

#### `internal/domain/goal_test.go`

```go
func TestValidateGoalTransition(t *testing.T) {
	// Same pattern as project transitions
}

func TestGoalAncestryChain_ContainsCycle(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	chain := domain.GoalAncestryChain{id1, id2}

	if !chain.ContainsCycle(id1) {
		t.Error("expected cycle detected for id1")
	}
	if chain.ContainsCycle(id3) {
		t.Error("expected no cycle for id3")
	}
}

func TestGoalAncestryChain_Depth(t *testing.T) {
	chain := domain.GoalAncestryChain{uuid.New(), uuid.New()}
	if chain.Depth() != 3 {
		t.Errorf("expected depth 3, got %d", chain.Depth())
	}

	empty := domain.GoalAncestryChain{}
	if empty.Depth() != 1 {
		t.Errorf("expected depth 1 for empty chain, got %d", empty.Depth())
	}
}
```

### Integration Tests

#### `internal/server/handlers/project_handler_test.go`

Test against a real (embedded) PostgreSQL instance:

1. **Create project** — verify 201, correct JSON, default status "active".
2. **Create duplicate name** — verify 409 with `PROJECT_NAME_TAKEN`.
3. **List projects** — create 3 projects, verify all returned, ordered by `createdAt DESC`.
4. **Update project name** — verify 200, name changed.
5. **Update project status (valid)** — active to completed, verify 200.
6. **Update project status (invalid)** — archived to completed, verify 422.
7. **Squad isolation** — create project in squad A, list from squad B, verify empty.
8. **Auth required** — request without JWT, verify 401.
9. **Membership required** — request to squad user is not a member of, verify 403.

#### `internal/server/handlers/goal_handler_test.go`

1. **Create top-level goal** — verify 201, `parentId` is null.
2. **Create sub-goal** — verify 201, `parentId` set correctly.
3. **Create goal with cross-squad parent** — verify 422 `CROSS_SQUAD_REFERENCE`.
4. **Cycle detection: self-reference** — create goal, update parentId to itself, verify 422.
5. **Cycle detection: indirect** — A -> B -> C, then update A.parentId = C, verify 422.
6. **Max depth: depth 5 allowed** — create chain of 5, verify success.
7. **Max depth: depth 6 rejected** — create chain of 5, add child to level 5, verify 422.
8. **List with parentId=null filter** — verify only top-level goals returned.
9. **List with parentId=uuid filter** — verify only direct children returned.
10. **Status transitions** — same matrix as projects.
11. **Squad isolation** — same pattern as projects.

### End-to-End Tests

1. **Full project lifecycle:** Create squad, create project, list projects, update to completed, update to archived, restore to active.
2. **Goal hierarchy workflow:** Create squad, create top-level goal, create 4 levels of sub-goals, attempt 6th level (fail), list with filter.
3. **Issue linkage:** Create project + goal, create issue with `projectId` and `goalId`, verify issue response includes both IDs.

---

## 13. Data Flow

```
Client (React UI / API consumer)
    │
    ▼
Auth Middleware (JWT validation, user context)
    │
    ▼
Squad Membership Middleware (verify user is member of target squad)
    │
    ├──▶ ProjectHandler / GoalHandler
    │        │
    │        ├── Validate request body (name/title length, status enum)
    │        ├── Business rules (uniqueness, status transitions, hierarchy)
    │        │       │
    │        │       └── GetGoalAncestors (recursive CTE for cycle/depth check)
    │        │
    │        └── sqlc Queries (parameterized SQL)
    │                │
    │                ▼
    │           PostgreSQL
    │
    ▼
JSON Response
```

---

## 14. Alternatives Considered

### Alternative 1: Materialized Path for Goal Hierarchy

**Description:** Store the full path (e.g., `/root-id/parent-id/child-id/`) in a `path` column instead of using a recursive CTE for ancestor lookups.

**Pros:**
- Depth and ancestry checks become simple string operations.
- No recursive CTE needed.

**Cons:**
- Path column must be updated for all descendants when a goal is re-parented.
- More complex write operations.
- Harder to maintain consistency.

**Rejected Because:** The recursive CTE approach is simpler for Phase 1 where goal trees are expected to be shallow (max depth 5) and re-parenting is infrequent. Materialized paths add write complexity we do not need yet.

### Alternative 2: Database-Level Cycle Detection via Trigger

**Description:** Use a PostgreSQL trigger on the `goals` table to prevent circular references at the database level.

**Pros:**
- Enforced at the lowest level; impossible to bypass.

**Cons:**
- Triggers are harder to test, debug, and maintain.
- Error messages from triggers are less user-friendly.
- Application-level validation gives us more control over error codes and messages.

**Rejected Because:** Application-level cycle detection is sufficient, testable, and produces clean error responses. A trigger can be added later as a safety net if needed.

---

## 15. Timeline Estimate

- Requirements: 1 day -- Complete
- Design: 1 day -- Complete
- Implementation: 3 days (migrations + sqlc + handlers + router wiring)
- Testing: 2 days (unit + integration + E2E)
- Total: 7 days

---

## 16. Requirement Traceability

| Requirement | Design Section | Implementation |
|-------------|---------------|----------------|
| REQ-PG-001 to REQ-PG-005 | 4.1 (projects table), 3.1 (Project struct) | `project.go`, `create_projects.sql` |
| REQ-PG-010 to REQ-PG-016 | 4.2 (goals table), 3.1 (Goal struct), 6 (hierarchy) | `goal.go`, `create_goals.sql` |
| REQ-PG-020 to REQ-PG-027 | 5.1-5.3 (project API contracts) | `project_handler.go` |
| REQ-PG-030 to REQ-PG-038 | 5.4-5.6 (goal API contracts) | `goal_handler.go` |
| REQ-PG-040 to REQ-PG-044 | 8 (issue linkage) | `add_issue_project_goal_fks.sql` |
| REQ-PG-050 to REQ-PG-052 | 7 (status transitions) | `ValidateProjectTransition`, `ValidateGoalTransition` |
| REQ-PG-060 to REQ-PG-063 | 10 (security, squad isolation) | Squad membership middleware + WHERE clauses |
| REQ-PG-070 | 8.4 (cost attribution) | Schema compatibility; Phase 2 implementation |
| REQ-PG-NF-001 to NF-003 | 11 (performance) | Database indexes |

---

## References

- [Requirements Document](./requirements.md)
- [Squad Management Requirements](../03-squad-management/requirements.md)
- [PRD](../../core/01-PRODUCT.md) -- Section 4.2 Core Entities
