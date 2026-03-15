# Feature 15: Agent Self-Service API (Extended) -- Requirements

## Overview

Expand the agent self-service API so that agents can query richer context about
themselves, their team, their budget, and their goals. Agents can also create
inbox items to ask humans for help and self-report cost events from external
tool usage.

All endpoints are authenticated via Run Token (JWT) and scoped to the agent's
own squad. Role-based filtering ensures captains see more than members.

**Depends on:** Feature 11 (Inbox), Feature 12 (Cost Events), Feature 13 (Agent Runtime)

---

## Functional Requirements

### 15.1 -- Assignments Endpoint

**REQ-ASS-001** (Functional)
When an authenticated agent sends `GET /api/agent/me/assignments`, the system
shall return all issues assigned to that agent within its squad, regardless of
status.

**REQ-ASS-002** (Functional)
Where the request includes a `status` query parameter, the system shall filter
the returned assignments to only those matching the given status value.

**REQ-ASS-003** (Functional)
Where the request includes a `type` query parameter, the system shall filter
the returned assignments to only those matching the given issue type (task,
bug, story, conversation).

**REQ-ASS-004** (Functional)
Where the request includes `limit` and `offset` query parameters, the system
shall paginate the result set accordingly, with a default limit of 50 and a
maximum limit of 100.

**REQ-ASS-005** (Functional)
The response shall include a `total` count of matching assignments alongside
the paginated list, so the agent can determine remaining pages.

**REQ-ASS-006** (Functional)
Each assignment in the response shall include `pipelineId` and
`currentStageId` fields (both nullable UUIDs). These columns are added by
Feature 14 migration; if Feature 15 is implemented first, both fields will be
null.

---

### 15.2 -- Team Endpoint

**REQ-ASS-010** (Functional)
When an authenticated agent sends `GET /api/agent/me/team`, the system shall
return the agent's immediate team context: its parent agent, its sibling
agents (agents sharing the same parent), and its direct child agents.

**REQ-ASS-011** (Functional)
Where the authenticated agent has role `member`, the system shall return only
the agent's parent and siblings (no children, since members have none).

**REQ-ASS-012** (Functional)
Where the authenticated agent has role `lead`, the system shall return the
agent's parent, siblings, and direct children.

**REQ-ASS-013** (Functional)
Where the authenticated agent has role `captain`, the system shall return only
`self` and `allAgents` (all agents in the squad). The `parent`, `siblings`,
and `children` fields are omitted for captains to avoid redundancy.

**REQ-ASS-014** (Functional)
Each agent entry in the team response shall include: id, name, shortName,
role, status, and parentAgentId.

---

### 15.3 -- Budget Endpoint

**REQ-ASS-020** (Functional)
When an authenticated agent sends `GET /api/agent/me/budget`, the system shall
return the agent's budget information for the current billing period.

**REQ-ASS-021** (Functional)
The response shall include: `spentCents` (total cost events this period),
`budgetCents` (the agent's monthly budget limit), `remainingCents` (budget
minus spent), `periodStart` and `periodEnd` (ISO 8601 timestamps), and
`thresholdStatus` (one of `ok`, `warning`, `exceeded`).

**REQ-ASS-022** (Functional)
The `thresholdStatus` shall be computed using `domain.ComputeThreshold()`:
`warning` when spend reaches 80% of budget, and `exceeded` when spend reaches
100% of budget.

**REQ-ASS-023** (Functional)
Where the agent has no explicit budget set (budget_monthly_cents is NULL), the
system shall return `budgetCents` as null and `thresholdStatus` as `ok`.

**REQ-ASS-024** (Functional)
The billing period shall be calculated using `domain.BillingPeriod(time.Now())`
which returns the first day of the current calendar month (UTC) through the
first day of the next month (UTC).

---

### 15.4 -- Goals Endpoint

**REQ-ASS-030** (Functional)
When an authenticated agent sends `GET /api/agent/me/goals`, the system shall
return all goals linked to the agent's currently assigned issues via the
issue's `goal_id` foreign key.

**REQ-ASS-031** (Functional)
The response shall include for each goal: id, title, description, status, and
the list of related issue identifiers that link the agent to that goal.

**REQ-ASS-032** (Functional)
Goals shall be deduplicated in the response (if multiple issues reference the
same goal, it appears once with all related issue identifiers listed).

**REQ-ASS-033** (Functional)
Where the authenticated agent has role `captain`, the system shall return all
goals in the squad, not just those linked to the captain's own assignments.

**REQ-ASS-034** (Functional)
Goal fetching shall use a batch `GetGoalsByIDs` query instead of per-goal
lookups to avoid N+1 query patterns.

---

### 15.5 -- Inbox Creation Endpoint

