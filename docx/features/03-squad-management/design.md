# Design: Squad Management

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 03-squad-management
**Dependencies:** 01-go-scaffold, 02-user-auth

---

## 1. Architecture Overview

Squad Management is the organizational backbone of Ari. Every resource in the system (agents, issues, projects, goals, cost events, activity log entries) belongs to exactly one squad, making the squad the unit of data isolation, budget enforcement, and team membership.

This feature introduces two new database tables (`squads` and `squad_memberships`), two handler files (`squad_handler.go` and `membership_handler.go`), domain types in `internal/domain/squad.go`, and a squad-scoping middleware that enforces membership checks on all squad-scoped routes. Handlers call sqlc-generated queries directly for simple CRUD operations, with business logic (validation, slug generation, settings merge, permission checks) implemented inline in the handler methods. For cross-cutting operations that span multiple features — such as atomic issue counter increments used by the future issue-creation handler — a thin service function is provided (e.g., `NextIssueIdentifier`). This avoids premature abstraction while keeping complex multi-step operations reusable.

### 1.1 Multi-Squad Architecture (BR-3.2)

Ari is designed from the ground up for **concurrent multi-squad operation**. A single Ari instance supports multiple squads running simultaneously with strict isolation between them.

**Core principles:**

1. **Users belong to multiple squads.** A user can be an owner of Squad A, an admin of Squad B, and a viewer of Squad C — all at the same time. Each membership is independent with its own role.

2. **No server-side "active squad" state.** The backend is stateless with respect to squad context. Every API request carries the squad ID explicitly in the URL path (`/api/squads/{squadId}/...`). The server never stores or assumes which squad a user is "currently" working in.

3. **Client-side squad switching.** The React UI maintains an "active squad" in client state (React Context + URL params). Switching squads is instant — it changes the squad ID used in API calls, with no server round-trip required. See Feature 07 (REQ-UI-054) for the squad selector UI.

4. **Complete data isolation.** Squad A's agents, issues, budgets, and activity logs are invisible to Squad B. This is enforced at both the middleware layer (membership check) and the database layer (`WHERE squad_id = $1` on every query).

5. **Independent operation.** Each squad has its own budget, issue counter, settings, and member roster. Pausing or archiving one squad has zero effect on others.

**API contract for multi-squad:**

| Endpoint Pattern | Squad Context | Example |
|------------------|---------------|---------|
| `POST /api/squads` | No squad context (creates new) | Create a squad |
| `GET /api/squads` | Returns all user's squads | List for squad selector |
| `GET /api/squads/{id}` | Explicit squad ID in path | View one squad |
| `* /api/squads/{id}/**` | Explicit squad ID in path | All squad-scoped operations |

**What this means for the frontend:**

```
User logs in
  → GET /api/squads → receives [{id: "aaa", name: "Alpha"}, {id: "bbb", name: "Beta"}]
  → UI sets activeSquad = squads[0] (or last-visited from localStorage)
  → All subsequent API calls use activeSquad.id in the URL path
  → User clicks squad selector → switches activeSquad → UI re-fetches data for new squad
```

**What this means for the backend:**

- No "current squad" session variable, cookie, or header
- No cross-squad aggregation endpoints in Phase 1
- Each request is fully self-contained: auth token + squad ID in URL = everything needed
- Multiple browser tabs can operate on different squads simultaneously

## 2. System Context

- **Depends On:**
  - `01-go-scaffold` -- HTTP server, router, middleware chain, database connection, migrations, sqlc codegen, JSON response helpers, structured logging
  - `02-user-auth` -- Authentication middleware, JWT/session validation, user identity in request context (`context.Value` with user ID and email)

- **Used By:**
  - All future squad-scoped features (agents, issues, projects, goals, cost events, activity log, inbox)
  - The React UI dashboard (squad selector, squad settings page, member management)

- **External Dependencies:** None (all data is local PostgreSQL)

---

## 3. Component Structure

### 3.1 Domain Types -- `internal/domain/squad.go`

**Responsibility:** Define all Squad and SquadMembership value types, enums, and validation logic. No database or HTTP dependencies.

```go
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// --- Enums ---

type SquadStatus string

const (
	SquadStatusActive   SquadStatus = "active"
	SquadStatusPaused   SquadStatus = "paused"
	SquadStatusArchived SquadStatus = "archived"
)

func (s SquadStatus) Valid() bool {
	switch s {
	case SquadStatusActive, SquadStatusPaused, SquadStatusArchived:
		return true
	}
	return false
}

// ValidTransition returns true if moving from current status to the target is allowed.
// Allowed: active <-> paused, active -> archived, paused -> archived.
// Disallowed: archived -> anything.
func (s SquadStatus) ValidTransition(target SquadStatus) bool {
	if s == target {
		return true
	}
	switch s {
	case SquadStatusActive:
		return target == SquadStatusPaused || target == SquadStatusArchived
	case SquadStatusPaused:
		return target == SquadStatusActive || target == SquadStatusArchived
	case SquadStatusArchived:
		return false // archived is terminal
	}
	return false
}

type MemberRole string

const (
	MemberRoleOwner  MemberRole = "owner"
	MemberRoleAdmin  MemberRole = "admin"
	MemberRoleViewer MemberRole = "viewer"
)

func (r MemberRole) Valid() bool {
	switch r {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleViewer:
		return true
	}
	return false
}

// CanManageMembers returns true if the role is allowed to add/remove members.
func (r MemberRole) CanManageMembers() bool {
	return r == MemberRoleOwner || r == MemberRoleAdmin
}

// CanEditSquad returns true if the role is allowed to update squad fields.
func (r MemberRole) CanEditSquad() bool {
	return r == MemberRoleOwner || r == MemberRoleAdmin
}

// CanGrantRole returns true if the actor role can assign the target role.
// Only owners can grant owner or admin.
func (r MemberRole) CanGrantRole(target MemberRole) bool {
	if target == MemberRoleOwner || target == MemberRoleAdmin {
		return r == MemberRoleOwner
	}
	return r == MemberRoleOwner || r == MemberRoleAdmin
}

// --- Squad Settings ---

// SquadSettings is the typed representation of the settings JSONB column.
// All fields are pointers to support partial updates (nil = not provided).
type SquadSettings struct {
	RequireApprovalForNewAgents *bool `json:"requireApprovalForNewAgents,omitempty"`
}

// DefaultSquadSettings returns settings with all defaults applied.
func DefaultSquadSettings() SquadSettings {
	f := false
	return SquadSettings{
		RequireApprovalForNewAgents: &f,
	}
}

// Merge applies non-nil fields from patch onto the receiver (partial update).
func (s *SquadSettings) Merge(patch SquadSettings) {
	if patch.RequireApprovalForNewAgents != nil {
		s.RequireApprovalForNewAgents = patch.RequireApprovalForNewAgents
	}
}

// --- Core Domain Types ---

type Squad struct {
	ID                 uuid.UUID     `json:"id"`
	Name               string        `json:"name"`
	Slug               string        `json:"slug"`
	IssuePrefix        string        `json:"issuePrefix"`
	Description        string        `json:"description"`
	Status             SquadStatus   `json:"status"`
	Settings           SquadSettings `json:"settings"`
	IssueCounter       int64         `json:"issueCounter"`
	BudgetMonthlyCents *int64        `json:"budgetMonthlyCents"` // nil = unlimited
	BrandColor         *string       `json:"brandColor,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

