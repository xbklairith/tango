# Design: Agent Management

**Created:** 2026-03-14
**Status:** Draft

## Architecture Overview

Agent Management is the core entity layer that models AI agents within squads. It implements CRUD operations, a strict tree hierarchy (captain > lead > member), and a finite-state status machine governing agent lifecycle. The feature sits between the squad management layer (which provides organizational scope and governance settings) and the future adapter/runtime layer (which will execute agents). All agent operations are squad-scoped, enforcing data isolation through mandatory `squad_id` filtering on every query.

## System Context

How this feature integrates with the existing system:

- **Depends On:** 01-go-scaffold (HTTP server, router, middleware), 02-user-auth (JWT auth, session middleware), 03-squad-management (squads table, squad membership authorization, squad settings)
- **Used By:** 05-issue-tracking (agent assignment), adapter layer (agent config for runtime), SSE real-time (agent status change events), cost tracking (per-agent budget enforcement)
- **External Dependencies:** None (all logic is internal; adapter execution is a future feature)

## Component Structure

### Components

#### 1. Agent Domain Model (`internal/domain/agent.go`)

**Responsibility:** Define agent types, enums, status machine logic, and hierarchy validation rules as pure Go functions with no database dependencies.

**Dependencies:**
- None (pure domain logic)

**Public Interface:**
```go
package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AgentRole represents the role of an agent in a squad hierarchy.
type AgentRole string

const (
	AgentRoleCaptain AgentRole = "captain"
	AgentRoleLead    AgentRole = "lead"
	AgentRoleMember  AgentRole = "member"
)

// ValidAgentRoles is the set of all valid agent roles.
var ValidAgentRoles = map[AgentRole]bool{
	AgentRoleCaptain: true,
	AgentRoleLead:    true,
	AgentRoleMember:  true,
}

// AgentStatus represents the lifecycle status of an agent.
type AgentStatus string

const (
	AgentStatusPendingApproval AgentStatus = "pending_approval"
	AgentStatusActive          AgentStatus = "active"
	AgentStatusIdle            AgentStatus = "idle"
	AgentStatusRunning         AgentStatus = "running"
	AgentStatusError           AgentStatus = "error"
	AgentStatusPaused          AgentStatus = "paused"
	AgentStatusTerminated      AgentStatus = "terminated"
)

// AdapterType represents the type of AI runtime adapter.
type AdapterType string

const (
	AdapterTypeClaudeLocal   AdapterType = "claude_local"
	AdapterTypeCodexLocal    AdapterType = "codex_local"
	AdapterTypeCursor        AdapterType = "cursor"
	AdapterTypeProcess       AdapterType = "process"
	AdapterTypeHTTP          AdapterType = "http"
	AdapterTypeOpenClawGW    AdapterType = "openclaw_gateway"
)

// Agent is the domain model for an AI agent.
type Agent struct {
	ID                uuid.UUID       `json:"id"`
	SquadID           uuid.UUID       `json:"squadId"`
	Name              string          `json:"name"`
	ShortName         string          `json:"shortName"`
	Role              AgentRole       `json:"role"`
	Status            AgentStatus     `json:"status"`
	ParentAgentID     *uuid.UUID      `json:"parentAgentId,omitempty"`
	AdapterType       *AdapterType    `json:"adapterType,omitempty"`
	AdapterConfig     json.RawMessage `json:"adapterConfig,omitempty"`
	SystemPrompt      *string         `json:"systemPrompt,omitempty"`
	Model             *string         `json:"model,omitempty"`
	BudgetMonthlyCents *int64         `json:"budgetMonthlyCents,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}
```

**Key Behaviors:**
- Status machine validation via `ValidateStatusTransition`
- Hierarchy constraint checking via `ValidateHierarchy`
- Input validation for name, shortName, role, budgetMonthlyCents
- All validation returns structured errors suitable for HTTP responses

#### 2. Agent Status Machine (`internal/domain/agent.go`)

**Responsibility:** Enforce valid state transitions for agent lifecycle, ensuring no invalid status changes occur.

**Dependencies:**
- None (pure function)

**Public Interface:**
```go
// validTransitions defines the allowed status transitions.
// The key is the current status; the value is the set of statuses it can transition to.
// "terminated" is reachable from any status (handled separately).
var validTransitions = map[AgentStatus]map[AgentStatus]bool{
	AgentStatusPendingApproval: {
		AgentStatusActive: true,
		// terminated handled below
	},
	AgentStatusActive: {
		AgentStatusIdle:   true,
		AgentStatusPaused: true,
	},
	AgentStatusIdle: {
		AgentStatusRunning: true,
		AgentStatusPaused:  true,
	},
	AgentStatusRunning: {
		AgentStatusIdle:   true,
		AgentStatusError:  true,
		AgentStatusPaused: true,
	},
	AgentStatusError: {
		// Only terminated is valid from error (handled below)
	},
	AgentStatusPaused: {
		AgentStatusActive: true,
	},
	AgentStatusTerminated: {
		// Terminal state: no transitions out
	},
}

