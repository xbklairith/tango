# Tasks: Issue Tracking

**Created:** 2026-03-14
**Status:** Not Started

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Technical design: [design.md](./design.md)
- Requirement coverage: REQ-ISS-001 through REQ-ISS-093, REQ-ISS-NF-001 through REQ-ISS-NF-004
- Missing coverage: None -- all Phase 1 requirements are mapped below

## Implementation Approach

Tasks are ordered by dependency: database schema first (migrations, sqlc queries), then domain types with pure-logic validation, followed by identifier generation, status machine, sub-task hierarchy, CRUD handlers, comment handlers, filtering/pagination, and finally integration tests. Each task follows TDD Red-Green-Refactor. The domain layer (enums, status machine) is tested with unit tests in `internal/domain/issue_test.go`; handlers and queries are tested with integration tests against embedded PostgreSQL in `internal/server/handlers/issue_handler_test.go`.

## Progress Summary

- Total Tasks: 24
- Completed: 0/24
- In Progress: None
- Test Coverage: 0%

## Tasks (TDD: Red-Green-Refactor)

---

### Component 1: Database Schema

#### Task 1.1: Create Issues Table Migration

**Linked Requirements:** REQ-ISS-001, REQ-ISS-004, REQ-ISS-005, REQ-ISS-006, REQ-ISS-007, REQ-ISS-075, REQ-ISS-076, REQ-ISS-NF-003

**RED Phase:**
- [ ] Write a migration test that runs goose up/down and verifies the `issues` table exists with all columns
  - Test case: After migration up, `issues` table has columns: `id`, `squad_id`, `identifier`, `type`, `title`, `description`, `status`, `priority`, `parent_id`, `project_id`, `goal_id`, `assignee_agent_id`, `assignee_user_id`, `billing_code`, `request_depth`, `created_at`, `updated_at`
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/XXXXXX_create_issues.sql`
  - Create `issue_type` enum (`task`, `conversation`)
  - Create `issue_status` enum (`backlog`, `todo`, `in_progress`, `done`, `blocked`, `cancelled`)
  - Create `issue_priority` enum (`critical`, `high`, `medium`, `low`)
  - Create `issues` table with all columns, defaults, CHECK constraints
  - Add `UNIQUE (squad_id, identifier)` constraint
  - Add `CHECK (parent_id IS NULL OR parent_id != id)` constraint
  - Add `updated_at` trigger via `update_updated_at_column()` function
  - Create all performance indexes per design Section 4.1

**REFACTOR Phase:**
- [ ] Verify index naming conventions match existing migrations
- [ ] Ensure down migration cleanly drops all objects in reverse order

**Acceptance Criteria:**
- [ ] Migration runs cleanly up and down
- [ ] All 3 enum types created with correct values
- [ ] Title CHECK constraint enforces 1-500 character length
- [ ] `uq_issues_squad_identifier` unique constraint exists
- [ ] `ck_issues_no_self_parent` CHECK constraint exists
- [ ] All 11 indexes created (squad_id, status, priority, assignee_agent_id, assignee_user_id, project_id, goal_id, parent_id, identifier, squad_created_at, squad_updated_at)
- [ ] `updated_at` trigger fires on UPDATE
- [ ] Partial indexes used for nullable FK columns

**Notes:**
- `project_id` and `goal_id` are plain UUID columns without FK constraints until feature 06 adds those tables
- Partial indexes (WHERE col IS NOT NULL) reduce index size for sparse nullable columns

---

#### Task 1.2: Create Issue Comments Table Migration

**Linked Requirements:** REQ-ISS-040, REQ-ISS-041, REQ-ISS-042, REQ-ISS-044, REQ-ISS-062

**RED Phase:**
- [ ] Write a migration test that verifies `issue_comments` table exists with correct columns after migration up
  - Test case: Table has `id`, `issue_id`, `author_type`, `author_id`, `body`, `created_at`, `updated_at`
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/XXXXXX_create_issue_comments.sql`
  - Create `comment_author_type` enum (`agent`, `user`, `system`)
  - Create `issue_comments` table with FK to `issues(id)` ON DELETE CASCADE
  - Add `CHECK (char_length(body) >= 1)` constraint
  - Create `idx_issue_comments_issue_id` index on `(issue_id, created_at ASC)`

**REFACTOR Phase:**
- [ ] Verify naming conventions are consistent with issues migration

**Acceptance Criteria:**
- [ ] Migration runs cleanly up and down
- [ ] `comment_author_type` enum has exactly 3 values: `agent`, `user`, `system`
- [ ] Cascading delete works: deleting an issue removes its comments
- [ ] Empty body is rejected by CHECK constraint
- [ ] Index exists for efficient comment listing by issue

---

#### Task 1.3: Write sqlc Queries for Issues

**Linked Requirements:** REQ-ISS-001, REQ-ISS-002, REQ-ISS-003, REQ-ISS-004, REQ-ISS-051, REQ-ISS-052, REQ-ISS-053, REQ-ISS-054, REQ-ISS-NF-004

**RED Phase:**
- [ ] Write `internal/database/queries/issues.sql` with all named queries
  - Run `make sqlc` and verify it generates Go code without errors

**GREEN Phase:**
- [ ] Implement all sqlc queries per design Section 4.3:
  - `CreateIssue` -- INSERT with 14 params, RETURNING *
  - `GetIssueByID` -- SELECT * WHERE id = $1
  - `GetIssueByIdentifier` -- SELECT * WHERE squad_id = $1 AND identifier = $2 (squad-scoped for data isolation)
  - `UpdateIssue` -- UPDATE with COALESCE/sqlc.narg for partial updates, RETURNING *
  - `DeleteIssue` -- DELETE WHERE id = $1
  - `CountSubTasks` -- SELECT count(*) WHERE parent_id = $1
  - `ListIssuesBySquad` -- SELECT with 8 optional filters via sqlc.narg, dynamic sort via CASE, LIMIT/OFFSET
  - `IncrementSquadIssueCounter` -- UPDATE squads SET issue_counter = issue_counter + 1 RETURNING issue_prefix, issue_counter
  - `GetIssueAncestors` -- Recursive CTE walking parent_id chain with depth limit 100
- [ ] Run `make sqlc` to generate Go code in `internal/database/db/`

**REFACTOR Phase:**
- [ ] Review generated Go code for correct types and nullability
- [ ] Verify sqlc.narg usage produces `sql.NullXxx` types for optional filters

**Acceptance Criteria:**
- [ ] `make sqlc` completes without errors
- [ ] All 9 queries generate correct Go functions in `internal/database/db/` (GetIssueBySquadAndIdentifier merged into GetIssueByIdentifier)
- [ ] `ListIssuesBySquad` supports 8 optional filters and 4 sort fields
- [ ] `IncrementSquadIssueCounter` returns both `issue_prefix` and `issue_counter`
- [ ] `GetIssueAncestors` CTE has depth safety limit of 100