// SquadWithRole is returned by list queries; includes the requesting user's role.
type SquadWithRole struct {
	Squad
	Role MemberRole `json:"role"`
}

type SquadMembership struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"userId"`
	SquadID   uuid.UUID  `json:"squadId"`
	Role      MemberRole `json:"role"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// --- Validation ---

var (
	slugRegex        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
	issuePrefixRegex = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)
	brandColorRegex  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func ValidateSquadName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ValidationError{Field: "name", Message: "must not be empty"}
	}
	if len(name) > 100 {
		return ValidationError{Field: "name", Message: "must not exceed 100 characters"}
	}
	return nil
}

func ValidateSquadSlug(slug string) error {
	if len(slug) < 2 || len(slug) > 50 {
		return ValidationError{Field: "slug", Message: "must be between 2 and 50 characters"}
	}
	if !slugRegex.MatchString(slug) {
		return ValidationError{Field: "slug", Message: "must be lowercase alphanumeric with hyphens"}
	}
	return nil
}

func ValidateIssuePrefix(prefix string) error {
	if !issuePrefixRegex.MatchString(prefix) {
		return ValidationError{Field: "issuePrefix", Message: "must be 2-10 uppercase alphanumeric characters"}
	}
	return nil
}

func ValidateBudget(cents *int64) error {
	if cents != nil && *cents <= 0 {
		return ValidationError{Field: "budgetMonthlyCents", Message: "must be a positive integer"}
	}
	return nil
}

func ValidateBrandColor(color *string) error {
	if color != nil && !brandColorRegex.MatchString(*color) {
		return ValidationError{Field: "brandColor", Message: "must be a valid hex color (e.g., #1a2b3c)"}
	}
	return nil
}

// GenerateSlug produces a URL-safe slug from a squad name.
// "My Squad Name!" -> "my-squad-name"
func GenerateSlug(name string) string {
	var b strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prev = r
		} else if prev != '-' {
			b.WriteRune('-')
			prev = '-'
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) < 2 {
		slug = slug + "-squad"
	}
	return slug
}

// --- Known Settings Keys (for validation) ---

var knownSettingsKeys = map[string]bool{
	"requireApprovalForNewAgents": true,
}

// ValidateSettingsKeys checks that a raw JSON map contains only known keys.
func ValidateSettingsKeys(raw map[string]interface{}) error {
	for key := range raw {
		if !knownSettingsKeys[key] {
			return ValidationError{
				Field:   "settings." + key,
				Message: "unknown settings key",
			}
		}
	}
	return nil
}
```

### 3.2 Squad Handler -- `internal/server/handlers/squad_handler.go`

**Responsibility:** HTTP routing and request/response marshalling for squad CRUD. Delegates all business logic to the service layer.

**Dependencies:**
- `internal/domain` -- types and validation
- `internal/database/db` -- sqlc Querier interface
- Auth middleware context (user ID from `02-user-auth`)

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"ari/internal/domain"
	"ari/internal/database/db"
)

type SquadHandler struct {
	queries db.Querier
}

func NewSquadHandler(q db.Querier) *SquadHandler {
	return &SquadHandler{queries: q}
}

// RegisterRoutes registers squad CRUD routes on the given mux.
// All routes are behind the auth middleware.
func (h *SquadHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads", h.Create)
	mux.HandleFunc("GET /api/squads", h.List)
	mux.HandleFunc("GET /api/squads/{id}", h.Get)
	mux.HandleFunc("PATCH /api/squads/{id}", h.Update)
	mux.HandleFunc("DELETE /api/squads/{id}", h.Delete)
	mux.HandleFunc("PATCH /api/squads/{id}/budgets", h.UpdateBudget)
}

// --- Request/Response Types ---

type CreateSquadRequest struct {
	Name               string                `json:"name"`
	IssuePrefix        string                `json:"issuePrefix"`
	Description        string                `json:"description,omitempty"`
	Settings           *domain.SquadSettings `json:"settings,omitempty"`
	BudgetMonthlyCents *int64                `json:"budgetMonthlyCents,omitempty"`
	BrandColor         *string               `json:"brandColor,omitempty"`
}

type UpdateSquadRequest struct {
	Name               *string               `json:"name,omitempty"`
	Description        *string               `json:"description,omitempty"`
	Status             *domain.SquadStatus   `json:"status,omitempty"`
	Settings           *domain.SquadSettings `json:"settings,omitempty"`
	BudgetMonthlyCents *int64                `json:"budgetMonthlyCents,omitempty"`
	BrandColor         *string               `json:"brandColor,omitempty"`
}

type UpdateBudgetRequest struct {
	BudgetMonthlyCents *int64 `json:"budgetMonthlyCents"` // null = unlimited
}

type ListSquadsParams struct {
	Limit  int // default 50, max 100
	Offset int // default 0
}
```

**Key Behaviors:**
- `Create` validates input, generates slug, creates squad + owner membership in a single transaction, returns 201
- `List` returns only squads where the authenticated user has a membership, supports pagination (limit/offset)
- `Get` returns squad details only if user has membership, else 404
- `Update` requires owner/admin role, validates status transitions, merges settings partially
- `Delete` requires owner role, checks for active agents/unresolved issues (409 if present), soft-deletes by setting status to archived
- `UpdateBudget` requires owner role, accepts positive integer or null

### 3.3 Membership Handler -- `internal/server/handlers/membership_handler.go`

**Responsibility:** HTTP routing for squad membership management (add, update role, remove members).

**Dependencies:**
- Same as SquadHandler
- Squad-scoped middleware (user must be a member)