// ValidateStatusTransition checks whether transitioning from `current` to `next` is allowed.
// Returns nil if valid, or an error describing the invalid transition.
func ValidateStatusTransition(current, next AgentStatus) error {
	// Terminated is a terminal state: no transitions out.
	if current == AgentStatusTerminated {
		return fmt.Errorf(
			"invalid status transition: cannot transition from %q (terminal state)",
			current,
		)
	}

	// Any status can transition to terminated.
	if next == AgentStatusTerminated {
		return nil
	}

	allowed, exists := validTransitions[current]
	if !exists {
		return fmt.Errorf("invalid status transition: unknown current status %q", current)
	}

	if !allowed[next] {
		return fmt.Errorf(
			"invalid status transition: cannot transition from %q to %q",
			current, next,
		)
	}

	return nil
}
```

**Key Behaviors:**
- Any status can transition to `terminated` (except from `terminated` itself)
- `terminated` is a terminal state with no outgoing transitions
- `pending_approval` can only transition to `active` or `terminated`
- `error` can only transition to `terminated`
- Returns descriptive errors used to build HTTP 400 responses

#### 3. Hierarchy Validation (`internal/domain/agent.go`)

**Responsibility:** Enforce the strict tree hierarchy rules for agents within a squad.

**Dependencies:**
- None (pure function operating on domain types)

**Public Interface:**
```go
// HierarchyContext provides the information needed to validate an agent's
// position in the hierarchy. Callers populate this from the database.
type HierarchyContext struct {
	// Role of the agent being validated.
	Role AgentRole

	// ParentAgentID is the proposed parent. Nil means no parent.
	ParentAgentID *uuid.UUID

	// ParentRole is the role of the parent agent (if ParentAgentID is set).
	// Callers must look this up from the DB before calling.
	ParentRole *AgentRole

	// ParentSquadID is the squad_id of the parent agent.
	// Used to verify parent is in the same squad.
	ParentSquadID *uuid.UUID

	// SquadID is the squad the agent belongs to.
	SquadID uuid.UUID

	// ExistingCaptainID is the UUID of the current captain in the squad, if any.
	// Nil means no captain exists yet.
	ExistingCaptainID *uuid.UUID

	// AgentID is the ID of the agent being validated (for update operations).
	// Nil for create operations.
	AgentID *uuid.UUID
}

// ValidateHierarchy checks that the agent's role and parent assignment
// conform to the strict tree hierarchy rules.
func ValidateHierarchy(ctx HierarchyContext) error {
	switch ctx.Role {
	case AgentRoleCaptain:
		// REQ-AGT-011: Captain must have no parent.
		if ctx.ParentAgentID != nil {
			return fmt.Errorf("captain must not have a parent agent")
		}
		// REQ-AGT-014: Only one captain per squad.
		if ctx.ExistingCaptainID != nil {
			// If updating an existing captain, allow it (same agent).
			if ctx.AgentID == nil || *ctx.AgentID != *ctx.ExistingCaptainID {
				return fmt.Errorf("squad already has a captain; only one captain is allowed per squad")
			}
		}

	case AgentRoleLead:
		// REQ-AGT-012: Lead's parent must be a captain in the same squad.
		if ctx.ParentAgentID == nil {
			return fmt.Errorf("lead must have a parent agent (captain)")
		}
		if ctx.ParentRole == nil || *ctx.ParentRole != AgentRoleCaptain {
			return fmt.Errorf("lead's parent must be a captain, got %v", ctx.ParentRole)
		}
		if ctx.ParentSquadID == nil || *ctx.ParentSquadID != ctx.SquadID {
			return fmt.Errorf("parent agent must be in the same squad")
		}

	case AgentRoleMember:
		// REQ-AGT-013: Member's parent must be a lead in the same squad.
		if ctx.ParentAgentID == nil {
			return fmt.Errorf("member must have a parent agent (lead)")
		}
		if ctx.ParentRole == nil || *ctx.ParentRole != AgentRoleLead {
			return fmt.Errorf("member's parent must be a lead, got %v", ctx.ParentRole)
		}
		if ctx.ParentSquadID == nil || *ctx.ParentSquadID != ctx.SquadID {
			return fmt.Errorf("parent agent must be in the same squad")
		}

	default:
		return fmt.Errorf("invalid agent role: %q", ctx.Role)
	}

	return nil
}
```

**Key Behaviors:**
- Captain: no parent, exactly one per squad
- Lead: parent must be a captain in the same squad
- Member: parent must be a lead in the same squad
- Cross-squad parent references are rejected
- On update, the existing captain check allows the same agent to remain captain

#### 4. Cycle Detection (Database-level)

**Responsibility:** Prevent cycles in the agent hierarchy tree when updating `parent_agent_id`.

**Implementation:**
```sql
-- Recursive CTE to detect cycles when updating parent_agent_id.
-- Given an agent @agent_id proposing to set parent to @new_parent_id,
-- walk up the ancestor chain from @new_parent_id. If we encounter
-- @agent_id in the chain, a cycle would be created.

WITH RECURSIVE ancestors AS (
    SELECT id, parent_agent_id, 1 AS depth
    FROM agents
    WHERE id = @new_parent_id

    UNION ALL

    SELECT a.id, a.parent_agent_id, anc.depth + 1
    FROM agents a
    JOIN ancestors anc ON a.id = anc.parent_agent_id
    WHERE anc.depth < 10  -- Safety limit; hierarchy is max 3 levels deep
)
SELECT EXISTS (
    SELECT 1 FROM ancestors WHERE id = @agent_id
) AS would_cycle;
```

In practice, the strict role-based hierarchy (captain > lead > member, max 3 levels) makes cycles structurally impossible when hierarchy validation passes. The recursive CTE is a defense-in-depth check used during parent updates.

#### 5. Agent Handler (`internal/server/handlers/agent_handler.go`)

**Responsibility:** HTTP handlers for all Agent CRUD and status transition endpoints. Parses requests, calls domain validation, executes database queries, returns JSON responses.

**Dependencies:**
- `internal/domain` (validation logic)
- `internal/database/db` (sqlc-generated query interface)
- `internal/server/middleware` (auth context extraction)

**Public Interface:**
```go
package handlers

import (
	"net/http"

	"ari/internal/database/db"
)