---

#### Task 1.4: Write sqlc Queries for Issue Comments

**Linked Requirements:** REQ-ISS-040, REQ-ISS-060, REQ-ISS-061, REQ-ISS-082

**RED Phase:**
- [ ] Write `internal/database/queries/issue_comments.sql` with named queries
  - Run `make sqlc` to verify generation

**GREEN Phase:**
- [ ] Implement sqlc queries per design Section 4.3:
  - `CreateIssueComment` -- INSERT (issue_id, author_type, author_id, body) RETURNING *
  - `ListIssueComments` -- SELECT WHERE issue_id = $1 ORDER BY created_at ASC, LIMIT/OFFSET
  - `CountIssueComments` -- SELECT count(*) WHERE issue_id = $1
- [ ] Run `make sqlc` to regenerate

**REFACTOR Phase:**
- [ ] Verify pagination parameter naming consistency (`@page_limit`, `@page_offset`)

**Acceptance Criteria:**
- [ ] `make sqlc` completes without errors
- [ ] All 3 comment queries generate correct Go functions
- [ ] `ListIssueComments` orders by `created_at ASC` (chronological)
- [ ] Pagination parameters use `@page_limit` and `@page_offset`

---

### Component 2: Domain Types

#### Task 2.1: Define Issue Enums with Validation

**Linked Requirements:** REQ-ISS-005, REQ-ISS-006, REQ-ISS-007, REQ-ISS-042, REQ-ISS-091

**RED Phase:**
- [ ] Write tests in `internal/domain/issue_test.go`:
  - `TestIssueTypeValid` -- "task" and "conversation" are valid; "invalid" and "" are not
  - `TestIssueStatusValid` -- all 6 statuses are valid; "unknown" and "" are not
  - `TestIssuePriorityValid` -- all 4 priorities are valid; "urgent" and "" are not
  - `TestCommentAuthorTypeValid` -- "agent", "user", "system" are valid; "bot" and "" are not
  - Expected failure: Types and methods do not exist

**GREEN Phase:**
- [ ] Implement in `internal/domain/issue.go`:
  - `IssueType` string type with constants `IssueTypeTask`, `IssueTypeConversation` and `Valid()` method
  - `IssueStatus` string type with 6 constants and `Valid()` method
  - `IssuePriority` string type with 4 constants and `Valid()` method
  - `CommentAuthorType` string type with 3 constants and `Valid()` method

**REFACTOR Phase:**
- [ ] Ensure consistent naming pattern with existing domain types in the codebase
- [ ] Verify all enum constant string values match exact database enum values

**Acceptance Criteria:**
- [ ] All 4 enum types defined with correct constants
- [ ] `Valid()` returns true only for defined constants
- [ ] `Valid()` returns false for empty string and arbitrary values
- [ ] All tests pass with `make test`

---

#### Task 2.2: Define Issue and IssueComment Structs

**Linked Requirements:** REQ-ISS-004, REQ-ISS-041

**RED Phase:**
- [ ] Write a compilation test that instantiates `Issue` and `IssueComment` structs with all fields
  - Expected failure: Structs do not exist

**GREEN Phase:**
- [ ] Define `Issue` struct in `internal/domain/issue.go` per design Section 3.1:
  - 17 fields: `ID`, `SquadID`, `Identifier`, `Type`, `Title`, `Description`, `Status`, `Priority`, `ParentID`, `ProjectID`, `GoalID`, `AssigneeAgentID`, `AssigneeUserID`, `BillingCode`, `RequestDepth`, `CreatedAt`, `UpdatedAt`
  - JSON tags in camelCase with `omitempty` on nullable fields
- [ ] Define `IssueComment` struct per design Section 3.1:
  - 7 fields: `ID`, `IssueID`, `AuthorType`, `AuthorID`, `Body`, `CreatedAt`, `UpdatedAt`

**REFACTOR Phase:**
- [ ] Verify JSON tag naming matches API contract (camelCase per design Section 10)
- [ ] Ensure nullable fields use pointer types with `omitempty`

**Acceptance Criteria:**
- [ ] `Issue` struct has 17 fields matching REQ-ISS-004
- [ ] `IssueComment` struct has 7 fields matching REQ-ISS-041
- [ ] JSON tags produce camelCase output matching API contracts
- [ ] Nullable fields (`Description`, `ParentID`, `ProjectID`, `GoalID`, `AssigneeAgentID`, `AssigneeUserID`, `BillingCode`) use pointer types

---

#### Task 2.3: Define Request/Response DTOs

**Linked Requirements:** REQ-ISS-008, REQ-ISS-009, REQ-ISS-010, REQ-ISS-011, REQ-ISS-050, REQ-ISS-053, REQ-ISS-060, REQ-ISS-051

**RED Phase:**
- [ ] Write a compilation test that instantiates all DTO structs with sample data
  - Expected failure: DTOs do not exist

**GREEN Phase:**
- [ ] Define `CreateIssueRequest` struct with optional fields as pointer types for default handling
- [ ] Define `UpdateIssueRequest` struct with all-pointer fields for partial update
- [ ] Define `CreateCommentRequest` struct with required `AuthorType`, `AuthorID`, `Body`
- [ ] Define `IssueListParams` struct with all filter fields, `Sort`, `Limit`, `Offset`

**REFACTOR Phase:**
- [ ] Ensure DTO field types align with sqlc-generated parameter types for clean mapping
- [ ] Add Go doc comments documenting default values

**Acceptance Criteria:**
- [ ] `CreateIssueRequest.Type` is `*IssueType` (nil defaults to "task" per REQ-ISS-010)
- [ ] `CreateIssueRequest.Status` is `*IssueStatus` (nil defaults to "backlog" per REQ-ISS-008)
- [ ] `CreateIssueRequest.Priority` is `*IssuePriority` (nil defaults to "medium" per REQ-ISS-009)
- [ ] `CreateIssueRequest.RequestDepth` is `*int` (nil defaults to 0 per REQ-ISS-011)
- [ ] `UpdateIssueRequest` has all fields as pointers for partial update
- [ ] `IssueListParams` supports 8 filter fields + `Sort` + `Limit` + `Offset`

---

### Component 3: Identifier Generation

#### Task 3.1: Implement Atomic Squad Counter Increment

**Linked Requirements:** REQ-ISS-002, REQ-ISS-003, REQ-ISS-076, REQ-ISS-NF-002, REQ-ISS-NF-004

**RED Phase:**
- [ ] Write integration test `TestGenerateIdentifier_Sequential`:
  - Create a squad with prefix "TEST" and counter 0
  - Call identifier generation 3 times in sequence
  - Assert identifiers are "TEST-1", "TEST-2", "TEST-3"
  - Expected failure: Function does not exist
- [ ] Write integration test `TestGenerateIdentifier_Concurrent`:
  - Launch 10 goroutines each creating 1 issue in the same squad
  - Collect all identifiers
  - Assert all 10 are unique, no duplicates
  - Expected failure: Concurrency not handled

