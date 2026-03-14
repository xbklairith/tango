# Tasks: Activity Log

**Created:** 2026-03-15
**Status:** Ready for Implementation

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-001 through REQ-044
- Missing coverage: None

## Implementation Approach

TDD Red-Green-Refactor in six logical groups:

1. **TASK-01** — DB migration + sqlc queries (foundation)
2. **TASK-02** — Domain types + `logActivity` helper + `changedFieldNames` utility
3. **TASK-03** — Squad handler activity integration (Create, Update, Delete, UpdateBudget)
4. **TASK-04** — Agent + Issue + Comment handler activity integration
5. **TASK-05** — Project + Goal + Membership handler activity integration (includes tx upgrade)
6. **TASK-06** — Activity feed handler (`GET /api/squads/{id}/activity`)
7. **TASK-07** — React activity feed widget on the dashboard

## Progress Summary

- Total Tasks: 7
- Completed: 0
- In Progress: None
- Test Coverage: TBD

---

## Tasks (TDD: Red-Green-Refactor)

---

### TASK-01: Database Migration and sqlc Query Generation

**Requirements:** REQ-027, REQ-028, REQ-029, REQ-040, REQ-043
**Estimated time:** 30–45 min

#### RED — Write failing test(s) first

Write a migration smoke test that verifies the `activity_log` table and its indexes exist after `database.Migrate` runs. Add to a new file `internal/database/migrations_test.go` (or extend the existing test harness in `internal/server/handlers/auth_integration_test.go`):

```go
// TestActivityLogTableExists
//   - Open embedded Postgres, run Migrate
//   - Query information_schema.tables WHERE table_name = 'activity_log'
//   - Assert row exists
//   - Query pg_indexes WHERE tablename = 'activity_log'
//   - Assert idx_activity_log_squad_created_at exists
//   - Assert idx_activity_log_squad_actor_type exists
//   - Assert idx_activity_log_squad_entity_type exists
```

This test will fail because the migration file does not yet exist.

Also write a compile-time check: in `internal/database/db/` the generated files will be absent, so any handler file that imports the forthcoming `db.InsertActivityEntryParams` will fail to compile. Use a `//go:build ignore` stub or keep this check implicit via `make sqlc` in the next step.

#### GREEN — Minimum implementation

1. Create migration file `internal/database/migrations/20260315000011_create_activity_log.sql` with the exact SQL from the design:
   - `CREATE TYPE activity_actor_type AS ENUM ('agent', 'user', 'system')`
   - `CREATE TABLE activity_log (id, squad_id, actor_type, actor_id, action, entity_type, entity_id, metadata JSONB DEFAULT '{}', created_at)`
   - `CREATE INDEX idx_activity_log_squad_created_at ON activity_log (squad_id, created_at DESC)`
   - `CREATE INDEX idx_activity_log_squad_actor_type ON activity_log (squad_id, actor_type)`
   - `CREATE INDEX idx_activity_log_squad_entity_type ON activity_log (squad_id, entity_type)`
   - `-- +goose Down` section: `DROP TABLE IF EXISTS activity_log; DROP TYPE IF EXISTS activity_actor_type;`

2. Create sqlc query file `internal/database/queries/activity_log.sql` with three queries:
   - `-- name: InsertActivityEntry :one` — INSERT with RETURNING *
   - `-- name: ListActivityBySquad :many` — SELECT with nullable filters for actorType, entityType, action; ORDER BY created_at DESC; LIMIT/OFFSET
   - `-- name: CountActivityBySquad :one` — COUNT with the same nullable filters

3. Run `make sqlc` to regenerate `internal/database/db/activity_log.sql.go` and update `internal/database/db/querier.go`.

4. Run `make test` — migration smoke test passes.

#### REFACTOR

- Confirm the goose Down section drops `activity_log` before dropping `activity_actor_type` (type dependency order).
- Confirm `metadata JSONB NOT NULL DEFAULT '{}'` — not nullable, no sentinel needed.
- Confirm query parameter names use `@named` syntax matching `sqlc.yaml` conventions already used in the project.

#### Acceptance Criteria

- [ ] Migration file exists at `internal/database/migrations/20260315000011_create_activity_log.sql`
- [ ] `activity_log` table created with all required columns and types
- [ ] `metadata` column is `NOT NULL DEFAULT '{}'`
- [ ] Three indexes are created: `squad_created_at`, `squad_actor_type`, `squad_entity_type`
- [ ] `internal/database/queries/activity_log.sql` contains `InsertActivityEntry`, `ListActivityBySquad`, `CountActivityBySquad`
- [ ] `make sqlc` runs without errors and regenerates `db/activity_log.sql.go`
- [ ] `make test` passes (migration smoke test green)

#### Files to Create / Modify

- **Create:** `internal/database/migrations/20260315000011_create_activity_log.sql`
- **Create:** `internal/database/queries/activity_log.sql`
- **Generated (do not edit):** `internal/database/db/activity_log.sql.go`, `internal/database/db/querier.go`, `internal/database/db/models.go`