```go
package handlers

import (
	"net/http"

	"ari/internal/database/db"
)

type MembershipHandler struct {
	queries db.Querier
}

func NewMembershipHandler(q db.Querier) *MembershipHandler {
	return &MembershipHandler{queries: q}
}

func (h *MembershipHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/squads/{id}/members", h.List)
	mux.HandleFunc("POST /api/squads/{id}/members", h.Add)
	mux.HandleFunc("PATCH /api/squads/{id}/members/{memberId}", h.UpdateRole)
	mux.HandleFunc("DELETE /api/squads/{id}/members/{memberId}", h.Remove)
	mux.HandleFunc("DELETE /api/squads/{id}/members/me", h.Leave)
}
```

**Key Behaviors:**
- `Add` requires owner/admin role; only owners can grant owner/admin roles (REQ-SM-026)
- `UpdateRole` requires owner role; enforces last-owner protection atomically via `DemoteOwnerIfNotLast` (REQ-SM-023)
- `Remove` requires owner role; enforces last-owner protection atomically via `DeleteSquadMembershipIfNotLastOwner`
- `Leave` allows any member to leave; enforces last-owner protection atomically via `DeleteSquadMembershipByUserIfNotLastOwner`
- `List` returns all members of a squad with their roles

### 3.4 Squad-Scoping Middleware -- `internal/server/middleware/squad_scope.go`

**Responsibility:** Extract squad ID from the URL path, verify the authenticated user has an active membership, inject the membership (including role) into the request context.

```go
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"ari/internal/database/db"
	"ari/internal/domain"
)

type squadContextKey struct{}
type membershipContextKey struct{}

// SquadFromContext retrieves the squad ID injected by the squad-scoping middleware.
func SquadFromContext(ctx context.Context) uuid.UUID {
	return ctx.Value(squadContextKey{}).(uuid.UUID)
}

// MembershipFromContext retrieves the user's membership for the current squad.
func MembershipFromContext(ctx context.Context) domain.SquadMembership {
	return ctx.Value(membershipContextKey{}).(domain.SquadMembership)
}

// RequireSquadMembership is middleware that:
// 1. Extracts {id} from the URL path
// 2. Queries squad_memberships for (user_id, squad_id)
// 3. If no membership found, returns 404 (not 403, to avoid leaking existence)
// 4. If found, injects squad_id and membership into context
func RequireSquadMembership(queries db.Querier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			squadIDStr := r.PathValue("id")
			squadID, err := uuid.Parse(squadIDStr)
			if err != nil {
				writeError(w, http.StatusNotFound, "SQUAD_NOT_FOUND", "Squad not found")
				return
			}

			userID := UserIDFromContext(r.Context()) // from auth middleware

			membership, err := queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
				UserID:  userID,
				SquadID: squadID,
			})
			if err != nil {
				writeError(w, http.StatusNotFound, "SQUAD_NOT_FOUND", "Squad not found")
				return
			}

			ctx := context.WithValue(r.Context(), squadContextKey{}, squadID)
			ctx = context.WithValue(ctx, membershipContextKey{}, toDomainMembership(membership))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

---

## 4. Database Schema

### 4.1 Migration: `squads` table

**File:** `internal/database/migrations/YYYYMMDDHHMMSS_create_squads.sql`

```sql
-- +goose Up
CREATE TABLE squads (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 VARCHAR(100)  NOT NULL,
    slug                 VARCHAR(50)   NOT NULL,
    issue_prefix         VARCHAR(10)   NOT NULL,
    description          TEXT          NOT NULL DEFAULT '',
    status               VARCHAR(20)   NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'paused', 'archived')),
    settings             JSONB         NOT NULL DEFAULT '{"requireApprovalForNewAgents": false}',
    issue_counter        BIGINT        NOT NULL DEFAULT 0,
    budget_monthly_cents BIGINT        CHECK (budget_monthly_cents IS NULL OR budget_monthly_cents > 0),
    brand_color          VARCHAR(7),
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT squads_slug_unique       UNIQUE (slug),
    CONSTRAINT squads_issue_prefix_unique UNIQUE (issue_prefix)
);

CREATE INDEX idx_squads_status ON squads (status);