**GREEN Phase:**
- [ ] Implement `generateIdentifier(ctx, qtx, squadID)` per design Section 5:
  - Call `qtx.IncrementSquadIssueCounter(ctx, squadID)` (UPDATE ... RETURNING)
  - Format as `fmt.Sprintf("%s-%d", row.IssuePrefix, row.IssueCounter)`
  - Must run within a transaction (receives `qtx` -- queries-with-tx)
- [ ] Implement `createIssueInTx` wrapping identifier generation + INSERT in one transaction per design Section 5

**REFACTOR Phase:**
- [ ] Extract identifier formatting into a pure helper `FormatIdentifier(prefix string, counter int) string`
- [ ] Add unit test for `FormatIdentifier`

**Acceptance Criteria:**
- [ ] Identifiers follow `{PREFIX}-{COUNTER}` format exactly
- [ ] Counter increments atomically via `UPDATE ... RETURNING` (row-level lock)
- [ ] 10 concurrent goroutines produce 10 unique identifiers
- [ ] Identifier generation and issue INSERT are in the same transaction
- [ ] Failed INSERT rolls back the counter increment (no gap on failure)

---

### Component 4: Status Machine

#### Task 4.1: Implement ValidateTransition

**Linked Requirements:** REQ-ISS-020, REQ-ISS-021, REQ-ISS-092

**RED Phase:**
- [ ] Write table-driven test `TestValidateTransition` in `internal/domain/issue_test.go` per design Section 14.1:
  - All 14 valid transitions from REQ-ISS-020 return nil
  - All invalid transitions return error, including:
    - backlog->done, backlog->blocked
    - todo->done
    - in_progress->backlog, in_progress->todo
    - done->in_progress, done->blocked, done->cancelled, done->backlog, done->done
    - cancelled->in_progress, cancelled->blocked, cancelled->done, cancelled->backlog
  - Expected failure: `ValidateTransition` function does not exist

**GREEN Phase:**
- [ ] Implement `validTransitions` map in `internal/domain/issue.go` per design Section 3.1
- [ ] Implement `ValidateTransition(from, to IssueStatus) error`:
  - Lookup `from` in map; if not found, return error "unknown current status"
  - Check if `to` is in the allowed list; if not, return error `cannot transition from "X" to "Y"`

**REFACTOR Phase:**
- [ ] Ensure error message format matches REQ-ISS-092: includes current and attempted status

**Acceptance Criteria:**
- [ ] All valid transitions from the REQ-ISS-020 table pass (nil error)
- [ ] All invalid transitions return non-nil error
- [ ] Unknown source status returns error
- [ ] Error message includes both current (`from`) and target (`to`) status values
- [ ] All tests pass with `make test`

---

#### Task 4.2: Implement IsReopen Detection

**Linked Requirements:** REQ-ISS-022

**RED Phase:**
- [ ] Write test `TestIsReopen` in `internal/domain/issue_test.go` per design Section 14.1:
  - `done -> todo` returns true
  - `cancelled -> todo` returns true
  - `backlog -> todo` returns false
  - `blocked -> todo` returns false
  - `blocked -> in_progress` returns false
  - Expected failure: `IsReopen` function does not exist

**GREEN Phase:**
- [ ] Implement `IsReopen(from, to IssueStatus) bool` per design Section 3.1:
  - Return true when `to == IssueStatusTodo` AND (`from == IssueStatusDone` OR `from == IssueStatusCancelled`)

**REFACTOR Phase:**
- [ ] Ensure IsReopen is only called after ValidateTransition passes (caller responsibility)

**Acceptance Criteria:**
- [ ] `done -> todo` detected as reopen
- [ ] `cancelled -> todo` detected as reopen
- [ ] No false positives for any non-reopen transition
- [ ] All tests pass

---

#### Task 4.3: Implement System Comment on Reopen

**Linked Requirements:** REQ-ISS-022, REQ-ISS-044

**RED Phase:**
- [ ] Write integration test `TestUpdateIssue_ReopenCreatesSystemComment`:
  - Create issue, transition backlog -> todo -> in_progress -> done
  - Update status to `todo` (reopen)
  - List comments for the issue
  - Assert a system comment exists with `authorType=system`, `authorId=uuid.Nil`, body containing "reopened"
  - Expected failure: No system comment is created

**GREEN Phase:**
- [ ] In `UpdateIssue` handler, after detecting a reopen via `IsReopen()` per design Section 6.3:
  - Insert system comment using `qtx.CreateIssueComment` within the same transaction
  - `authorType = "system"`, `authorId = uuid.Nil`
  - Body: `"Issue reopened: status changed from {old} to {new}"`

**REFACTOR Phase:**
- [ ] Extract reopen comment creation into a helper `createReopenComment(ctx, qtx, issueID, from, to)`

**Acceptance Criteria:**
- [ ] Reopening from `done` creates exactly one system comment
- [ ] Reopening from `cancelled` creates exactly one system comment
- [ ] Comment `authorType` is `"system"`
- [ ] Comment `authorId` is `uuid.Nil` (00000000-0000-0000-0000-000000000000)
- [ ] Comment body contains old and new status values
- [ ] Non-reopen transitions do not create system comments

---

### Component 5: Sub-task Hierarchy

#### Task 5.1: Implement Parent Validation (Same-Squad Check)

**Linked Requirements:** REQ-ISS-030

**RED Phase:**
- [ ] Write integration test `TestCreateIssue_ParentSameSquad`:
  - Create parent issue in squad A
  - Create child issue in squad A with parentId -> assert 201
  - Expected failure: No parent validation
- [ ] Write integration test `TestCreateIssue_ParentDifferentSquad`:
  - Create parent issue in squad A
  - Attempt to create child issue in squad B with parentId from squad A
  - Assert HTTP 400 VALIDATION_ERROR
  - Expected failure: Cross-squad parent accepted

**GREEN Phase:**
- [ ] Implement `validateParent(ctx, queries, squadID, parentID)` per design Section 7.2:
  - Fetch parent issue by ID via `GetIssueByID`
  - Return 404 NOT_FOUND if parent does not exist
  - Return 400 VALIDATION_ERROR if parent's `squad_id` differs from the child's squad

**REFACTOR Phase:**
- [ ] Share validation logic between create and update handlers

**Acceptance Criteria:**
- [ ] Parent in same squad is accepted
- [ ] Parent in different squad returns 400 VALIDATION_ERROR
- [ ] Non-existent parent returns 404 NOT_FOUND
- [ ] Validation runs on both create and update paths

---

#### Task 5.2: Implement Cycle Detection via Recursive CTE

**Linked Requirements:** REQ-ISS-031, REQ-ISS-033