---

### TASK-02: Domain Types, `logActivity` Helper, and `changedFieldNames` Utility

**Requirements:** REQ-026, REQ-029, REQ-031, REQ-037, REQ-042, REQ-043, REQ-044
**Estimated time:** 45–60 min

#### RED — Write failing test(s) first

Create `internal/server/handlers/activity_test.go` (package `handlers` internal test, not `handlers_test`). Write unit tests before any implementation exists — these will fail to compile initially:

```go
// TestLogActivity_NilMetadataProducesEmptyObject
//   - Call logActivity with Metadata: nil
//   - Assert InsertActivityEntry receives Metadata.RawMessage == []byte(`{}`)

// TestLogActivity_MetadataIsMarshalled
//   - Call logActivity with Metadata: map[string]any{"from": "todo", "to": "done"}
//   - Assert RawMessage contains the marshalled JSON

// TestLogActivity_MarshalError_ReturnsError
//   - Call logActivity with Metadata: a value that json.Marshal cannot encode (e.g. a channel)
//   - Assert logActivity returns a non-nil error without calling InsertActivityEntry

// TestChangedFieldNames_ReturnsAlphabeticallySortedKeys
//   - rawBody: {"title": ..., "status": ..., "description": ...}
//   - Expected: []string{"description", "status", "title"}

// TestChangedFieldNames_ExcludesSpecifiedKeys
//   - rawBody: {"status": ..., "sentinelField": ...}, exclude: "sentinelField"
//   - Expected: []string{"status"}

// TestActivityActorType_Valid
//   - "user", "agent", "system" → true
//   - "robot", "" → false
```

Use a minimal stub `*db.Queries` (nil pointer is fine for compile check; use interface substitution or a real test DB if needed for `logActivity` call-through tests — prefer a real embedded DB fixture from the shared `testDB`).

#### GREEN — Minimum implementation

1. Create `internal/domain/activity.go` with:
   - `ActivityActorType` string type with constants `ActivityActorUser`, `ActivityActorAgent`, `ActivityActorSystem`
   - `func (a ActivityActorType) Valid() bool`
   - `var ValidActivityEntityTypes map[string]bool` (7 values: squad, agent, issue, comment, project, goal, member)
   - `ActivityEntry` struct with all JSON-tagged fields matching the API contract

2. Create `internal/server/handlers/activity.go` with:
   - `ActivityParams` struct
   - `logActivity(ctx, qtx *db.Queries, p ActivityParams) error` — marshals metadata (nil → `{}`), calls `qtx.InsertActivityEntry`, returns error
   - `changedFieldNames(rawBody map[string]json.RawMessage, exclude ...string) []string` — collects keys, sorts alphabetically, skips excluded keys

#### REFACTOR

- Move `changedFieldNames` to a separate unexported helper file if it grows; for now keeping it in `activity.go` is acceptable.
- Ensure `logActivity` does not log or swallow errors — it returns them raw so the caller decides the HTTP response (REQ-044).
- Confirm `ActivityParams.Metadata any` accepts `nil`, `map[string]any`, or any struct.

#### Acceptance Criteria

- [ ] `internal/domain/activity.go` exists with `ActivityActorType`, `Valid()`, `ValidActivityEntityTypes`, `ActivityEntry`
- [ ] `internal/server/handlers/activity.go` exists with `ActivityParams`, `logActivity`, `changedFieldNames`
- [ ] `logActivity` with nil metadata produces `Metadata.RawMessage = []byte("{}")`
- [ ] `logActivity` with non-nil metadata marshals it to JSON correctly
- [ ] `logActivity` propagates marshal errors without calling the DB
- [ ] `changedFieldNames` returns sorted keys and respects exclusions
- [ ] `ActivityActorType.Valid()` returns true only for the three valid values
- [ ] All unit tests in `activity_test.go` pass
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/domain/activity.go`
- **Create:** `internal/server/handlers/activity.go`
- **Create:** `internal/server/handlers/activity_test.go`

---

### TASK-03: Squad Handler Activity Integration

**Requirements:** REQ-001, REQ-002, REQ-003, REQ-004, REQ-026, REQ-037, REQ-043, REQ-044
**Estimated time:** 45–60 min

#### RED — Write failing test(s) first

Add to a new file `internal/server/handlers/activity_integration_test.go` (package `handlers_test`). Write integration tests against the shared embedded Postgres `testDB`. Tests will fail because `logActivity` is not yet wired into the squad handlers:

```go
// TestActivity_SquadCreated (REQ-001)
//   - Register + login user
//   - POST /api/squads → creates squad (handler already transactional)
//   - GET /api/squads/{id}/activity  (endpoint does not exist yet; skip this assertion
//     or stub it — the assertion here is on the DB row directly via testDB query)
//   - SELECT * FROM activity_log WHERE squad_id = $1
//   - Assert exactly one entry: action="squad.created", entityType="squad", actorType="user"

