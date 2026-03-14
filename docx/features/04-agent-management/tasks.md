# Tasks: Agent Management

**Created:** 2026-03-14
**Status:** Not Started

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Technical design: [design.md](./design.md)
- Requirement coverage: REQ-AGT-001 through REQ-AGT-073, REQ-AGT-NF-001 through NF-003
- Missing coverage: None -- all REQ-AGT-* requirements are mapped to tasks below

| Requirement(s) | Task(s) |
|-----------------|---------|
| REQ-AGT-001, 002, 005, 006 | Task 1 (schema), Task 3 (domain struct) |
| REQ-AGT-003, 004, 060, 061, 062, 063, 064 | Task 4 (input validation) |
| REQ-AGT-010, 011, 012, 013, 014, 015, 016 | Task 1 (partial index), Task 6 (hierarchy validation) |
| REQ-AGT-017, 065 | Task 9 (create handler) |
| REQ-AGT-020, 021, 022, 023 | Task 5 (status machine) |
| REQ-AGT-030, 031, 032, 033 | Task 9 (create handler) |
| REQ-AGT-034, 035 | Task 10 (list handler) |
| REQ-AGT-036, 037 | Task 10 (get handler) |
| REQ-AGT-038, 039, 040, 041, 042 | Task 11 (update handler) |
| REQ-AGT-050, 051, 052 | Task 8 (squad auth middleware) |
| REQ-AGT-070, 071 | Task 9, 11 (conflict handling) |
| REQ-AGT-072 | Task 9, 11 (parent validation errors) |
| REQ-AGT-073 | Task 12 (transition endpoint) |
| REQ-AGT-NF-001, NF-002, NF-003 | Task 1 (indexes) |

## Implementation Approach

Build bottom-up: database schema first, then pure domain types and validation logic (testable without DB), then sqlc queries, then HTTP handlers. Each component is fully tested before the next begins. Domain logic (status machine, hierarchy validation, input validation) is implemented as pure functions with table-driven unit tests. Handlers are tested via integration tests against embedded PostgreSQL. The dependency chain is: Schema -> sqlc -> Domain Types -> Status Machine -> Hierarchy Validation -> Input Validation -> Handler scaffold -> CRUD Handlers -> Integration Tests.

## Progress Summary

- Total Tasks: 15
- Completed: 0/15
- In Progress: None
- Test Coverage: 0%

---

## Tasks (TDD: Red-Green-Refactor)

### Component 1: Database Schema

#### Task 1: Create agents table migration with enums and indexes

**Linked Requirements:** REQ-AGT-001, REQ-AGT-002, REQ-AGT-005, REQ-AGT-006, REQ-AGT-014, REQ-AGT-NF-001, REQ-AGT-NF-002, REQ-AGT-NF-003