**RED Phase:**
- [ ] Write integration test `TestUpdateIssue_DirectSelfParent`:
  - Create issue A
  - Attempt to set A's parentId to A
  - Assert HTTP 400 (enforced by both DB CHECK constraint and application logic)
  - Expected failure: Self-parent accepted at application level
- [ ] Write integration test `TestUpdateIssue_IndirectCycle`:
  - Create A (no parent), B (parent=A), C (parent=B)
  - Attempt to set A's parentId to C (would create A->C->B->A cycle)
  - Assert HTTP 400 VALIDATION_ERROR "circular parent reference detected"
  - Expected failure: Transitive cycle not detected
- [ ] Write integration test `TestCreateIssue_DeepNesting`:
  - Create a chain of 5+ nested sub-tasks
  - Assert all are created successfully (no enforced depth limit per REQ-ISS-033)

**GREEN Phase:**
- [ ] Implement `detectCycle(ctx, queries, issueID, newParentID)` per design Section 7.3:
  - Call `GetIssueAncestors(ctx, newParentID)` to walk the ancestor chain via recursive CTE
  - If `issueID` appears in the ancestor set, return 400 "circular parent reference detected"
- [ ] Call `detectCycle` in update handler when parentId changes
- [ ] For create handler: since issue doesn't exist yet, validate parentID chain does not already contain a cycle

**REFACTOR Phase:**
- [ ] Ensure CTE depth limit of 100 prevents runaway queries on degenerate data
- [ ] Verify error response uses code `VALIDATION_ERROR`

**Acceptance Criteria:**
- [ ] Direct self-reference rejected (A->A)
- [ ] Indirect cycle rejected (A->B->C->A)
- [ ] Deep nesting without cycles succeeds (5+ levels)
- [ ] CTE has depth safety limit of 100
- [ ] Cycle detection works on both create and update

---

#### Task 5.3: Implement Deletion Guard for Parent Issues

**Linked Requirements:** REQ-ISS-032

**RED Phase:**
- [ ] Write integration test `TestDeleteIssue_HasSubTasks`:
  - Create parent issue, create child issue with parentId
  - Attempt to delete parent
  - Assert HTTP 409 CONFLICT "cannot delete issue with active sub-tasks"
  - Expected failure: Parent deleted despite having children
- [ ] Write integration test `TestDeleteIssue_LeafIssue`:
  - Create standalone issue (no children)
  - Delete it
  - Assert HTTP 200

**GREEN Phase:**
- [ ] In `DeleteIssue` handler, before deletion per design Section 7.4:
  - Call `CountSubTasks(ctx, issueID)`
  - If count > 0, return 409 CONFLICT with message "cannot delete issue with active sub-tasks"

**REFACTOR Phase:**
- [ ] Verify error response uses code `CONFLICT` per design Section 11

**Acceptance Criteria:**
- [ ] Issue with sub-tasks cannot be deleted (409 CONFLICT)
- [ ] Issue without sub-tasks can be deleted (200)
- [ ] After all children are deleted, parent can then be deleted
- [ ] Error response includes `"code": "CONFLICT"` and descriptive message

---

### Component 6: Issue CRUD Handler

#### Task 6.1: Implement CreateIssue Handler

**Linked Requirements:** REQ-ISS-001, REQ-ISS-002, REQ-ISS-003, REQ-ISS-008, REQ-ISS-009, REQ-ISS-010, REQ-ISS-011, REQ-ISS-050, REQ-ISS-056, REQ-ISS-057, REQ-ISS-071, REQ-ISS-072, REQ-ISS-075, REQ-ISS-091

**RED Phase:**
- [ ] Write integration test `TestCreateIssue_Success`:
  - POST valid issue with title, description, priority
  - Assert 201, response has UUID `id`, auto-generated `identifier`, correct defaults applied
  - Expected failure: Handler not implemented
- [ ] Write integration test `TestCreateIssue_Defaults`:
  - POST with only `title` field
  - Assert response has status=backlog, priority=medium, type=task, requestDepth=0
- [ ] Write integration test `TestCreateIssue_MissingTitle`:
  - POST with empty/missing title
  - Assert 400 VALIDATION_ERROR
- [ ] Write integration test `TestCreateIssue_TitleTooLong`:
  - POST with title > 500 characters
  - Assert 400 VALIDATION_ERROR
- [ ] Write integration test `TestCreateIssue_InvalidEnums`:
  - POST with invalid type/status/priority values
  - Assert 400 VALIDATION_ERROR for each case

**GREEN Phase:**
- [ ] Implement `IssueHandler` struct and `NewIssueHandler(q *db.Queries)` constructor per design Section 3.2
- [ ] Implement `CreateIssue` handler following the flow in design Section 3.2:
  1. Parse and validate request body (title required, length <= 500, enum validation)
  2. Apply defaults for missing optional fields
  3. Validate parentId if provided (same squad, no cycle -- calls Tasks 5.1/5.2)
  4. Validate assigneeAgentId / assigneeUserId belong to squad if provided
  5. Begin transaction
  6. Generate identifier via atomic counter increment (calls Task 3.1)
  7. INSERT issue via `CreateIssue` query
  8. Commit transaction
  9. Return 201 with issue JSON
- [ ] Register route: `POST /api/squads/{squadId}/issues`

**REFACTOR Phase:**
- [ ] Extract validation logic into a reusable `validateCreateIssueRequest` function
- [ ] Extract transaction logic into `createIssueInTx` per design Section 5

**Acceptance Criteria:**
- [ ] Valid request returns 201 with full issue JSON
- [ ] Identifier auto-generated in `{PREFIX}-{N}` format
- [ ] Defaults applied: status=backlog, priority=medium, type=task, requestDepth=0
- [ ] Missing title returns 400 VALIDATION_ERROR
- [ ] Title > 500 chars returns 400 VALIDATION_ERROR
- [ ] Invalid enum values return 400 VALIDATION_ERROR
- [ ] Assignee not in squad returns 404 NOT_FOUND
- [ ] JWT required (401 without auth)
- [ ] Squad membership required (403 for non-members)

---

#### Task 6.2: Implement GetIssue Handler (UUID + Identifier Lookup)

**Linked Requirements:** REQ-ISS-052, REQ-ISS-055, REQ-ISS-056, REQ-ISS-057, REQ-ISS-070, REQ-ISS-090

**RED Phase:**
- [ ] Write integration test `TestGetIssue_ByUUID`:
  - Create issue, GET by UUID
  - Assert 200, full issue returned with all fields
- [ ] Write integration test `TestGetIssue_ByIdentifier`:
  - Create issue, GET by identifier string (e.g., "ARI-1")
  - Assert 200, same issue returned as UUID lookup
- [ ] Write integration test `TestGetIssue_NotFound`:
  - GET with non-existent UUID
  - Assert 404 NOT_FOUND
- [ ] Write integration test `TestGetIssue_InvalidId`:
  - GET with "not-a-uuid-or-identifier"
  - Assert 400 INVALID_ID