// TestActivity_SquadUpdated (REQ-002)
//   - Create squad, then PATCH /api/squads/{id} with {"name": "New Name"}
//   - Assert activity entry: action="squad.updated", metadata.changedFields=["name"]

// TestActivity_SquadDeleted (REQ-003)
//   - Create squad, then DELETE /api/squads/{id}
//   - Assert activity entry: action="squad.deleted"

// TestActivity_SquadBudgetUpdated (REQ-004)
//   - Create squad, then PATCH /api/squads/{id}/budgets with budgetMonthlyCents
//   - Assert activity entry: action="squad.budget_updated", metadata.budgetMonthlyCents present

// TestActivity_RollbackOnActivityFailure_SquadCreate
//   - This test is deferred to TASK-06 where the endpoint allows observing rollback indirectly;
//     mark as TODO placeholder for now
```

Tests asserting DB rows directly: use `testDB.QueryRow` to count/check `activity_log` rows.

#### GREEN — Minimum implementation

Modify `internal/server/handlers/squad_handler.go`:

1. **`SquadHandler.Create`** — already uses `BeginTx`. After `qtx.CreateSquadMembership`, before `tx.Commit()`, call `logActivity` with `action="squad.created"`, no metadata. Check error → 500 on failure.

2. **`SquadHandler.Update`** — currently calls `h.queries.UpdateSquad` directly. Wrap in `h.dbConn.BeginTx` / `defer tx.Rollback()` / `qtx := h.queries.WithTx(tx)` pattern. Parse raw body bytes before decoding struct (to capture `changedFieldNames`). After `qtx.UpdateSquad`, call `logActivity` with `action="squad.updated"`, `metadata={"changedFields": changedFieldNames(rawBody)}`. Commit.

3. **`SquadHandler.Delete`** — wrap `h.queries.SoftDeleteSquad` in a transaction. Call `logActivity` with `action="squad.deleted"`, no metadata. Commit.

4. **`SquadHandler.UpdateBudget`** — wrap `h.queries.UpdateSquad` (or whatever query updates budget) in a transaction. Call `logActivity` with `action="squad.budget_updated"`, `metadata={"budgetMonthlyCents": req.BudgetMonthlyCents}`. Commit.

`SquadHandler` already holds `dbConn *sql.DB` — no constructor change needed.

#### REFACTOR

- Extract a `readRawBody` helper (if not already present) that reads and re-buffers `r.Body` so both the JSON decoder and `changedFieldNames` can see the raw keys.
- Confirm `Update` handler reads the raw body once, then decodes it, rather than reading it twice.
- Ensure every `logActivity` call site follows the exact pattern: check error → log with `slog.Error` → write 500 → `return` (Commit never called → deferred Rollback fires).

#### Acceptance Criteria

- [ ] `POST /api/squads` creates an `activity_log` entry with `action="squad.created"`
- [ ] `PATCH /api/squads/{id}` creates an entry with `action="squad.updated"` and `changedFields` in metadata
- [ ] `DELETE /api/squads/{id}` creates an entry with `action="squad.deleted"`
- [ ] `PATCH /api/squads/{id}/budgets` creates an entry with `action="squad.budget_updated"` and `budgetMonthlyCents` in metadata
- [ ] Activity entry `actorType="user"` and `actorId` matches authenticated user's ID on all four
- [ ] Activity entry `squadId` is derived from the entity, not the request body (REQ-030)
- [ ] All integration tests in `activity_integration_test.go` for squad actions pass
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/squad_handler.go`
- **Create:** `internal/server/handlers/activity_integration_test.go`

---

### TASK-04: Agent, Issue, and Comment Handler Activity Integration

**Requirements:** REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-026, REQ-037, REQ-043, REQ-044
**Estimated time:** 60 min

#### RED — Write failing test(s) first

Extend `activity_integration_test.go` with agent, issue, and comment tests. All will fail initially:

```go
// TestActivity_AgentCreated (REQ-005)
//   - Create agent via POST /api/agents
//   - Assert entry: action="agent.created", entityType="agent",
//     metadata.role and metadata.name present

// TestActivity_AgentUpdated (REQ-006)
//   - Create agent, PATCH /api/agents/{id} with {"model": "gpt-4o"}
//   - Assert entry: action="agent.updated", metadata.changedFields=["model"]

// TestActivity_AgentStatusChanged (REQ-007)
//   - Create agent, POST /api/agents/{id}/transition
//   - Assert entry: action="agent.status_changed", metadata.from and metadata.to present

// TestActivity_IssueCreated (REQ-008)
//   - Create issue via POST /api/squads/{id}/issues
//   - Assert entry: action="issue.created", metadata.identifier, metadata.title, metadata.status present

// TestActivity_IssueUpdated_NoStatusChange (REQ-009)
//   - Create issue, PATCH /api/issues/{id} with {"title": "New Title"}
//   - Assert entry: action="issue.updated", metadata.changedFields=["title"]

// TestActivity_IssueStatusChanged (REQ-010)
//   - Create issue, PATCH /api/issues/{id} with {"status": "in_progress"}
//   - Assert entry: action="issue.status_changed", metadata.from, metadata.to, metadata.identifier

// TestActivity_IssueDeleted (REQ-011)
//   - Create issue, DELETE /api/issues/{id}
//   - Assert entry: action="issue.deleted", metadata.identifier present

// TestActivity_CommentCreated (REQ-012)
//   - Create issue, POST /api/issues/{id}/comments
//   - Assert entry: action="comment.created", entityType="comment",
//     metadata.issueId and metadata.authorType present
```