**RED Phase:**
- [ ] Write a Go test that runs the migration up/down against embedded PostgreSQL
  - Test case: Apply migration, verify `agents` table exists with correct columns and types
  - Test case: Verify `agent_role`, `agent_status`, `adapter_type` enums are created with correct values
  - Test case: Verify `idx_agents_squad_short_name` unique index exists
  - Test case: Verify `idx_agents_one_captain_per_squad` partial unique index exists
  - Test case: Verify `idx_agents_squad_id` and `idx_agents_parent_agent_id` indexes exist
  - Test case: Apply migration down, verify table and types are dropped cleanly
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/XXXXXX_create_agents.sql`
  - Create `agent_role` enum: captain, lead, member
  - Create `agent_status` enum: pending_approval, active, idle, running, error, paused, terminated
  - Create `adapter_type` enum: claude_local, codex_local, cursor, process, http, openclaw_gateway
  - Create `agents` table with all columns per design.md schema
  - Create `idx_agents_squad_short_name` unique composite index on (squad_id, short_name)
  - Create `idx_agents_squad_id` index
  - Create `idx_agents_parent_agent_id` index
  - Create `idx_agents_one_captain_per_squad` partial unique index: `ON agents(squad_id) WHERE role = 'captain' AND status != 'terminated'`
  - Create `update_agents_updated_at` trigger function and `trg_agents_updated_at` trigger
  - Implement `+goose Down` section dropping all objects in reverse order

**REFACTOR Phase:**
- [ ] Review migration for correctness, ensure goose annotations are correct
  - Focus: Verify ON DELETE CASCADE for squad_id, ON DELETE SET NULL for parent_agent_id
  - Focus: Verify CHECK constraint on budget_monthly_cents
  - Focus: Verify default values (gen_random_uuid, now(), 'active', '{}')

**Acceptance Criteria:**
- [ ] Migration applies cleanly on a fresh database with existing squads table
- [ ] Migration rolls back cleanly leaving no orphaned types
- [ ] All three enums contain exactly the values specified in REQ-AGT-001
- [ ] Partial unique index prevents two non-terminated captains in the same squad
- [ ] `updated_at` trigger fires on row update
- [ ] `budget_monthly_cents` CHECK allows NULL and rejects negative values
- [ ] `make test` passes

**Notes:**
- Migration number must be sequenced after the squad management migration
- The partial unique index is database-level defense for one-captain-per-squad (REQ-AGT-014)

---

#### Task 2: Write sqlc queries and generate Go code

**Linked Requirements:** REQ-AGT-001, REQ-AGT-014, REQ-AGT-016, REQ-AGT-034, REQ-AGT-036

**RED Phase:**
- [ ] Write the SQL queries in `internal/database/queries/agents.sql` and run `make sqlc`
  - Test case: Generated Go code compiles without errors
  - Test case: Query parameter types match expected Go types (uuid.UUID, sql.NullString, etc.)
  - Expected failure: sqlc generation fails or type mismatches

**GREEN Phase:**
- [ ] Implement all sqlc queries per design.md:
  - `CreateAgent :one` -- INSERT RETURNING *
  - `GetAgentByID :one` -- SELECT * WHERE id
  - `ListAgentsBySquad :many` -- SELECT * WHERE squad_id ORDER BY created_at ASC
  - `UpdateAgent :one` -- UPDATE with COALESCE pattern for partial updates, RETURNING *
  - `GetSquadCaptain :one` -- SELECT * WHERE squad_id AND role='captain' AND status!='terminated'
  - `GetAgentParent :one` -- SELECT id, squad_id, role WHERE id (lightweight parent lookup)
  - `CheckCycleInHierarchy :one` -- Recursive CTE walking ancestors to detect cycles
  - `CountAgentsBySquad :one` -- COUNT(*) WHERE squad_id AND status!='terminated'
  - `ListAgentChildren :many` -- SELECT * WHERE parent_agent_id ORDER BY created_at ASC
- [ ] Run `make sqlc` to generate Go code in `internal/database/db/`

**REFACTOR Phase:**
- [ ] Review generated code for type correctness
  - Focus: Ensure nullable fields map to sql.Null* types or pointer types
  - Focus: Verify UpdateAgent uses `sqlc.narg` for optional fields and `sqlc.arg('set_parent')`/`sqlc.arg('set_budget')` sentinel booleans

**Acceptance Criteria:**
- [ ] `make sqlc` runs without errors
- [ ] Generated `db.Querier` interface includes all 9 query methods
- [ ] Generated parameter structs have correct Go types for nullable fields
- [ ] `make test` passes (compilation check)

**Notes:**
- The UpdateAgent query uses COALESCE + sentinel boolean pattern for true partial updates
- `set_parent` and `set_budget` sentinel booleans distinguish "not provided" from "set to NULL"

---

### Component 2: Domain Types

#### Task 3: Define Agent struct, role/status/adapter enums, and DTOs

**Linked Requirements:** REQ-AGT-001, REQ-AGT-064

**RED Phase:**
- [ ] Write test in `internal/domain/agent_test.go` that references `domain.Agent`, `domain.AgentRole`, `domain.AgentStatus`, `domain.AdapterType`, `domain.CreateAgentRequest`, `domain.UpdateAgentRequest`, `domain.TransitionRequest`
  - Test case: `ValidAgentRoles` map contains exactly captain, lead, member
  - Test case: All seven `AgentStatus` constants are defined
  - Test case: All six `AdapterType` constants are defined
  - Expected failure: Compilation error -- types do not exist

**GREEN Phase:**
- [ ] Create `internal/domain/agent.go` with:
  - `AgentRole` string type + constants (captain, lead, member) + `ValidAgentRoles` map
  - `AgentStatus` string type + all 7 constants
  - `AdapterType` string type + all 6 constants
  - `Agent` struct with all fields per design.md
  - `CreateAgentRequest` struct with JSON tags
  - `UpdateAgentRequest` struct with pointer fields and sentinel booleans (`SetParent`, `SetBudget`)
  - `TransitionRequest` struct
  - `HierarchyContext` struct
  - `AgentParentInfo` struct

**REFACTOR Phase:**
- [ ] Ensure consistent JSON tag naming (camelCase) and omitempty where appropriate
  - Focus: Pointer fields for nullable JSON fields
  - Focus: `json.RawMessage` for AdapterConfig

**Acceptance Criteria:**
- [ ] All types compile and match the design.md interface exactly
- [ ] `ValidAgentRoles` correctly identifies valid/invalid roles
- [ ] JSON marshaling of Agent struct produces camelCase keys
- [ ] `make test` passes

---

### Component 3: Status Machine

#### Task 5: Implement ValidateStatusTransition with transition map

**Linked Requirements:** REQ-AGT-020, REQ-AGT-021, REQ-AGT-022, REQ-AGT-023

**RED Phase:**
- [ ] Write table-driven test `TestValidateStatusTransition` in `internal/domain/agent_test.go`
  - Test all 9 valid transitions: pending_approval->active, active->idle, idle->running, running->idle, running->error, active->paused, idle->paused, running->paused, paused->active
  - Test all 6 "any->terminated" transitions (from each non-terminated status)
  - Test terminated->active is rejected (terminal state, REQ-AGT-022)
  - Test terminated->terminated is rejected
  - Test pending_approval->idle is rejected (REQ-AGT-023: only active or terminated)
  - Test pending_approval->running is rejected
  - Test pending_approval->paused is rejected
  - Test active->running is rejected (must go through idle)
  - Test active->error is rejected
  - Test idle->active is rejected
  - Test idle->error is rejected
  - Test running->active is rejected
  - Test error->active is rejected (error only goes to terminated)
  - Test error->idle is rejected
  - Test error->running is rejected
  - Test error->paused is rejected
  - Test paused->idle is rejected
  - Test paused->running is rejected
  - Expected failure: `ValidateStatusTransition` function does not exist

**GREEN Phase:**
- [ ] Implement `validTransitions` map and `ValidateStatusTransition` function in `internal/domain/agent.go`
  - Package-level map with current->allowed-next mappings
  - Special-case terminated as target (any non-terminated -> terminated = valid)
  - Special-case terminated as source (no transitions out)
  - Return descriptive `error` with current and requested status in message

**REFACTOR Phase:**
- [ ] Verify error messages are suitable for HTTP responses
  - Focus: Messages include both current and requested status for debugging
  - Focus: No internal implementation details leaked in error messages

**Acceptance Criteria:**
- [ ] All 9 valid transitions return nil error
- [ ] All 6 any->terminated transitions return nil error
- [ ] All invalid transitions return non-nil error with descriptive message
- [ ] Terminated is a true terminal state (no outgoing transitions)
- [ ] pending_approval only allows active and terminated
- [ ] error only allows terminated
- [ ] `make test` passes with all subtests green

---

### Component 4: Hierarchy Validation

#### Task 6: Implement ValidateHierarchy for role-parent rules

**Linked Requirements:** REQ-AGT-010, REQ-AGT-011, REQ-AGT-012, REQ-AGT-013, REQ-AGT-014, REQ-AGT-015

**RED Phase:**
- [ ] Write table-driven test `TestValidateHierarchy` in `internal/domain/agent_test.go`
  - Test: Captain with no parent, no existing captain -> valid
  - Test: Captain with parent -> rejected (REQ-AGT-011)
  - Test: Second captain in squad (different agent) -> rejected (REQ-AGT-014)
  - Test: Existing captain updating self -> allowed
  - Test: Lead with captain parent in same squad -> valid (REQ-AGT-012)
  - Test: Lead with no parent -> rejected
  - Test: Lead with lead parent -> rejected (parent must be captain)
  - Test: Lead with member parent -> rejected
  - Test: Lead with captain parent in different squad -> rejected (REQ-AGT-015)
  - Test: Member with lead parent in same squad -> valid (REQ-AGT-013)
  - Test: Member with no parent -> rejected
  - Test: Member with captain parent -> rejected (parent must be lead)
  - Test: Member with member parent -> rejected
  - Test: Member with lead parent in different squad -> rejected (REQ-AGT-015)
  - Test: Invalid role string -> rejected
  - Expected failure: `ValidateHierarchy` function does not exist

**GREEN Phase:**
- [ ] Implement `ValidateHierarchy(ctx HierarchyContext) error` in `internal/domain/agent.go`
  - Switch on ctx.Role (captain, lead, member, default)
  - Captain: require ParentAgentID == nil, check ExistingCaptainID (allow self-update)
  - Lead: require parent exists, parent role == captain, parent squad == agent squad
  - Member: require parent exists, parent role == lead, parent squad == agent squad
  - Default: return invalid role error

**REFACTOR Phase:**
- [ ] Extract common same-squad check into a helper if duplication warrants it
  - Focus: Error messages clearly identify the constraint that was violated

**Acceptance Criteria:**
- [ ] All role-parent combinations are correctly validated
- [ ] Cross-squad parent references are rejected
- [ ] One-captain-per-squad is enforced at application level (DB partial index is backup)
- [ ] Self-update of existing captain is permitted
- [ ] `make test` passes

---

#### Task 7: Implement ValidateHierarchyChange for update operations

**Linked Requirements:** REQ-AGT-041, REQ-AGT-016

**RED Phase:**
- [ ] Write test `TestValidateHierarchyChange` in `internal/domain/agent_test.go`
  - Test: Role change from captain to lead with children -> rejected (orphan prevention)
  - Test: Role change from lead to member with children -> rejected
  - Test: Role change from lead to member with zero children -> allowed (plus valid parent)
  - Test: Parent change with valid hierarchy -> allowed
  - Test: Role change to captain when captain already exists -> rejected
  - Expected failure: `ValidateHierarchyChange` function does not exist

**GREEN Phase:**
- [ ] Implement `ValidateHierarchyChange` in `internal/domain/agent.go`
  - Merge effective role and parent from existing agent + patch fields
  - If role is changing and agent has children, reject to prevent orphans
  - Build HierarchyContext and delegate to ValidateHierarchy

**REFACTOR Phase:**
- [ ] Ensure function signature matches design.md exactly
  - Focus: Clear separation between "child orphan" errors and "hierarchy rule" errors

**Acceptance Criteria:**
- [ ] Role changes that would orphan children are rejected with descriptive error
- [ ] Role changes without children pass through to hierarchy validation
- [ ] Merged context correctly inherits unchanged fields from existing agent
- [ ] `make test` passes

---

### Component 5: Input Validation

#### Task 4: Implement CreateAgent and UpdateAgent input validation

**Linked Requirements:** REQ-AGT-003, REQ-AGT-060, REQ-AGT-061, REQ-AGT-062, REQ-AGT-063, REQ-AGT-064, REQ-AGT-065

**RED Phase:**
- [ ] Write table-driven test `TestValidateCreateAgentInput` in `internal/domain/agent_test.go`
  - Test: Valid input with all fields -> no error
  - Test: Empty name -> error (REQ-AGT-003)
  - Test: Name > 255 chars -> error (REQ-AGT-060)
  - Test: Empty shortName -> error (REQ-AGT-003)
  - Test: shortName > 50 chars -> error (REQ-AGT-061)
  - Test: shortName with uppercase -> error (REQ-AGT-061)
  - Test: shortName with spaces -> error
  - Test: shortName with underscores -> error
  - Test: Valid shortName "my-agent-01" -> no error
  - Test: Invalid role "boss" -> error (REQ-AGT-064)
  - Test: Negative budgetMonthlyCents -> error (REQ-AGT-063)
  - Test: Zero budgetMonthlyCents -> no error
  - Test: Invalid adapterConfig JSON -> error (REQ-AGT-062)
  - Test: Valid adapterConfig JSON -> no error
  - Test: Nil optional fields -> no error
  - Expected failure: `ValidateCreateAgentInput` function does not exist

**GREEN Phase:**
- [ ] Implement `ValidateCreateAgentInput(input CreateAgentRequest) error` in `internal/domain/agent.go`
  - Validate name required and <= 255 chars
  - Validate shortName required, <= 50 chars, matches `^[a-z0-9-]+$`
  - Validate role is in ValidAgentRoles
  - Validate budgetMonthlyCents >= 0 when provided
  - Validate adapterConfig is valid JSON when provided
- [ ] Implement `ValidateUpdateAgentInput(input UpdateAgentRequest) error` for PATCH validation
  - Same rules but all fields optional (only validate if provided)
  - Reject if `squadId` field is present (REQ-AGT-042)

**REFACTOR Phase:**
- [ ] Compile `shortNameRegex` once at package level
  - Focus: Consistent error message format across all validators

**Acceptance Criteria:**
- [ ] All validation rules from REQ-AGT-060 through REQ-AGT-064 are enforced
- [ ] shortName regex `^[a-z0-9-]+$` correctly rejects invalid characters
- [ ] Nil/zero optional fields are allowed without error
- [ ] Error messages are user-facing quality (no stack traces, no internal jargon)
- [ ] `make test` passes

---

### Component 6: Agent CRUD Handlers

#### Task 8: Scaffold AgentHandler with route registration and squad auth check

**Linked Requirements:** REQ-AGT-050, REQ-AGT-051, REQ-AGT-052

**RED Phase:**
- [ ] Write test that creates `AgentHandler` and verifies routes are registered on a mux
  - Test case: `NewAgentHandler(queries)` returns non-nil handler
  - Test case: Unauthenticated request to POST /api/agents returns 401 (REQ-AGT-051)
  - Test case: Authenticated request to a squad the user does not belong to returns 403 (REQ-AGT-052)
  - Expected failure: `AgentHandler` struct does not exist

**GREEN Phase:**
- [ ] Create `internal/server/handlers/agent_handler.go`
  - `AgentHandler` struct with `queries db.Querier`
  - `NewAgentHandler(queries db.Querier) *AgentHandler`
  - `RegisterRoutes(mux *http.ServeMux)` registering all 5 routes
  - Helper method `verifySquadMembership(ctx, userID, squadID)` that queries squad_memberships and returns 403 on failure
- [ ] Wire AgentHandler into the server's route registration

**REFACTOR Phase:**
- [ ] Extract common response helpers (writeJSON, writeError) if not already in shared package
  - Focus: Consistent error response format `{"error": "...", "code": "..."}`

**Acceptance Criteria:**
- [ ] All 5 routes are registered: POST /api/agents, GET /api/agents, GET /api/agents/{id}, PATCH /api/agents/{id}, POST /api/agents/{id}/transition
- [ ] Auth middleware rejects unauthenticated requests with 401
- [ ] Squad membership check returns 403 for unauthorized users
- [ ] `make test` passes

---

#### Task 9: Implement CreateAgent handler

**Linked Requirements:** REQ-AGT-017, REQ-AGT-030, REQ-AGT-031, REQ-AGT-032, REQ-AGT-033, REQ-AGT-065, REQ-AGT-070, REQ-AGT-071, REQ-AGT-072

**RED Phase:**
- [ ] Write handler test for POST /api/agents
  - Test case: Valid captain creation returns 201 with complete agent object (REQ-AGT-032)
  - Test case: Valid lead creation with captain parent returns 201
  - Test case: Missing required fields returns 400 VALIDATION_ERROR (REQ-AGT-033)
  - Test case: Invalid hierarchy (lead without captain parent) returns 400 VALIDATION_ERROR (REQ-AGT-072)
  - Test case: Duplicate shortName returns 409 CONFLICT (REQ-AGT-070)
  - Test case: Second captain returns 409 CONFLICT (REQ-AGT-071)
  - Test case: Squad with `requireApprovalForNewAgents=true` sets status to pending_approval (REQ-AGT-017)
  - Test case: Squad with `requireApprovalForNewAgents=false` sets status to active (REQ-AGT-065)
  - Test case: Status field in request body is ignored (REQ-AGT-065)
  - Expected failure: `CreateAgent` method does not exist

**GREEN Phase:**
- [ ] Implement `(h *AgentHandler) CreateAgent(w, r)` following the design.md data flow:
  1. Parse and validate request body via `domain.ValidateCreateAgentInput`
  2. Extract user_id from auth context, verify squad membership
  3. Query squad settings for `requireApprovalForNewAgents`; set initial status accordingly
  4. If role has parent requirement, fetch parent via `GetAgentParent`, build `HierarchyContext`, call `domain.ValidateHierarchy`
  5. If role is captain, call `GetSquadCaptain` to check for existing captain
  6. Call `queries.CreateAgent`
  7. Handle DB constraint violations: unique shortName -> 409, captain partial index -> 409
  8. Return 201 with created agent JSON

**REFACTOR Phase:**
- [ ] Extract DB constraint error detection into a helper function
  - Focus: Map PostgreSQL unique_violation error codes to appropriate HTTP status codes

**Acceptance Criteria:**
- [ ] Created agent has a UUID id, correct timestamps, and all provided fields
- [ ] Initial status respects squad governance settings (REQ-AGT-017)
- [ ] Caller-provided status field is ignored (REQ-AGT-065)
- [ ] Hierarchy validation errors return 400 with clear message
- [ ] Constraint violations return 409 with CONFLICT code
- [ ] Response body matches the API contract in design.md
- [ ] `make test` passes

---

#### Task 10: Implement ListAgents and GetAgent handlers

**Linked Requirements:** REQ-AGT-034, REQ-AGT-035, REQ-AGT-036, REQ-AGT-037

**RED Phase:**
- [ ] Write handler tests for GET /api/agents and GET /api/agents/{id}
  - Test case: GET /api/agents?squadId=... returns 200 with array of agents (REQ-AGT-034)
  - Test case: GET /api/agents without squadId returns 400 VALIDATION_ERROR (REQ-AGT-035)
  - Test case: GET /api/agents?squadId=... with invalid UUID returns 400
  - Test case: GET /api/agents/{id} returns 200 with single agent (REQ-AGT-036)
  - Test case: GET /api/agents/{non-existent-id} returns 404 NOT_FOUND (REQ-AGT-037)
  - Test case: GET /api/agents/{invalid-uuid} returns 400
  - Expected failure: `ListAgents` and `GetAgent` methods do not exist

**GREEN Phase:**
- [ ] Implement `(h *AgentHandler) ListAgents(w, r)`:
  1. Parse squadId from query parameter, validate UUID
  2. Verify squad membership
  3. Call `queries.ListAgentsBySquad`
  4. Return 200 with JSON array
- [ ] Implement `(h *AgentHandler) GetAgent(w, r)`:
  1. Parse id from path parameter, validate UUID
  2. Call `queries.GetAgentByID` (return 404 if sql.ErrNoRows)
  3. Verify squad membership for agent's squad_id
  4. Return 200 with JSON object

**REFACTOR Phase:**
- [ ] Ensure consistent UUID parsing and error handling across both handlers
  - Focus: Reuse path/query param parsing helpers

**Acceptance Criteria:**
- [ ] List returns agents ordered by created_at ASC
- [ ] List requires squadId query parameter
- [ ] Get returns 404 with `{"error": "agent not found", "code": "NOT_FOUND"}` for missing agents
- [ ] Both handlers enforce squad membership authorization
- [ ] `make test` passes

---

#### Task 11: Implement UpdateAgent handler

**Linked Requirements:** REQ-AGT-038, REQ-AGT-039, REQ-AGT-040, REQ-AGT-041, REQ-AGT-042

**RED Phase:**
- [ ] Write handler tests for PATCH /api/agents/{id}
  - Test case: Update name only returns 200 with updated agent (REQ-AGT-038)
  - Test case: Update shortName returns 200
  - Test case: Update with invalid shortName format returns 400
  - Test case: Update status triggers status machine validation (REQ-AGT-040)
  - Test case: Invalid status transition returns 400 INVALID_STATUS_TRANSITION
  - Test case: Update role triggers hierarchy re-validation (REQ-AGT-041)
  - Test case: Update role from captain to lead with children returns 400 (orphan prevention)
  - Test case: Update parentAgentId triggers hierarchy re-validation
  - Test case: Update parentAgentId to agent in different squad returns 400
  - Test case: Attempt to change squadId returns 400 (REQ-AGT-042)
  - Test case: Duplicate shortName on update returns 409
  - Test case: Update non-existent agent returns 404
  - Expected failure: `UpdateAgent` method does not exist

**GREEN Phase:**
- [ ] Implement `(h *AgentHandler) UpdateAgent(w, r)` following design.md update flow:
  1. Parse id from path, parse request body
  2. Fetch existing agent (404 if not found)
  3. Verify squad membership
  4. If status in request: call `domain.ValidateStatusTransition(current, new)`
  5. If role or parentAgentId in request: fetch parent info, fetch existing captain, count children, call `domain.ValidateHierarchyChange`; run `CheckCycleInHierarchy` CTE
  6. Validate input fields via `domain.ValidateUpdateAgentInput`
  7. Build `UpdateAgentParams` with COALESCE/sentinel pattern
  8. Call `queries.UpdateAgent`
  9. Handle constraint violations -> 409
  10. Return 200 with updated agent

**REFACTOR Phase:**
- [ ] Extract hierarchy re-validation into a helper called by both CreateAgent and UpdateAgent
  - Focus: Reduce code duplication between create and update hierarchy checks

**Acceptance Criteria:**
- [ ] Partial updates only change provided fields (COALESCE pattern)
- [ ] Status changes are validated against the state machine
- [ ] Role/parent changes trigger full hierarchy re-validation including orphan check
- [ ] Cycle detection CTE runs as defense-in-depth on parent changes
- [ ] squadId cannot be changed after creation
- [ ] Constraint violations return 409
- [ ] `make test` passes

---

#### Task 12: Implement TransitionAgentStatus endpoint

**Linked Requirements:** REQ-AGT-020, REQ-AGT-073

**RED Phase:**
- [ ] Write handler tests for POST /api/agents/{id}/transition
  - Test case: Valid transition (active->idle) returns 200 with updated agent
  - Test case: Invalid transition (active->running) returns 400 INVALID_STATUS_TRANSITION with message including current and requested status (REQ-AGT-073)
  - Test case: Transition from terminated returns 400 (REQ-AGT-022)
  - Test case: Transition to terminated from any non-terminated status returns 200
  - Test case: Missing status in body returns 400
  - Test case: Non-existent agent returns 404
  - Expected failure: `TransitionAgentStatus` method does not exist

**GREEN Phase:**
- [ ] Implement `(h *AgentHandler) TransitionAgentStatus(w, r)`:
  1. Parse id from path, parse `TransitionRequest` body
  2. Fetch existing agent (404 if not found)
  3. Verify squad membership
  4. Call `domain.ValidateStatusTransition(agent.Status, req.Status)`
  5. Call `queries.UpdateAgent` with only status field
  6. Return 200 with updated agent

**REFACTOR Phase:**
- [ ] Ensure error response format matches REQ-AGT-073: `{"error": "cannot transition from 'active' to 'running'", "code": "INVALID_STATUS_TRANSITION"}`
  - Focus: Error message always includes both current and requested status

**Acceptance Criteria:**
- [ ] Dedicated endpoint provides clear separation from general PATCH
- [ ] Status machine rules are enforced identically to PATCH status updates
- [ ] Error response includes current status, requested status, and INVALID_STATUS_TRANSITION code
- [ ] `make test` passes

---

### Component 7: Integration Tests

#### Task 13: Agent CRUD integration test flow

**Linked Requirements:** REQ-AGT-001, REQ-AGT-002, REQ-AGT-003, REQ-AGT-004, REQ-AGT-030 through REQ-AGT-042

**RED Phase:**
- [ ] Write end-to-end integration test in `internal/server/handlers/agent_handler_test.go` (or `_integration_test.go`)
  - Test flow: Create squad -> Create captain -> Create lead -> Create member -> List agents -> Get agent by ID -> Update agent name -> Verify updated_at changed
  - Test: Created agent has UUID id and timestamps
  - Test: List returns all 3 agents in creation order
  - Test: Get by ID returns correct agent
  - Test: Update changes only specified fields
  - Expected failure: Full flow has not been wired together

**GREEN Phase:**
- [ ] Implement integration test against embedded PostgreSQL
  - Set up test database with migrations applied
  - Create test user and squad with membership
  - Execute full CRUD flow via HTTP test server
  - Assert response status codes and body contents at each step

**REFACTOR Phase:**
- [ ] Extract test helpers for common setup (create user, create squad, auth token)
  - Focus: Test isolation -- each test function gets a clean database state

**Acceptance Criteria:**
- [ ] Full create->list->get->update flow works end-to-end
- [ ] Response bodies match API contracts from design.md
- [ ] Timestamps are correctly set and updated
- [ ] `make test` passes

---

#### Task 14: Status transition and hierarchy enforcement integration tests

**Linked Requirements:** REQ-AGT-014, REQ-AGT-020 through REQ-AGT-023, REQ-AGT-070, REQ-AGT-071, REQ-AGT-072, REQ-AGT-073

**RED Phase:**
- [ ] Write integration tests for status machine enforcement:
  - Test: Create agent (active) -> transition to idle -> transition to running -> transition to error -> transition to terminated (valid chain)
  - Test: Create agent -> attempt idle->active (rejected by state machine)
  - Test: Create agent -> terminate -> attempt transition (rejected, terminal state)
  - Test: Create agent with requireApprovalForNewAgents -> verify pending_approval initial status -> approve -> verify active
- [ ] Write integration tests for hierarchy enforcement:
  - Test: Create captain -> create second captain -> expect 409 CONFLICT
  - Test: Create lead without captain parent -> expect 400
  - Test: Create lead with parent in different squad -> expect 400
  - Test: Create member with captain parent (wrong level) -> expect 400
  - Test: Duplicate shortName in same squad -> expect 409
  - Test: Same shortName in different squad -> expect 201 (allowed)
- [ ] Write integration tests for one-captain partial unique index:
  - Test: Create captain -> terminate captain -> create new captain -> expect 201 (terminated captain excluded from index)

**GREEN Phase:**
- [ ] Implement all integration tests with proper setup/teardown
  - Use the real database with all migrations applied
  - Verify exact HTTP status codes and error response codes

**REFACTOR Phase:**
- [ ] Group related tests using t.Run subtests for clear output
  - Focus: Each subtest is independent and can run in isolation

**Acceptance Criteria:**
- [ ] All valid status transition chains succeed
- [ ] All invalid status transitions return 400 INVALID_STATUS_TRANSITION
- [ ] Terminated is truly terminal (no transitions out)
- [ ] One-captain-per-squad is enforced at both app and DB level
- [ ] Hierarchy rules are enforced for all create and update operations
- [ ] Duplicate shortName within a squad returns 409
- [ ] Same shortName across squads is allowed
- [ ] Terminated captain does not block new captain creation
- [ ] `make test` passes

---

### Final Verification Tasks

#### Task 15: Pre-Merge Checklist

**Final Checks:**

- [ ] All tasks 1-14 completed
- [ ] All tests passing: `make test`
- [ ] No linter errors
- [ ] No type errors
- [ ] Test coverage meets threshold (>= 80% for domain package, >= 70% for handlers)
- [ ] `make sqlc` produces no diff (generated code is committed)
- [ ] Migration tested up and down on clean database
- [ ] Code review completed
- [ ] No debug code or fmt.Println statements
- [ ] No commented-out code
- [ ] All API responses match design.md contracts
- [ ] Error codes match REQ-AGT-070 through REQ-AGT-073 specifications
- [ ] All checkboxes above marked

**Acceptance Criteria:**
- [ ] Feature is production-ready
- [ ] All quality gates passed
- [ ] All REQ-AGT-* requirements have at least one test covering them
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
git add internal/domain/ internal/server/ internal/database/
git commit -m "test: Add failing tests for [functionality]"

# After GREEN phase
git add internal/domain/ internal/server/ internal/database/
git commit -m "feat: Implement [functionality]"

# After REFACTOR phase
git add internal/domain/ internal/server/ internal/database/
git commit -m "refactor: Clean up [component]"
```