- [ ] Write integration test `TestGetIssue_WrongSquad`:
  - Create issue in squad A, user only has membership in squad B
  - GET the issue
  - Assert 403 FORBIDDEN

**GREEN Phase:**
- [ ] Implement `GetIssue` handler per design Section 3.2 and Section 8:
  - Parse path parameter `id`
  - Match against `identifierPattern` regex `^[A-Z]{2,10}-\d+$`
  - If identifier match: resolve squad_id from user's accessible squads, then query via `GetIssueByIdentifier(ctx, squadID, identifier)`
  - If valid UUID: query via `GetIssueByID`
  - Otherwise: return 400 INVALID_ID
  - Verify authenticated user's squad membership for the issue's `squad_id`
  - Return 200 with issue JSON
- [ ] Register route: `GET /api/issues/{id}`

**REFACTOR Phase:**
- [ ] Extract `resolveIssueID` helper per design Section 8 for reuse in update/delete handlers

**Acceptance Criteria:**
- [ ] UUID lookup returns correct issue (200)
- [ ] Identifier lookup (e.g., "ARI-1") returns correct issue (200)
- [ ] Non-existent issue returns 404 NOT_FOUND
- [ ] Invalid format (neither UUID nor identifier) returns 400 INVALID_ID
- [ ] Cross-squad access returns 403 FORBIDDEN
- [ ] Regex matches "ARI-39", "DEV-1", "LONGPREFIX-999" but not "ari-1", "A-1", "ARI-"

---

#### Task 6.3: Implement UpdateIssue Handler

**Linked Requirements:** REQ-ISS-020, REQ-ISS-021, REQ-ISS-022, REQ-ISS-053, REQ-ISS-056, REQ-ISS-057, REQ-ISS-071, REQ-ISS-072, REQ-ISS-075, REQ-ISS-076, REQ-ISS-091, REQ-ISS-092

**RED Phase:**
- [ ] Write integration test `TestUpdateIssue_Title`:
  - Create issue, PATCH with new title
  - Assert 200, title updated, `updatedAt` changed
- [ ] Write integration test `TestUpdateIssue_ValidStatusTransition`:
  - Create issue (backlog), PATCH status to "todo"
  - Assert 200, status is "todo"
- [ ] Write integration test `TestUpdateIssue_InvalidStatusTransition`:
  - Create issue (backlog), PATCH status to "done"
  - Assert 422 INVALID_STATUS_TRANSITION
- [ ] Write integration test `TestUpdateIssue_PartialUpdate`:
  - PATCH only priority; all other fields remain unchanged
  - Assert 200, only priority changed
- [ ] Write integration test `TestUpdateIssue_IdentifierImmutable`:
  - Verify identifier field is not in UpdateIssueRequest and cannot be changed

**GREEN Phase:**
- [ ] Implement `UpdateIssue` handler per design Section 3.2 flow:
  1. Resolve issue by UUID or identifier (reuse `resolveIssueID`)
  2. Verify squad membership
  3. Parse partial update body into `UpdateIssueRequest`
  4. If status changed: call `ValidateTransition`, detect reopen via `IsReopen`, create system comment if reopen
  5. If parentId changed: validate same-squad + no cycle
  6. If assignee changed: validate belongs to squad
  7. Execute `UpdateIssue` sqlc query with COALESCE for partial fields
     **Note:** For nullable fields (parent_id, project_id, goal_id, assignee_agent_id, assignee_user_id, billing_code), the COALESCE/sqlc.narg pattern cannot distinguish "not provided" from "set to NULL". The handler must read the current issue first and pass current values for omitted nullable fields, only passing NULL when the client explicitly sends `"field": null`. See design Section 4.3 note.
  8. Return 200 with updated issue JSON
- [ ] Register route: `PATCH /api/issues/{id}`

**REFACTOR Phase:**
- [ ] Extract status change handling into `handleStatusChange` helper per design Section 6.3
- [ ] Ensure `updatedAt` is set by the database trigger, not application code

**Acceptance Criteria:**
- [ ] Partial updates only modify specified fields
- [ ] Valid status transitions succeed (200)
- [ ] Invalid status transitions return 422 with `INVALID_STATUS_TRANSITION` code
- [ ] Reopen transitions create system comments (done->todo, cancelled->todo)
- [ ] Identifier is immutable (not in update DTO)
- [ ] `updatedAt` is automatically refreshed by DB trigger
- [ ] Validation errors return 400 VALIDATION_ERROR

---

#### Task 6.4: Implement DeleteIssue Handler

**Linked Requirements:** REQ-ISS-032, REQ-ISS-054, REQ-ISS-056, REQ-ISS-057, REQ-ISS-090

**RED Phase:**
- [ ] Write integration test `TestDeleteIssue_Success`:
  - Create issue, DELETE it
  - Assert 200, response body `{"message": "issue deleted"}`
  - GET same issue returns 404
- [ ] Write integration test `TestDeleteIssue_NotFound`:
  - DELETE non-existent issue UUID
  - Assert 404 NOT_FOUND
- [ ] Write integration test `TestDeleteIssue_WithSubTasks`:
  - Create parent + child, DELETE parent
  - Assert 409 CONFLICT

**GREEN Phase:**
- [ ] Implement `DeleteIssue` handler per design Section 3.2:
  1. Resolve issue by UUID or identifier
  2. Verify squad membership
  3. Check sub-task count via `CountSubTasks`; reject if > 0 with 409 CONFLICT
  4. Execute `DeleteIssue` query (hard delete)
  5. Return 200 with `{"message": "issue deleted"}`
- [ ] Register route: `DELETE /api/issues/{id}`

**REFACTOR Phase:**
- [ ] Ensure consistent error response format across all handlers per design Section 11

**Acceptance Criteria:**
- [ ] Leaf issue deleted successfully (200)
- [ ] Issue with sub-tasks returns 409 CONFLICT
- [ ] Non-existent issue returns 404 NOT_FOUND
- [ ] Squad membership enforced (403 for non-members)
- [ ] Hard delete: row removed from database

---

#### Task 6.5: Implement ListIssues Handler

**Linked Requirements:** REQ-ISS-051, REQ-ISS-056, REQ-ISS-057, REQ-ISS-070, REQ-ISS-080, REQ-ISS-081

**RED Phase:**
- [ ] Write integration test `TestListIssues_Basic`:
  - Create 3 issues in a squad, GET list
  - Assert 200, array of 3 issues, pagination metadata present
- [ ] Write integration test `TestListIssues_FilterByStatus`:
  - Create issues with different statuses, filter by ?status=todo
  - Assert only matching issues returned
- [ ] Write integration test `TestListIssues_Pagination`:
  - Create 5 issues, request ?limit=2&offset=2
  - Assert 2 issues returned, correct total count in pagination
- [ ] Write integration test `TestListIssues_SortByPriority`:
  - Create issues with different priorities, ?sort=priority
  - Assert correct ordering