#### GREEN — Minimum implementation

**`internal/server/handlers/agent_handler.go`:**

1. **`AgentHandler.CreateAgent`** — inspect existing code; if not already transactional, wrap in `BeginTx`. After the `CreateAgent` DB call, call `logActivity` with `action="agent.created"`, `metadata={"role": agent.Role, "name": agent.Name}`.

2. **`AgentHandler.UpdateAgent`** — wrap existing `UpdateAgent` call in a transaction (read raw body first for `changedFieldNames`). Call `logActivity` with `action="agent.updated"`, `metadata={"changedFields": changedFieldNames(rawBody)}`.

3. **`AgentHandler.TransitionAgentStatus`** — capture `oldStatus` before update. Wrap update in a transaction. Call `logActivity` with `action="agent.status_changed"`, `metadata={"from": oldStatus, "to": newStatus}`.

**`internal/server/handlers/issue_handler.go`:**

4. **`IssueHandler.CreateIssue`** — already transactional. After `qtx.CreateIssue`, call `logActivity` with `action="issue.created"`, `metadata={"identifier": issue.Identifier, "title": issue.Title, "status": issue.Status}`.

5. **`IssueHandler.UpdateIssue`** — two paths:
   - Status changed (including reopen path): call `logActivity` with `action="issue.status_changed"`, `metadata={"from": existingStatus, "to": newStatus, "identifier": existing.Identifier}`.
   - No status change: call `logActivity` with `action="issue.updated"`, `metadata={"changedFields": changedFieldNames(rawBody, "status")}`. For the non-reopen path that currently does not use a transaction, wrap in `BeginTx` before the `UpdateIssue` call.

6. **`IssueHandler.DeleteIssue`** — currently non-transactional. Upgrade to `BeginTx` pattern. Call `logActivity` with `action="issue.deleted"`, `metadata={"identifier": existing.Identifier}` (fetch the issue first to get the identifier). Commit.

7. **`IssueHandler.CreateComment`** — currently non-transactional. Upgrade to `BeginTx`. After `qtx.CreateIssueComment`, call `logActivity` with `action="comment.created"`, `metadata={"issueId": issue.ID.String(), "authorType": req.AuthorType}`.

#### REFACTOR

- Ensure `AgentHandler` holds `dbConn *sql.DB`; add it if missing (check constructor).
- For `UpdateIssue`, confirm the raw-body reading approach is consistent with how `UpdateIssue` already parses its partial-update fields — do not break the existing sentinel/partial-update logic.
- Confirm `issue.deleted` reads the issue record before deleting it (to capture `identifier` for metadata) and that the entire sequence (fetch → delete → logActivity) is inside a single transaction.
- REQ-042: `agent.updated` metadata records field names only — never the value of `adapterConfig`, `systemPrompt`, or `model`.

#### Acceptance Criteria

- [ ] `POST /api/agents` creates `action="agent.created"` entry with `role` and `name` in metadata
- [ ] `PATCH /api/agents/{id}` creates `action="agent.updated"` entry with `changedFields` list
- [ ] `POST /api/agents/{id}/transition` creates `action="agent.status_changed"` with `from`/`to`
- [ ] `POST /api/squads/{id}/issues` creates `action="issue.created"` with `identifier`, `title`, `status`
- [ ] `PATCH /api/issues/{id}` (no status change) creates `action="issue.updated"` with `changedFields`
- [ ] `PATCH /api/issues/{id}` (status changes) creates `action="issue.status_changed"` with `from`, `to`, `identifier`
- [ ] `DELETE /api/issues/{id}` creates `action="issue.deleted"` with `identifier`
- [ ] `POST /api/issues/{id}/comments` creates `action="comment.created"` with `issueId` and `authorType`
- [ ] No sensitive values (systemPrompt, adapterConfig content) appear in any metadata
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/agent_handler.go`
- **Modify:** `internal/server/handlers/issue_handler.go`
- **Modify:** `internal/server/handlers/activity_integration_test.go` (extend)

---

### TASK-05: Project, Goal, and Membership Handler Activity Integration (Transaction Upgrade)

**Requirements:** REQ-013, REQ-014, REQ-015, REQ-016, REQ-017, REQ-018, REQ-019, REQ-020, REQ-026, REQ-037, REQ-043, REQ-044
**Estimated time:** 60 min

#### RED — Write failing test(s) first

Extend `activity_integration_test.go` with project, goal, and membership tests. All will fail initially:

```go
// TestActivity_ProjectCreated (REQ-013)
//   - Create project via POST /api/squads/{id}/projects
//   - Assert entry: action="project.created", entityType="project", metadata.name present