## Task Dependency Graph

```
Task 1 (Migration)
  └── Task 2 (sqlc queries)
        └── Task 8 (Handler scaffold)
              ├── Task 9  (Create handler)
              ├── Task 10 (List/Get handlers)
              ├── Task 11 (Update handler)
              └── Task 12 (Transition endpoint)

Task 3 (Domain types) ─── independent of DB tasks
  ├── Task 4 (Input validation)
  ├── Task 5 (Status machine)
  ├── Task 6 (Hierarchy validation)
  └── Task 7 (Hierarchy change validation)

Tasks 9-12 depend on Tasks 3-7 (domain logic) AND Task 2 (sqlc)

Task 13 (CRUD integration) depends on Tasks 9-10
Task 14 (Status/hierarchy integration) depends on Tasks 11-12
Task 15 (Pre-merge) depends on all above
```

## Notes

### Implementation Notes

- Domain types and validation (Tasks 3-7) can be developed in parallel with DB schema (Tasks 1-2) since they have no shared dependencies
- The UpdateAgent sqlc query uses a COALESCE + sentinel boolean pattern -- pay close attention to the `set_parent` and `set_budget` parameters during Task 2. **Important:** COALESCE cannot distinguish "not provided" from "set to NULL" for nullable fields (`adapter_type`, `system_prompt`, `model`). These fields cannot be cleared to NULL via the COALESCE pattern; clearing is not supported in Phase 1. If needed later, migrate to the sentinel boolean pattern (see design.md).
- The partial unique index on captains excludes terminated agents, so a terminated captain does not block a new captain appointment
- Cycle detection via CTE is defense-in-depth only; the strict 3-level role hierarchy makes cycles structurally impossible when role validation passes
- PRD field naming: `short_name` maps to PRD `urlKey`, `parent_agent_id` maps to PRD `reportsTo`. See design.md for full mapping table.
- PRD fields deferred from Phase 1: `title`, `capabilities`, `runtime_config`, `permissions`, `last_heartbeat_at`. These will be added in later migrations when their corresponding features are built.

### Blockers

- [ ] Depends on 03-squad-management providing: squads table, squad_memberships table, squad settings (requireApprovalForNewAgents)
- [ ] Depends on 02-user-auth providing: JWT auth middleware, user_id context extraction

### Future Improvements

- Agent soft-delete (currently using terminated status, not actual deletion)
- Batch agent creation for bootstrapping squads
- Agent configuration templates
- Activity log entries on status transitions (planned for activity-log feature)
- Add deferred PRD fields: `title`, `capabilities`, `runtime_config`, `permissions`, `last_heartbeat_at` (see design.md for deferral rationale)
- Support clearing nullable fields (`adapter_type`, `system_prompt`, `model`) to NULL via sentinel boolean pattern in UpdateAgent query

### Lessons Learned

[Document insights gained during implementation]
