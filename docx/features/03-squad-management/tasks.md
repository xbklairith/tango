# Tasks: Squad Management

**Created:** 2026-03-14
**Status:** Not Started

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Source design: [design.md](./design.md)
- Requirement coverage: REQ-SM-001 through REQ-SM-093 (all mapped below)
- Missing coverage: None (REQ-SM-062/063/064/093 deferred with TODOs pending future features)

## Implementation Approach

Build bottom-up: database schema first (migrations + sqlc codegen), then pure domain types with validation logic (no dependencies), then the squad-scoping middleware, then CRUD handlers (squad, membership, budget), and finally integration tests that exercise the full HTTP stack. Each task follows TDD Red-Green-Refactor. The issue counter and settings JSONB are isolated tasks because they have concurrency and merge semantics that warrant dedicated test coverage.

## Progress Summary

- Total Tasks: 27
- Completed: 0/27
- In Progress: None
- Test Coverage: 0%

---

## Tasks (TDD: Red-Green-Refactor)

### Component 1: Database Schema

#### Task 1.1: Create `squads` Table Migration

**Linked Requirements:** REQ-SM-001, REQ-SM-002, REQ-SM-004, REQ-SM-005, REQ-SM-006, REQ-SM-007, REQ-SM-008, REQ-SM-009