-- +goose Down
DROP TABLE IF EXISTS squads;
```

### 4.2 Migration: `squad_memberships` table

**File:** `internal/database/migrations/YYYYMMDDHHMMSS_create_squad_memberships.sql`

```sql
-- +goose Up
CREATE TABLE squad_memberships (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    squad_id   UUID        NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    role       VARCHAR(20) NOT NULL DEFAULT 'viewer'
               CHECK (role IN ('owner', 'admin', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT squad_memberships_user_squad_unique UNIQUE (user_id, squad_id)
);

CREATE INDEX idx_squad_memberships_squad_id ON squad_memberships (squad_id);
CREATE INDEX idx_squad_memberships_user_id  ON squad_memberships (user_id);

-- +goose Down
DROP TABLE IF EXISTS squad_memberships;
```

### 4.3 sqlc Queries -- `internal/database/queries/squads.sql`

```sql
-- name: CreateSquad :one
INSERT INTO squads (name, slug, issue_prefix, description, status, settings, budget_monthly_cents, brand_color)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSquadByID :one
SELECT * FROM squads WHERE id = $1;

-- name: GetSquadBySlug :one
SELECT * FROM squads WHERE slug = $1;

-- name: ListSquadsByUser :many
-- Returns all squads where the given user has a membership, with the user's role.
SELECT s.*, sm.role, sm.id AS membership_id
FROM squads s
INNER JOIN squad_memberships sm ON sm.squad_id = s.id
WHERE sm.user_id = $1
  AND s.status != 'archived'
ORDER BY s.name ASC
LIMIT $2 OFFSET $3;

-- name: UpdateSquad :one
UPDATE squads
SET name                 = COALESCE(sqlc.narg('name'), name),
    slug                 = COALESCE(sqlc.narg('slug'), slug),
    description          = COALESCE(sqlc.narg('description'), description),
    status               = COALESCE(sqlc.narg('status'), status),
    settings             = COALESCE(sqlc.narg('settings'), settings),
    budget_monthly_cents = CASE
                             WHEN sqlc.arg('update_budget')::BOOLEAN THEN sqlc.narg('budget_monthly_cents')
                             ELSE budget_monthly_cents
                           END,
    brand_color          = COALESCE(sqlc.narg('brand_color'), brand_color),
    updated_at           = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- DESIGN NOTE: COALESCE/sqlc.narg limitation
-- The COALESCE(sqlc.narg('field'), existing_field) pattern cannot distinguish between
-- "field not provided" (Go nil) and "field explicitly set to NULL". This means nullable
-- fields like brand_color cannot be cleared to NULL via this query. The budget_monthly_cents
-- field works around this using a separate boolean flag (update_budget) + CASE expression.
-- For Phase 1, brand_color uses COALESCE and therefore cannot be unset once set. If clearing
-- nullable fields becomes a requirement, use the same CASE/flag pattern as budget, or use
-- a dedicated UPDATE query per field.

-- name: SoftDeleteSquad :one
UPDATE squads
SET status = 'archived', updated_at = now()
WHERE id = $1
RETURNING *;

-- name: IncrementIssueCounter :one
-- Atomically increments the issue counter and returns the new value.
-- Used when creating issues to generate "{prefix}-{counter}" identifiers.
UPDATE squads
SET issue_counter = issue_counter + 1, updated_at = now()
WHERE id = $1
RETURNING issue_counter, issue_prefix;

-- name: GetSquadSettings :one
SELECT settings FROM squads WHERE id = $1;

-- NOTE: Slug and issuePrefix uniqueness are enforced by UNIQUE constraints on the
-- squads table (squads_slug_unique, squads_issue_prefix_unique). Do NOT pre-check
-- with SELECT EXISTS before INSERT — this has a TOCTOU race under concurrent requests.
-- Instead, attempt the INSERT directly and handle PostgreSQL unique-violation error
-- (code 23505) by mapping the constraint name to the appropriate 409 response
-- (SLUG_TAKEN or ISSUE_PREFIX_TAKEN). See Section 10.3 for the error mapping logic.
```

### 4.4 sqlc Queries -- `internal/database/queries/squad_memberships.sql`

```sql
-- name: CreateSquadMembership :one
INSERT INTO squad_memberships (user_id, squad_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSquadMembership :one
SELECT * FROM squad_memberships
WHERE user_id = $1 AND squad_id = $2;

-- name: GetSquadMembershipByID :one
SELECT * FROM squad_memberships
WHERE id = $1 AND squad_id = $2;

-- name: ListSquadMembers :many
SELECT sm.*, u.email, u.display_name
FROM squad_memberships sm
INNER JOIN users u ON u.id = sm.user_id
WHERE sm.squad_id = $1
ORDER BY sm.created_at ASC;

-- name: UpdateSquadMembershipRole :one
UPDATE squad_memberships
SET role = $1, updated_at = now()
WHERE id = $2 AND squad_id = $3
RETURNING *;

-- name: DeleteSquadMembership :exec
DELETE FROM squad_memberships
WHERE id = $1 AND squad_id = $2;

-- name: CountSquadOwners :one
SELECT COUNT(*) FROM squad_memberships
WHERE squad_id = $1 AND role = 'owner';

-- name: DeleteSquadMembershipIfNotLastOwner :execrows
-- Atomically deletes a membership only if it would not leave zero owners.
-- Returns 0 rows affected if the member is the last owner (caller should return 409).
DELETE FROM squad_memberships
WHERE id = $1 AND squad_id = $2
  AND NOT (
    role = 'owner'
    AND (SELECT COUNT(*) FROM squad_memberships WHERE squad_id = $2 AND role = 'owner') = 1
  );

-- name: DemoteOwnerIfNotLast :execrows
-- Atomically updates an owner's role only if at least one other owner exists.
-- Returns 0 rows affected if the member is the last owner (caller should return 409).
UPDATE squad_memberships
SET role = $1, updated_at = now()
WHERE id = $2 AND squad_id = $3
  AND NOT (
    role = 'owner'
    AND (SELECT COUNT(*) FROM squad_memberships WHERE squad_id = $3 AND role = 'owner') = 1
  );

-- name: DeleteSquadMembershipByUserAndSquad :exec
DELETE FROM squad_memberships
WHERE user_id = $1 AND squad_id = $2;

-- name: DeleteSquadMembershipByUserIfNotLastOwner :execrows
-- Atomic variant for the "leave squad" flow.
-- Returns 0 rows affected if the user is the last owner.
DELETE FROM squad_memberships
WHERE user_id = $1 AND squad_id = $2
  AND NOT (
    role = 'owner'
    AND (SELECT COUNT(*) FROM squad_memberships WHERE squad_id = $2 AND role = 'owner') = 1
  );
```

---

## 5. API Contracts

### 5.1 POST /api/squads -- Create Squad

**Request:**
```json
{
  "name": "Acme Engineering",
  "issuePrefix": "ACME",
  "description": "The core engineering squad",
  "settings": {
    "requireApprovalForNewAgents": true
  },
  "budgetMonthlyCents": 500000,
  "brandColor": "#3b82f6"
}
```

**Response (201 Created):**
```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "name": "Acme Engineering",
  "slug": "acme-engineering",
  "issuePrefix": "ACME",
  "description": "The core engineering squad",
  "status": "active",
  "settings": {
    "requireApprovalForNewAgents": true
  },
  "issueCounter": 0,
  "budgetMonthlyCents": 500000,
  "brandColor": "#3b82f6",
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T10:00:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` -- missing or invalid fields
- `409 SLUG_TAKEN` -- generated slug conflicts with existing squad
- `409 ISSUE_PREFIX_TAKEN` -- issuePrefix already in use

### 5.2 GET /api/squads -- List My Squads

**Query Parameters:**
- `limit` (int, default 50, max 100)
- `offset` (int, default 0)

**Response (200 OK):**
```json
[
  {
    "id": "a1b2c3d4-...",
    "name": "Acme Engineering",
    "slug": "acme-engineering",
    "issuePrefix": "ACME",
    "description": "The core engineering squad",
    "status": "active",
    "settings": { "requireApprovalForNewAgents": true },
    "issueCounter": 42,
    "budgetMonthlyCents": 500000,
    "brandColor": "#3b82f6",
    "role": "owner",
    "createdAt": "2026-03-14T10:00:00Z",
    "updatedAt": "2026-03-14T12:00:00Z"
  }
]
```

### 5.3 GET /api/squads/{id} -- Get Squad

**Response (200 OK):** Same shape as a single squad object (without `role`).

**Errors:**
- `404 SQUAD_NOT_FOUND` -- squad does not exist or user has no membership

### 5.4 PATCH /api/squads/{id} -- Update Squad

**Request (all fields optional):**
```json
{
  "name": "Acme Engineering v2",
  "description": "Updated description",
  "status": "paused",
  "settings": {
    "requireApprovalForNewAgents": false
  },
  "brandColor": "#ef4444"
}
```

**Response (200 OK):** Updated squad object.

**Errors:**
- `400 VALIDATION_ERROR` -- invalid field values
- `400 INVALID_STATUS_TRANSITION` -- e.g., archived -> active
- `403 FORBIDDEN` -- user role is viewer
- `404 SQUAD_NOT_FOUND` -- no membership

### 5.5 DELETE /api/squads/{id} -- Delete (Archive) Squad

**Response (200 OK):** Archived squad object.

**Errors:**
- `403 FORBIDDEN` -- user is not owner
- `404 SQUAD_NOT_FOUND` -- no membership
- `409 SQUAD_HAS_ACTIVE_RESOURCES` -- squad has active agents or unresolved issues

### 5.6 PATCH /api/squads/{id}/budgets -- Update Budget

**Request:**
```json
{
  "budgetMonthlyCents": 1000000
}
```

Or to set unlimited:
```json
{
  "budgetMonthlyCents": null
}
```

**Response (200 OK):** Updated squad object.

**Errors:**
- `400 VALIDATION_ERROR` -- budget is zero or negative
- `403 FORBIDDEN` -- user is not owner

### 5.7 GET /api/squads/{id}/members -- List Members

**Response (200 OK):**
```json
[
  {
    "id": "mem-uuid-1",
    "userId": "user-uuid-1",
    "squadId": "squad-uuid-1",
    "role": "owner",
    "email": "alice@example.com",
    "displayName": "Alice",
    "createdAt": "2026-03-14T10:00:00Z",
    "updatedAt": "2026-03-14T10:00:00Z"
  }
]
```

### 5.8 POST /api/squads/{id}/members -- Add Member

**Request:**
```json
{
  "userId": "user-uuid-2",
  "role": "admin"
}
```

**Response (201 Created):** Membership object.

**Errors:**
- `400 VALIDATION_ERROR` -- invalid role or missing userId
- `403 FORBIDDEN` -- user lacks permission to add with given role
- `409 MEMBER_EXISTS` -- user already a member

### 5.9 PATCH /api/squads/{id}/members/{memberId} -- Update Role

**Request:**
```json
{
  "role": "viewer"
}
```

**Response (200 OK):** Updated membership object.

**Errors:**
- `403 FORBIDDEN` -- only owners can change roles
- `409 LAST_OWNER` -- cannot demote the last owner

### 5.10 DELETE /api/squads/{id}/members/{memberId} -- Remove Member

**Response (200 OK):** `{"message": "Member removed"}`

**Errors:**
- `403 FORBIDDEN` -- only owners can remove members
- `409 LAST_OWNER` -- cannot remove the last owner

### 5.11 DELETE /api/squads/{id}/members/me -- Leave Squad

**Response (200 OK):** `{"message": "You have left the squad"}`

**Errors:**
- `409 LAST_OWNER` -- cannot leave if you are the last owner

---

## 6. Data Flow

### 6.1 Squad Creation Flow

```
Client                  Handler              Database (sqlc)
  |                       |                      |
  |  POST /api/squads     |                      |
  |---------------------> |                      |
  |                       | validate input       |
  |                       | generate slug        |
  |                       |                      |
  |                       | BEGIN TX             |
  |                       |--------------------> |
  |                       |                      | INSERT squads
  |                       |                      |   (handle 23505 →
  |                       |                      |    409 SLUG_TAKEN or
  |                       |                      |    ISSUE_PREFIX_TAKEN)
  |                       |                      | INSERT squad_memberships
  |                       |                      |   (role = 'owner',
  |                       |                      |    user_id = auth user)
  |                       | COMMIT TX            |
  |                       |<-------------------- |
  |                       |                      |
  |  201 + squad JSON     |                      |
  |<--------------------- |                      |
```

**Detailed Steps:**

1. **Validate input:** Check name (non-empty, <= 100 chars), issuePrefix (uppercase alphanum, 2-10 chars), budgetMonthlyCents (positive or null), brandColor (hex or null), settings keys (known keys only).
2. **Generate slug:** Convert name to lowercase, replace non-alphanum with hyphens, trim.
3. **Begin transaction:** Both the squad INSERT and the owner membership INSERT must succeed or neither persists.
4. **Insert squad:** With status `active`, issueCounter `0`, default settings merged with any provided settings. Do NOT pre-check slug or issuePrefix uniqueness — rely on the UNIQUE constraints. If the INSERT fails with a unique-violation (PG code 23505), map the constraint name to 409 SLUG_TAKEN or ISSUE_PREFIX_TAKEN. If a slug collision occurs, retry with a numeric suffix (`-2`, `-3`, etc.) within the same handler call.
5. **Insert owner membership:** user_id from auth context, squad_id from newly created squad, role `owner`.
6. **Commit and return:** Return the created squad with HTTP 201.

### 6.2 Issue Identifier Generation (Atomic Counter)

```
Agent/Client            Handler              PostgreSQL
  |                       |                      |
  |  POST .../issues      |                      |
  |---------------------> |                      |
  |                       | UPDATE squads        |
  |                       | SET issue_counter =  |
  |                       |   issue_counter + 1  |
  |                       | WHERE id = $squad_id |
  |                       | RETURNING            |
  |                       |   issue_counter,     |
  |                       |   issue_prefix       |
  |                       |--------------------> |
  |                       |                      | row-level lock
  |                       |                      | increment + return
  |                       |<-------------------- |
  |                       |                      |
  |                       | identifier =         |
  |                       |  "{prefix}-{counter}"|
  |                       |                      |
  |                       | INSERT issues        |
  |                       |   (identifier, ...)  |
  |                       |--------------------> |
  |                       |<-------------------- |
  |  201 + issue JSON     |                      |
  |<--------------------- |                      |
```

The `UPDATE ... RETURNING` acquires a row-level lock on the squad row, ensuring concurrent issue creation serializes through PostgreSQL's row locking. No explicit `SELECT FOR UPDATE` is needed because `UPDATE` implicitly locks the row. This guarantees unique, gap-free identifiers under concurrent access.

```go
// Service-layer usage (called from issue creation handler):
func (s *SquadService) NextIssueIdentifier(ctx context.Context, squadID uuid.UUID) (string, error) {
	result, err := s.queries.IncrementIssueCounter(ctx, squadID)
	if err != nil {
		return "", fmt.Errorf("increment issue counter: %w", err)
	}
	return fmt.Sprintf("%s-%d", result.IssuePrefix, result.IssueCounter), nil
}
```

---

## 7. Squad-Scoped Data Isolation

### 7.1 Multi-Squad Isolation Model

When a user belongs to multiple squads, each squad operates as a fully independent workspace. The isolation model ensures:

- **Request-level isolation:** Each API request targets exactly one squad via the URL path. There is no way to query data across squads in a single request.
- **No cross-contamination:** A bug or misconfiguration in Squad A cannot leak data to Squad B. Isolation is enforced at both middleware and SQL levels.
- **Concurrent access:** A user can have multiple browser tabs open, each operating on a different squad. The backend handles this naturally because there is no server-side squad state.

**Example: User with 3 squads**

```
Tab 1: GET /api/squads/aaa/agents  → returns Squad Alpha's agents
Tab 2: GET /api/squads/bbb/issues  → returns Squad Beta's issues
Tab 3: POST /api/squads/ccc/agents → creates agent in Squad Gamma

All three requests can happen concurrently. Each is independently
authorized against the user's membership in that specific squad.
```

### 7.2 Middleware Approach

Every route under `/api/squads/{id}/...` passes through `RequireSquadMembership` middleware. This middleware:

1. Parses `{id}` as a UUID
2. Queries `squad_memberships` for `(user_id, squad_id)`
3. Returns 404 if no membership found (not 403, to prevent leaking squad existence per REQ-SM-031)
4. Injects `squad_id` and `membership` (including role) into the request context

Downstream handlers retrieve the squad ID and membership via typed context accessors:

```go
squadID := middleware.SquadFromContext(r.Context())
membership := middleware.MembershipFromContext(r.Context())

if !membership.Role.CanEditSquad() {
    writeError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
    return
}
```

### 7.3 Query-Level Enforcement

Every sqlc query for squad-scoped entities includes `WHERE squad_id = $1`:

```sql
-- Every squad-scoped query follows this pattern:
-- name: ListAgentsBySquad :many
SELECT * FROM agents WHERE squad_id = $1 ORDER BY name;

-- name: GetIssueBySquad :one
SELECT * FROM issues WHERE id = $1 AND squad_id = $2;
```

There are no "list all" queries without a squad_id filter. This is a defense-in-depth measure: even if the middleware were bypassed, the database layer cannot return cross-squad data.

### 7.4 Route Registration Pattern

```go
func RegisterSquadScopedRoutes(mux *http.ServeMux, queries db.Querier) {
	squadMiddleware := middleware.RequireSquadMembership(queries)

	// Squad-level routes (membership check done in handler since {id} is top-level)
	squadHandler := handlers.NewSquadHandler(queries)
	squadHandler.RegisterRoutes(mux) // POST /api/squads, GET /api/squads handled separately

	// Member routes -- wrapped with squad membership middleware
	memberHandler := handlers.NewMembershipHandler(queries)
	memberHandler.RegisterRoutes(mux)

	// Future: agent routes, issue routes, etc. all go through squadMiddleware
}
```

---

## 8. Settings JSONB -- Schema and Partial Merge

### 8.1 Schema Definition

The `settings` column is a JSONB field with a well-defined schema. Only keys present in `domain.SquadSettings` are accepted.

**Current known keys:**

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `requireApprovalForNewAgents` | boolean | `false` | When true, new agents start in `pending_approval` status |

### 8.2 Partial Merge Strategy

When a PATCH request includes `settings`, the system does a shallow merge -- not a full replacement:

```go
func (h *SquadHandler) mergeSettings(ctx context.Context, squadID uuid.UUID, patch domain.SquadSettings) (domain.SquadSettings, error) {
	// 1. Fetch current settings from DB
	raw, err := h.queries.GetSquadSettings(ctx, squadID)
	if err != nil {
		return domain.SquadSettings{}, err
	}

	// 2. Unmarshal into typed struct
	var current domain.SquadSettings
	if err := json.Unmarshal(raw, &current); err != nil {
		return domain.SquadSettings{}, err
	}

	// 3. Merge patch onto current (nil fields in patch are skipped)
	current.Merge(patch)

	return current, nil
}
```

### 8.3 Validation

Before merge, the handler validates the incoming JSON body for unknown keys:

```go
// In the Update handler:
var rawBody map[string]interface{}
json.Unmarshal(body, &rawBody)

if settingsRaw, ok := rawBody["settings"].(map[string]interface{}); ok {
    if err := domain.ValidateSettingsKeys(settingsRaw); err != nil {
        writeError(w, 400, "VALIDATION_ERROR", err.Error())
        return
    }
}
```

### 8.4 Extensibility

Adding a new setting requires:
1. Add the field (as a pointer) to `domain.SquadSettings`
2. Add the key to `domain.knownSettingsKeys`
3. Update `Merge()` to handle the new field
4. Add a migration to set a default in existing rows (or rely on Go-level defaults)

---

## 9. Issue Counter -- Atomic Increment

### 9.1 The Problem

Multiple agents may create issues in the same squad concurrently. The issue identifier must be unique and sequential (e.g., ARI-1, ARI-2, ARI-3).

### 9.2 The Solution: UPDATE ... RETURNING

```sql
-- name: IncrementIssueCounter :one
UPDATE squads
SET issue_counter = issue_counter + 1, updated_at = now()
WHERE id = $1
RETURNING issue_counter, issue_prefix;
```

This single statement is atomic. PostgreSQL acquires a row-level exclusive lock on the squad row during the UPDATE, serializing concurrent increments. The RETURNING clause gives us both the new counter value and the prefix in one round trip.

### 9.3 Concurrency Guarantees

- **No duplicates:** The row lock ensures only one transaction increments at a time.
- **No gaps under normal operation:** Each successful UPDATE increments by exactly 1.
- **Gaps after failed transactions:** If the transaction that incremented the counter rolls back, the counter reverts. If the counter was already committed but the subsequent issue INSERT fails, no gap occurs because both happen in the same transaction.
- **Performance:** Row-level locking means concurrent issue creation for *different* squads is fully parallel. Only same-squad concurrent creates serialize.

### 9.4 Transaction Pattern

```go
func (s *IssueService) CreateIssue(ctx context.Context, squadID uuid.UUID, req CreateIssueRequest) (*domain.Issue, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	// Atomic increment -- acquires row lock on the squad
	counter, err := qtx.IncrementIssueCounter(ctx, squadID)
	if err != nil {
		return nil, fmt.Errorf("increment counter: %w", err)
	}

	identifier := fmt.Sprintf("%s-%d", counter.IssuePrefix, counter.IssueCounter)

	issue, err := qtx.CreateIssue(ctx, db.CreateIssueParams{
		SquadID:    squadID,
		Identifier: identifier,
		Title:      req.Title,
		// ...
	})
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return toDomainIssue(issue), nil
}
```

---

## 10. Error Handling

### 10.1 Error Codes

| HTTP Status | Code | When |
|-------------|------|------|
| 400 | `VALIDATION_ERROR` | Missing/invalid fields, unknown settings keys |
| 400 | `INVALID_STATUS_TRANSITION` | e.g., archived -> active |
| 403 | `FORBIDDEN` | Insufficient role for the operation |
| 404 | `SQUAD_NOT_FOUND` | Squad does not exist or user has no membership |
| 409 | `SLUG_TAKEN` | Duplicate slug |
| 409 | `ISSUE_PREFIX_TAKEN` | Duplicate issue prefix |
| 409 | `MEMBER_EXISTS` | User already a member of this squad |
| 409 | `LAST_OWNER` | Cannot remove/demote the last owner |
| 409 | `SQUAD_HAS_ACTIVE_RESOURCES` | Cannot delete squad with active agents or open issues |

### 10.2 Error Response Format

All errors follow the scaffold convention:

```json
{
  "error": "Human-readable message describing what went wrong",
  "code": "MACHINE_READABLE_CODE"
}
```

### 10.3 Database Constraint Error Mapping

sqlc-generated code returns `pgx` errors. The handler layer maps PostgreSQL unique violation errors (code `23505`) to the appropriate application error:

```go
func mapPgError(err error, field string) (int, string, string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch {
		case strings.Contains(pgErr.ConstraintName, "slug"):
			return 409, "SLUG_TAKEN", "A squad with this slug already exists"
		case strings.Contains(pgErr.ConstraintName, "issue_prefix"):
			return 409, "ISSUE_PREFIX_TAKEN", "A squad with this issue prefix already exists"
		case strings.Contains(pgErr.ConstraintName, "user_squad"):
			return 409, "MEMBER_EXISTS", "User is already a member of this squad"
		}
	}
	return 500, "INTERNAL_ERROR", "An unexpected error occurred"
}
```

---

## 11. Security Considerations

### 11.1 Authorization Model

| Operation | Required Role |
|-----------|--------------|
| Create squad | Any authenticated user |
| View squad | Any member (owner, admin, viewer) |
| Update squad fields | owner, admin |
| Delete (archive) squad | owner |
| Update budget | owner |
| List members | Any member |
| Add member | owner, admin |
| Grant owner/admin role | owner |
| Change member role | owner |
| Remove member | owner |
| Leave squad | Any member (with last-owner check) |

### 11.2 Data Isolation

- Squad membership is checked at the middleware layer before any handler runs
- Non-members receive 404 (not 403) to avoid information leakage (REQ-SM-031)
- All database queries include `squad_id` in the WHERE clause
- No admin/superuser bypass for squad scoping in the API layer

### 11.3 Input Validation

- All string inputs are trimmed and length-checked
- Slug is generated server-side (not user-controlled)
- Issue prefix is validated against `^[A-Z0-9]{2,10}$`
- JSONB settings are validated against a known key whitelist
- UUID path parameters are parsed and validated before database queries
- Budget values are checked for positive integers

---

## 12. Performance Considerations

### 12.1 Indexes

| Table | Index | Purpose |
|-------|-------|---------|
| `squads` | `PRIMARY KEY (id)` | Lookup by ID |
| `squads` | `UNIQUE (slug)` | Slug uniqueness and lookup |
| `squads` | `UNIQUE (issue_prefix)` | Prefix uniqueness |
| `squads` | `idx_squads_status` | Filter by status |
| `squad_memberships` | `PRIMARY KEY (id)` | Lookup by ID |
| `squad_memberships` | `UNIQUE (user_id, squad_id)` | Membership uniqueness and fast auth check |
| `squad_memberships` | `idx_squad_memberships_squad_id` | List members of a squad |
| `squad_memberships` | `idx_squad_memberships_user_id` | List squads for a user |

### 12.2 Performance Targets

- Squad CRUD operations: < 200ms (p95) (REQ-SM-090)
- Squad list with membership join: < 200ms (p95) for up to 100 squads
- Issue counter increment: < 50ms (p95) -- single row UPDATE with row lock
- Membership check middleware: < 10ms (p95) -- indexed lookup on (user_id, squad_id)

### 12.3 Pagination

The list endpoint uses `LIMIT`/`OFFSET` pagination (REQ-SM-091):
- Default limit: 50
- Maximum limit: 100
- Offset: 0-based

For Phase 1 this is sufficient. Cursor-based pagination can be added later if squad counts grow large.

---

## 13. Testing Strategy

### 13.1 Unit Tests -- `internal/domain/squad_test.go`

**Coverage target: 90%+ for domain package**

| Test | What it validates |
|------|-------------------|
| `TestSquadStatus_Valid` | All valid/invalid status values |
| `TestSquadStatus_ValidTransition` | All allowed and disallowed status transitions |
| `TestMemberRole_Valid` | All valid/invalid role values |
| `TestMemberRole_CanGrantRole` | Role permission matrix |
| `TestGenerateSlug` | Name-to-slug conversion: spaces, special chars, unicode, short names |
| `TestValidateSquadName` | Empty, too long, valid names |
| `TestValidateSquadSlug` | Regex matching, length bounds |
| `TestValidateIssuePrefix` | Uppercase alphanum, length bounds |
| `TestValidateBudget` | Nil, zero, negative, positive values |
| `TestValidateBrandColor` | Valid hex, invalid formats |
| `TestSquadSettings_Merge` | Partial merge, nil fields skipped |
| `TestValidateSettingsKeys` | Known keys pass, unknown keys rejected |

### 13.2 Integration Tests -- `internal/server/handlers/squad_handler_test.go`

**Run against real PostgreSQL (embedded-postgres-go in test mode)**

| Test | What it validates |
|------|-------------------|
| `TestCreateSquad_Success` | Happy path: creates squad + owner membership |
| `TestCreateSquad_DuplicatePrefix` | Returns 409 ISSUE_PREFIX_TAKEN |
| `TestCreateSquad_DuplicateSlug` | Returns 409 SLUG_TAKEN |
| `TestCreateSquad_InvalidInput` | Returns 400 for missing name, bad prefix |
| `TestListSquads_OnlyMySquads` | User A sees only their squads, not User B's |
| `TestListSquads_Pagination` | Limit/offset work correctly |
| `TestGetSquad_NonMember` | Returns 404, not 403 |
| `TestUpdateSquad_OwnerSuccess` | Owner can update all fields |
| `TestUpdateSquad_ViewerForbidden` | Viewer gets 403 |
| `TestUpdateSquad_InvalidTransition` | archived -> active returns 400 |
| `TestUpdateSquad_SettingsMerge` | Partial settings update preserves existing keys |
| `TestDeleteSquad_OwnerSuccess` | Soft-deletes to archived status |
| `TestDeleteSquad_NotOwner` | Admin/viewer gets 403 |
| `TestUpdateBudget_OwnerOnly` | Admin gets 403, owner succeeds |
| `TestUpdateBudget_NullUnlimited` | Setting null removes budget limit |

### 13.3 Integration Tests -- `internal/server/handlers/membership_handler_test.go`

| Test | What it validates |
|------|-------------------|
| `TestAddMember_Success` | Owner adds viewer |
| `TestAddMember_AdminAddsViewer` | Admin can add viewers |
| `TestAddMember_AdminCannotAddOwner` | Admin adding owner role returns 403 |
| `TestAddMember_Duplicate` | Returns 409 MEMBER_EXISTS |
| `TestUpdateRole_OwnerSuccess` | Owner changes admin to viewer |
| `TestUpdateRole_LastOwner` | Demoting last owner returns 409 |
| `TestRemoveMember_Success` | Owner removes viewer |
| `TestRemoveMember_LastOwner` | Removing last owner returns 409 |
| `TestLeaveSquad_Success` | Non-owner leaves |
| `TestLeaveSquad_LastOwner` | Last owner cannot leave |

### 13.4 Concurrency Tests

| Test | What it validates |
|------|-------------------|
| `TestIssueCounter_ConcurrentIncrement` | 100 goroutines increment simultaneously; all identifiers unique, counter = 100 |

```go
func TestIssueCounter_ConcurrentIncrement(t *testing.T) {
	// Setup: create a squad with issue_counter = 0

	var wg sync.WaitGroup
	results := make(chan string, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := queries.IncrementIssueCounter(ctx, squadID)
			require.NoError(t, err)
			results <- fmt.Sprintf("%s-%d", result.IssuePrefix, result.IssueCounter)
		}()
	}

	wg.Wait()
	close(results)

	seen := make(map[string]bool)
	for id := range results {
		require.False(t, seen[id], "duplicate identifier: %s", id)
		seen[id] = true
	}
	require.Len(t, seen, 100)
}
```

---

## 14. Activity Logging

All squad mutations are recorded in the activity log (REQ-SM-093). Activity log entries are created within the same transaction as the mutation:

| Action | Actor | Details |
|--------|-------|---------|
| `squad.created` | User | squad ID, name, issuePrefix |
| `squad.updated` | User | squad ID, changed fields |
| `squad.archived` | User | squad ID |
| `squad.budget_updated` | User | squad ID, old budget, new budget |
| `membership.added` | User | squad ID, target user ID, role |
| `membership.role_changed` | User | squad ID, target user ID, old role, new role |
| `membership.removed` | User | squad ID, target user ID |
| `membership.left` | User | squad ID |

---

## 15. Open Questions

- [x] Slug collision strategy: attempt INSERT, handle UNIQUE violation by appending numeric suffix (`-2`, `-3`) and retrying -- decided (no pre-check SELECT to avoid TOCTOU race)
- [x] Should archived squads be visible in the list endpoint with a filter, or completely hidden? **Decision: Hidden by default. `GET /api/squads` returns only active/paused squads. A `?status=archived` filter can be added later.**
- [x] Should the `DELETE /api/squads/{id}/members/me` route take precedence over `DELETE /api/squads/{id}/members/{memberId}` when `{memberId}` equals `"me"`? **Decision: Yes. Go 1.22+ ServeMux matches more-specific literal patterns (`/me`) over wildcards (`/{memberId}`) automatically. Registration order is irrelevant.**
- [ ] Should we add a `GET /api/squads/{id}/budget` endpoint that returns current spend vs. budget, or defer to a dedicated cost/analytics feature?
- [x] How does multi-squad switching work? **Decision: Client-side only. No server-side "active squad" state. The UI stores the active squad ID in React Context + localStorage (for persistence across sessions). The backend receives squad ID in every URL path. See Section 1.1.**
- [x] What is the default squad on login? **Decision: UI uses last-visited squad from localStorage. If no history (first login), defaults to the first squad in the list. If user has no squads, shows a "Create your first squad" onboarding flow.**
- [x] Should there be cross-squad aggregation endpoints? **Decision: Not in Phase 1. Each request is squad-scoped. A future "dashboard overview" feature may aggregate across squads, but this is out of scope for v0.1.**
- [x] Can multiple browser tabs work on different squads? **Decision: Yes. The backend is stateless w.r.t. squad context. Each tab sends its own squad ID in the URL.**

---

## 16. Alternatives Considered

### Alternative 1: Separate `squad_settings` Table

**Description:** Store settings in a normalized key-value table instead of JSONB.

**Pros:**
- Easier to query individual settings
- Standard relational pattern

**Cons:**
- Requires JOIN for every squad fetch
- Harder to add new settings (migration per key)
- More complex partial update logic

**Rejected Because:** JSONB with typed Go structs gives us the best of both worlds -- structured access in Go code with flexible storage in PostgreSQL. Adding new settings only requires updating the Go struct and the key whitelist.

### Alternative 2: Sequence-Based Issue Counter

**Description:** Use a PostgreSQL SEQUENCE per squad for issue counters.

**Pros:**
- Native PostgreSQL pattern for auto-increment
- Very fast

**Cons:**
- Requires dynamic sequence creation/deletion per squad
- Sequences are not transactional (gaps on rollback)
- Harder to query current counter value
- More complex migration and cleanup logic

**Rejected Because:** `UPDATE ... RETURNING` on the squad row is simpler, transactional, and performs well for the expected concurrency levels. Sequences add operational complexity without meaningful performance benefit at our scale.

---

## 17. Timeline Estimate

- Requirements: 1 day -- Complete
- Design: 1 day -- This document
- Implementation: 3-4 days (migrations, sqlc queries, domain types, handlers, middleware, tests)
- Testing: Included in implementation (TDD approach)
- Total: 5-6 days

---

## References

- [Requirements](./requirements.md)
- [PRD -- Section 4.2: Squad, SquadMembership](../../core/01-PRODUCT.md)
- [Go Scaffold](../01-go-scaffold/requirements.md)
- [User Auth](../02-user-auth/requirements.md)
