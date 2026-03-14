# Requirements: Agent Management

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 04-agent-management
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management

## Overview

Agent Management provides the ability to create, read, update, and manage AI agents within squads. Agents are organized in a strict tree hierarchy (captain > lead > member) and follow a defined status machine governing their lifecycle. Each agent belongs to exactly one squad and is configured with an adapter for its AI runtime.

## EARS Requirements

### Agent Entity

**REQ-AGT-001:** The system shall store agents with the following fields: id (UUID), squadId (FK), name (string), shortName (string, mapped from PRD `urlKey`), role (enum: captain, lead, member), status (enum: pending_approval, active, idle, running, error, paused, terminated), parentAgentId (FK self-referencing, nullable, mapped from PRD `reportsTo`), adapterType (enum), adapterConfig (JSONB), systemPrompt (text), model (string), budgetMonthlyCents (integer, nullable where NULL means unlimited), createdAt (timestamp), and updatedAt (timestamp).

> **PRD fields deferred to later phases:** title (string), capabilities (text), runtimeConfig (JSONB), permissions (JSONB), lastHeartbeatAt (timestamp). See design.md for rationale.

**REQ-AGT-002:** When an agent is created, the system shall assign a UUID as the primary key.

**REQ-AGT-003:** The system shall require name and shortName for every agent.

**REQ-AGT-004:** The system shall enforce that shortName is unique within a squad.

**REQ-AGT-005:** The system shall require squadId as a foreign key referencing an existing squad.

**REQ-AGT-006:** The system shall automatically set createdAt and updatedAt timestamps when an agent is created, and update updatedAt on every modification.

### Agent Hierarchy

**REQ-AGT-010:** The system shall enforce a strict tree hierarchy for agents within a squad: captain at the root, leads under the captain, and members under leads.

**REQ-AGT-011:** When an agent has role "captain", the system shall require that parentAgentId is NULL.

**REQ-AGT-012:** When an agent has role "lead", the system shall require that parentAgentId references an agent with role "captain" in the same squad.

**REQ-AGT-013:** When an agent has role "member", the system shall require that parentAgentId references an agent with role "lead" in the same squad.

**REQ-AGT-014:** The system shall enforce that only one agent with role "captain" exists per squad.

**REQ-AGT-015:** The system shall validate that parentAgentId, when provided, references an existing agent within the same squad.

**REQ-AGT-016:** The system shall reject any hierarchy change that would create a cycle in the agent tree.

**REQ-AGT-017:** If a squad has `requireApprovalForNewAgents` enabled, when a new agent is created, the system shall set the agent status to "pending_approval" instead of "active".

### Agent Status Machine

**REQ-AGT-020:** The system shall enforce the following valid status transitions for agents:
- pending_approval -> active
- active -> idle
- idle -> running
- running -> idle
- running -> error
- active -> paused
- idle -> paused
- running -> paused
- paused -> active
- any status -> terminated

**REQ-AGT-021:** When a status transition is requested that is not in the valid transitions list defined in REQ-AGT-020, the system shall reject the request with a validation error.

**REQ-AGT-022:** When an agent transitions to "terminated", the system shall treat this as a terminal state from which no further non-terminated transitions are allowed.

**REQ-AGT-023:** When an agent is in "pending_approval" status, the system shall only allow transition to "active" (approval) or "terminated" (rejection/removal).

### Agent CRUD API

**REQ-AGT-030:** The system shall provide a `POST /api/agents` endpoint that creates a new agent within a squad.

**REQ-AGT-031:** The `POST /api/agents` endpoint shall accept the following fields in the request body: squadId, name, shortName, role, parentAgentId (optional), adapterType (optional), adapterConfig (optional), systemPrompt (optional), model (optional), budgetMonthlyCents (optional).

**REQ-AGT-032:** When `POST /api/agents` succeeds, the system shall return HTTP 201 with the created agent object including all fields and generated id.

**REQ-AGT-033:** When `POST /api/agents` fails validation, the system shall return HTTP 400 with an error response in the format `{"error": "message", "code": "VALIDATION_ERROR"}`.

**REQ-AGT-034:** The system shall provide a `GET /api/agents` endpoint that returns a list of agents, filterable by squadId query parameter.