// TestActivity_ProjectUpdated_NoStatusChange (REQ-014)
//   - Create project, PATCH /api/projects/{id} with {"description": "Updated"}
//   - Assert entry: action="project.updated", metadata.changedFields=["description"]

// TestActivity_ProjectStatusChanged (REQ-014)
//   - Create project, PATCH /api/projects/{id} with {"status": "completed"}
//   - Assert entry: action="project.status_changed", metadata.from and metadata.to present

// TestActivity_GoalCreated (REQ-015)
//   - Create goal via POST /api/squads/{id}/goals
//   - Assert entry: action="goal.created", entityType="goal", metadata.title present

// TestActivity_GoalUpdated_NoStatusChange (REQ-016)
//   - Create goal, PATCH /api/goals/{id} with {"description": "Updated"}
//   - Assert entry: action="goal.updated", metadata.changedFields=["description"]

// TestActivity_GoalStatusChanged (REQ-016)
//   - Create goal, PATCH /api/goals/{id} with {"status": "completed"}
//   - Assert entry: action="goal.status_changed", metadata.from and metadata.to present

// TestActivity_MemberAdded (REQ-017)
//   - Register second user, POST /api/squads/{id}/members
//   - Assert entry: action="member.added", entityType="member", metadata.userId and metadata.role present

// TestActivity_MemberRoleChanged (REQ-018)
//   - Add member, PATCH /api/squads/{id}/members/{memberId}
//   - Assert entry: action="member.role_changed", metadata.userId, metadata.from, metadata.to

// TestActivity_MemberRemoved (REQ-019)
//   - Add member, DELETE /api/squads/{id}/members/{memberId}
//   - Assert entry: action="member.removed", metadata.userId present