- [ ] Write integration test `TestListIssues_EmptySquad`:
  - List issues in squad with no issues
  - Assert 200, empty data array, total=0

**GREEN Phase:**
- [ ] Implement `ListIssues` handler per design Section 3.2:
  1. Parse squadId from path
  2. Verify squad membership
  3. Parse query parameters into `IssueListParams` via `parseIssueListParams` (Task 8.1)
  4. Execute `ListIssuesBySquad` query
  5. Return 200 with `{"data": [...], "pagination": {"limit": N, "offset": N, "total": N}}`
- [ ] Register route: `GET /api/squads/{squadId}/issues`

**REFACTOR Phase:**
- [ ] Extract pagination response wrapper into a shared helper for reuse by comment listing
- [ ] Validate sort field allowlist: `created_at`, `updated_at`, `priority`, `status`

**Acceptance Criteria:**
- [ ] All 8 filters work: status, priority, type, assigneeAgentId, assigneeUserId, projectId, goalId, parentId
- [ ] Default limit is 50, max limit capped at 200
- [ ] Default sort is `created_at` descending
- [ ] Invalid filter values return 400 VALIDATION_ERROR
- [ ] Response includes `data` array and `pagination` object with `total`
- [ ] Squad-scoped: only issues from the specified squad returned (403 for non-members)

---

### Component 7: Comment Handler

#### Task 7.1: Implement CreateComment Handler

**Linked Requirements:** REQ-ISS-040, REQ-ISS-042, REQ-ISS-043, REQ-ISS-044, REQ-ISS-060, REQ-ISS-062, REQ-ISS-063, REQ-ISS-090, REQ-ISS-091

**RED Phase:**
- [ ] Write integration test `TestCreateComment_Success`:
  - Create issue, POST comment with authorType=user, valid authorId, body text
  - Assert 201, comment returned with all fields including generated UUID
- [ ] Write integration test `TestCreateComment_EmptyBody`:
  - POST with empty body string
  - Assert 400 VALIDATION_ERROR
- [ ] Write integration test `TestCreateComment_InvalidAuthorType`:
  - POST with authorType="bot"
  - Assert 400 VALIDATION_ERROR
- [ ] Write integration test `TestCreateComment_AgentAuthor`:
  - POST with authorType=agent, valid agent authorId from the same squad
  - Assert 201
- [ ] Write integration test `TestCreateComment_IssueNotFound`:
  - POST comment on non-existent issue UUID
  - Assert 404 NOT_FOUND

**GREEN Phase:**
- [ ] Implement `CreateComment` handler per design Section 3.2:
  1. Parse issueId from path, fetch issue via `GetIssueByID`
  2. Verify squad membership for the issue's squad
  3. Parse and validate request body: body non-empty (REQ-ISS-062), authorType valid enum
  4. If authorType=agent: validate agent exists via agent query
  5. If authorType=user: validate user exists via user query
  6. INSERT comment via `CreateIssueComment`
  7. Return 201 with comment JSON
- [ ] Register route: `POST /api/issues/{issueId}/comments`

**REFACTOR Phase:**
- [ ] Extract author validation into a reusable helper function

**Acceptance Criteria:**
- [ ] Valid comment returns 201 with full comment JSON
- [ ] Empty body rejected with 400 VALIDATION_ERROR
- [ ] Invalid authorType rejected with 400 VALIDATION_ERROR
- [ ] Agent author validated against agents table (REQ-ISS-043)
- [ ] User author validated against users table (REQ-ISS-043)
- [ ] Issue not found returns 404 NOT_FOUND
- [ ] Squad membership enforced via parent issue's squad_id

---

#### Task 7.2: Implement ListComments Handler

**Linked Requirements:** REQ-ISS-061, REQ-ISS-063, REQ-ISS-082

**RED Phase:**
- [ ] Write integration test `TestListComments_Chronological`:
  - Create issue, add 3 comments
  - GET comments
  - Assert ordered by createdAt ascending (oldest first)
- [ ] Write integration test `TestListComments_Pagination`:
  - Create 5 comments, request ?limit=2&offset=1
  - Assert 2 comments returned with correct pagination metadata
- [ ] Write integration test `TestListComments_EmptyIssue`:
  - List comments for issue with no comments
  - Assert 200, empty data array, total=0

**GREEN Phase:**
- [ ] Implement `ListComments` handler per design Section 3.2:
  1. Parse issueId from path, fetch issue, verify squad membership
  2. Parse limit/offset from query params (default limit=50, max 200)
  3. Execute `ListIssueComments` query (ordered by created_at ASC)
  4. Execute `CountIssueComments` for total count
  5. Return 200 with `{"data": [...], "pagination": {"limit": N, "offset": N, "total": N}}`
- [ ] Register route: `GET /api/issues/{issueId}/comments`

**REFACTOR Phase:**
- [ ] Reuse pagination parsing and response wrapper helpers from Task 6.5

**Acceptance Criteria:**
- [ ] Comments returned in chronological order (created_at ASC)
- [ ] Pagination works with limit/offset
- [ ] Default limit is 50, max capped at 200
- [ ] Response includes `data` array and `pagination` object with `total`
- [ ] Squad membership enforced via parent issue's squad_id

---

### Component 8: Filtering and Pagination

#### Task 8.1: Implement Query Parameter Parser

**Linked Requirements:** REQ-ISS-051, REQ-ISS-080, REQ-ISS-081, REQ-ISS-091

**RED Phase:**
- [ ] Write unit test `TestParseIssueListParams_Defaults`:
  - Parse empty query string
  - Assert limit=50, offset=0, sort="created_at", all filters nil
- [ ] Write unit test `TestParseIssueListParams_AllFilters`:
  - Parse query with all 8 filter params set to valid values
  - Assert all fields populated correctly
- [ ] Write unit test `TestParseIssueListParams_InvalidStatus`:
  - Parse ?status=invalid
  - Assert error returned
- [ ] Write unit test `TestParseIssueListParams_LimitCap`:
  - Parse ?limit=500
  - Assert limit capped at 200 (silent cap, no error)
- [ ] Write unit test `TestParseIssueListParams_InvalidSort`:
  - Parse ?sort=invalid_field
  - Assert error returned

**GREEN Phase:**
- [ ] Implement `parseIssueListParams(r *http.Request, squadID uuid.UUID) (domain.IssueListParams, error)` per design Section 9.2:
  - Parse each query param, validate enum values and UUID formats
  - Apply defaults: limit=50, offset=0, sort="created_at"
  - Cap limit at 200 silently
  - Validate sort against allowlist: `created_at`, `updated_at`, `priority`, `status`

**REFACTOR Phase:**
- [ ] Extract UUID parsing helper for DRY across the 5 UUID filter params
- [ ] Extract limit/offset parsing into a shared pagination helper reusable for comments