**REQ-AGT-035:** The `GET /api/agents` endpoint shall require a squadId query parameter to enforce squad-scoped access.

**REQ-AGT-036:** The system shall provide a `GET /api/agents/:id` endpoint that returns a single agent by ID.

**REQ-AGT-037:** When `GET /api/agents/:id` references a non-existent agent, the system shall return HTTP 404 with an error response in the format `{"error": "agent not found", "code": "NOT_FOUND"}`.

**REQ-AGT-038:** The system shall provide a `PATCH /api/agents/:id` endpoint that updates specified fields of an existing agent.

**REQ-AGT-039:** The `PATCH /api/agents/:id` endpoint shall support updating: name, shortName, role, parentAgentId, status, adapterType, adapterConfig, systemPrompt, model, budgetMonthlyCents.

**REQ-AGT-040:** When `PATCH /api/agents/:id` updates the status field, the system shall validate the transition against the status machine rules defined in REQ-AGT-020.

**REQ-AGT-041:** When `PATCH /api/agents/:id` updates the role or parentAgentId fields, the system shall re-validate the hierarchy constraints defined in REQ-AGT-010 through REQ-AGT-016.

**REQ-AGT-042:** The system shall not allow changing an agent's squadId after creation.

### Squad Scoping and Authorization

**REQ-AGT-050:** The system shall ensure that all agent operations are scoped to the squad the authenticated user has access to.

**REQ-AGT-051:** When an unauthenticated request is made to any agent endpoint, the system shall return HTTP 401.

**REQ-AGT-052:** When an authenticated user requests agents from a squad they do not belong to, the system shall return HTTP 403.

### Data Validation

**REQ-AGT-060:** When the name field exceeds 255 characters, the system shall reject the request with a validation error.

**REQ-AGT-061:** When the shortName field exceeds 50 characters or contains characters other than lowercase alphanumeric and hyphens, the system shall reject the request with a validation error.

**REQ-AGT-062:** When adapterConfig is provided, the system shall validate it is valid JSON.

**REQ-AGT-063:** When budgetMonthlyCents is provided, the system shall validate it is a non-negative integer.

**REQ-AGT-064:** When role is provided, the system shall validate it is one of: captain, lead, member.

**REQ-AGT-065:** When status is provided on creation, the system shall ignore it and set the initial status based on squad governance settings (REQ-AGT-017) or default to "active".

### Error Handling

**REQ-AGT-070:** When a duplicate shortName is detected within the same squad, the system shall return HTTP 409 with error code "CONFLICT".

**REQ-AGT-071:** When a second captain is added to a squad that already has one, the system shall return HTTP 409 with error code "CONFLICT" and a message indicating only one captain is allowed per squad.

**REQ-AGT-072:** When an invalid parentAgentId is provided (non-existent or wrong squad), the system shall return HTTP 400 with error code "VALIDATION_ERROR".

**REQ-AGT-073:** When an invalid status transition is attempted, the system shall return HTTP 400 with error code "INVALID_STATUS_TRANSITION" and a message indicating the current and requested statuses.

## Non-Functional Requirements

**REQ-AGT-NF-001:** Agent list queries shall respond within 200ms for squads with up to 100 agents.

**REQ-AGT-NF-002:** The agents database table shall have indexes on squadId and parentAgentId for efficient hierarchy queries.

**REQ-AGT-NF-003:** The agents database table shall have a unique composite index on (squadId, shortName).

## Traceability

| Requirement | PRD Section | Description |
|-------------|-------------|-------------|
| REQ-AGT-001 | 4.2 Agent | Agent entity fields (shortName = PRD urlKey, parentAgentId = PRD reportsTo) |
| REQ-AGT-001 (deferred) | 4.2 Agent | PRD fields deferred: title, capabilities, runtimeConfig, permissions, lastHeartbeatAt |
| REQ-AGT-010-016 | 10.1, 5.1 | Hierarchy invariants |
| REQ-AGT-014 | 10.1 | One captain per squad |
| REQ-AGT-020-023 | 10.2 | Agent status machine |
| REQ-AGT-030-042 | Phase 1 scope | Agent CRUD API |
| REQ-AGT-050-052 | 2.1, 5.2.5 | Squad scoping and auth |
| REQ-AGT-017 | 5.1.2 | Approval gate for new agents |