**REQ-ASS-040** (Functional)
When an authenticated agent sends `POST /api/agent/me/inbox`, the system shall
create a new inbox item in the agent's squad with `requested_by_agent_id` set
to the agent's ID from the Run Token.

**REQ-ASS-041** (Functional)
The request body shall require: `category` (approval | question | decision),
`title` (1-500 chars), and `body` (optional text). The `alert` category shall
not be available to agents (alerts are system-generated only).

**REQ-ASS-042** (Functional)
The request body may optionally include: `urgency` (critical | normal | low,
default normal), `relatedIssueId` (UUID), and `payload` (arbitrary JSON
object).

**REQ-ASS-043** (Functional)
The system shall set `related_agent_id` to the requesting agent's ID (same as
`requested_by_agent_id`) and `related_run_id` to the Run Token's run_id
claim.

**REQ-ASS-044** (Functional)
Upon successful creation, the system shall emit an SSE event
`inbox.item.created` to the squad channel (reusing the event emitted by
InboxService.Create()) and log an activity entry with action
`inbox.item.created`.

---

### 15.6 -- Reply Endpoint (Already Implemented)

**REQ-ASS-050** (Informational)
`POST /api/agent/me/reply` is already implemented and allows agents to post
messages to conversations they are assigned to. No changes required.

---

### 15.7 -- Cost Reporting Endpoint

**REQ-ASS-060** (Functional)
When an authenticated agent sends `POST /api/agent/me/cost`, the system shall
create a new cost event recording the agent's self-reported usage.

**REQ-ASS-061** (Functional)
The request body shall require: `amountCents` (positive integer, maximum
100000 cents / $1000 per event), `eventType` (string, max 50 chars), and
`model` (string, max 100 chars).

**REQ-ASS-062** (Functional)
The request body may optionally include: `inputTokens` (non-negative integer),
`outputTokens` (non-negative integer), and `metadata` (arbitrary JSON object).

**REQ-ASS-063** (Functional)
The system shall set the `agent_id` and `squad_id` fields from the Run Token
claims, preventing agents from reporting costs for other agents.

**REQ-ASS-064** (Functional)
After recording the cost event, the system shall delegate budget threshold
checking to `BudgetEnforcementService.RecordAndEnforce()`, which handles both
agent-level and squad-level budget enforcement, including auto-pause and inbox
alert creation.

**REQ-ASS-065** (Functional)
Upon successful creation, the system shall emit an SSE event
`cost.event.created` to the squad channel and log an activity entry with
action `cost.event.created`.

---

### 15.8 -- Role-Based Response Filtering

**REQ-ASS-070** (Functional)
Where the authenticated agent has role `captain`, all endpoints shall return
squad-wide data (all agents, all issues, all goals).

**REQ-ASS-071** (Functional)
Where the authenticated agent has role `lead`, team and assignment endpoints
shall return data for the agent itself and its direct children (subtree depth
1).

**REQ-ASS-072** (Functional)
Where the authenticated agent has role `member`, team and assignment endpoints
shall return data only for the agent itself.

**REQ-ASS-073** (Functional)
Budget and cost endpoints shall always return data only for the requesting
agent, regardless of role.

---

### 15.9 -- Terminated Agent Guard

**REQ-ASS-074** (Security)
If the authenticated agent's current status in the database is `terminated`,
all endpoints shall reject the request with HTTP 403 and error
`{"error": "Agent terminated", "code": "FORBIDDEN"}`.

---

### 15.10 -- Integration Tests

**REQ-ASS-080** (Verification)
Integration tests shall cover each new endpoint with at least: a success case,
an authentication failure case, a squad-scoping violation case, and a
role-based filtering case for each role.

**REQ-ASS-081** (Verification)
Integration tests shall verify that agents cannot create inbox items with
category `alert`.

**REQ-ASS-082** (Verification)
Integration tests shall verify that cost self-reporting triggers budget
threshold alerts correctly via BudgetEnforcementService.

**REQ-ASS-083** (Verification)
Integration tests shall verify that terminated agents receive 403 on all
endpoints.

---

## Non-Functional Requirements

**REQ-ASS-090** (Performance)
All new endpoints shall respond within 200ms at p95 for squads with up to
100 agents and 10,000 issues.

**REQ-ASS-091** (Security)
All new endpoints shall reject requests that lack a valid Run Token with
HTTP 401 and error code `UNAUTHENTICATED`.

**REQ-ASS-092** (Security)
All new endpoints shall enforce squad-scoping: an agent shall never see data
belonging to a different squad than the one in its Run Token.

**REQ-ASS-093** (Compatibility)
All new endpoints shall follow the existing API conventions: JSON responses,
camelCase field names, standard error format `{"error": "...", "code": "..."}`.
