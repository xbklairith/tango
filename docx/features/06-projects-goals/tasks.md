# Tasks: Projects & Goals

**Created:** 2026-03-14
**Status:** Not Started

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Requirement coverage:
  - REQ-PG-001..005 -> Tasks 1, 2, 3 (Database Schema, Domain Types, Project CRUD)
  - REQ-PG-010..016 -> Tasks 1, 2, 4, 5 (Database Schema, Domain Types, Goal CRUD, Goal Hierarchy)
  - REQ-PG-020..027 -> Tasks 3, 9b (Project CRUD Handler, Get Project by ID)
  - REQ-PG-030..038 -> Tasks 4, 13b (Goal CRUD Handler, Get Goal by ID)
  - REQ-PG-040..044 -> Task 6 (Issue Linkage)
  - REQ-PG-050..052 -> Tasks 2, 3, 4 (Domain Types, Project CRUD, Goal CRUD)
  - REQ-PG-060..063 -> Tasks 3, 4 (Project CRUD, Goal CRUD)
  - REQ-PG-070 -> Task 1 (Database Schema, forward-looking columns)
  - REQ-PG-NF-001..003 -> Task 1 (Database indexes)
- Missing coverage: None

## Implementation Approach

Build bottom-up: database schema and migrations first, then domain types with status transition logic, followed by HTTP handlers for projects (simpler, flat entity) and goals (complex, hierarchical). Goal hierarchy validation (cycle detection, max depth) is isolated as its own task because it involves the recursive CTE and application-level checks. Issue linkage modifies the existing issues table and handler. Integration tests verify end-to-end flows across all components. Each task follows strict TDD Red-Green-Refactor.

## Progress Summary

- Total Tasks: 23
- Completed: 0/23
- In Progress: None
- Test Coverage: 0%

## Tasks (TDD: Red-Green-Refactor)

### Component 1: Database Schema

#### Task 1: Create Projects Table Migration

**Linked Requirements:** REQ-PG-001, REQ-PG-002, REQ-PG-003, REQ-PG-004, REQ-PG-005, REQ-PG-NF-002

**RED Phase:**
- [ ] Write a Go test that runs goose migrations and verifies the `projects` table exists with all expected columns
  - Test case: Apply migrations, query `information_schema.columns` for `projects` table, assert columns `id`, `squad_id`, `name`, `description`, `status`, `created_at`, `updated_at` exist with correct types
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/XXXXXX_create_projects.sql`
  - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
  - `squad_id UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE`
  - `name VARCHAR(255) NOT NULL`
  - `description TEXT`
  - `status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived'))`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `CONSTRAINT uq_projects_squad_name UNIQUE (squad_id, name)`
  - `CREATE INDEX idx_projects_squad_id ON projects(squad_id)`
  - `CREATE INDEX idx_projects_status ON projects(squad_id, status)`

**REFACTOR Phase:**
- [ ] Verify down migration drops the table cleanly
  - Focus: Ensure rollback is safe and idempotent

**Acceptance Criteria:**
- [ ] Migration applies without error on a clean database
- [ ] `projects` table has all 7 columns with correct types and constraints
- [ ] Unique constraint on `(squad_id, name)` is enforced
- [ ] `status` CHECK constraint rejects invalid values
- [ ] Default status is `'active'`
- [ ] Indexes `idx_projects_squad_id` and `idx_projects_status` exist
- [ ] Down migration drops the table

**Notes:**
- Migration sequence number must follow the last existing migration

---

#### Task 2: Create Goals Table Migration

**Linked Requirements:** REQ-PG-010, REQ-PG-011, REQ-PG-012, REQ-PG-013, REQ-PG-014, REQ-PG-NF-002, REQ-PG-NF-003

**RED Phase:**
- [ ] Write a Go test that runs goose migrations and verifies the `goals` table exists with all expected columns including the self-referential `parent_id`
  - Test case: Apply migrations, verify columns `id`, `squad_id`, `parent_id`, `title`, `description`, `status`, `created_at`, `updated_at` exist; verify `parent_id` FK references `goals(id)`
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/XXXXXX_create_goals.sql`
  - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
  - `squad_id UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE`
  - `parent_id UUID REFERENCES goals(id) ON DELETE SET NULL`
  - `title VARCHAR(255) NOT NULL`
  - `description TEXT`
  - `status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived'))`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - Indexes: `idx_goals_squad_id`, `idx_goals_parent_id`, `idx_goals_status`

**REFACTOR Phase:**
- [ ] Verify down migration drops cleanly
  - Focus: Ensure FK self-reference does not block drop

**Acceptance Criteria:**
- [ ] Migration applies without error
- [ ] `goals` table has all 8 columns with correct types
- [ ] `parent_id` is nullable and references `goals(id)` with `ON DELETE SET NULL`
- [ ] Default status is `'active'`
- [ ] All three indexes exist
- [ ] Down migration drops the table

---

#### Task 3: Add Issue Linkage Columns Migration

**Linked Requirements:** REQ-PG-040, REQ-PG-041, REQ-PG-070