// AgentHandler holds dependencies for agent HTTP handlers.
type AgentHandler struct {
	queries db.Querier
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(queries db.Querier) *AgentHandler {
	return &AgentHandler{queries: queries}
}

// RegisterRoutes registers agent routes on the given mux.
func (h *AgentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/agents", h.CreateAgent)
	mux.HandleFunc("GET /api/agents", h.ListAgents)
	mux.HandleFunc("GET /api/agents/{id}", h.GetAgent)
	mux.HandleFunc("PATCH /api/agents/{id}", h.UpdateAgent)
	mux.HandleFunc("POST /api/agents/{id}/transition", h.TransitionAgentStatus)
}
```

**Key Behaviors:**
- All handlers extract `user_id` from auth middleware context
- All handlers verify squad membership before proceeding
- Create validates hierarchy, checks squad settings for `requireApprovalForNewAgents`
- Update re-validates hierarchy when role or parentAgentId changes
- Status transition uses a dedicated endpoint for clarity (also supported via PATCH)

#### 6. Agent Queries (`internal/database/queries/agents.sql`)

**Responsibility:** SQL queries consumed by sqlc to generate type-safe Go database access code.

**Dependencies:**
- `agents` table (migration)
- `squads` table (FK reference)
- `squad_memberships` table (authorization checks)

## Database Schema

### Migration: `internal/database/migrations/XXXXXX_create_agents.sql`

```sql
-- +goose Up

CREATE TYPE agent_role AS ENUM ('captain', 'lead', 'member');

CREATE TYPE agent_status AS ENUM (
    'pending_approval',
    'active',
    'idle',
    'running',
    'error',
    'paused',
    'terminated'
);

CREATE TYPE adapter_type AS ENUM (
    'claude_local',
    'codex_local',
    'cursor',
    'process',
    'http',
    'openclaw_gateway'
);

CREATE TABLE agents (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id            UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    short_name          VARCHAR(50) NOT NULL,
    role                agent_role NOT NULL,
    status              agent_status NOT NULL DEFAULT 'active',
    parent_agent_id     UUID REFERENCES agents(id) ON DELETE SET NULL,
    adapter_type        adapter_type,
    adapter_config      JSONB DEFAULT '{}',
    system_prompt       TEXT,
    model               VARCHAR(100),
    budget_monthly_cents BIGINT CHECK (budget_monthly_cents IS NULL OR budget_monthly_cents >= 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- REQ-AGT-NF-003: Unique short_name within a squad.
CREATE UNIQUE INDEX idx_agents_squad_short_name ON agents(squad_id, short_name);

-- REQ-AGT-NF-002: Index on squad_id for list queries.
CREATE INDEX idx_agents_squad_id ON agents(squad_id);

-- REQ-AGT-NF-002: Index on parent_agent_id for hierarchy queries.
CREATE INDEX idx_agents_parent_agent_id ON agents(parent_agent_id);

-- REQ-AGT-014: Only one captain per squad (partial unique index).
CREATE UNIQUE INDEX idx_agents_one_captain_per_squad
    ON agents(squad_id)
    WHERE role = 'captain' AND status != 'terminated';

-- Auto-update updated_at on row modification.
CREATE OR REPLACE FUNCTION update_agents_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_agents_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE FUNCTION update_agents_updated_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_agents_updated_at ON agents;
DROP FUNCTION IF EXISTS update_agents_updated_at;
DROP TABLE IF EXISTS agents;
DROP TYPE IF EXISTS adapter_type;
DROP TYPE IF EXISTS agent_status;
DROP TYPE IF EXISTS agent_role;
```

**Design Decisions:**
- `ON DELETE SET NULL` for `parent_agent_id`: if a parent is deleted, children become orphans rather than cascading deletion. The application layer should reassign or terminate children before removing a parent.
- `ON DELETE CASCADE` for `squad_id`: if a squad is deleted, all its agents are removed.
- Partial unique index `idx_agents_one_captain_per_squad` enforces the one-captain-per-squad invariant at the database level, excluding terminated agents.
- `budget_monthly_cents` uses `BIGINT` to avoid integer overflow on large budgets.
- `adapter_config` defaults to empty JSON object `'{}'` rather than NULL for simpler downstream handling.

**PRD Field Name Mapping:**
The following field names diverge from the PRD (Section 4.2, Agent entity) for clarity. The mapping is:

| PRD Name | Design Name | Rationale |
|----------|-------------|-----------|
| `urlKey` | `short_name` | "shortName" is more descriptive of purpose; used as unique identifier within squad |
| `reportsTo` | `parent_agent_id` | Explicit self-referencing FK naming; clearer in SQL queries |

**PRD Fields Deferred to Later Phases:**
The following PRD-defined Agent fields are intentionally excluded from Phase 1. They will be added in subsequent features/migrations:

| PRD Field | Type | Deferred Reason |
|-----------|------|-----------------|
| `title` | string | Position title display; cosmetic, not required for core agent lifecycle |
| `capabilities` | text | Agent capability descriptions; useful for marketplace/sharing (Phase 2+) |
| `runtimeConfig` | JSONB | Runtime parameters; deferred until adapter execution feature is built |
| `permissions` | JSONB | `{canCreateAgents: bool}` etc.; deferred until agent-level RBAC is implemented |
| `lastHeartbeatAt` | timestamp | Last execution time; deferred until heartbeat scheduler feature is built |

Note: `systemPrompt` and `model` are included in this design but are not explicitly listed in the PRD Agent entity table. They are necessary for adapter configuration and were added during detailed design.

### sqlc Queries: `internal/database/queries/agents.sql`

```sql
-- name: CreateAgent :one
INSERT INTO agents (
    squad_id, name, short_name, role, status,
    parent_agent_id, adapter_type, adapter_config,
    system_prompt, model, budget_monthly_cents
) VALUES (
    @squad_id, @name, @short_name, @role, @status,
    @parent_agent_id, @adapter_type, @adapter_config,
    @system_prompt, @model, @budget_monthly_cents
)
RETURNING *;

-- name: GetAgentByID :one
SELECT * FROM agents
WHERE id = @id;

-- name: ListAgentsBySquad :many
SELECT * FROM agents
WHERE squad_id = @squad_id
ORDER BY created_at ASC;

-- name: UpdateAgent :one
UPDATE agents SET
    name = COALESCE(sqlc.narg('name'), name),
    short_name = COALESCE(sqlc.narg('short_name'), short_name),
    role = COALESCE(sqlc.narg('role'), role),
    status = COALESCE(sqlc.narg('status'), status),
    parent_agent_id = CASE
        WHEN sqlc.arg('set_parent') THEN sqlc.narg('parent_agent_id')
        ELSE parent_agent_id
    END,
    adapter_type = COALESCE(sqlc.narg('adapter_type'), adapter_type),
    adapter_config = COALESCE(sqlc.narg('adapter_config'), adapter_config),
    system_prompt = COALESCE(sqlc.narg('system_prompt'), system_prompt),
    model = COALESCE(sqlc.narg('model'), model),
    budget_monthly_cents = CASE
        WHEN sqlc.arg('set_budget') THEN sqlc.narg('budget_monthly_cents')
        ELSE budget_monthly_cents
    END
WHERE id = @id
RETURNING *;

-- name: GetSquadCaptain :one
SELECT * FROM agents
WHERE squad_id = @squad_id
  AND role = 'captain'
  AND status != 'terminated'
LIMIT 1;

-- name: GetAgentParent :one
SELECT id, squad_id, role FROM agents
WHERE id = @id;

-- name: CheckCycleInHierarchy :one
-- Walks up the ancestor chain from @start_id to check if @target_id is an ancestor.
WITH RECURSIVE ancestors AS (
    SELECT id, parent_agent_id, 1 AS depth
    FROM agents
    WHERE id = @start_id

    UNION ALL

    SELECT a.id, a.parent_agent_id, anc.depth + 1
    FROM agents a
    JOIN ancestors anc ON a.id = anc.parent_agent_id
    WHERE anc.depth < 10
)
SELECT EXISTS (
    SELECT 1 FROM ancestors WHERE id = @target_id
) AS would_cycle;

-- name: CountAgentsBySquad :one
SELECT COUNT(*) FROM agents
WHERE squad_id = @squad_id
  AND status != 'terminated';

-- name: ListAgentChildren :many
SELECT * FROM agents
WHERE parent_agent_id = @parent_agent_id
ORDER BY created_at ASC;
```

**COALESCE/sqlc.narg Partial Update Pattern -- Design Note:**

The `UpdateAgent` query uses `COALESCE(sqlc.narg('field'), existing_field)` for most fields. This pattern treats SQL NULL as "not provided" -- if the caller passes NULL, COALESCE falls through to the existing value, meaning the field is left unchanged.

**Limitation:** For nullable fields (`adapter_type`, `system_prompt`, `model`), this approach cannot distinguish between "not provided" (keep current value) and "explicitly set to NULL" (clear the value). Since `COALESCE(NULL, existing)` always returns `existing`, there is no way to clear these fields via the COALESCE pattern alone.

**Chosen approach for nullable fields that need clearing:**
- `parent_agent_id` and `budget_monthly_cents` use the **sentinel boolean pattern**: `CASE WHEN sqlc.arg('set_parent') THEN sqlc.narg('parent_agent_id') ELSE parent_agent_id END`. The caller sets `set_parent = true` to signal "I am providing a value (which may be NULL to clear)".
- `adapter_type`, `system_prompt`, and `model` use plain COALESCE because clearing them to NULL is not a supported operation in Phase 1. If this becomes a requirement, these fields should be migrated to the sentinel boolean pattern.
- `adapter_config` uses COALESCE and defaults to `'{}'` (empty JSON), so clearing is done by sending `{}` rather than NULL.

## API Contracts

### POST /api/agents -- Create Agent

**Purpose:** Create a new agent within a squad.

**Request:**
```json
{
  "squadId": "uuid",
  "name": "Alice",
  "shortName": "alice",
  "role": "captain",
  "parentAgentId": null,
  "adapterType": "claude_local",
  "adapterConfig": {"workDir": "/workspace"},
  "systemPrompt": "You are Alice, the team captain.",
  "model": "claude-opus-4-6",
  "budgetMonthlyCents": 50000
}
```

**Response (201 Created):**
```json
{
  "id": "a1b2c3d4-...",
  "squadId": "s1q2u3a4-...",
  "name": "Alice",
  "shortName": "alice",
  "role": "captain",
  "status": "active",
  "parentAgentId": null,
  "adapterType": "claude_local",
  "adapterConfig": {"workDir": "/workspace"},
  "systemPrompt": "You are Alice, the team captain.",
  "model": "claude-opus-4-6",
  "budgetMonthlyCents": 50000,
  "createdAt": "2026-03-14T10:00:00Z",
  "updatedAt": "2026-03-14T10:00:00Z"
}
```

**Possible Errors:**
- `400 VALIDATION_ERROR`: Missing required fields, invalid role, invalid shortName format, name too long, negative budget, invalid adapterConfig JSON
- `400 VALIDATION_ERROR`: Hierarchy violation (e.g., lead without captain parent)
- `401 UNAUTHORIZED`: No valid auth token
- `403 FORBIDDEN`: User not a member of the squad
- `409 CONFLICT`: Duplicate shortName within squad
- `409 CONFLICT`: Squad already has a captain

---

### GET /api/agents?squadId={squadId} -- List Agents

**Purpose:** List all agents in a squad.

**Request:** Query parameter `squadId` (required).

**Response (200 OK):**
```json
[
  {
    "id": "a1b2c3d4-...",
    "squadId": "s1q2u3a4-...",
    "name": "Alice",
    "shortName": "alice",
    "role": "captain",
    "status": "active",
    "parentAgentId": null,
    "adapterType": "claude_local",
    "adapterConfig": {},
    "systemPrompt": "...",
    "model": "claude-opus-4-6",
    "budgetMonthlyCents": 50000,
    "createdAt": "2026-03-14T10:00:00Z",
    "updatedAt": "2026-03-14T10:00:00Z"
  }
]
```

**Possible Errors:**
- `400 VALIDATION_ERROR`: Missing or invalid squadId
- `401 UNAUTHORIZED`: No valid auth token
- `403 FORBIDDEN`: User not a member of the squad

---

### GET /api/agents/{id} -- Get Agent

**Purpose:** Get a single agent by ID.

**Response (200 OK):** Single agent object (same schema as list item).

**Possible Errors:**
- `401 UNAUTHORIZED`: No valid auth token
- `403 FORBIDDEN`: User not a member of the agent's squad
- `404 NOT_FOUND`: Agent does not exist

---

### PATCH /api/agents/{id} -- Update Agent

**Purpose:** Partially update an agent. Supports updating any mutable field except `squadId`.

**Request:**
```json
{
  "name": "Alice v2",
  "shortName": "alice-v2",
  "role": "lead",
  "parentAgentId": "captain-uuid-here",
  "adapterType": "http",
  "adapterConfig": {"url": "https://example.com/agent"},
  "systemPrompt": "Updated prompt.",
  "model": "claude-sonnet-4-20250514",
  "budgetMonthlyCents": 100000
}
```
All fields are optional. Only provided fields are updated.

**Response (200 OK):** Updated agent object.

**Possible Errors:**
- `400 VALIDATION_ERROR`: Invalid field values, hierarchy violation on role/parent change
- `400 INVALID_STATUS_TRANSITION`: Invalid status transition (if status included)
- `401 UNAUTHORIZED`: No valid auth token
- `403 FORBIDDEN`: User not a member of the agent's squad
- `404 NOT_FOUND`: Agent does not exist
- `409 CONFLICT`: Duplicate shortName, or captain conflict on role change

---

### POST /api/agents/{id}/transition -- Transition Agent Status

**Purpose:** Explicitly transition an agent's status. Separated from PATCH for clarity and auditability.

**Request:**
```json
{
  "status": "paused"
}
```

**Response (200 OK):** Updated agent object with new status.

**Possible Errors:**
- `400 INVALID_STATUS_TRANSITION`: Transition not allowed (includes current and requested status in message)
- `401 UNAUTHORIZED`: No valid auth token
- `403 FORBIDDEN`: User not a member of the agent's squad
- `404 NOT_FOUND`: Agent does not exist

## Agent Status Machine

### State Diagram

```
                        +-----------------+
                        | pending_approval|
                        +--------+--------+
                                 |
                          approve|
                                 v
                        +--------+--------+
               +------->|     active      |<-------+
               |        +--------+--------+        |
               |                 |                  |
          resume|          no work|            resume|
               |                 v                  |
        +------+------+  +------+------+   +-------+------+
        |   paused    |<-+    idle     |   |   paused     |
        +------+------+  +------+------+   +-------+------+
               ^                 |                  ^
               |        trigger  |                  |
               |                 v                  |
               |        +-------+-------+           |
               +--------+    running    +-----------+
                        +---+-------+---+
                            |       |
                      done  |       | fail
                            v       v
                        +---+---+ +-+-----+
                        | idle  | | error |
                        +-------+ +-------+

        *** Any non-terminated status --> terminated ***
```

### Transition Table

| From               | To                 | Trigger                        |
|--------------------|--------------------|--------------------------------|
| pending_approval   | active             | User approves agent            |
| pending_approval   | terminated         | User rejects agent             |
| active             | idle               | No work available              |
| active             | paused             | User pauses or budget exceeded |
| idle               | running            | Heartbeat triggered            |
| idle               | paused             | User pauses or budget exceeded |
| running            | idle               | Heartbeat completed            |
| running            | error              | Heartbeat failed               |
| running            | paused             | User pauses or budget exceeded |
| paused             | active             | User resumes                   |
| error              | terminated         | User removes agent             |
| any (non-terminal) | terminated         | User terminates agent          |

### Go Implementation

The status machine is implemented as a pure function in `internal/domain/agent.go` (shown in Component 2 above). Key design decisions:

1. **Transition map is a package-level variable**, not a method on Agent. This keeps it testable without constructing full Agent instances.
2. **`terminated` is handled as a special case** rather than listing it in every source state's map, reducing duplication.
3. **`error` state has no outgoing transitions** except `terminated`. An agent in error must be explicitly terminated and recreated or investigated. This prevents accidental recovery without human review.
4. **The function returns `error`**, not a boolean, so callers get a descriptive message suitable for HTTP responses.

## Hierarchy Validation

### Rules Summary

| Role    | Parent Required | Parent Role | Max per Squad |
|---------|----------------|-------------|---------------|
| captain | No (must be nil)| N/A        | 1             |
| lead    | Yes            | captain     | Unlimited     |
| member  | Yes            | lead        | Unlimited     |

### Database-Level Enforcement

The partial unique index `idx_agents_one_captain_per_squad` enforces the one-captain-per-squad rule at the database level. This is a safety net in addition to application-level validation.

For the parent role constraints, application-level validation is used (see Component 3) because PostgreSQL check constraints cannot reference other rows. The application fetches the parent agent's role and squad before insert/update.

### Cycle Detection Strategy

Given the strict 3-level hierarchy (captain > lead > member), cycles are structurally impossible when role-based validation passes:
- A captain has no parent (root node).
- A lead's parent is always a captain (level 1 -> level 0).
- A member's parent is always a lead (level 2 -> level 1).

No node can point upward or sideways in a way that creates a cycle. The recursive CTE (`CheckCycleInHierarchy`) is a defense-in-depth measure for cases where role changes and parent changes happen simultaneously, ensuring the invariant holds even under race conditions.

### Hierarchy Validation on Update

When a PATCH request changes `role` or `parentAgentId`:

1. Fetch the current agent from the database.
2. Compute the "effective" new role and parent (merge patch fields with existing values).
3. If parent is changing, fetch the parent agent to get its role and squad_id.
4. Call `ValidateHierarchy` with the merged context.
5. If the agent is becoming a captain, check for existing captain (excluding self).
6. If the agent was a captain and is changing to a different role, check that no children (leads) are orphaned -- return an error if the agent has children.

```go
// ValidateHierarchyChange is called during PATCH when role or parent changes.
// It checks both the new hierarchy position and ensures no children are orphaned.
func ValidateHierarchyChange(
	agent Agent,
	newRole *AgentRole,
	newParentID *uuid.UUID,
	parentInfo *AgentParentInfo, // fetched from DB: role, squad_id
	existingCaptainID *uuid.UUID,
	childCount int,
) error {
	effectiveRole := agent.Role
	if newRole != nil {
		effectiveRole = *newRole
	}

	// If agent was a captain/lead and is changing role, ensure no orphans.
	if newRole != nil && *newRole != agent.Role {
		if childCount > 0 {
			return fmt.Errorf(
				"cannot change role from %q to %q: agent has %d children that would be orphaned",
				agent.Role, *newRole, childCount,
			)
		}
	}

	ctx := HierarchyContext{
		Role:              effectiveRole,
		ParentAgentID:     newParentID,
		SquadID:           agent.SquadID,
		AgentID:           &agent.ID,
		ExistingCaptainID: existingCaptainID,
	}

	if parentInfo != nil {
		ctx.ParentRole = &parentInfo.Role
		ctx.ParentSquadID = &parentInfo.SquadID
	}

	return ValidateHierarchy(ctx)
}

// AgentParentInfo holds the minimal info about a parent agent needed for validation.
type AgentParentInfo struct {
	Role    AgentRole
	SquadID uuid.UUID
}
```

## Data Flow

### Agent Creation Flow

```
Client (POST /api/agents)
     |
     v
[Auth Middleware] -- extract user_id from JWT
     |
     v
[AgentHandler.CreateAgent]
     |
     +-- 1. Parse & validate request body
     |      - name (required, <= 255 chars)
     |      - shortName (required, <= 50 chars, ^[a-z0-9-]+$)
     |      - role (required, valid enum)
     |      - squadId (required, valid UUID)
     |      - budgetMonthlyCents (optional, >= 0)
     |      - adapterConfig (optional, valid JSON)
     |
     +-- 2. Verify squad membership
     |      - Query squad_memberships WHERE user_id AND squad_id
     |      - 403 if not a member
     |
     +-- 3. Determine initial status
     |      - Query squad settings for requireApprovalForNewAgents
     |      - If true: status = "pending_approval"
     |      - If false: status = "active"
     |
     +-- 4. Validate hierarchy
     |      - If role = captain: check no existing captain
     |      - If role = lead: fetch parent, verify parent.role = captain, same squad
     |      - If role = member: fetch parent, verify parent.role = lead, same squad
     |      - Call domain.ValidateHierarchy(ctx)
     |
     +-- 5. Insert into database
     |      - Call queries.CreateAgent(...)
     |      - Handle unique constraint violation (shortName conflict) -> 409
     |      - Handle captain partial unique index violation -> 409
     |
     +-- 6. Return 201 with created agent
```

### Status Transition Flow

```
Client (POST /api/agents/{id}/transition)
     |
     v
[Auth Middleware]
     |
     v
[AgentHandler.TransitionAgentStatus]
     |
     +-- 1. Parse request body: { "status": "paused" }
     |
     +-- 2. Fetch agent by ID (404 if not found)
     |
     +-- 3. Verify squad membership (403 if not authorized)
     |
     +-- 4. Validate transition
     |      - Call domain.ValidateStatusTransition(agent.Status, newStatus)
     |      - 400 INVALID_STATUS_TRANSITION if invalid
     |
     +-- 5. Update agent status in database
     |      - Call queries.UpdateAgent with status field
     |
     +-- 6. Return 200 with updated agent
```

### Agent Update Flow (with hierarchy re-validation)

```
Client (PATCH /api/agents/{id})
     |
     v
[Auth Middleware]
     |
     v
[AgentHandler.UpdateAgent]
     |
     +-- 1. Parse request body (all fields optional)
     |
     +-- 2. Fetch existing agent (404 if not found)
     |
     +-- 3. Verify squad membership (403 if not authorized)
     |
     +-- 4. If "status" in request:
     |      - Call domain.ValidateStatusTransition
     |
     +-- 5. If "role" or "parentAgentId" in request:
     |      - Fetch parent agent info (if parentAgentId provided)
     |      - Fetch existing captain (if role changing to captain)
     |      - Count children (if role changing from captain/lead)
     |      - Call domain.ValidateHierarchyChange
     |      - Run cycle detection CTE (defense-in-depth)
     |
     +-- 6. If "shortName" in request:
     |      - Validate format (^[a-z0-9-]+$, <= 50 chars)
     |
     +-- 7. Update in database
     |      - Handle constraint violations -> 409
     |
     +-- 8. Return 200 with updated agent
```

## Error Handling

### Error Response Format

All errors follow the standard Ari error format:

```go
type APIError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}
```

### Error Codes

| HTTP | Code                       | When                                                    |
|------|----------------------------|---------------------------------------------------------|
| 400  | VALIDATION_ERROR           | Missing fields, invalid format, hierarchy violation     |
| 400  | INVALID_STATUS_TRANSITION  | Status transition not allowed by the state machine      |
| 401  | UNAUTHORIZED               | No valid auth token                                     |
| 403  | FORBIDDEN                  | User not a member of the agent's squad                  |
| 404  | NOT_FOUND                  | Agent or referenced entity does not exist               |
| 409  | CONFLICT                   | Duplicate shortName or second captain in squad          |
| 500  | INTERNAL_ERROR             | Unexpected database or server error                     |

### Input Validation Rules

```go
import (
	"fmt"
	"regexp"
)

var shortNameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateCreateAgentInput validates the input for creating an agent.
func ValidateCreateAgentInput(input CreateAgentRequest) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 255 {
		return fmt.Errorf("name must not exceed 255 characters")
	}
	if input.ShortName == "" {
		return fmt.Errorf("shortName is required")
	}
	if len(input.ShortName) > 50 {
		return fmt.Errorf("shortName must not exceed 50 characters")
	}
	if !shortNameRegex.MatchString(input.ShortName) {
		return fmt.Errorf("shortName must contain only lowercase alphanumeric characters and hyphens")
	}
	if !ValidAgentRoles[input.Role] {
		return fmt.Errorf("role must be one of: captain, lead, member")
	}
	if input.BudgetMonthlyCents != nil && *input.BudgetMonthlyCents < 0 {
		return fmt.Errorf("budgetMonthlyCents must be a non-negative integer")
	}
	if input.AdapterConfig != nil && !json.Valid(input.AdapterConfig) {
		return fmt.Errorf("adapterConfig must be valid JSON")
	}
	return nil
}

// CreateAgentRequest is the parsed request body for POST /api/agents.
type CreateAgentRequest struct {
	SquadID            uuid.UUID       `json:"squadId"`
	Name               string          `json:"name"`
	ShortName          string          `json:"shortName"`
	Role               AgentRole       `json:"role"`
	ParentAgentID      *uuid.UUID      `json:"parentAgentId,omitempty"`
	AdapterType        *AdapterType    `json:"adapterType,omitempty"`
	AdapterConfig      json.RawMessage `json:"adapterConfig,omitempty"`
	SystemPrompt       *string         `json:"systemPrompt,omitempty"`
	Model              *string         `json:"model,omitempty"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents,omitempty"`
}

// UpdateAgentRequest is the parsed request body for PATCH /api/agents/{id}.
type UpdateAgentRequest struct {
	Name               *string         `json:"name,omitempty"`
	ShortName          *string         `json:"shortName,omitempty"`
	Role               *AgentRole      `json:"role,omitempty"`
	Status             *AgentStatus    `json:"status,omitempty"`
	ParentAgentID      *uuid.UUID      `json:"parentAgentId,omitempty"`
	SetParent          bool            `json:"-"` // true when parentAgentId key is present in JSON
	AdapterType        *AdapterType    `json:"adapterType,omitempty"`
	AdapterConfig      json.RawMessage `json:"adapterConfig,omitempty"`
	SystemPrompt       *string         `json:"systemPrompt,omitempty"`
	Model              *string         `json:"model,omitempty"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents,omitempty"`
	SetBudget          bool            `json:"-"` // true when budgetMonthlyCents key is present in JSON
}

// TransitionRequest is the parsed request body for POST /api/agents/{id}/transition.
type TransitionRequest struct {
	Status AgentStatus `json:"status"`
}
```

## Security Considerations

### Authentication and Authorization

- All agent endpoints require a valid JWT token (enforced by auth middleware).
- Squad membership is verified on every request by querying `squad_memberships` for the authenticated user and the agent's `squad_id`.
- The system returns 403 (not 404) when a user lacks squad access, per REQ-AGT-052. Note: for future consideration, REQ-SM-031 suggests returning 404 to avoid leaking squad existence. The agent endpoints use 403 since the squad is already known to exist (referenced by the agent).

### Input Validation

- All string inputs are length-bounded.
- `shortName` is restricted to `^[a-z0-9-]+$` to prevent injection and ensure URL safety.
- `adapterConfig` is validated as well-formed JSON before storage.
- UUID parameters are parsed with strict UUID validation.
- `squadId` cannot be changed after creation (REQ-AGT-042).

## Testing Strategy

### Unit Tests: Status Machine (`internal/domain/agent_test.go`)

```go
package domain_test

import (
	"testing"

	"ari/internal/domain"
)

func TestValidateStatusTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    domain.AgentStatus
		to      domain.AgentStatus
		wantErr bool
	}{
		// Valid transitions
		{"pending_approval -> active", domain.AgentStatusPendingApproval, domain.AgentStatusActive, false},
		{"active -> idle", domain.AgentStatusActive, domain.AgentStatusIdle, false},
		{"idle -> running", domain.AgentStatusIdle, domain.AgentStatusRunning, false},
		{"running -> idle", domain.AgentStatusRunning, domain.AgentStatusIdle, false},
		{"running -> error", domain.AgentStatusRunning, domain.AgentStatusError, false},
		{"active -> paused", domain.AgentStatusActive, domain.AgentStatusPaused, false},
		{"idle -> paused", domain.AgentStatusIdle, domain.AgentStatusPaused, false},
		{"running -> paused", domain.AgentStatusRunning, domain.AgentStatusPaused, false},
		{"paused -> active", domain.AgentStatusPaused, domain.AgentStatusActive, false},

		// Any -> terminated
		{"pending_approval -> terminated", domain.AgentStatusPendingApproval, domain.AgentStatusTerminated, false},
		{"active -> terminated", domain.AgentStatusActive, domain.AgentStatusTerminated, false},
		{"idle -> terminated", domain.AgentStatusIdle, domain.AgentStatusTerminated, false},
		{"running -> terminated", domain.AgentStatusRunning, domain.AgentStatusTerminated, false},
		{"error -> terminated", domain.AgentStatusError, domain.AgentStatusTerminated, false},
		{"paused -> terminated", domain.AgentStatusPaused, domain.AgentStatusTerminated, false},

		// Invalid transitions
		{"terminated -> active", domain.AgentStatusTerminated, domain.AgentStatusActive, true},
		{"terminated -> terminated", domain.AgentStatusTerminated, domain.AgentStatusTerminated, true},
		{"pending_approval -> idle", domain.AgentStatusPendingApproval, domain.AgentStatusIdle, true},
		{"pending_approval -> running", domain.AgentStatusPendingApproval, domain.AgentStatusRunning, true},
		{"pending_approval -> paused", domain.AgentStatusPendingApproval, domain.AgentStatusPaused, true},
		{"active -> running", domain.AgentStatusActive, domain.AgentStatusRunning, true},
		{"active -> error", domain.AgentStatusActive, domain.AgentStatusError, true},
		{"idle -> active", domain.AgentStatusIdle, domain.AgentStatusActive, true},
		{"idle -> error", domain.AgentStatusIdle, domain.AgentStatusError, true},
		{"running -> active", domain.AgentStatusRunning, domain.AgentStatusActive, true},
		{"error -> active", domain.AgentStatusError, domain.AgentStatusActive, true},
		{"error -> idle", domain.AgentStatusError, domain.AgentStatusIdle, true},
		{"error -> running", domain.AgentStatusError, domain.AgentStatusRunning, true},
		{"error -> paused", domain.AgentStatusError, domain.AgentStatusPaused, true},
		{"paused -> idle", domain.AgentStatusPaused, domain.AgentStatusIdle, true},
		{"paused -> running", domain.AgentStatusPaused, domain.AgentStatusRunning, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateStatusTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStatusTransition(%q, %q) error = %v, wantErr %v",
					tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}
```

### Unit Tests: Hierarchy Validation (`internal/domain/agent_test.go`)

```go
func TestValidateHierarchy(t *testing.T) {
	squadID := uuid.New()
	otherSquadID := uuid.New()
	captainID := uuid.New()
	leadID := uuid.New()
	captainRole := domain.AgentRoleCaptain
	leadRole := domain.AgentRoleLead
	memberRole := domain.AgentRoleMember

	tests := []struct {
		name    string
		ctx     domain.HierarchyContext
		wantErr bool
	}{
		{
			name: "captain with no parent, no existing captain",
			ctx: domain.HierarchyContext{
				Role:              domain.AgentRoleCaptain,
				ParentAgentID:     nil,
				SquadID:           squadID,
				ExistingCaptainID: nil,
			},
			wantErr: false,
		},
		{
			name: "captain with parent -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleCaptain,
				ParentAgentID: &captainID,
				SquadID:       squadID,
			},
			wantErr: true,
		},
		{
			name: "second captain in squad -- rejected",
			ctx: domain.HierarchyContext{
				Role:              domain.AgentRoleCaptain,
				ParentAgentID:     nil,
				SquadID:           squadID,
				ExistingCaptainID: &captainID,
				AgentID:           nil, // new agent
			},
			wantErr: true,
		},
		{
			name: "existing captain updating self -- allowed",
			ctx: domain.HierarchyContext{
				Role:              domain.AgentRoleCaptain,
				ParentAgentID:     nil,
				SquadID:           squadID,
				ExistingCaptainID: &captainID,
				AgentID:           &captainID, // same agent
			},
			wantErr: false,
		},
		{
			name: "lead with captain parent in same squad",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleLead,
				ParentAgentID: &captainID,
				ParentRole:    &captainRole,
				ParentSquadID: &squadID,
				SquadID:       squadID,
			},
			wantErr: false,
		},
		{
			name: "lead with no parent -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleLead,
				ParentAgentID: nil,
				SquadID:       squadID,
			},
			wantErr: true,
		},
		{
			name: "lead with member parent -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleLead,
				ParentAgentID: &leadID,
				ParentRole:    &memberRole,
				ParentSquadID: &squadID,
				SquadID:       squadID,
			},
			wantErr: true,
		},
		{
			name: "lead with captain in different squad -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleLead,
				ParentAgentID: &captainID,
				ParentRole:    &captainRole,
				ParentSquadID: &otherSquadID,
				SquadID:       squadID,
			},
			wantErr: true,
		},
		{
			name: "member with lead parent in same squad",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleMember,
				ParentAgentID: &leadID,
				ParentRole:    &leadRole,
				ParentSquadID: &squadID,
				SquadID:       squadID,
			},
			wantErr: false,
		},
		{
			name: "member with captain parent -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleMember,
				ParentAgentID: &captainID,
				ParentRole:    &captainRole,
				ParentSquadID: &squadID,
				SquadID:       squadID,
			},
			wantErr: true,
		},
		{
			name: "member with no parent -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleMember,
				ParentAgentID: nil,
				SquadID:       squadID,
			},
			wantErr: true,
		},
		{
			name: "member with lead in different squad -- rejected",
			ctx: domain.HierarchyContext{
				Role:          domain.AgentRoleMember,
				ParentAgentID: &leadID,
				ParentRole:    &leadRole,
				ParentSquadID: &otherSquadID,
				SquadID:       squadID,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateHierarchy(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHierarchy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

### Integration Tests

**Database Integration Tests (`internal/database/agents_integration_test.go`):**
- Create agent and verify all fields are persisted correctly
- Verify unique constraint on `(squad_id, short_name)` returns conflict error
- Verify partial unique index rejects second captain in same squad
- Verify `parent_agent_id` FK rejects non-existent parent
- Verify `squad_id` FK rejects non-existent squad
- Verify `ListAgentsBySquad` only returns agents for the given squad
- Verify `CheckCycleInHierarchy` CTE detects would-be cycles
- Verify `updated_at` trigger fires on UPDATE

**HTTP Integration Tests (`internal/server/handlers/agent_handler_test.go`):**
- POST /api/agents: happy path (captain, lead, member creation)
- POST /api/agents: pending_approval when squad setting enabled
- POST /api/agents: 409 on duplicate shortName
- POST /api/agents: 409 on second captain
- POST /api/agents: 400 on invalid hierarchy (lead without captain parent)
- POST /api/agents: 401 without auth
- POST /api/agents: 403 for non-squad-member
- GET /api/agents?squadId=: returns correct agents
- GET /api/agents/{id}: 404 for non-existent
- PATCH /api/agents/{id}: update fields, verify hierarchy re-validation
- PATCH /api/agents/{id}: reject squadId change
- POST /api/agents/{id}/transition: all valid transitions
- POST /api/agents/{id}/transition: reject invalid transitions with correct error code

### Test Coverage Goals

| Component                    | Target |
|------------------------------|--------|
| Status machine transitions   | 100%   |
| Hierarchy validation rules   | 100%   |
| Input validation             | 100%   |
| Handler happy paths          | 100%   |
| Handler error paths          | 90%+   |
| Database constraint checks   | 90%+   |

## Open Questions

- [ ] Should `error` state allow transition to `active` (after investigation) or only to `terminated`? Current design requires termination. An alternative is `error -> active` for recovery.
- [ ] Should agent deletion be a hard delete or always a soft delete (transition to `terminated`)? Current design uses status transition only, no DELETE endpoint.
- [ ] Should the `POST /api/agents/{id}/transition` endpoint be the only way to change status, or should PATCH also support it? Current design supports both for flexibility.
- [ ] Should `adapterType` be required on creation, or optional (set later during configuration)?

## References

- [requirements.md](./requirements.md)
- [Squad Management requirements](../03-squad-management/requirements.md)
- [PRD Section 4.2 -- Agent Entity](../../core/01-PRODUCT.md)
- [PRD Section 10.2 -- Status Machines](../../core/01-PRODUCT.md)