**RED Phase:**
- [ ] Write a Go test that runs goose migrations and asserts the `squads` table exists with all expected columns
  - Test case: Apply migrations, query `information_schema.columns` for table `squads`, verify column names, types, and constraints
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/YYYYMMDDHHMMSS_create_squads.sql`
  - Columns: `id` (UUID PK), `name` (VARCHAR 100 NOT NULL), `slug` (VARCHAR 50 NOT NULL UNIQUE), `issue_prefix` (VARCHAR 10 NOT NULL UNIQUE), `description` (TEXT DEFAULT ''), `status` (VARCHAR 20 CHECK IN active/paused/archived), `settings` (JSONB DEFAULT with requireApprovalForNewAgents=false), `issue_counter` (BIGINT DEFAULT 0), `budget_monthly_cents` (BIGINT CHECK NULL OR > 0), `brand_color` (VARCHAR 7), `created_at` (TIMESTAMPTZ), `updated_at` (TIMESTAMPTZ)
  - Index: `idx_squads_status` on `status`
  - Unique constraints on `slug` and `issue_prefix`

**REFACTOR Phase:**
- [ ] Verify down migration drops table cleanly
- [ ] Confirm migration is idempotent (up + down + up)

**Acceptance Criteria:**
- [ ] Migration applies without errors on a clean database
- [ ] `squads` table has all 12 columns with correct types
- [ ] UNIQUE constraints exist on `slug` and `issue_prefix`
- [ ] CHECK constraint on `status` rejects invalid values
- [ ] CHECK constraint on `budget_monthly_cents` rejects zero/negative
- [ ] `issue_counter` defaults to 0
- [ ] `settings` defaults to `{"requireApprovalForNewAgents": false}`
- [ ] Down migration drops the table

---

#### Task 1.2: Create `squad_memberships` Table Migration

**Linked Requirements:** REQ-SM-020, REQ-SM-021

**RED Phase:**
- [ ] Write a Go test that runs migrations and asserts `squad_memberships` table exists with correct columns and constraints
  - Test case: Query `information_schema.columns` and `pg_indexes` for the table
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create migration file `internal/database/migrations/YYYYMMDDHHMMSS_create_squad_memberships.sql`
  - Columns: `id` (UUID PK), `user_id` (UUID FK to users ON DELETE CASCADE), `squad_id` (UUID FK to squads ON DELETE CASCADE), `role` (VARCHAR 20 CHECK IN owner/admin/viewer DEFAULT viewer), `created_at` (TIMESTAMPTZ), `updated_at` (TIMESTAMPTZ)
  - UNIQUE constraint on `(user_id, squad_id)`
  - Indexes on `squad_id` and `user_id`

**REFACTOR Phase:**
- [ ] Verify FK cascades work (delete squad removes memberships)
- [ ] Confirm down migration drops table

**Acceptance Criteria:**
- [ ] Migration applies without errors
- [ ] `squad_memberships` has all 6 columns with correct types
- [ ] UNIQUE constraint on `(user_id, squad_id)` prevents duplicate memberships
- [ ] FK to `users` and `squads` with CASCADE delete
- [ ] CHECK constraint on `role` rejects invalid values
- [ ] Indexes exist on `squad_id` and `user_id`

---

#### Task 1.3: Write sqlc Queries for Squads

**Linked Requirements:** REQ-SM-010, REQ-SM-012, REQ-SM-013, REQ-SM-014, REQ-SM-017, REQ-SM-040, REQ-SM-042

**RED Phase:**
- [ ] Write Go tests that call each generated sqlc function and verify behavior
  - Test cases: CreateSquad returns all fields, GetSquadByID returns correct row, ListSquadsByUser returns only joined squads, UpdateSquad applies partial updates, SoftDeleteSquad sets status to archived, IncrementIssueCounter atomically increments, CreateSquad with duplicate slug returns unique-violation error, CreateSquad with duplicate issuePrefix returns unique-violation error
  - Expected failure: Generated code does not exist

**GREEN Phase:**
- [ ] Create `internal/database/queries/squads.sql` with all queries:
  - `CreateSquad :one` -- INSERT RETURNING *
  - `GetSquadByID :one` -- SELECT by id
  - `GetSquadBySlug :one` -- SELECT by slug
  - `ListSquadsByUser :many` -- JOIN squad_memberships, filter by user_id, exclude archived, LIMIT/OFFSET
  - `UpdateSquad :one` -- COALESCE pattern for partial updates, CASE for budget nullability
  - `SoftDeleteSquad :one` -- SET status=archived
  - `IncrementIssueCounter :one` -- UPDATE SET issue_counter=issue_counter+1 RETURNING issue_counter, issue_prefix
  - `GetSquadSettings :one` -- SELECT settings
  - NOTE: No `CheckSlugExists` or `CheckIssuePrefixExists` queries — slug and issuePrefix uniqueness is enforced by UNIQUE constraints, with PG error 23505 mapped to 409 responses (see design Section 10.3)
- [ ] Run `make sqlc` to generate Go code

**REFACTOR Phase:**
- [ ] Verify all generated types are correct
- [ ] Ensure nullable fields map to pointer types in Go

**Acceptance Criteria:**
- [ ] `make sqlc` succeeds with no errors
- [ ] All 8 query functions are generated in `internal/database/db/`
- [ ] ListSquadsByUser includes user role from the join
- [ ] UpdateSquad supports partial updates (NULL args skip columns)
- [ ] IncrementIssueCounter returns both counter and prefix

---

#### Task 1.4: Write sqlc Queries for Squad Memberships

**Linked Requirements:** REQ-SM-020, REQ-SM-022, REQ-SM-024, REQ-SM-025, REQ-SM-027, REQ-SM-028

**RED Phase:**
- [ ] Write Go tests for each membership query function
  - Test cases: CreateSquadMembership, GetSquadMembership (by user+squad), GetSquadMembershipByID, ListSquadMembers (with user email/displayName), UpdateSquadMembershipRole, DeleteSquadMembership, CountSquadOwners, DeleteSquadMembershipByUserAndSquad
  - Expected failure: Generated code does not exist

**GREEN Phase:**
- [ ] Create `internal/database/queries/squad_memberships.sql` with all queries:
  - `CreateSquadMembership :one` -- INSERT RETURNING *
  - `GetSquadMembership :one` -- WHERE user_id AND squad_id
  - `GetSquadMembershipByID :one` -- WHERE id AND squad_id
  - `ListSquadMembers :many` -- JOIN users for email/display_name, WHERE squad_id
  - `UpdateSquadMembershipRole :one` -- SET role, updated_at
  - `DeleteSquadMembership :exec` -- WHERE id AND squad_id
  - `CountSquadOwners :one` -- COUNT WHERE role=owner
  - `DeleteSquadMembershipIfNotLastOwner :execrows` -- Atomic delete that returns 0 rows if member is last owner (prevents TOCTOU race)
  - `DemoteOwnerIfNotLast :execrows` -- Atomic role update that returns 0 rows if member is last owner (prevents TOCTOU race)
  - `DeleteSquadMembershipByUserAndSquad :exec` -- WHERE user_id AND squad_id
  - `DeleteSquadMembershipByUserIfNotLastOwner :execrows` -- Atomic leave that returns 0 rows if user is last owner
- [ ] Run `make sqlc`

**REFACTOR Phase:**
- [ ] Verify ListSquadMembers includes user info from the JOIN
- [ ] Confirm delete queries scope to squad_id for safety

**Acceptance Criteria:**
- [ ] `make sqlc` succeeds
- [ ] All 11 membership query functions are generated
- [ ] ListSquadMembers returns email and display_name from users table
- [ ] CountSquadOwners returns integer count

---

### Component 2: Domain Types

#### Task 2.1: Define Squad Enums and Validation Functions

**Linked Requirements:** REQ-SM-001, REQ-SM-003, REQ-SM-004, REQ-SM-005, REQ-SM-009, REQ-SM-083

**RED Phase:**
- [ ] Write unit tests in `internal/domain/squad_test.go`:
  - `TestSquadStatus_Valid` -- active/paused/archived return true, "invalid" returns false
  - `TestSquadStatus_ValidTransition` -- active->paused OK, active->archived OK, paused->active OK, paused->archived OK, archived->active FAIL, archived->paused FAIL
  - `TestMemberRole_Valid` -- owner/admin/viewer return true, "invalid" returns false
  - `TestMemberRole_CanManageMembers` -- owner/admin true, viewer false
  - `TestMemberRole_CanEditSquad` -- owner/admin true, viewer false
  - `TestMemberRole_CanGrantRole` -- owner can grant all, admin can grant viewer only, viewer cannot grant
  - `TestValidateSquadName` -- empty fails, 101 chars fails, "Valid Name" passes
  - `TestValidateSquadSlug` -- "a" fails (too short), 51 chars fails, "my-squad" passes, "MY_SQUAD" fails
  - `TestValidateIssuePrefix` -- "A" fails (too short), "ABCDEFGHIJK" fails (11 chars), "ACME" passes, "acme" fails
  - `TestValidateBudget` -- nil passes, positive passes, zero fails, negative fails
  - `TestValidateBrandColor` -- nil passes, "#3b82f6" passes, "red" fails, "#GGG" fails
  - Expected failure: Functions do not exist

**GREEN Phase:**
- [ ] Implement in `internal/domain/squad.go`:
  - `SquadStatus` type with constants and `Valid()`, `ValidTransition()` methods
  - `MemberRole` type with constants and `Valid()`, `CanManageMembers()`, `CanEditSquad()`, `CanGrantRole()` methods
  - `ValidateSquadName()`, `ValidateSquadSlug()`, `ValidateIssuePrefix()`, `ValidateBudget()`, `ValidateBrandColor()`
  - `ValidationError` struct implementing `error` interface

**REFACTOR Phase:**
- [ ] Extract regex patterns to package-level compiled variables
- [ ] Ensure error messages are user-friendly

**Acceptance Criteria:**
- [ ] All validation functions return `ValidationError` with field name and message
- [ ] Status transitions match the state machine: active <-> paused, (active|paused) -> archived, archived is terminal
- [ ] Role permission methods match the RBAC matrix from design
- [ ] All unit tests pass

---

#### Task 2.2: Define Squad and SquadMembership Structs

**Linked Requirements:** REQ-SM-001, REQ-SM-020

**RED Phase:**
- [ ] Write tests that create Squad and SquadMembership structs, verify JSON marshalling produces correct camelCase keys
  - Test case: Marshal a Squad to JSON, verify keys `id`, `name`, `slug`, `issuePrefix`, `description`, `status`, `settings`, `issueCounter`, `budgetMonthlyCents`, `brandColor`, `createdAt`, `updatedAt`
  - Test case: Marshal a SquadWithRole, verify `role` key is present
  - Test case: Marshal with nil `budgetMonthlyCents` outputs `null`, nil `brandColor` omits field
  - Expected failure: Structs do not exist

**GREEN Phase:**
- [ ] Implement `Squad`, `SquadWithRole`, and `SquadMembership` structs in `internal/domain/squad.go`
  - Use `json` struct tags with camelCase
  - `BudgetMonthlyCents *int64` (nil = unlimited, serialized as null)
  - `BrandColor *string` with `omitempty`

**REFACTOR Phase:**
- [ ] Ensure consistent naming between Go fields and JSON keys
- [ ] Verify struct tags match API contract from design doc section 5

**Acceptance Criteria:**
- [ ] Squad struct has all 12 fields from REQ-SM-001
- [ ] SquadMembership struct has all 6 fields from REQ-SM-020
- [ ] JSON serialization matches API contract
- [ ] Nil budget serializes as JSON `null`
- [ ] Nil brandColor is omitted from JSON

---

#### Task 2.3: Implement SquadSettings and Slug Generation

**Linked Requirements:** REQ-SM-002, REQ-SM-007, REQ-SM-050, REQ-SM-051, REQ-SM-052

**RED Phase:**
- [ ] Write unit tests:
  - `TestDefaultSquadSettings` -- returns `requireApprovalForNewAgents: false`
  - `TestSquadSettings_Merge` -- nil fields in patch are skipped, non-nil fields overwrite
  - `TestValidateSettingsKeys` -- known key passes, unknown key returns error
  - `TestGenerateSlug` -- "My Squad Name!" -> "my-squad-name", "  spaces  " -> "spaces", "A" -> "a-squad" (minimum length pad), "UPPER-case" -> "upper-case"
  - Expected failure: Functions do not exist

**GREEN Phase:**
- [ ] Implement `SquadSettings` struct with pointer fields for partial update support
- [ ] Implement `DefaultSquadSettings()` returning defaults
- [ ] Implement `Merge(patch)` method -- shallow merge, nil fields skipped
- [ ] Implement `ValidateSettingsKeys(raw map[string]interface{})` against `knownSettingsKeys` map
- [ ] Implement `GenerateSlug(name string) string` -- lowercase, replace non-alphanum with hyphens, trim, pad if < 2 chars

**REFACTOR Phase:**
- [ ] Ensure `knownSettingsKeys` is easy to extend for future settings
- [ ] Verify slug generation handles unicode and edge cases

**Acceptance Criteria:**
- [ ] Default settings have all known keys with their default values
- [ ] Merge only overwrites non-nil fields (partial update)
- [ ] Unknown settings keys are rejected with a clear error message
- [ ] Slug generation produces URL-safe, lowercase, hyphenated strings
- [ ] Slugs shorter than 2 characters get padded with "-squad"

---

### Component 3: Squad CRUD Handler

#### Task 3.1: Implement Create Squad Handler (POST /api/squads)

**Linked Requirements:** REQ-SM-010, REQ-SM-011, REQ-SM-002, REQ-SM-003, REQ-SM-005, REQ-SM-006, REQ-SM-007, REQ-SM-070, REQ-SM-072, REQ-SM-080, REQ-SM-081, REQ-SM-084

**RED Phase:**
- [ ] Write HTTP test in `internal/server/handlers/squad_handler_test.go`:
  - Test: Valid request returns 201 with full squad JSON including all fields
  - Test: Response includes auto-generated slug from name
  - Test: Squad is created with `status=active`, `issueCounter=0`
  - Test: Owner membership is automatically created for the requesting user
  - Test: Missing name returns 400 VALIDATION_ERROR
  - Test: Invalid issuePrefix returns 400 VALIDATION_ERROR
  - Test: Duplicate slug returns 409 SLUG_TAKEN
  - Test: Duplicate issuePrefix returns 409 ISSUE_PREFIX_TAKEN
  - Test: Custom settings are merged with defaults
  - Test: Budget of 0 returns 400 VALIDATION_ERROR
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `SquadHandler.Create` method:
  1. Decode JSON body into `CreateSquadRequest`
  2. Validate name, issuePrefix, budget, brandColor, settings keys
  3. Generate slug from name
  4. Begin transaction
  5. Insert squad with defaults (status=active, issueCounter=0, merged settings). Do NOT pre-check slug or issuePrefix uniqueness — rely on UNIQUE constraints. Handle PG error 23505: map constraint name to 409 SLUG_TAKEN or ISSUE_PREFIX_TAKEN. On slug collision, retry with numeric suffix (`-2`, `-3`).
  6. Insert owner membership for authenticated user
  7. Commit transaction
  8. Return 201 with created squad

**REFACTOR Phase:**
- [ ] Extract transaction logic into a helper
- [ ] Ensure all error paths return proper JSON error format

**Acceptance Criteria:**
- [ ] POST /api/squads with valid payload returns 201 with complete squad object
- [ ] Slug is auto-generated from name and is unique
- [ ] Owner membership is created atomically with the squad (same transaction)
- [ ] All validation errors return 400 with VALIDATION_ERROR code
- [ ] Duplicate slug returns 409 SLUG_TAKEN
- [ ] Duplicate issuePrefix returns 409 ISSUE_PREFIX_TAKEN
- [ ] Response Content-Type is application/json

---

#### Task 3.2: Implement List Squads Handler (GET /api/squads)

**Linked Requirements:** REQ-SM-012, REQ-SM-070, REQ-SM-073, REQ-SM-091

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Returns only squads where user has a membership
  - Test: Does not return squads the user is not a member of
  - Test: Does not return archived squads
  - Test: Each squad object includes the user's `role`
  - Test: Default pagination is limit=50, offset=0
  - Test: Custom limit/offset are respected
  - Test: Limit is capped at 100
  - Test: Empty list returns `[]`, not null
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `SquadHandler.List` method:
  1. Extract user ID from auth context
  2. Parse `limit` and `offset` query params (defaults: 50, 0; max limit: 100)
  3. Call `ListSquadsByUser` query
  4. Map results to `[]SquadWithRole`
  5. Return 200 with JSON array

**REFACTOR Phase:**
- [ ] Extract pagination parsing into a reusable helper
- [ ] Ensure empty results return `[]` not `null`

**Acceptance Criteria:**
- [ ] Only squads where user has membership are returned
- [ ] Archived squads are excluded
- [ ] Each squad includes the user's role
- [ ] Pagination defaults to limit=50, offset=0
- [ ] Limit cannot exceed 100
- [ ] Response is a JSON array (empty array for no results)

---

#### Task 3.3: Implement Get Squad Handler (GET /api/squads/:id)

**Linked Requirements:** REQ-SM-013, REQ-SM-031, REQ-SM-070, REQ-SM-082

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Returns squad details when user has membership, status 200
  - Test: Returns 404 SQUAD_NOT_FOUND when user has no membership (not 403)
  - Test: Returns 404 for non-existent squad ID
  - Test: Returns 404 for invalid UUID format
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `SquadHandler.Get` method:
  1. Parse squad ID from URL path
  2. Verify user membership via context (set by middleware) or query
  3. Fetch squad by ID
  4. Return 200 with squad JSON

**REFACTOR Phase:**
- [ ] Ensure 404 is used consistently (never 403) to prevent leaking squad existence

**Acceptance Criteria:**
- [ ] Returns squad details with 200 for members
- [ ] Returns 404 SQUAD_NOT_FOUND for non-members (not 403)
- [ ] Returns 404 for invalid/non-existent IDs
- [ ] Response matches API contract from design section 5.3

---

#### Task 3.4: Implement Update Squad Handler (PATCH /api/squads/:id)

**Linked Requirements:** REQ-SM-014, REQ-SM-015, REQ-SM-051, REQ-SM-052, REQ-SM-070, REQ-SM-083, REQ-SM-084

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Owner can update name, description, status, settings, brandColor -- returns 200
  - Test: Admin can update -- returns 200
  - Test: Viewer gets 403 FORBIDDEN
  - Test: Partial update (only name) leaves other fields unchanged
  - Test: Settings merge: sending `{"requireApprovalForNewAgents": true}` merges, does not replace
  - Test: Unknown settings key returns 400 VALIDATION_ERROR
  - Test: Invalid status transition (archived -> active) returns 400 INVALID_STATUS_TRANSITION
  - Test: Name update regenerates slug
  - Test: Invalid name (empty, > 100 chars) returns 400
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `SquadHandler.Update` method:
  1. Parse squad ID, extract membership from context
  2. Check role is owner or admin (else 403)
  3. Decode `UpdateSquadRequest`
  4. Validate provided fields (name, status transition, settings keys, brandColor)
  5. If settings provided, fetch current settings and merge
  6. If name changed, regenerate slug and check uniqueness
  7. Call UpdateSquad query with COALESCE pattern
  8. Return 200 with updated squad

**REFACTOR Phase:**
- [ ] Extract settings merge into a dedicated helper method
- [ ] Ensure status transition validation is centralized in domain layer

**Acceptance Criteria:**
- [ ] Owner and admin can update; viewer gets 403
- [ ] Partial updates work (unchanged fields preserved)
- [ ] Settings are merged, not replaced
- [ ] Unknown settings keys are rejected with 400
- [ ] Invalid status transitions return 400 INVALID_STATUS_TRANSITION
- [ ] Name change regenerates slug

---

#### Task 3.5: Implement Delete Squad Handler (DELETE /api/squads/:id)

**Linked Requirements:** REQ-SM-016, REQ-SM-017

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Owner can delete a squad with no active agents or unresolved issues -- returns 200 with archived squad
  - Test: Non-owner gets 403 FORBIDDEN
  - Test: Squad with active agents returns 409 SQUAD_HAS_ACTIVE_RESOURCES
  - Test: Squad with unresolved issues returns 409 SQUAD_HAS_ACTIVE_RESOURCES
  - Test: Squad is soft-deleted (status set to archived, not physically removed)
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `SquadHandler.Delete` method:
  1. Parse squad ID, extract membership from context
  2. Check role is owner (else 403)
  3. Check for active agents in squad (query count)
  4. Check for unresolved issues in squad (query count)
  5. If either > 0, return 409 with SQUAD_HAS_ACTIVE_RESOURCES
  6. Call SoftDeleteSquad (sets status=archived)
  7. Return 200 with archived squad

**REFACTOR Phase:**
- [ ] Consider whether active-resource checks need to be in a transaction
- [ ] Ensure soft-delete is the only deletion path (no hard delete)

**Acceptance Criteria:**
- [ ] Only owner can delete
- [ ] Deletion is a soft-delete (status -> archived)
- [ ] Cannot delete squad with active agents (409)
- [ ] Cannot delete squad with unresolved issues (409)
- [ ] Returns the archived squad object on success

**Notes:**
- Active agents and unresolved issues checks depend on those tables existing. Initially, stub the checks to return 0 until agent/issue features are implemented. Add a TODO comment referencing the future features (04-agent-management, 05-issue-tracking).

---

### Component 4: Membership Handler

#### Task 4.1: Implement Add Member Handler (POST /api/squads/:id/members)

**Linked Requirements:** REQ-SM-024, REQ-SM-026, REQ-SM-021, REQ-SM-084

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Owner adds a viewer member -- returns 201 with membership
  - Test: Admin adds a viewer member -- returns 201
  - Test: Owner grants admin role -- returns 201
  - Test: Admin attempts to grant owner role -- returns 403 (only owners can grant owner/admin)
  - Test: Viewer attempts to add member -- returns 403
  - Test: Adding already-existing member returns 409 MEMBER_EXISTS
  - Test: Missing userId returns 400 VALIDATION_ERROR
  - Test: Invalid role returns 400 VALIDATION_ERROR
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `MembershipHandler.Add` method:
  1. Parse squad ID, extract actor's membership from context
  2. Check actor role can manage members (owner or admin)
  3. Decode request (userId, role)
  4. Check actor role can grant the target role (`CanGrantRole`)
  5. Create membership
  6. Handle unique constraint violation -> 409 MEMBER_EXISTS
  7. Return 201 with created membership

**REFACTOR Phase:**
- [ ] Extract permission checks into a helper
- [ ] Ensure error messages clearly indicate why the operation failed

**Acceptance Criteria:**
- [ ] Owner/admin can add members
- [ ] Only owner can grant owner or admin roles
- [ ] Duplicate membership returns 409
- [ ] Missing/invalid fields return 400
- [ ] Response includes full membership object with 201

---

#### Task 4.2: Implement List Members Handler (GET /api/squads/:id/members)

**Linked Requirements:** REQ-SM-024 (implicit list), REQ-SM-070

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Returns all members with role, email, displayName
  - Test: Members are ordered by createdAt ASC
  - Test: Non-member gets 404 (enforced by squad-scope middleware)
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `MembershipHandler.List` method:
  1. Extract squad ID from context
  2. Call ListSquadMembers query (JOINs users table)
  3. Map to response objects with email and displayName
  4. Return 200 with JSON array

**REFACTOR Phase:**
- [ ] Ensure empty result returns `[]` not null

**Acceptance Criteria:**
- [ ] Returns all squad members with role, email, displayName
- [ ] Ordered by creation time ascending
- [ ] Response is JSON array

---

#### Task 4.3: Implement Update Member Role Handler (PATCH /api/squads/:id/members/:memberId)

**Linked Requirements:** REQ-SM-025, REQ-SM-026, REQ-SM-022, REQ-SM-023

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Owner changes member role from viewer to admin -- returns 200
  - Test: Non-owner gets 403
  - Test: Demoting the last owner returns 409 LAST_OWNER
  - Test: Owner can promote another member to owner
  - Test: Admin cannot change roles (only owners can per REQ-SM-025)
  - Test: Invalid role returns 400
  - Test: Non-existent memberId returns 404
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `MembershipHandler.UpdateRole` method:
  1. Parse squad ID and member ID, extract actor's membership
  2. Check actor is owner (else 403)
  3. Decode request (role)
  4. Validate role
  5. If target member is currently owner and new role is not owner, use `DemoteOwnerIfNotLast` (atomic: checks count and updates in one query, no TOCTOU race). If 0 rows affected, return 409 LAST_OWNER.
  6. Otherwise, use `UpdateSquadMembershipRole` for non-owner-demotion cases
  7. Return 200 with updated membership

**REFACTOR Phase:**
- [ ] Verify atomic query handles concurrent demotion requests safely

**Acceptance Criteria:**
- [ ] Only owners can update roles
- [ ] Last owner cannot be demoted (409 LAST_OWNER)
- [ ] Valid role changes return 200 with updated membership
- [ ] Invalid role returns 400

---

#### Task 4.4: Implement Remove Member Handler (DELETE /api/squads/:id/members/:memberId)

**Linked Requirements:** REQ-SM-027, REQ-SM-022, REQ-SM-023

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Owner removes a member -- returns 200
  - Test: Non-owner gets 403
  - Test: Removing the last owner returns 409 LAST_OWNER
  - Test: Removing non-existent member returns 404
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `MembershipHandler.Remove` method:
  1. Parse squad ID and member ID, extract actor's membership
  2. Check actor is owner (else 403)
  3. Use `DeleteSquadMembershipIfNotLastOwner` (atomic: checks owner count and deletes in one query, no TOCTOU race). If 0 rows affected, return 409 LAST_OWNER.
  4. Return 200 with `{"message": "Member removed"}`

**REFACTOR Phase:**
- [ ] Verify atomic query handles concurrent removal requests safely

**Acceptance Criteria:**
- [ ] Only owners can remove members
- [ ] Last owner cannot be removed (409 LAST_OWNER)
- [ ] Successful removal returns 200 with confirmation message

---

#### Task 4.5: Implement Leave Squad Handler (DELETE /api/squads/:id/members/me)

**Linked Requirements:** REQ-SM-028, REQ-SM-022, REQ-SM-023

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Member leaves squad -- returns 200
  - Test: Last owner cannot leave -- returns 409 LAST_OWNER
  - Test: After leaving, user can no longer access squad (404)
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `MembershipHandler.Leave` method:
  1. Extract squad ID and user ID from context
  2. Use `DeleteSquadMembershipByUserIfNotLastOwner` (atomic: checks owner count and deletes in one query, no TOCTOU race). If 0 rows affected, return 409 LAST_OWNER.
  3. Return 200 with `{"message": "You have left the squad"}`

**REFACTOR Phase:**
- [ ] Ensure route `/members/me` does not conflict with `/members/{memberId}` in routing

**Acceptance Criteria:**
- [ ] Any member can leave their squad
- [ ] Last owner cannot leave (409 LAST_OWNER)
- [ ] After leaving, the user cannot access the squad

**Notes:**
- The route `DELETE /api/squads/{id}/members/me` must be registered before `DELETE /api/squads/{id}/members/{memberId}` to avoid `me` being parsed as a memberId. Verify Go 1.22+ `net/http` mux handles literal path segments before wildcards correctly.

---

### Component 5: Squad-Scoped Middleware

#### Task 5.1: Implement RequireSquadMembership Middleware

**Linked Requirements:** REQ-SM-030, REQ-SM-031, REQ-SM-032, REQ-SM-033

**RED Phase:**
- [ ] Write HTTP tests in `internal/server/middleware/squad_scope_test.go`:
  - Test: Request with valid squad ID and active membership passes through, context contains squad ID and membership
  - Test: Request with valid squad ID but no membership returns 404 (not 403)
  - Test: Request with invalid UUID returns 404
  - Test: Request with non-existent squad ID returns 404
  - Test: Context accessors `SquadFromContext` and `MembershipFromContext` return correct values
  - Expected failure: Middleware does not exist

**GREEN Phase:**
- [ ] Implement `RequireSquadMembership` middleware in `internal/server/middleware/squad_scope.go`:
  1. Extract `{id}` from URL path value
  2. Parse as UUID (404 if invalid)
  3. Extract user ID from auth context
  4. Query `GetSquadMembership(user_id, squad_id)`
  5. If not found, return 404 SQUAD_NOT_FOUND
  6. Inject squad ID and membership into context
  7. Call next handler
- [ ] Implement `SquadFromContext()` and `MembershipFromContext()` typed context accessors

**REFACTOR Phase:**
- [ ] Ensure context key types are unexported (prevent external collision)
- [ ] Verify middleware integrates with auth middleware's user ID context

**Acceptance Criteria:**
- [ ] Non-members get 404 (not 403) to prevent leaking squad existence
- [ ] Squad ID and membership are available in downstream handler context
- [ ] Invalid UUID format returns 404
- [ ] Middleware chains correctly with auth middleware

---

#### Task 5.2: Register Squad-Scoped Routes with Middleware

**Linked Requirements:** REQ-SM-030, REQ-SM-032

**RED Phase:**
- [ ] Write integration test that verifies all squad-scoped routes (members, future agents/issues) pass through the membership middleware
  - Test: Unauthenticated request to `/api/squads/{id}/members` returns 401 (auth middleware)
  - Test: Authenticated but non-member request to `/api/squads/{id}/members` returns 404 (squad middleware)
  - Expected failure: Route registration does not exist

**GREEN Phase:**
- [ ] Implement `RegisterSquadScopedRoutes` function in `internal/server/routes.go` (or similar):
  1. Create squad middleware instance
  2. Register squad handler routes (POST/GET /api/squads are not squad-scoped)
  3. Wrap GET/PATCH/DELETE /api/squads/{id} with squad membership middleware
  4. Wrap all /api/squads/{id}/members/* routes with squad membership middleware
  5. Wrap PATCH /api/squads/{id}/budgets with squad membership middleware

**REFACTOR Phase:**
- [ ] Ensure route registration is declarative and easy to extend
- [ ] Document which routes are squad-scoped vs. not

**Acceptance Criteria:**
- [ ] POST /api/squads does NOT require squad membership (user is creating a new squad)
- [ ] GET /api/squads does NOT require squad membership (filtered by query)
- [ ] All /api/squads/{id}/* routes require squad membership
- [ ] Middleware chain: auth -> squad-scope -> handler

---

### Component 6: Issue Counter

#### Task 6.1: Implement Atomic Issue Counter Increment

**Linked Requirements:** REQ-SM-040, REQ-SM-041, REQ-SM-042, REQ-SM-092

**RED Phase:**
- [ ] Write tests in a dedicated service test file:
  - Test: `IncrementIssueCounter` returns counter=1 and correct prefix on first call
  - Test: Sequential calls return 1, 2, 3, ...
  - Test: Returned identifier format is "{prefix}-{counter}" (e.g., "ACME-1")
  - Test: Counter starts at 0, first issue gets identifier with counter=1
  - Expected failure: Service function does not exist

**GREEN Phase:**
- [ ] Implement `NextIssueIdentifier(ctx, squadID) (string, error)` in the service layer:
  1. Call `IncrementIssueCounter` sqlc query (UPDATE ... SET issue_counter=issue_counter+1 RETURNING)
  2. Format result as `"{issue_prefix}-{issue_counter}"`
  3. Return the identifier string

**REFACTOR Phase:**
- [ ] Ensure the function is usable from future issue-creation handlers
- [ ] Add documentation about the row-level lock behavior

**Acceptance Criteria:**
- [ ] Counter increments atomically via UPDATE...RETURNING
- [ ] Identifier format is "{prefix}-{counter}"
- [ ] No explicit SELECT FOR UPDATE needed (UPDATE implicitly locks)
- [ ] Function is safe for concurrent callers (tested in Task 9.3)

---

### Component 7: Settings JSONB

#### Task 7.1: Implement Settings Partial Merge and Validation in Handler

**Linked Requirements:** REQ-SM-050, REQ-SM-051, REQ-SM-052

**RED Phase:**
- [ ] Write HTTP-level tests:
  - Test: PATCH /api/squads/:id with `{"settings": {"requireApprovalForNewAgents": true}}` merges into existing settings, returns updated squad with merged settings
  - Test: Existing settings keys not in patch are preserved
  - Test: Unknown settings key `{"settings": {"unknownKey": true}}` returns 400 VALIDATION_ERROR
  - Test: Settings validation happens before database write
  - Expected failure: Merge logic not wired into handler

**GREEN Phase:**
- [ ] Wire settings merge into the Update handler:
  1. If request includes `settings`, first validate raw JSON keys against known schema
  2. Fetch current settings from DB (`GetSquadSettings`)
  3. Unmarshal current into `SquadSettings`
  4. Merge patch onto current
  5. Marshal merged settings back to JSON for the update query

**REFACTOR Phase:**
- [ ] Ensure the merge is idempotent
- [ ] Verify edge case: empty settings object `{}` is a no-op

**Acceptance Criteria:**
- [ ] Settings are merged (partial update), not replaced
- [ ] Unknown keys return 400 before any DB write
- [ ] Known keys are updated, unmentioned keys are preserved
- [ ] Empty settings object `{}` is a valid no-op

---

### Component 8: Budget Fields

#### Task 8.1: Implement Budget Update Handler (PATCH /api/squads/:id/budgets)

**Linked Requirements:** REQ-SM-060, REQ-SM-061, REQ-SM-062, REQ-SM-065, REQ-SM-008, REQ-SM-009

**RED Phase:**
- [ ] Write HTTP tests:
  - Test: Owner sets budget to positive integer -- returns 200 with updated squad
  - Test: Owner sets budget to null (unlimited) -- returns 200, budgetMonthlyCents is null
  - Test: Admin gets 403 FORBIDDEN
  - Test: Viewer gets 403 FORBIDDEN
  - Test: Budget of 0 returns 400 VALIDATION_ERROR
  - Test: Negative budget returns 400 VALIDATION_ERROR
  - Expected failure: Handler does not exist

**GREEN Phase:**
- [ ] Implement `SquadHandler.UpdateBudget` method:
  1. Parse squad ID, extract membership from context
  2. Check role is owner (else 403)
  3. Decode `UpdateBudgetRequest` (budgetMonthlyCents can be positive integer or null)
  4. Validate: if not null, must be > 0
  5. Update squad's budget_monthly_cents via UpdateSquad query (use the `update_budget` flag)
  6. Return 200 with updated squad

**REFACTOR Phase:**
- [ ] Ensure null handling is explicit (distinguish between "not provided" and "set to null")

**Acceptance Criteria:**
- [ ] Only owner can update budget
- [ ] Positive integer sets the budget
- [ ] Null value sets budget to unlimited
- [ ] Zero and negative values return 400
- [ ] Admin and viewer get 403

**Notes:**
- Budget enforcement (80% warning at REQ-SM-063, 100% hard stop at REQ-SM-064) depends on CostEvents table from a future feature. The spend computation (REQ-SM-062) requires SUM of cost_events. Add TODO comments referencing those requirements. The budget field itself is fully functional in this task.

---

### Component 9: Integration Tests

#### Task 9.1: Full Squad CRUD Integration Test

**Linked Requirements:** REQ-SM-010, REQ-SM-011, REQ-SM-012, REQ-SM-013, REQ-SM-014, REQ-SM-015, REQ-SM-016, REQ-SM-017, REQ-SM-070, REQ-SM-072, REQ-SM-073, REQ-SM-090, REQ-SM-093

**RED Phase:**
- [ ] Write end-to-end test that exercises the full lifecycle against a real embedded PostgreSQL:
  1. Create user (from 02-user-auth)
  2. POST /api/squads -- create squad, verify 201 + all fields + owner membership auto-created
  3. GET /api/squads -- verify squad appears in list with role=owner
  4. GET /api/squads/:id -- verify full squad details
  5. PATCH /api/squads/:id -- update name, verify slug regenerated, verify 200
  6. PATCH /api/squads/:id -- update settings with partial merge, verify merge
  7. PATCH /api/squads/:id -- invalid status transition, verify 400
  8. DELETE /api/squads/:id -- verify soft-delete (status=archived), verify 200
  9. GET /api/squads -- verify archived squad no longer in list
  - Expected failure: Full flow not wired

**GREEN Phase:**
- [ ] Ensure all handlers are registered, middleware is chained, database is migrated
- [ ] Fix any integration issues discovered

**REFACTOR Phase:**
- [ ] Extract test helpers (createTestUser, createTestSquad, authenticatedRequest)
- [ ] Ensure test database is cleaned between test runs

**Acceptance Criteria:**
- [ ] Full CRUD lifecycle passes end-to-end
- [ ] All responses match expected status codes and JSON shapes
- [ ] Owner membership is verified at each step
- [ ] Archived squad is excluded from list
- [ ] Test runs against real PostgreSQL (embedded-postgres-go)

---

#### Task 9.2: Full Membership Management Integration Test

**Linked Requirements:** REQ-SM-021, REQ-SM-022, REQ-SM-023, REQ-SM-024, REQ-SM-025, REQ-SM-026, REQ-SM-027, REQ-SM-028, REQ-SM-031

**RED Phase:**
- [ ] Write end-to-end test:
  1. Create squad (user A is owner)
  2. Create user B and user C
  3. POST /api/squads/:id/members -- owner A adds user B as admin (201)
  4. POST /api/squads/:id/members -- admin B adds user C as viewer (201)
  5. POST /api/squads/:id/members -- admin B tries to add user as owner (403)
  6. GET /api/squads/:id/members -- verify 3 members with correct roles
  7. PATCH /api/squads/:id/members/:B -- owner A promotes B to owner (200)
  8. PATCH /api/squads/:id/members/:A -- owner B demotes A to viewer (200, since B is now also owner)
  9. DELETE /api/squads/:id/members/me -- viewer A leaves (200)
  10. DELETE /api/squads/:id/members/:B -- owner B tries to remove self as last owner (409 LAST_OWNER)
  11. Verify user A can no longer access squad (404)
  - Expected failure: Full membership flow not wired

**GREEN Phase:**
- [ ] Fix any integration issues in the membership handler chain
- [ ] Ensure last-owner protection works across all operations

**REFACTOR Phase:**
- [ ] Ensure test is deterministic and does not depend on execution order

**Acceptance Criteria:**
- [ ] Full membership lifecycle passes (add, list, update role, remove, leave)
- [ ] RBAC is enforced at every step
- [ ] Last-owner protection works for role change, removal, and leave
- [ ] Non-member gets 404 after leaving
- [ ] Duplicate membership returns 409

---

#### Task 9.3: Concurrent Issue Counter Test

**Linked Requirements:** REQ-SM-040, REQ-SM-041, REQ-SM-042, REQ-SM-092

**RED Phase:**
- [ ] Write a concurrency test:
  - Spawn 20 goroutines, each calling `NextIssueIdentifier` for the same squad
  - Collect all returned identifiers
  - Assert: all 20 identifiers are unique
  - Assert: identifiers are "PREFIX-1" through "PREFIX-20" (no gaps, no duplicates)
  - Assert: final issue_counter on the squad row is 20
  - Expected failure: Concurrency issues or duplicate identifiers

**GREEN Phase:**
- [ ] Verify the `UPDATE ... RETURNING` pattern provides sufficient row-level locking
- [ ] Fix any race conditions if discovered

**REFACTOR Phase:**
- [ ] Add a timeout to the test (e.g., 10 seconds) to catch deadlocks
- [ ] Consider increasing goroutine count to stress test

**Acceptance Criteria:**
- [ ] 20 concurrent goroutines produce 20 unique identifiers
- [ ] No duplicate counters under concurrent access
- [ ] No deadlocks or timeouts
- [ ] Final counter value matches the number of increments
- [ ] Test completes within a reasonable time (< 10s)

---

#### Task 9.4: Data Isolation Integration Test

**Linked Requirements:** REQ-SM-030, REQ-SM-031, REQ-SM-032, REQ-SM-033

**RED Phase:**
- [ ] Write integration test:
  1. Create two squads (Squad A with user X, Squad B with user Y)
  2. User X requests GET /api/squads/:squadB_id -- verify 404
  3. User Y requests GET /api/squads/:squadA_id -- verify 404
  4. User X requests GET /api/squads -- verify only Squad A returned
  5. User Y requests GET /api/squads -- verify only Squad B returned
  6. (Future) When agents/issues exist: verify queries always include squad_id filter
  - Expected failure: Data leaks across squads

**GREEN Phase:**
- [ ] Ensure all queries are squad-scoped
- [ ] Ensure middleware returns 404 for non-members

**REFACTOR Phase:**
- [ ] Add negative test: attempt to access Squad B's members as user X

**Acceptance Criteria:**
- [ ] Users can only see squads they belong to
- [ ] Cross-squad access returns 404, never data
- [ ] List endpoints only return data from the user's squads
- [ ] No unscoped queries exist in the codebase

---

#### Task 9.5: Budget and Settings Integration Test

**Linked Requirements:** REQ-SM-008, REQ-SM-009, REQ-SM-050, REQ-SM-051, REQ-SM-052, REQ-SM-060, REQ-SM-061, REQ-SM-065

**RED Phase:**
- [ ] Write integration test:
  1. Create squad with no budget (null) -- verify budgetMonthlyCents is null
  2. PATCH /api/squads/:id/budgets with 500000 -- verify budget is set
  3. PATCH /api/squads/:id/budgets with null -- verify budget is unlimited again
  4. PATCH /api/squads/:id/budgets with 0 -- verify 400
  5. PATCH /api/squads/:id with `{"settings": {"requireApprovalForNewAgents": true}}` -- verify merge
  6. PATCH /api/squads/:id with `{"settings": {}}` -- verify no-op (existing settings preserved)
  7. PATCH /api/squads/:id with `{"settings": {"badKey": true}}` -- verify 400
  8. Non-owner attempts budget update -- verify 403
  - Expected failure: Budget or settings flow not fully wired

**GREEN Phase:**
- [ ] Fix any issues in the budget/settings update flow

**REFACTOR Phase:**
- [ ] Ensure test covers the null vs. zero distinction for budget

**Acceptance Criteria:**
- [ ] Budget can be set, updated, and cleared (null = unlimited)
- [ ] Invalid budget values are rejected
- [ ] Settings merge preserves existing keys
- [ ] Unknown settings keys are rejected
- [ ] Only owner can update budget

---

### Final Verification Tasks

#### Task 10.1: Pre-Merge Checklist

**Final Checks:**

- [ ] All tasks above completed (27/27)
- [ ] All tests passing: `make test`
- [ ] No linter errors
- [ ] No type errors
- [ ] Test coverage meets threshold (>= 80%)
- [ ] `make sqlc` produces no diff
- [ ] Migrations tested (up + down + up)
- [ ] No debug code or fmt.Println statements
- [ ] No commented-out code
- [ ] All TODO comments reference specific requirement IDs or feature numbers
- [ ] API error format matches `{"error": "message", "code": "CODE"}` everywhere

**Acceptance Criteria:**
- [ ] Feature is production-ready
- [ ] All quality gates passed
- [ ] Ready for PR/merge

---

## Dependency Graph

```
Task 1.1 (squads migration)
  +-- Task 1.2 (memberships migration) [depends on squads table]
        +-- Task 1.3 (squad queries) + Task 1.4 (membership queries) [depend on both tables]