// TestActivity_MemberLeft (REQ-020)
//   - Add second user as member, login as second user, DELETE /api/squads/{id}/members/me
//   - Assert entry: action="member.left", metadata.userId = second user's ID
```

#### GREEN — Minimum implementation

All three handlers currently lack `dbConn *sql.DB`. Add it to each:

**`internal/server/handlers/project_handler.go`:**

1. Add `dbConn *sql.DB` field to `ProjectHandler` struct.
2. Update `NewProjectHandler(q *db.Queries, dbConn *sql.DB) *ProjectHandler`.
3. **`CreateProject`** — wrap in `BeginTx`. After `qtx.CreateProject`, call `logActivity` with `action="project.created"`, `metadata={"name": project.Name}`.
4. **`UpdateProject`** — wrap in `BeginTx`. Fetch existing project before update (to detect status change). Read raw body bytes. After `qtx.UpdateProject`:
   - If `status` changed: `action="project.status_changed"`, `metadata={"from": oldStatus, "to": newStatus}`
   - Otherwise: `action="project.updated"`, `metadata={"changedFields": changedFieldNames(rawBody, "status")}`

**`internal/server/handlers/goal_handler.go`:**

5. Add `dbConn *sql.DB` field to `GoalHandler` struct.
6. Update `NewGoalHandler(q *db.Queries, dbConn *sql.DB) *GoalHandler`.
7. **`CreateGoal`** — wrap in `BeginTx`. After `qtx.CreateGoal`, call `logActivity` with `action="goal.created"`, `metadata={"title": goal.Title}`.
8. **`UpdateGoal`** — wrap in `BeginTx`. Fetch existing goal. Read raw body bytes. After `qtx.UpdateGoal`:
   - If `status` changed: `action="goal.status_changed"`, `metadata={"from": oldStatus, "to": newStatus}`
   - Otherwise: `action="goal.updated"`, `metadata={"changedFields": changedFieldNames(rawBody, "status")}`

**`internal/server/handlers/membership_handler.go`:**

9. Add `dbConn *sql.DB` field to `MembershipHandler` struct.
10. Update `NewMembershipHandler(q *db.Queries, dbConn *sql.DB) *MembershipHandler`.
11. **`Add`** — wrap `CreateSquadMembership` in `BeginTx`. Call `logActivity` with `action="member.added"`, `entityType="member"`, `entityId=membership.ID`, `metadata={"userId": req.UserID, "role": req.Role}`.
12. **`UpdateRole`** — wrap `UpdateSquadMembershipRole` in `BeginTx`. Fetch existing membership to get old role. Call `logActivity` with `action="member.role_changed"`, `metadata={"userId": membership.UserID, "from": oldRole, "to": newRole}`.
13. **`Remove`** — wrap `DeleteSquadMembership` in `BeginTx`. Fetch existing membership first (to get `userId`). Call `logActivity` with `action="member.removed"`, `metadata={"userId": membership.UserID}`.
14. **`Leave`** — wrap `DeleteSquadMembershipByUserIfNotLastOwner` in `BeginTx`. Call `logActivity` with `action="member.left"`, `actorId=identity.UserID`, `metadata={"userId": identity.UserID.String()}`.

**Router / server wiring:**

15. Update `internal/server/router.go` (or wherever `NewProjectHandler`, `NewGoalHandler`, `NewMembershipHandler` are called) to pass `dbConn` as the second argument.

#### REFACTOR

- Confirm all three handler constructors are updated in the router wiring — compile error will catch this if missed.
- For `UpdateProject` and `UpdateGoal`, the existing raw-body pattern may differ from the issue handler; align the approach with `changedFieldNames` consistently.
- Confirm `actorId` for `member.left` is `identity.UserID` (the user leaving), not `uuid.Nil` (that is reserved for system actors per REQ-031).

#### Acceptance Criteria

- [ ] `ProjectHandler`, `GoalHandler`, `MembershipHandler` each have `dbConn *sql.DB` and updated constructors
- [ ] Router passes `dbConn` to all three constructors
- [ ] `POST /api/squads/{id}/projects` creates `action="project.created"` with `name`
- [ ] `PATCH /api/projects/{id}` (no status change) creates `action="project.updated"` with `changedFields`
- [ ] `PATCH /api/projects/{id}` (status change) creates `action="project.status_changed"` with `from`/`to`
- [ ] `POST /api/squads/{id}/goals` creates `action="goal.created"` with `title`
- [ ] `PATCH /api/goals/{id}` (no status change) creates `action="goal.updated"` with `changedFields`
- [ ] `PATCH /api/goals/{id}` (status change) creates `action="goal.status_changed"` with `from`/`to`
- [ ] `POST /api/squads/{id}/members` creates `action="member.added"` with `userId` and `role`
- [ ] `PATCH /api/squads/{id}/members/{memberId}` creates `action="member.role_changed"` with `from`/`to`
- [ ] `DELETE /api/squads/{id}/members/{memberId}` creates `action="member.removed"` with `userId`
- [ ] `DELETE /api/squads/{id}/members/me` creates `action="member.left"` with `userId`
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/project_handler.go`
- **Modify:** `internal/server/handlers/goal_handler.go`
- **Modify:** `internal/server/handlers/membership_handler.go`
- **Modify:** `internal/server/router.go` (constructor call sites)
- **Modify:** `internal/server/handlers/activity_integration_test.go` (extend)

---

### TASK-06: Activity Feed Handler (`GET /api/squads/{id}/activity`)

**Requirements:** REQ-021, REQ-022, REQ-023, REQ-024, REQ-025, REQ-032, REQ-033, REQ-034, REQ-035, REQ-036, REQ-037, REQ-038, REQ-041
**Estimated time:** 60 min

#### RED — Write failing test(s) first

Extend `activity_integration_test.go` with feed endpoint tests. All will return 404 until the handler is registered:

```go
// TestActivityFeed_ReturnsPaginatedEntries (REQ-021, REQ-022, REQ-034)
//   - Create squad (triggers squad.created entry)
//   - Create two issues (triggers two issue.created entries)
//   - GET /api/squads/{id}/activity
//   - Assert HTTP 200
//   - Assert response has "data" array and "pagination" object with limit, offset, total
//   - Assert entries are ordered by createdAt descending
//   - Assert total >= 3

// TestActivityFeed_DefaultPagination (REQ-022)
//   - GET /api/squads/{id}/activity (no query params)
//   - Assert pagination.limit = 50, pagination.offset = 0

// TestActivityFeed_CustomPagination (REQ-022)
//   - Create 5 issues → 6 entries (squad.created + 5 issue.created)
//   - GET /api/squads/{id}/activity?limit=2&offset=0
//   - Assert len(data) = 2, pagination.total >= 6

// TestActivityFeed_FilterByActorType (REQ-023, REQ-035)
//   - Create squad and issue (both user actor)
//   - GET ?actorType=user → assert results returned
//   - GET ?actorType=agent → assert data is empty (or only agent-actor entries if any)
//   - GET ?actorType=robot → assert 400 VALIDATION_ERROR

// TestActivityFeed_FilterByEntityType (REQ-024, REQ-036)
//   - Create squad, one issue, one project
//   - GET ?entityType=issue → only issue entries
//   - GET ?entityType=widget → assert 400 VALIDATION_ERROR

// TestActivityFeed_FilterByAction (REQ-038)
//   - Create issue, update status
//   - GET ?action=issue.created → only the created entry
//   - GET ?action=issue.status_changed → only the status-changed entry

// TestActivityFeed_AllRequiredFieldsPresent (REQ-029)
//   - GET /api/squads/{id}/activity
//   - Assert each entry has: id, squadId, actorType, actorId, action, entityType, entityId, createdAt
//   - Assert metadata is a JSON object (never null or absent)

// TestActivityFeed_SquadScopeEnforced (REQ-041)
//   - Create two squads belonging to different users
//   - GET /api/squads/{squadA.id}/activity as user A
//   - Assert zero entries from squad B appear

// TestActivityFeed_Unauthorized (REQ-032)
//   - GET /api/squads/{id}/activity without session cookie
//   - Assert 401 UNAUTHENTICATED

// TestActivityFeed_NonMember (REQ-033)
//   - Register user B who is not in the squad
//   - GET /api/squads/{id}/activity as user B
//   - Assert 404 SQUAD_NOT_FOUND

// TestActivityFeed_RollbackOnActivityWriteFailure (REQ-037, REQ-043, REQ-044)
//   - This test verifies that when a logActivity call fails the mutation is also absent.
//   - Approach: use a DB-level approach — issue a PATCH that triggers an activity write,
//     but simulate failure by temporarily revoking INSERT on activity_log via a savepoint
//     trick, OR use a test double that injects an error into the qtx.
//   - Simpler approach: confirm that after a 500 response from a mutation the entity
//     count in the DB is the same as before. Inject a poisoned request designed to
//     fail the activity INSERT (e.g., force a constraint violation) and assert the
//     primary entity was not persisted.
//   - Assert HTTP 500 INTERNAL_ERROR is returned and primary entity absent from DB.
```