**Acceptance Criteria:**
- [ ] Default values applied when params missing
- [ ] All 8 filter params parsed correctly (status, priority, type, assigneeAgentId, assigneeUserId, projectId, goalId, parentId)
- [ ] Invalid enum values return descriptive error
- [ ] Invalid UUID values return descriptive error
- [ ] Limit capped at 200 silently (no error returned)
- [ ] Negative offset returns error
- [ ] Sort field validated against allowlist of 4 values

---

#### Task 8.2: Implement Dynamic WHERE Clauses in sqlc

**Linked Requirements:** REQ-ISS-051, REQ-ISS-NF-001

**RED Phase:**
- [ ] Write integration test `TestListIssues_MultipleFilters`:
  - Create issues with varying status/priority/assignee combinations
  - Query with ?status=todo&priority=high
  - Assert only issues matching BOTH filters returned
- [ ] Write integration test `TestListIssues_FilterByAssigneeAgent`:
  - Assign agents to issues, filter by assigneeAgentId
  - Assert correct filtering
- [ ] Write integration test `TestListIssues_FilterByParentId`:
  - Create parent + children, filter by parentId
  - Assert only direct children returned (not grandchildren)

**GREEN Phase:**
- [ ] Verify `ListIssuesBySquad` sqlc query handles NULL filter params correctly per design Section 4.3:
  - Pattern: `(sqlc.narg('field')::TYPE IS NULL OR field = sqlc.narg('field'))`
  - NULL param = no filtering on that column; non-NULL = exact match
- [ ] Map `IssueListParams` fields to sqlc query parameters in the handler

**REFACTOR Phase:**
- [ ] Verify query plan uses indexes for the most common filter combinations (squad + status)
- [ ] Ensure CASE-based sort expressions work correctly with sqlc

**Acceptance Criteria:**
- [ ] Multiple filters combine with AND logic
- [ ] NULL filter params return all rows (no filtering on that column)
- [ ] Each of the 8 filter types works independently
- [ ] Combined filters return correct intersection of results
- [ ] Query performance uses indexes for squad + status filter (REQ-ISS-NF-001)

---

### Component 9: Integration Tests

#### Task 9.1: Full CRUD Lifecycle Test

**Linked Requirements:** REQ-ISS-050, REQ-ISS-052, REQ-ISS-053, REQ-ISS-054, REQ-ISS-055

**RED Phase:**
- [ ] Write `TestIssueCRUDLifecycle` in `internal/server/handlers/issue_handler_test.go`:
  - Create issue -> verify 201 with identifier pattern
  - Get by UUID -> verify 200 with all fields matching create response
  - Get by identifier -> verify 200 with same data as UUID lookup
  - Update title + priority -> verify 200 with changes applied
  - Delete -> verify 200
  - Get again -> verify 404

**GREEN Phase:**
- [ ] All handlers implemented (should pass if prior tasks are complete)

**REFACTOR Phase:**
- [ ] Ensure test cleanup uses transaction rollback or dedicated test squad

**Acceptance Criteria:**
- [ ] Full create -> read (UUID) -> read (identifier) -> update -> delete -> read (404) flow succeeds
- [ ] Identifier lookup returns identical data to UUID lookup
- [ ] Deleted issue returns 404 on subsequent GET

---

#### Task 9.2: Status Transition Integration Tests

**Linked Requirements:** REQ-ISS-020, REQ-ISS-021, REQ-ISS-022, REQ-ISS-092

**RED Phase:**
- [ ] Write `TestStatusTransition_FullLifecycle`:
  - backlog -> todo -> in_progress -> done (happy path through completion)
- [ ] Write `TestStatusTransition_BlockAndUnblock`:
  - todo -> blocked -> in_progress -> done
- [ ] Write `TestStatusTransition_ReopenFromDone`:
  - Complete issue (done), then reopen to todo
  - Verify system comment created with correct body
- [ ] Write `TestStatusTransition_ReopenFromCancelled`:
  - Cancel issue, then reopen to todo
  - Verify system comment created
- [ ] Write `TestStatusTransition_InvalidReturns422`:
  - Attempt backlog -> done
  - Assert 422 with code `INVALID_STATUS_TRANSITION` and message containing both status values

**GREEN Phase:**
- [ ] All handlers implemented (should pass if prior tasks are complete)

**REFACTOR Phase:**
- [ ] Consolidate test helpers for multi-step status transitions

**Acceptance Criteria:**
- [ ] All valid transition paths in the state machine succeed
- [ ] Invalid transitions return 422 with `INVALID_STATUS_TRANSITION` code
- [ ] Error message includes current and attempted status per REQ-ISS-092
- [ ] Reopen creates system comment with from/to status in body

---

#### Task 9.3: Sub-task Hierarchy Integration Tests

**Linked Requirements:** REQ-ISS-030, REQ-ISS-031, REQ-ISS-032, REQ-ISS-033

**RED Phase:**
- [ ] Write `TestSubTask_CreateWithParent`:
  - Create parent, create child with parentId
  - Assert child response has correct parentId field
- [ ] Write `TestSubTask_FilterByParentId`:
  - Create parent + 3 children
  - List issues with ?parentId=<parentUUID>
  - Assert exactly 3 results
- [ ] Write `TestSubTask_CrossSquadParentRejected`:
  - Parent in squad A, attempt child in squad B with that parentId
  - Assert 400 VALIDATION_ERROR
- [ ] Write `TestSubTask_CycleDetection_A_B_A`:
  - Create A, B(parent=A), then update A's parent to B
  - Assert 400 "circular parent reference detected"
- [ ] Write `TestSubTask_CycleDetection_A_B_C_A`:
  - Create A, B(parent=A), C(parent=B), then update A's parent to C
  - Assert 400 "circular parent reference detected"
- [ ] Write `TestSubTask_DeleteParentBlocked`:
  - Create parent + child, attempt delete parent
  - Assert 409 CONFLICT

**GREEN Phase:**
- [ ] All handlers implemented (should pass if prior tasks are complete)

**REFACTOR Phase:**
- [ ] Add test helper for building sub-task chains of arbitrary depth

**Acceptance Criteria:**
- [ ] Sub-tasks can be created with valid parentId
- [ ] Sub-tasks can be queried by parentId filter
- [ ] Cross-squad parent references rejected (400)
- [ ] Direct and transitive cycles detected and rejected (400)
- [ ] Parent with children cannot be deleted (409)

---

#### Task 9.4: Comment Integration Tests

**Linked Requirements:** REQ-ISS-060, REQ-ISS-061, REQ-ISS-062, REQ-ISS-043, REQ-ISS-044

**RED Phase:**
- [ ] Write `TestComment_CreateAndList`:
  - Create issue, add 3 comments (one each: user, agent, system author types)
  - List comments
  - Assert 3 comments in chronological order with correct author types
- [ ] Write `TestComment_EmptyBodyRejected`:
  - POST with empty body
  - Assert 400 VALIDATION_ERROR