Task 2.1 + 2.2 + 2.3 (domain types) [no DB dependency, can run in parallel with Component 1]

Task 5.1 (middleware) [needs: 1.3, 1.4, 2.1, 2.2]
  +-- Task 5.2 (route registration) [needs: 5.1]

Task 3.1-3.5 (squad handlers) [need: 1.3, 2.1-2.3, 5.1]
Task 4.1-4.5 (membership handlers) [need: 1.4, 2.1-2.2, 5.1]
Task 6.1 (issue counter) [needs: 1.3, 2.2]
Task 7.1 (settings merge in handler) [needs: 2.3, 3.4]
Task 8.1 (budget handler) [needs: 1.3, 2.1, 3.1 or 5.1]

Task 9.1-9.5 (integration tests) [need: all above]
Task 10.1 (pre-merge) [needs: all above]
```

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-SM-001 | 1.1, 2.1, 2.2 |
| REQ-SM-002 | 1.1, 2.3, 3.1 |
| REQ-SM-003 | 2.1, 3.1 |
| REQ-SM-004 | 1.1, 2.1 |
| REQ-SM-005 | 1.1, 2.1, 3.1 |
| REQ-SM-006 | 1.1, 3.1 |
| REQ-SM-007 | 1.1, 2.3 |
| REQ-SM-008 | 1.1, 8.1, 9.5 |
| REQ-SM-009 | 1.1, 2.1, 8.1, 9.5 |
| REQ-SM-010 | 1.3, 3.1 |
| REQ-SM-011 | 3.1 |
| REQ-SM-012 | 1.3, 3.2 |
| REQ-SM-013 | 1.3, 3.3 |
| REQ-SM-014 | 1.3, 3.4 |
| REQ-SM-015 | 3.4 |
| REQ-SM-016 | 3.5 |
| REQ-SM-017 | 1.3, 3.5 |
| REQ-SM-020 | 1.2, 1.4, 2.2 |
| REQ-SM-021 | 1.2, 4.1, 9.2 |
| REQ-SM-022 | 4.3, 4.4, 4.5, 9.2 |
| REQ-SM-023 | 4.3, 4.4, 4.5, 9.2 |
| REQ-SM-024 | 1.4, 4.1, 9.2 |
| REQ-SM-025 | 1.4, 4.3, 9.2 |
| REQ-SM-026 | 4.1, 4.3, 9.2 |
| REQ-SM-027 | 1.4, 4.4, 9.2 |
| REQ-SM-028 | 1.4, 4.5, 9.2 |
| REQ-SM-030 | 5.1, 5.2, 9.4 |
| REQ-SM-031 | 3.3, 5.1, 9.4 |
| REQ-SM-032 | 5.1, 5.2, 9.4 |
| REQ-SM-033 | 5.1, 9.4 |
| REQ-SM-040 | 1.3, 6.1, 9.3 |
| REQ-SM-041 | 6.1, 9.3 |
| REQ-SM-042 | 1.3, 6.1, 9.3 |
| REQ-SM-050 | 2.3, 7.1, 9.5 |
| REQ-SM-051 | 2.3, 3.4, 7.1, 9.5 |
| REQ-SM-052 | 2.3, 7.1, 9.5 |
| REQ-SM-060 | 8.1, 9.5 |
| REQ-SM-061 | 8.1, 9.5 |
| REQ-SM-062 | 8.1 (TODO: depends on CostEvents) |
| REQ-SM-063 | 8.1 (TODO: depends on CostEvents + notifications) |
| REQ-SM-064 | 8.1 (TODO: depends on CostEvents + agent runtime) |
| REQ-SM-065 | 8.1, 9.5 |
| REQ-SM-070 | 3.1, 3.2, 3.3, 3.4, 4.2, 8.1 |
| REQ-SM-071 | All handler tasks (error format) |
| REQ-SM-072 | 3.1 |
| REQ-SM-073 | 3.2 |
| REQ-SM-080 | 3.1 |
| REQ-SM-081 | 3.1 |
| REQ-SM-082 | 3.3 |
| REQ-SM-083 | 2.1, 3.4 |
| REQ-SM-084 | 3.1, 3.4, 4.1, 8.1 |
| REQ-SM-090 | 9.1 (performance validation) |
| REQ-SM-091 | 3.2 |
| REQ-SM-092 | 6.1, 9.3 |
| REQ-SM-093 | 9.1 (TODO: depends on activity log feature) |

## Deferred Requirements

The following requirements are partially addressed with TODO placeholders, pending future features:

| Requirement | Dependency | Notes |
|-------------|-----------|-------|
| REQ-SM-016 | 04-agent-management, 05-issue-tracking | Active agent / unresolved issue checks stubbed until those tables exist |
| REQ-SM-062 | CostEvents table | Monthly spend computed from cost_events (future feature) |
| REQ-SM-063 | CostEvents + notifications | 80% budget warning (future feature) |
| REQ-SM-064 | CostEvents + agent runtime | 100% budget hard stop (future feature) |
| REQ-SM-093 | Activity log feature | Mutation logging deferred |

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

- Go 1.22+ `net/http` ServeMux supports method-based routing (e.g., `"POST /api/squads"`) and path wildcards (`{id}`)
- Literal path segments (e.g., `/members/me`) take precedence over wildcards (`/members/{memberId}`) in Go 1.22+ mux
- The `UPDATE ... RETURNING` pattern for issue counter implicitly acquires a row-level lock -- no explicit `SELECT FOR UPDATE` needed
- The `COALESCE(sqlc.narg(...), column)` pattern in sqlc enables partial updates without conditional SQL

### Blockers

- [ ] None identified

### Future Improvements

- Pagination could be upgraded to cursor-based for better performance at scale
- Settings schema could be extracted to a JSON Schema definition for programmatic validation
- Budget enforcement (spend tracking, alerts, hard stops) will be implemented with the CostEvents feature

### Lessons Learned

[Document insights gained during implementation]