**RED Phase:**
- [ ] Write a test that verifies `issues` table has `project_id` and `goal_id` columns after migration
  - Test case: Apply all migrations, query `information_schema.columns` for `project_id` and `goal_id` on `issues`
  - Expected failure: Columns do not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/XXXXXX_add_issue_project_goal_fks.sql`
  - `ALTER TABLE issues ADD COLUMN project_id UUID REFERENCES projects(id) ON DELETE SET NULL`
  - `ALTER TABLE issues ADD COLUMN goal_id UUID REFERENCES goals(id) ON DELETE SET NULL`
  - `CREATE INDEX idx_issues_project_id ON issues(project_id)`
  - `CREATE INDEX idx_issues_goal_id ON issues(goal_id)`

**REFACTOR Phase:**
- [ ] Verify down migration removes columns cleanly
  - Focus: Ensure existing issue data is not affected

**Acceptance Criteria:**
- [ ] Both columns are nullable UUID with correct FK references
- [ ] `ON DELETE SET NULL` behavior is correct for both FKs
- [ ] Indexes `idx_issues_project_id` and `idx_issues_goal_id` exist
- [ ] Down migration drops both columns and indexes
- [ ] Existing issues are unaffected (columns default to NULL)

---

#### Task 4: Write sqlc Queries for Projects

**Linked Requirements:** REQ-PG-001, REQ-PG-002, REQ-PG-005, REQ-PG-020, REQ-PG-027

**RED Phase:**
- [ ] Write tests against the generated sqlc code for project queries (compile-time verification)
  - Test case: Verify `CreateProject`, `GetProjectByID`, `ListProjectsBySquad`, `UpdateProject`, `ProjectExistsByName`, `ProjectExistsByNameExcluding` are generated and callable
  - Expected failure: Generated code does not exist

**GREEN Phase:**
- [ ] Create `internal/database/queries/projects.sql` with all 6 queries as specified in design
  - `CreateProject :one` — INSERT with default status 'active'
  - `GetProjectByID :one` — SELECT by id
  - `ListProjectsBySquad :many` — SELECT WHERE squad_id, ORDER BY created_at DESC
  - `UpdateProject :one` — COALESCE-based partial update with RETURNING
  - `ProjectExistsByName :one` — EXISTS check for (squad_id, name)
  - `ProjectExistsByNameExcluding :one` — EXISTS check excluding a specific project id
- [ ] Run `make sqlc` to generate Go code

**REFACTOR Phase:**
- [ ] Review generated Go types for correctness
  - Focus: Ensure nullable fields map to proper Go types (e.g., `sql.NullString` for description)

**Acceptance Criteria:**
- [ ] `make sqlc` succeeds without errors
- [ ] All 6 query functions are generated in `internal/database/db/`
- [ ] `ListProjectsBySquad` orders by `created_at DESC`
- [ ] `UpdateProject` uses COALESCE for partial updates
- [ ] `ProjectExistsByName` returns a boolean

---

#### Task 5: Write sqlc Queries for Goals (Including Recursive CTE)

**Linked Requirements:** REQ-PG-010, REQ-PG-014, REQ-PG-016, REQ-PG-030, REQ-PG-037, REQ-PG-038

**RED Phase:**
- [ ] Write tests verifying generated sqlc code for goal queries is callable
  - Test case: Verify `CreateGoal`, `GetGoalByID`, `ListGoalsBySquad`, `ListTopLevelGoalsBySquad`, `ListGoalsBySquadAndParent`, `UpdateGoal`, `GetGoalAncestors` are generated
  - Expected failure: Generated code does not exist

**GREEN Phase:**
- [ ] Create `internal/database/queries/goals.sql` with all 7 queries as specified in design
  - `CreateGoal :one` — INSERT with parent_id nullable, default status 'active'
  - `GetGoalByID :one` — SELECT by id
  - `ListGoalsBySquad :many` — SELECT WHERE squad_id, ORDER BY created_at DESC
  - `ListTopLevelGoalsBySquad :many` — SELECT WHERE squad_id AND parent_id IS NULL
  - `ListGoalsBySquadAndParent :many` — SELECT WHERE squad_id AND parent_id = $2
  - `UpdateGoal :one` — COALESCE-based partial update with RETURNING
  - `GetGoalAncestors :many` — Recursive CTE walking parent_id chain upward, bounded at depth < 6
- [ ] Run `make sqlc` to generate Go code

**REFACTOR Phase:**
- [ ] Verify recursive CTE has safety bound (`depth < 6`) to prevent runaway recursion
  - Focus: Ensure CTE terminates even if a cycle exists in the data

**Acceptance Criteria:**
- [ ] `make sqlc` succeeds without errors
- [ ] All 7 query functions are generated
- [ ] `GetGoalAncestors` returns ancestor IDs from immediate parent to root
- [ ] Recursive CTE is bounded at depth 6
- [ ] `ListTopLevelGoalsBySquad` filters on `parent_id IS NULL`
- [ ] `ListGoalsBySquadAndParent` filters on specific `parent_id`

---

### Component 2: Domain Types

#### Task 6: Implement Project Domain Type and Status Transitions

**Linked Requirements:** REQ-PG-001, REQ-PG-002, REQ-PG-004, REQ-PG-050, REQ-PG-052

**RED Phase:**
- [ ] Write `internal/domain/project_test.go` with table-driven tests for `ValidateProjectTransition`
  - Test cases:
    - `active -> completed` (valid)
    - `active -> archived` (valid)
    - `completed -> active` (valid, reopen)
    - `completed -> archived` (valid)
    - `archived -> active` (valid, restore)
    - `archived -> completed` (INVALID, must return error)
    - Same status no-op (valid, no error)
  - Expected failure: Function does not exist

**GREEN Phase:**
- [ ] Create `internal/domain/project.go`
  - Define `ProjectStatus` type with constants (`active`, `completed`, `archived`)
  - Define `ValidProjectStatuses` map
  - Define `validProjectTransitions` map
  - Implement `ValidateProjectTransition(from, to) error`
  - Define `Project` struct, `CreateProjectInput`, `UpdateProjectInput`

**REFACTOR Phase:**
- [ ] Extract shared status validation pattern if applicable
  - Focus: Clean type naming, ensure JSON tags use camelCase

**Acceptance Criteria:**
- [ ] All 7 transition test cases pass
- [ ] `ValidateProjectTransition` returns descriptive error with from/to status for invalid transitions
- [ ] `Project` struct has correct JSON tags matching API contract
- [ ] `CreateProjectInput` has `SquadID`, `Name`, `Description` fields
- [ ] `UpdateProjectInput` uses pointer fields for optional updates

---

#### Task 7: Implement Goal Domain Type, Status Transitions, and Ancestry Chain

**Linked Requirements:** REQ-PG-010, REQ-PG-011, REQ-PG-013, REQ-PG-015, REQ-PG-016, REQ-PG-051, REQ-PG-052

**RED Phase:**
- [ ] Write `internal/domain/goal_test.go` with tests for:
  - `ValidateGoalTransition` — same 7 transition cases as project
  - `GoalAncestryChain.ContainsCycle` — chain with and without the target ID
  - `GoalAncestryChain.Depth` — empty chain returns 1, chain of 4 returns 5
  - `MaxGoalDepth` constant equals 5
  - Expected failure: Types and functions do not exist

**GREEN Phase:**
- [ ] Create `internal/domain/goal.go`
  - Define `GoalStatus` type with constants
  - Define `validGoalTransitions` map
  - Implement `ValidateGoalTransition(from, to) error`
  - Define `Goal` struct, `CreateGoalInput`, `UpdateGoalInput`
  - Define `GoalAncestryChain` type with `ContainsCycle(goalID)` and `Depth()` methods
  - Define `MaxGoalDepth = 5` constant

**REFACTOR Phase:**
- [ ] Ensure `UpdateGoalInput.ParentID` semantics are clear (nil = no change, &uuid.Nil = unset parent)
  - Focus: Add doc comments explaining ParentID update semantics

**Acceptance Criteria:**
- [ ] All transition test cases pass (same rules as project)
- [ ] `GoalAncestryChain.ContainsCycle` returns true when goalID is in chain, false otherwise
- [ ] `GoalAncestryChain.Depth` returns `len(chain) + 1`
- [ ] `MaxGoalDepth` is 5
- [ ] `Goal` struct has `ParentID *uuid.UUID` with correct JSON tags
- [ ] `CreateGoalInput` includes `ParentID *uuid.UUID`

---

### Component 3: Project CRUD Handler

#### Task 8: Implement Create Project Handler

**Linked Requirements:** REQ-PG-002, REQ-PG-003, REQ-PG-004, REQ-PG-005, REQ-PG-021, REQ-PG-023, REQ-PG-024, REQ-PG-025, REQ-PG-060

**RED Phase:**
- [ ] Write `internal/server/handlers/project_handler_test.go` with tests for `Create`:
  - Test: Valid creation returns 201 with project JSON, status defaults to "active"
  - Test: Missing name returns 400 `VALIDATION_ERROR`
  - Test: Name exceeding 255 chars returns 400 `VALIDATION_ERROR`
  - Test: Duplicate name in same squad returns 409 `PROJECT_NAME_TAKEN`
  - Test: Unauthenticated request returns 401
  - Test: Non-member request returns 403
  - Test: Non-existent squad returns 404
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Create `internal/server/handlers/project_handler.go`
  - Define `ProjectHandler` struct with `ProjectQueries` interface
  - Implement `Create` method:
    1. Extract squadID from URL path
    2. Verify squad membership from middleware context
    3. Decode and validate `createProjectRequest` (name required, 1-255 chars)
    4. Check uniqueness via `ProjectExistsByName`
    5. Insert via `CreateProject`
    6. Return 201 with project JSON

**REFACTOR Phase:**
- [ ] Extract input validation into a helper if reused
  - Focus: Consistent error response format

**Acceptance Criteria:**
- [ ] POST returns 201 with correct project JSON (id, squadId, name, description, status, timestamps)
- [ ] Default status is "active"
- [ ] Empty name is rejected with 400
- [ ] Name > 255 chars is rejected with 400
- [ ] Duplicate name within squad returns 409
- [ ] Squad membership is enforced (403 for non-members)
- [ ] Missing auth returns 401

---

#### Task 9: Implement List Projects Handler

**Linked Requirements:** REQ-PG-020, REQ-PG-023, REQ-PG-024, REQ-PG-025, REQ-PG-027, REQ-PG-060

**RED Phase:**
- [ ] Write tests for `List`:
  - Test: Returns 200 with array of projects ordered by createdAt DESC
  - Test: Returns empty array `[]` when no projects exist
  - Test: Only returns projects for the specified squad (data isolation)
  - Test: Unauthenticated returns 401
  - Test: Non-member returns 403
  - Expected failure: Handler method does not exist

**GREEN Phase:**
- [ ] Implement `List` method on `ProjectHandler`
  1. Extract squadID from URL path
  2. Verify squad membership
  3. Query `ListProjectsBySquad(ctx, squadID)`
  4. Map db rows to domain.Project slice
  5. Return JSON array (empty array if none, never null)

**REFACTOR Phase:**
- [ ] Ensure empty result returns `[]` not `null` in JSON
  - Focus: Initialize slice before marshaling

**Acceptance Criteria:**
- [ ] GET returns 200 with JSON array of projects
- [ ] Projects are ordered by createdAt descending (newest first)
- [ ] Empty squad returns `[]` not `null`
- [ ] Only projects belonging to the squad are returned
- [ ] Auth and membership checks are enforced

---

#### Task 9b: Implement Get Project by ID Handler

**Linked Requirements:** REQ-PG-023, REQ-PG-024, REQ-PG-025, REQ-PG-026, REQ-PG-060

**RED Phase:**
- [ ] Write tests for `GetByID`:
  - Test: Returns 200 with project JSON for a valid project in the squad
  - Test: Non-existent project returns 404
  - Test: Project belonging to a different squad returns 404 (data isolation)
  - Test: Unauthenticated returns 401
  - Test: Non-member returns 403
  - Expected failure: Handler method does not exist

**GREEN Phase:**
- [ ] Implement `GetByID` method on `ProjectHandler`
  1. Extract squadID and projectID from URL path
  2. Verify squad membership via middleware context
  3. Fetch project via `GetProjectByID(ctx, projectID)`
     - If not found, return 404
  4. Verify `project.SquadID == squadID` (return 404 if mismatch for isolation)
  5. Return 200 with project JSON

**REFACTOR Phase:**
- [ ] Ensure consistent error response format with other handlers
  - Focus: Reuse existing error helpers

**Acceptance Criteria:**
- [ ] GET returns 200 with correct project JSON (id, squadId, name, description, status, timestamps)
- [ ] Non-existent project returns 404
- [ ] Project from another squad returns 404 (not 403, to avoid leaking existence)
- [ ] Auth and membership checks are enforced

---

#### Task 10: Implement Update Project Handler with Status Transition Validation

**Linked Requirements:** REQ-PG-004, REQ-PG-005, REQ-PG-022, REQ-PG-023, REQ-PG-024, REQ-PG-026, REQ-PG-044, REQ-PG-050, REQ-PG-052, REQ-PG-062

**RED Phase:**
- [ ] Write tests for `Update`:
  - Test: Valid name update returns 200 with updated project
  - Test: Valid status transition (active -> completed) returns 200
  - Test: Invalid status transition (archived -> completed) returns 422 `INVALID_STATUS_TRANSITION` with current and attempted status in message
  - Test: Duplicate name update within squad returns 409
  - Test: Non-existent project returns 404
  - Test: Update project from another squad returns 403
  - Test: Unauthenticated returns 401
  - Test: Partial update (only description) leaves other fields unchanged
  - Expected failure: Handler method does not exist

**GREEN Phase:**
- [ ] Implement `Update` method on `ProjectHandler`
  1. Extract projectID from URL path
  2. Fetch existing project via `GetProjectByID` (404 if not found)
  3. Verify squad membership for `project.SquadID`
  4. Decode `updateProjectRequest`
  5. If status change: validate via `ValidateProjectTransition` (422 if invalid)
  6. If name change: check uniqueness via `ProjectExistsByNameExcluding` (409 if duplicate)
  7. Apply via `UpdateProject`
  8. Return 200 with updated project

**REFACTOR Phase:**
- [ ] Ensure error messages include both current and attempted status per REQ-PG-052
  - Focus: Error message formatting consistency

**Acceptance Criteria:**
- [ ] PATCH returns 200 with updated project JSON
- [ ] Only provided fields are updated (COALESCE behavior)
- [ ] Invalid status transitions return 422 with descriptive error
- [ ] Name uniqueness is checked excluding the current project
- [ ] Squad-scoped authorization is enforced
- [ ] Status changes are allowed even when issues reference the project (REQ-PG-044)

---

#### Task 11: Register Project Routes in Router

**Linked Requirements:** REQ-PG-020, REQ-PG-021, REQ-PG-022

**RED Phase:**
- [ ] Write a test that verifies project routes are registered and return non-405 responses
  - Test case: Send requests to `GET /api/squads/{id}/projects`, `GET /api/squads/{id}/projects/{id}`, `POST /api/squads/{id}/projects`, `PATCH /api/projects/{id}` and verify they do not return 405 Method Not Allowed
  - Expected failure: Routes not registered, returns 404/405

**GREEN Phase:**
- [ ] Wire up `ProjectHandler` in `internal/server/router.go`
  - `GET /api/squads/{squadID}/projects` -> `authMiddleware(projectHandler.List)`
  - `GET /api/squads/{squadID}/projects/{projectID}` -> `authMiddleware(projectHandler.GetByID)`
  - `POST /api/squads/{squadID}/projects` -> `authMiddleware(projectHandler.Create)`
  - `PATCH /api/projects/{projectID}` -> `authMiddleware(projectHandler.Update)`
- [ ] Instantiate `ProjectHandler` with sqlc queries in server setup

**REFACTOR Phase:**
- [ ] Verify route patterns match the API contract exactly
  - Focus: Consistent path parameter naming (`squadID`, `projectID`)

**Acceptance Criteria:**
- [ ] All four project routes are registered
- [ ] Routes require authentication middleware
- [ ] Path parameters are correctly extracted
- [ ] Server starts without errors

---

### Component 4: Goal CRUD Handler

#### Task 12: Implement Create Goal Handler

**Linked Requirements:** REQ-PG-011, REQ-PG-012, REQ-PG-013, REQ-PG-014, REQ-PG-015, REQ-PG-016, REQ-PG-031, REQ-PG-033, REQ-PG-034, REQ-PG-035, REQ-PG-061

**RED Phase:**
- [ ] Write `internal/server/handlers/goal_handler_test.go` with tests for `Create`:
  - Test: Valid top-level goal creation (no parentId) returns 201 with status "active"
  - Test: Valid sub-goal creation (with parentId) returns 201
  - Test: Missing title returns 400 `VALIDATION_ERROR`
  - Test: Title exceeding 255 chars returns 400
  - Test: Parent goal in different squad returns 422 `CROSS_SQUAD_REFERENCE`
  - Test: Non-existent parent goal returns 404
  - Test: Parent that would exceed max depth (5) returns 422 `MAX_DEPTH_EXCEEDED`
  - Test: Unauthenticated returns 401
  - Test: Non-member returns 403
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Create `internal/server/handlers/goal_handler.go`
  - Define `GoalHandler` struct with `GoalQueries` interface
  - Implement `Create` method:
    1. Extract squadID from URL path
    2. Verify squad membership
    3. Decode and validate `createGoalRequest` (title required, 1-255 chars)
    4. If parentID provided:
       a. Fetch parent goal, verify exists
       b. Verify parent belongs to same squad
       c. Get ancestor chain via `GetGoalAncestors`
       d. Check depth: `len(ancestors) + 2 <= MaxGoalDepth`
    5. Insert via `CreateGoal`
    6. Return 201 with goal JSON

**REFACTOR Phase:**
- [ ] Extract hierarchy validation into `validateGoalHierarchy` helper function
  - Focus: Reusable for both create and update operations

**Acceptance Criteria:**
- [ ] POST returns 201 with correct goal JSON (id, squadId, parentId, title, description, status, timestamps)
- [ ] Default status is "active"
- [ ] Top-level goals (no parentId) are created successfully
- [ ] Sub-goals under valid parents are created successfully
- [ ] Cross-squad parent reference returns 422
- [ ] Exceeding max depth returns 422
- [ ] Auth and membership are enforced

---

#### Task 13: Implement List Goals Handler with parentId Filter

**Linked Requirements:** REQ-PG-030, REQ-PG-033, REQ-PG-034, REQ-PG-035, REQ-PG-037, REQ-PG-038, REQ-PG-061

**RED Phase:**
- [ ] Write tests for `List`:
  - Test: No filter returns 200 with all goals for squad, ordered by createdAt DESC
  - Test: `?parentId=null` returns only top-level goals (parentId IS NULL)
  - Test: `?parentId=<uuid>` returns only direct children of that goal
  - Test: Returns empty array when no goals match
  - Test: Unauthenticated returns 401
  - Test: Non-member returns 403
  - Expected failure: Handler method does not exist

**GREEN Phase:**
- [ ] Implement `List` method on `GoalHandler`
  1. Extract squadID from URL path
  2. Verify squad membership
  3. Parse `parentId` query parameter:
     - Absent -> `ListGoalsBySquad(ctx, squadID)` (all goals)
     - `"null"` -> `ListTopLevelGoalsBySquad(ctx, squadID)`
     - UUID value -> `ListGoalsBySquadAndParent(ctx, {squadID, parentID})`
  4. Map and return JSON array

**REFACTOR Phase:**
- [ ] Ensure consistent query parameter parsing (case-insensitive "null")
  - Focus: Edge cases in parentId parsing

**Acceptance Criteria:**
- [ ] GET with no filter returns all goals for the squad
- [ ] GET with `?parentId=null` returns only root goals
- [ ] GET with `?parentId=<uuid>` returns only direct children
- [ ] Results are ordered by createdAt descending
- [ ] Empty result returns `[]` not `null`
- [ ] Invalid parentId UUID returns 400

---

#### Task 13b: Implement Get Goal by ID Handler

**Linked Requirements:** REQ-PG-033, REQ-PG-034, REQ-PG-035, REQ-PG-036, REQ-PG-061

**RED Phase:**
- [ ] Write tests for `GetByID`:
  - Test: Returns 200 with goal JSON for a valid goal in the squad
  - Test: Non-existent goal returns 404
  - Test: Goal belonging to a different squad returns 404 (data isolation)
  - Test: Unauthenticated returns 401
  - Test: Non-member returns 403
  - Expected failure: Handler method does not exist

**GREEN Phase:**
- [ ] Implement `GetByID` method on `GoalHandler`
  1. Extract squadID and goalID from URL path
  2. Verify squad membership via middleware context
  3. Fetch goal via `GetGoalByID(ctx, goalID)`
     - If not found, return 404
  4. Verify `goal.SquadID == squadID` (return 404 if mismatch for isolation)
  5. Return 200 with goal JSON

**REFACTOR Phase:**
- [ ] Ensure consistent error response format with other handlers
  - Focus: Reuse existing error helpers

**Acceptance Criteria:**
- [ ] GET returns 200 with correct goal JSON (id, squadId, parentId, title, description, status, timestamps)
- [ ] Non-existent goal returns 404
- [ ] Goal from another squad returns 404 (not 403, to avoid leaking existence)
- [ ] Auth and membership checks are enforced

---

#### Task 14: Implement Update Goal Handler with Status and Hierarchy Validation

**Linked Requirements:** REQ-PG-013, REQ-PG-014, REQ-PG-015, REQ-PG-016, REQ-PG-032, REQ-PG-033, REQ-PG-034, REQ-PG-036, REQ-PG-044, REQ-PG-051, REQ-PG-052, REQ-PG-063

**RED Phase:**
- [ ] Write tests for `Update`:
  - Test: Valid title update returns 200
  - Test: Valid status transition (active -> completed) returns 200
  - Test: Invalid status transition (archived -> completed) returns 422 `INVALID_STATUS_TRANSITION`
  - Test: Valid parent change returns 200
  - Test: Self-referential parent (goalID == newParentID) returns 422 `CIRCULAR_REFERENCE`
  - Test: Circular reference via ancestor chain returns 422 `CIRCULAR_REFERENCE`
  - Test: Parent change exceeding max depth returns 422 `MAX_DEPTH_EXCEEDED`
  - Test: Parent in different squad returns 422 `CROSS_SQUAD_REFERENCE`
  - Test: Non-existent goal returns 404
  - Test: Non-member returns 403
  - Expected failure: Handler method does not exist

**GREEN Phase:**
- [ ] Implement `Update` method on `GoalHandler`
  1. Extract goalID from URL path
  2. Fetch existing goal via `GetGoalByID` (404 if not found)
  3. Verify squad membership for `goal.SquadID`
  4. Decode `updateGoalRequest`
  5. If status change: validate via `ValidateGoalTransition` (422 if invalid)
  6. If parentID change:
     a. Validate new parent exists and belongs to same squad
     b. Check self-reference: `goalID == newParentID`
     c. Get ancestor chain of new parent via `GetGoalAncestors`
     d. Check cycle: `GoalAncestryChain.ContainsCycle(goalID)`
     e. Check depth: `len(ancestors) + 2 <= MaxGoalDepth`
  7. Apply via `UpdateGoal`
  8. Return 200 with updated goal

**REFACTOR Phase:**
- [ ] Ensure `validateGoalHierarchy` helper is shared between Create and Update
  - Focus: DRY principle for hierarchy validation logic

**Acceptance Criteria:**
- [ ] PATCH returns 200 with updated goal JSON
- [ ] Only provided fields are updated
- [ ] Invalid status transitions return 422 with both statuses in error message
- [ ] Self-referential parent returns 422 `CIRCULAR_REFERENCE`
- [ ] Indirect cycle returns 422 `CIRCULAR_REFERENCE`
- [ ] Depth > 5 after reparenting returns 422 `MAX_DEPTH_EXCEEDED`
- [ ] Cross-squad parent returns 422 `CROSS_SQUAD_REFERENCE`
- [ ] Status changes allowed even with linked issues (REQ-PG-044)

---

#### Task 15: Register Goal Routes in Router

**Linked Requirements:** REQ-PG-030, REQ-PG-031, REQ-PG-032

**RED Phase:**
- [ ] Write a test that verifies goal routes are registered and return non-405 responses
  - Test case: Send requests to `GET /api/squads/{id}/goals`, `GET /api/squads/{id}/goals/{id}`, `POST /api/squads/{id}/goals`, `PATCH /api/goals/{id}` and verify they do not return 405
  - Expected failure: Routes not registered

**GREEN Phase:**
- [ ] Wire up `GoalHandler` in `internal/server/router.go`
  - `GET /api/squads/{squadID}/goals` -> `authMiddleware(goalHandler.List)`
  - `GET /api/squads/{squadID}/goals/{goalID}` -> `authMiddleware(goalHandler.GetByID)`
  - `POST /api/squads/{squadID}/goals` -> `authMiddleware(goalHandler.Create)`
  - `PATCH /api/goals/{goalID}` -> `authMiddleware(goalHandler.Update)`
- [ ] Instantiate `GoalHandler` with sqlc queries in server setup

**REFACTOR Phase:**
- [ ] Verify route patterns match the API contract
  - Focus: Consistent path parameter naming (`squadID`, `goalID`)

**Acceptance Criteria:**
- [ ] All four goal routes are registered
- [ ] Routes require authentication middleware
- [ ] Path parameters are correctly extracted
- [ ] Server starts without errors

---

### Component 5: Goal Hierarchy

#### Task 16: Implement Cycle Detection via Ancestor Chain

**Linked Requirements:** REQ-PG-015

**RED Phase:**
- [ ] Write unit tests for `validateGoalHierarchy` helper function:
  - Test: Goal set as its own parent returns `CIRCULAR_REFERENCE` error
  - Test: Goal A -> parent B -> parent A (indirect cycle) returns `CIRCULAR_REFERENCE`
  - Test: Deep chain (A -> B -> C -> D -> E) with no cycle returns nil
  - Test: Goal reparented to its own descendant returns `CIRCULAR_REFERENCE`
  - Expected failure: Function does not exist

**GREEN Phase:**
- [ ] Implement `validateGoalHierarchy(ctx, queries, goalID, newParentID) error`
  1. Self-reference check: `goalID == newParentID`
  2. Fetch ancestor chain of `newParentID` via `GetGoalAncestors`
  3. Use `GoalAncestryChain.ContainsCycle(goalID)` to detect cycle
  4. Return `AppError` with code `CIRCULAR_REFERENCE` and HTTP 422 if cycle found

**REFACTOR Phase:**
- [ ] Ensure function is stateless and testable with mock queries interface
  - Focus: Interface-based design for testability

**Acceptance Criteria:**
- [ ] Direct self-reference is detected
- [ ] Indirect cycles (A -> B -> A) are detected
- [ ] Multi-level indirect cycles are detected
- [ ] Valid hierarchy changes return no error
- [ ] Error includes code `CIRCULAR_REFERENCE` and descriptive message

---

#### Task 17: Implement Max Depth Validation (5 Levels)

**Linked Requirements:** REQ-PG-016

**RED Phase:**
- [ ] Write unit tests for depth validation within `validateGoalHierarchy`:
  - Test: Creating a goal at depth 5 (chain of 4 ancestors) succeeds
  - Test: Creating a goal at depth 6 (chain of 5 ancestors) returns `MAX_DEPTH_EXCEEDED`
  - Test: Reparenting a goal such that resulting depth exceeds 5 returns `MAX_DEPTH_EXCEEDED`
  - Test: Top-level goal (depth 1, no ancestors) always succeeds
  - Expected failure: Depth check not implemented

**GREEN Phase:**
- [ ] Add depth check to `validateGoalHierarchy`:
  - Calculate `newDepth = len(ancestors) + 2` (ancestors of parent + parent + the goal itself)
  - If `newDepth > MaxGoalDepth`, return `AppError` with code `MAX_DEPTH_EXCEEDED` and HTTP 422

**REFACTOR Phase:**
- [ ] Add depth examples in code comments for clarity
  - Focus: Document the depth calculation formula

**Acceptance Criteria:**
- [ ] Depth 1 through 5 are allowed
- [ ] Depth 6+ is rejected with 422 `MAX_DEPTH_EXCEEDED`
- [ ] Error message includes the max depth value (5)
- [ ] Depth calculation accounts for the goal itself and all ancestors

**Notes:**
- Depth formula: `newDepth = len(ancestors_of_parent) + 2`. The `+2` accounts for the parent itself and the goal being created/moved.

---

#### Task 18: Cross-Squad Parent Validation

**Linked Requirements:** REQ-PG-014, REQ-PG-012

**RED Phase:**
- [ ] Write tests for cross-squad validation in goal create and update:
  - Test: Creating a goal with parentId in a different squad returns 422 `CROSS_SQUAD_REFERENCE`
  - Test: Updating a goal's parentId to a goal in a different squad returns 422 `CROSS_SQUAD_REFERENCE`
  - Test: Same-squad parent reference succeeds
  - Expected failure: Cross-squad check not implemented

**GREEN Phase:**
- [ ] In both `Create` and `Update` handlers, after fetching the parent goal, compare `parentGoal.SquadID` with the target `squadID`
  - If mismatch, return `AppError{Code: "CROSS_SQUAD_REFERENCE", Status: 422}`

**REFACTOR Phase:**
- [ ] Ensure cross-squad check runs before cycle/depth checks (fail fast)
  - Focus: Correct validation ordering

**Acceptance Criteria:**
- [ ] Cross-squad parent on create returns 422 with `CROSS_SQUAD_REFERENCE`
- [ ] Cross-squad parent on update returns 422 with `CROSS_SQUAD_REFERENCE`
- [ ] Same-squad parent passes validation
- [ ] Error message clearly states the violation

---

### Component 6: Issue Linkage

#### Task 19: Implement projectId/goalId Validation on Issue Create/Update

**Linked Requirements:** REQ-PG-040, REQ-PG-041, REQ-PG-042, REQ-PG-043, REQ-PG-044

**RED Phase:**
- [ ] Write tests in existing issue handler test file:
  - Test: Creating an issue with valid projectId succeeds, response includes projectId
  - Test: Creating an issue with valid goalId succeeds, response includes goalId
  - Test: Creating an issue with non-existent projectId returns 404
  - Test: Creating an issue with non-existent goalId returns 404
  - Test: Creating an issue with projectId from different squad returns 422 `CROSS_SQUAD_REFERENCE`
  - Test: Creating an issue with goalId from different squad returns 422 `CROSS_SQUAD_REFERENCE`
  - Test: Updating an issue's projectId with same-squad check succeeds
  - Test: Updating an issue's goalId with same-squad check succeeds
  - Test: Issue with null projectId/goalId is valid (optional fields)
  - Expected failure: Issue handler does not validate projectId/goalId

**GREEN Phase:**
- [ ] Modify the existing issue handler (create and update):
  1. If `projectId` provided:
     a. Fetch project via `GetProjectByID`
     b. If not found, return 404
     c. If `project.SquadID != issue.SquadID`, return 422 `CROSS_SQUAD_REFERENCE`
  2. If `goalId` provided:
     a. Fetch goal via `GetGoalByID`
     b. If not found, return 404
     c. If `goal.SquadID != issue.SquadID`, return 422 `CROSS_SQUAD_REFERENCE`
- [ ] Update issue create/update request structs to include `ProjectID` and `GoalID` optional fields
- [ ] Update sqlc issue queries to include `project_id` and `goal_id` columns

**REFACTOR Phase:**
- [ ] Extract the FK validation pattern into a reusable helper
  - Focus: DRY pattern for "fetch entity, check existence, check same squad"

**Acceptance Criteria:**
- [ ] Issues can be created/updated with optional projectId and goalId
- [ ] Non-existent project/goal reference returns 404
- [ ] Cross-squad project/goal reference returns 422
- [ ] Null projectId/goalId is accepted (optional)
- [ ] Existing issue tests still pass (no regression)
- [ ] Issue response JSON includes `projectId` and `goalId` fields

---

### Component 7: Integration Tests

#### Task 20: Integration Tests for Project and Goal CRUD Flows

**Linked Requirements:** REQ-PG-001..005, REQ-PG-010..016, REQ-PG-020..027, REQ-PG-030..038, REQ-PG-050..052, REQ-PG-060..063

**RED Phase:**
- [ ] Write integration tests that exercise the full HTTP stack with a real database:
  - **Project CRUD flow:**
    - Create a project -> verify 201 and JSON shape
    - List projects -> verify the created project appears, ordered correctly
    - Update project name -> verify 200 and updated name
    - Update project status (active -> completed) -> verify 200
    - Attempt invalid transition (archived -> completed) -> verify 422
    - Attempt duplicate name in same squad -> verify 409
  - **Goal CRUD flow:**
    - Create a top-level goal -> verify 201
    - Create a sub-goal under the top-level goal -> verify 201 with parentId
    - List all goals -> verify both appear
    - List with `?parentId=null` -> verify only top-level
    - List with `?parentId=<uuid>` -> verify only sub-goal
    - Update goal status -> verify transition
    - Attempt invalid transition -> verify 422
  - **Goal hierarchy:**
    - Create a 5-level deep chain -> verify all succeed
    - Attempt to create a 6th level -> verify 422 `MAX_DEPTH_EXCEEDED`
    - Attempt to create a cycle (A -> B -> A) -> verify 422 `CIRCULAR_REFERENCE`
    - Reparent a goal to create a cycle -> verify 422
  - **Data isolation:**
    - Create projects/goals in squad A
    - Verify they are not visible from squad B's endpoints
  - Expected failure: Some or all flows fail

**GREEN Phase:**
- [ ] Fix any issues found during integration testing
  - Ensure migrations, handlers, and queries all work together end-to-end

**REFACTOR Phase:**
- [ ] Organize integration tests into clear subtests with descriptive names
  - Focus: Test readability and maintainability

**Acceptance Criteria:**
- [ ] All project CRUD operations work end-to-end
- [ ] All goal CRUD operations work end-to-end
- [ ] Goal hierarchy enforcement works (cycle detection, max depth)
- [ ] Status transitions are enforced for both entities
- [ ] Data isolation between squads is verified
- [ ] All integration tests pass with `make test`

---

#### Task 21: Integration Tests for Issue Linkage

**Linked Requirements:** REQ-PG-040..044

**RED Phase:**
- [ ] Write integration tests for issue-project and issue-goal linkage:
  - Create a project and goal in squad A
  - Create an issue in squad A with projectId and goalId -> verify 201
  - Verify issue response includes correct projectId and goalId
  - Update an issue to change projectId -> verify 200
  - Update an issue to set goalId to null -> verify 200
  - Attempt to link issue to project in squad B -> verify 422 `CROSS_SQUAD_REFERENCE`
  - Attempt to link issue to goal in squad B -> verify 422 `CROSS_SQUAD_REFERENCE`
  - Archive a project that has linked issues -> verify project status changes to archived (no cascade blocking)
  - Archive a goal that has linked issues -> verify goal status changes to archived (no cascade blocking)
  - Expected failure: Linkage not fully wired

**GREEN Phase:**
- [ ] Fix any issues found during integration testing

**REFACTOR Phase:**
- [ ] Verify test coverage for all edge cases
  - Focus: Ensure no cascade blocking per REQ-PG-044

**Acceptance Criteria:**
- [ ] Issues can reference projects and goals in the same squad
- [ ] Cross-squad references are rejected
- [ ] Null references are accepted (optional linkage)
- [ ] Project/goal status changes are not blocked by linked issues
- [ ] All integration tests pass with `make test`

---

### Final Verification Tasks

#### Task 22: Pre-Merge Checklist

**Final Checks:**

- [ ] All tasks above completed (Tasks 1-21, 9b, 13b)
- [ ] All tests passing: `make test`
- [ ] No linter errors
- [ ] No type errors
- [ ] `make sqlc` generates without errors
- [ ] All database migrations apply and rollback cleanly
- [ ] Test coverage meets threshold
- [ ] No debug code or commented-out code
- [ ] API responses match documented contracts (design.md section 5)
- [ ] All REQ-PG-* requirements are covered by at least one test
- [ ] Error codes match the error handling table (design.md section 9)

**Acceptance Criteria:**
- [ ] Feature is production-ready
- [ ] All quality gates passed
- [ ] Ready for PR/merge

---

## Task Tracking Legend

- `[ ]` - Not started
- `[~]` - In progress
- `[x]` - Completed

## Commit Strategy

After each completed task:
```bash
# After RED phase
git add internal/
git commit -m "test: Add failing tests for [functionality]"

# After GREEN phase
git add internal/
git commit -m "feat: Implement [functionality]"

# After REFACTOR phase
git add internal/
git commit -m "refactor: Clean up [component]"
```

## Notes

### Implementation Notes

- Tasks 1-5 (Database Schema) must be completed before any handler tasks
- Tasks 6-7 (Domain Types) must be completed before handler tasks
- Tasks 8-11 (Project CRUD) can be done in parallel with Tasks 12-15 (Goal CRUD) but both depend on Tasks 1-7
- Tasks 16-18 (Goal Hierarchy) are logically part of Task 12/14 but isolated for clarity
- Task 19 (Issue Linkage) depends on Tasks 1-3 (migrations) and modifies existing issue handler code
- Tasks 20-21 (Integration Tests) must run last after all components are in place

### Blockers

- [ ] Existing issue handler must be available and functional before Task 19
- [ ] Squad management (feature 03) must be complete for membership checks

### Future Improvements

- Goal progress tracking / percentage completion
- Bulk operations on projects or goals
- Project/goal assignment to agents
- Cost attribution on CostEvent (Phase 2, REQ-PG-070)

### Lessons Learned

[Document insights gained during implementation]