#### GREEN — Minimum implementation

1. Create `internal/server/handlers/activity_handler.go`:
   - `ActivityHandler` struct with `queries *db.Queries`
   - `NewActivityHandler(q *db.Queries) *ActivityHandler`
   - `RegisterRoutes` — registers `GET /api/squads/{id}/activity`
   - `ListActivity` method:
     - Parse `{id}` path value as UUID; 404 on parse failure
     - Extract identity from context; 401 if absent
     - Check `GetSquadMembership`; 404 `SQUAD_NOT_FOUND` if `sql.ErrNoRows`
     - Parse `limit` (default 50, max 200) and `offset` (default 0)
     - Validate optional `actorType` query param via `domain.ActivityActorType.Valid()`; 400 on invalid
     - Validate optional `entityType` query param via `domain.ValidActivityEntityTypes`; 400 on invalid
     - Accept optional `action` query param without validation (exact match passthrough)
     - Call `h.queries.ListActivityBySquad` and `h.queries.CountActivityBySquad`
     - Map rows to `activityEntryResponse` via `dbActivityToResponse` (parses `Metadata.RawMessage` to `any`; falls back to empty map on invalid JSON)
     - Return `{"data": [...], "pagination": {"limit": N, "offset": M, "total": T}}`
   - Define local response types: `activityEntryResponse`, `activityListResponse`
   - Define `dbActivityToResponse(db.ActivityLog) activityEntryResponse`

2. Register `ActivityHandler` in the router:
   - In `internal/server/router.go`, instantiate `handlers.NewActivityHandler(queries)` and call `RegisterRoutes(mux)`.

#### REFACTOR

- `dbActivityToResponse` should handle `Metadata.Valid = false` gracefully (return `map[string]any{}`) so the contract `metadata is never null` (REQ-029) is upheld at the response layer too.
- The `paginationMeta` type is already defined in the project (used by the issues list response) — reuse it rather than defining a new type.
- Cap `limit` at 200 to prevent oversized queries (noted in API contract).
- Confirm `WHERE squad_id = $1` is present in both `ListActivityBySquad` and `CountActivityBySquad` queries (REQ-041).

#### Acceptance Criteria