- [ ] Write `TestComment_InvalidAuthorType`:
  - POST with authorType="bot"
  - Assert 400 VALIDATION_ERROR
- [ ] Write `TestComment_PaginationWorks`:
  - Add 5 comments, request ?limit=2&offset=1
  - Assert 2 comments in response, pagination total=5

**GREEN Phase:**
- [ ] All handlers implemented (should pass if prior tasks are complete)

**REFACTOR Phase:**
- [ ] Verify comment immutability: no update/delete endpoints exist in Phase 1 (REQ-ISS-044)

**Acceptance Criteria:**
- [ ] Comments created by all 3 author types (user, agent, system)
- [ ] Comments listed in chronological order (created_at ASC)
- [ ] Empty body rejected with 400
- [ ] Invalid author type rejected with 400
- [ ] Pagination works with correct total count
- [ ] No edit/delete endpoints for comments exist (Phase 1 immutability)

---

#### Task 9.5: Concurrent Identifier Generation Test

**Linked Requirements:** REQ-ISS-003, REQ-ISS-NF-002, REQ-ISS-NF-004

**RED Phase:**
- [ ] Write `TestConcurrentIdentifierGeneration`:
  - Create a squad with prefix "RACE"
  - Launch 20 goroutines, each creating 1 issue via the HTTP endpoint
  - Collect all 20 identifiers from responses
  - Assert: all 20 are unique, all match `RACE-N` format, counter values span 1-20 (no duplicates)

**GREEN Phase:**
- [ ] Atomic counter increment via `UPDATE ... RETURNING` ensures serialization at the row-lock level (should pass from Task 3.1 implementation)

**REFACTOR Phase:**
- [ ] Add timeout to goroutine sync to prevent test hanging on deadlock
- [ ] Use `sync.WaitGroup` and `errgroup` for clean concurrent test structure

**Acceptance Criteria:**
- [ ] 20 concurrent creates produce 20 unique identifiers
- [ ] No duplicate counter values
- [ ] All identifiers match `RACE-N` format
- [ ] No database deadlocks or errors
- [ ] Test completes within reasonable timeout (< 10s)

---

#### Task 9.6: Filtering Integration Tests

**Linked Requirements:** REQ-ISS-051, REQ-ISS-080, REQ-ISS-081, REQ-ISS-NF-001

**RED Phase:**
- [ ] Write `TestListIssues_FilterCombinations`:
  - Create 10 issues with varied status/priority/type/assignee values
  - Test single filters: ?status=todo, ?priority=high, ?type=conversation
  - Test combined filters: ?status=todo&priority=high
  - Assert correct result counts for each combination
- [ ] Write `TestListIssues_SortOrders`:
  - Test ?sort=created_at (default, descending)
  - Test ?sort=updated_at
  - Test ?sort=priority
  - Test ?sort=status
  - Assert correct ordering for each sort field
- [ ] Write `TestListIssues_PaginationEdgeCases`:
  - Offset beyond total count -> empty data array, total still correct
  - Limit=1 -> single item per page
  - Limit=200 (max) -> capped correctly, no error

**GREEN Phase:**
- [ ] All handlers implemented (should pass if prior tasks are complete)

**REFACTOR Phase:**
- [ ] Verify list query response time < 200ms for test data (REQ-ISS-NF-001)

**Acceptance Criteria:**
- [ ] All filter types work individually and in combination
- [ ] All 4 sort fields produce correct ordering
- [ ] Pagination edge cases handled (offset > total, limit=1, limit=max)
- [ ] Invalid filter values return 400 with descriptive errors
- [ ] Combined AND filtering returns correct intersection

---

### Final Verification Tasks

#### Task 10.1: Route Registration and Middleware Wiring

**Linked Requirements:** REQ-ISS-050, REQ-ISS-051, REQ-ISS-052, REQ-ISS-053, REQ-ISS-054, REQ-ISS-060, REQ-ISS-061, REQ-ISS-056, REQ-ISS-057

**RED Phase:**
- [ ] Write test verifying all 7 routes are registered and respond (not 405/404):
  - `POST /api/squads/{squadId}/issues`
  - `GET /api/squads/{squadId}/issues`
  - `GET /api/issues/{id}`
  - `PATCH /api/issues/{id}`
  - `DELETE /api/issues/{id}`
  - `POST /api/issues/{issueId}/comments`
  - `GET /api/issues/{issueId}/comments`

**GREEN Phase:**
- [ ] Implement `RegisterRoutes(mux *http.ServeMux)` on `IssueHandler` per design Section 3.2
- [ ] Wire into main server router with AuthMiddleware in the middleware chain

**REFACTOR Phase:**
- [ ] Verify middleware chain: AuthMiddleware -> SquadScopeMiddleware -> Handler

**Acceptance Criteria:**
- [ ] All 7 routes respond (not 404 Method Not Allowed)
- [ ] All routes require JWT auth (401 without token)
- [ ] Squad-scoped routes enforce membership (403 for non-members)
- [ ] Routes use correct HTTP methods (POST, GET, PATCH, DELETE)

---

#### Task 10.2: Pre-Merge Checklist

**Objective:** Final verification before feature merge

- [ ] `make test` -- all tests pass (0 failures)
- [ ] `make sqlc` -- regeneration produces no diff
- [ ] `make build` -- binary compiles without errors
- [ ] No linter violations
- [ ] No type errors
- [ ] All REQ-ISS-* requirements have at least one test covering them
- [ ] All error responses follow `{"error": "message", "code": "CODE"}` format per REQ-ISS-093
- [ ] Migrations run cleanly up and down (tested against embedded postgres)
- [ ] No TODO/FIXME comments left in implementation code
- [ ] Handler file follows existing codebase patterns (error helpers, response format)
- [ ] All JSON responses use camelCase field names
- [ ] Identifier uniqueness holds under concurrent load (Task 9.5 passes)

---

## Commit Strategy

After each completed task:
```bash
# After RED phase
git add internal/domain/issue_test.go
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

- The `update_updated_at_column()` trigger function may already exist from a prior migration; use `CREATE OR REPLACE FUNCTION` to avoid conflicts
- `project_id` and `goal_id` columns have no FK constraints yet; validation deferred to feature 06
- System comments use `uuid.Nil` as `authorId` -- ensure the comment query and handler accept this value

### Blockers

- [ ] Depends on `03-squad-management` for squads table with `issue_prefix` and `issue_counter` columns
- [ ] Depends on `04-agent-management` for agents table (assignee validation)
- [ ] Depends on `02-user-auth` for JWT middleware and user context

### Future Improvements (Phase 2+)

- Checkout/lock mechanism (checkoutRunId, executionLockedAt)
- IssuePipeline and advance/reject-stage endpoints
- Conversation type behavior (real-time agent invocation on comment)
- IssueAttachments and IssueLabels
- IssueApprovals (governance links)
- Soft delete with activity log audit trail
- Status history table for full audit trail