- [ ] `GET /api/squads/{id}/activity` returns 200 with `data` array and `pagination` object
- [ ] Entries are ordered by `createdAt` descending
- [ ] Default pagination: `limit=50`, `offset=0`
- [ ] `?limit=N&offset=M` applied correctly; `limit` capped at 200
- [ ] `?actorType=user/agent/system` filters results; invalid value returns 400 VALIDATION_ERROR
- [ ] `?entityType=<valid>` filters results; invalid value returns 400 VALIDATION_ERROR
- [ ] `?action=<value>` filters by exact action string
- [ ] Every entry in `data` has all required fields; `metadata` is always a JSON object
- [ ] 401 returned when no session cookie present
- [ ] 404 `SQUAD_NOT_FOUND` returned for non-members
- [ ] Entries from other squads never appear in response
- [ ] `ActivityHandler` registered in router
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/server/handlers/activity_handler.go`
- **Modify:** `internal/server/router.go`
- **Modify:** `internal/server/handlers/activity_integration_test.go` (extend)

---

### TASK-07: React Activity Feed Widget on Dashboard

**Requirements:** REQ-025
**Estimated time:** 45–60 min

#### RED — Write failing test(s) first

This task uses visual/manual verification rather than automated tests (no existing frontend test suite). Write the type definitions and query key first; the build will fail until the component files exist:

1. Attempt `make ui-build` — it will fail if imports are broken.
2. As a lightweight "compile-time" test, add the `ActivityEntry` type and `queryKeys.activity` key — the TypeScript compiler (`tsc --noEmit`) will flag any type mismatches when used in the component.

Functional correctness is verified manually:
- Dashboard shows the activity feed card
- Card displays up to 20 most recent entries in descending order
- Each row shows actor badge, action label, entity identifier, and relative timestamp
- Empty state message appears when no activity exists

#### GREEN — Minimum implementation

1. **Add `ActivityEntry` type** in `web/src/types/activity.ts`:
   ```ts
   export interface ActivityEntry {
     id: string;
     squadId: string;
     actorType: "user" | "agent" | "system";
     actorId: string;
     action: string;
     entityType: string;
     entityId: string;
     metadata: Record<string, unknown>;
     createdAt: string;
   }
   ```

2. **Add `activity` query key** in `web/src/lib/query.ts`:
   ```ts
   activity: {
     list: (squadId: string) => ["activity", { squadId }] as const,
   },
   ```

3. **Create `ActivityFeed` component** in `web/src/features/dashboard/activity-feed.tsx`:
   - Accept `squadId: string` prop
   - `useQuery` with `queryKeys.activity.list(squadId)`, fetching `api.get<PaginatedResponse<ActivityEntry>>(`/squads/${squadId}/activity?limit=20`)`
   - While loading: render 3 skeleton rows
   - On error: render an error message
   - On empty data: render "No recent activity"
   - For each entry render a row with:
     - Actor type badge (colored chip: user=blue, agent=purple, system=gray)
     - Human-readable action label (replace `.` with space, e.g. `issue.created` → `issue created`)
     - Entity identifier from `metadata` if present (`identifier` key), otherwise `entityType`
     - Relative timestamp (e.g. "2 min ago") using `Intl.RelativeTimeFormat` or a simple helper
   - Wrap in a `Card` with `CardHeader` (title "Recent Activity") and `CardContent`

4. **Mount `ActivityFeed` on the dashboard** in `web/src/features/dashboard/dashboard-page.tsx`:
   - Import and render `<ActivityFeed squadId={activeSquad.squadId} />` below the "Issues by Status" section

5. **Run `make ui-build`** — assert no TypeScript/build errors.

#### REFACTOR

- Extract a `relativeTime(dateStr: string): string` utility function to `web/src/lib/utils.ts` so it is reusable.
- Confirm `ActivityFeed` handles `squadId` being an empty string gracefully (disable the query when empty, matching the pattern used by other dashboard queries with `enabled: !!activeSquad`).
- Confirm metadata `Record<string, unknown>` type does not require casting to access `identifier` (use optional chaining: `(entry.metadata?.identifier as string) ?? entry.entityType`).

#### Acceptance Criteria

- [ ] `web/src/types/activity.ts` exists with `ActivityEntry` interface
- [ ] `queryKeys.activity.list` added to `web/src/lib/query.ts`
- [ ] `ActivityFeed` component renders with loading skeleton, error, empty, and data states
- [ ] Dashboard page renders `<ActivityFeed>` below the issues breakdown section
- [ ] `make ui-build` completes without TypeScript errors
- [ ] Manual check: dashboard shows up to 20 most recent activity entries for the active squad
- [ ] Manual check: entries show actor badge, action, entity reference, and timestamp
- [ ] Manual check: empty state shown when squad has no activity

#### Files to Create / Modify

- **Create:** `web/src/types/activity.ts`
- **Create:** `web/src/features/dashboard/activity-feed.tsx`
- **Modify:** `web/src/lib/query.ts`
- **Modify:** `web/src/features/dashboard/dashboard-page.tsx`

---

## Notes

### Blockers

- TASK-01 must complete before all other tasks (generated sqlc types needed)
- TASK-02 must complete before TASK-03/04/05 (logActivity helper needed)
- TASK-06 must complete before TASK-07 (endpoint needed for the React component)
- TASK-03/04/05 can run in parallel once TASK-02 is complete

### Dependency Order

```
TASK-01 (migration + sqlc)
    └── TASK-02 (domain + helper)
            ├── TASK-03 (squad handlers)
            ├── TASK-04 (agent/issue/comment handlers)
            └── TASK-05 (project/goal/member handlers)
                    └── TASK-06 (feed endpoint + router)
                                └── TASK-07 (React widget)
```

### Future Improvements

- Full-text search on activity entries (out of scope v0.1)
- Webhook notifications on activity events
- Activity export (CSV / JSON)
- Real-time SSE push of `activity.appended` events (tracked in SSE feature)
- Retention policies / automatic expiry of old entries
- Before/after field snapshots (diff-style change records)
