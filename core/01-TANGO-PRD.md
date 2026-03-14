# TANGO — Product Requirements Document (PRD)

**Product Name:** Tango
**Tagline:** Deploy. Govern. Share. The Control Plane for AI Agents.
**Version:** 1.0
**Date:** 2026-03-11
**Status:** Draft

---

## 1. Executive Summary

### 1.1 Vision

Agents are moving into production. Production requires control.

**Tango** is the control plane for AI agent workforces. It provides the infrastructure to deploy autonomous agents, govern their actions through human-in-the-loop oversight, and share agent capabilities across teams and organizations.

Unlike agent frameworks that help you *build* agents, Tango helps you *run* them — with organizational structure, budget enforcement, approval gates, audit trails, and real-time visibility.

### 1.2 Problem Statement

| Problem | Impact |
|---------|--------|
| No organizational structure for AI agents | Agents operate in isolation with no coordination, delegation, or accountability |
| No cost control | Runaway token spend, quota exhaustion, no attribution to projects or goals |
| No governance | Agents make decisions without human oversight; no approval gates for critical actions |
| No audit trail | No record of what agents did, why, or what it cost |
| No persistent context | Agent state lost between sessions; repeated work, lost progress |
| No production readiness | Dev-only tools with no deployment modes, auth, or multi-tenancy |
| No sharing | Agent configurations, company templates, and workflows locked to individual setups |

### 1.3 Solution

Tango provides three core capabilities:

1. **Deploy** — One-command setup with embedded database, zero-config agent onboarding, pluggable adapters for any AI runtime
2. **Govern** — Human-in-the-loop approval gates, budget enforcement, permission-based access control, immutable audit logs
3. **Share** — Portable company templates, agent marketplace, cross-organization agent sharing

### 1.4 Target Users

| Persona | Description | Primary Need |
|---------|-------------|-------------|
| **Board Operator** | Human who oversees the AI workforce | Governance, visibility, cost control |
| **Agent Developer** | Engineer building and configuring agents | Deployment, adapter integration, debugging |
| **Organization Admin** | Manages multiple companies/teams | Multi-tenancy, access control, budgets |
| **Agent Consumer** | Uses pre-built agent templates from marketplace | Quick deployment, trust, sharing |

---

## 2. Product Principles

### 2.1 Core Principles

1. **Control Plane, Not Execution Plane** — Tango orchestrates agents; it does not run them. Agents execute externally and report back via API. This keeps Tango lightweight and adapter-agnostic.

2. **Human-in-the-Loop by Default** — Every critical decision has an approval gate. Humans can override any agent action. The system is designed for trust but verifies through governance.

3. **Single Binary, Zero Dependencies** — Ship as one Go binary with embedded PostgreSQL. `./tango run` gives you a fully working system. No Docker, no Node.js, no external databases required.

4. **Production-First** — Auth, encryption, audit logs, backup, and monitoring are not afterthoughts. They ship in v1.

5. **Adapter-Agnostic** — Any AI runtime (Claude, GPT, Codex, Cursor, custom HTTP endpoints) can be an agent if it can call HTTP APIs.

6. **Cost-Aware by Design** — Every token, every API call, every dollar is tracked and attributed to the agent, task, project, and goal that incurred it.

---

## 3. System Architecture

### 3.1 High-Level Architecture

```
┌─────────────────────────────────────────────┐
│           Single Go Binary                  │
│  ┌───────────┐ ┌──────────┐ ┌────────────┐ │
│  │ CLI (cobra)│ │API Server│ │SSE Realtime│ │
│  └───────────┘ │(net/http)│ │  Push      │ │
│                └──────────┘ └────────────┘ │
│  ┌───────────┐ ┌──────────┐ ┌────────────┐ │
│  │ Heartbeat │ │ Adapter  │ │ Embedded   │ │
│  │ Scheduler │ │ Registry │ │ React UI   │ │
│  │(goroutines│ │          │ │ (go:embed) │ │
│  └───────────┘ └──────────┘ └────────────┘ │
├─────────────────────────────────────────────┤
│  PostgreSQL (embedded-postgres-go / external)│
├─────────────────────────────────────────────┤
│  React SPA (Vite + Tailwind + shadcn/ui)    │
└─────────────────────────────────────────────┘
```

### 3.2 Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Backend | Go + stdlib `net/http` | Single binary, native concurrency, low memory |
| Database | PostgreSQL + sqlc + goose | Type-safe SQL, zero ORM overhead, full PG compatibility |
| Embedded DB | embedded-postgres-go | Zero-config dev experience |
| Frontend | React 19 + Vite + Tailwind + shadcn/ui | Mature ecosystem, fast builds |
| CLI | cobra + bubbletea | Same binary as server, beautiful TUI |
| Real-time | Server-Sent Events (SSE) | Simpler than WebSocket, works through proxies |
| Auth | JWT + bcrypt + sessions | No external auth library needed |
| Validation | Go structs + validator | Compile-time type safety |
| Testing | Go testing + Vitest + Playwright | Native Go tests, JS frontend tests, E2E |

### 3.3 Deployment Modes

| Mode | Exposure | Auth | Use Case |
|------|----------|------|----------|
| `local_trusted` | Loopback only | None | Single operator, local dev |
| `authenticated` | Private network | Required | Team use, VPN/Tailscale |
| `authenticated` | Public internet | Required | Cloud deployment, SaaS |

---

## 4. Domain Model

### 4.1 Entity Relationship Overview

```
Company (top-level org)
├── Agents (AI employees, tree hierarchy)
│   ├── AgentRuntimeState (execution state, session persistence)
│   ├── AgentTaskSessions (per-task session state)
│   ├── AgentAPIKeys (long-lived auth tokens)
│   ├── AgentConfigRevisions (config change history)
│   └── WakeupRequests (invocation queue)
├── Issues (work units)
│   ├── IssueComments (discussion thread)
│   ├── IssueAttachments (file uploads)
│   ├── IssueLabels (tagging)
│   └── IssueApprovals (governance links)
├── Projects (work groupings)
│   └── ProjectWorkspaces (repo/directory configs)
├── Goals (strategic hierarchy)
├── Approvals (governance gates)
│   └── ApprovalComments (discussion)
├── CostEvents (financial tracking)
├── CompanySecrets (encrypted credentials)
│   └── SecretVersions (rotation history)
├── ActivityLog (immutable audit trail)
└── Memberships & Permissions (RBAC)
```

### 4.2 Core Entities

#### Company
The top-level organizational unit. Everything is company-scoped for strict data isolation.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| name | string | Company name |
| description | text | Mission/purpose |
| status | enum | active, paused, archived |
| issuePrefix | string | E.g., "TANGO" for TANGO-1, TANGO-2 |
| issueCounter | int | Auto-incrementing issue number |
| budgetMonthlyCents | int | Monthly spend limit |
| spentMonthlyCents | int | Current month spend |
| requireBoardApprovalForNewAgents | bool | Governance flag |
| brandColor | string | UI customization |

#### Agent
Every AI employee in the organization.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| companyId | FK | Company scope |
| name | string | Display name |
| urlKey | string | Unique shortname (e.g., "alice") |
| role | enum | ceo, manager, general |
| title | string | Job title |
| status | enum | active, idle, running, error, paused, terminated, pending_approval |
| reportsTo | FK(self) | Manager in hierarchy |
| capabilities | text | What this agent does |
| adapterType | enum | claude_local, codex_local, cursor, process, http, openclaw_gateway |
| adapterConfig | JSONB | Adapter-specific configuration |
| runtimeConfig | JSONB | Runtime parameters |
| budgetMonthlyCents | int | Per-agent budget |
| spentMonthlyCents | int | Current spend |
| permissions | JSONB | {canCreateAgents: bool} |
| lastHeartbeatAt | timestamp | Last execution time |

#### Issue (Task)
The unit of work. Every piece of work is an issue.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| companyId | FK | Company scope |
| identifier | string | Human-readable (e.g., "TANGO-39") |
| title | string | Task title |
| description | text | Full description (markdown) |
| status | enum | backlog, todo, in_progress, in_review, done, blocked, cancelled |
| priority | enum | critical, high, medium, low |
| parentId | FK(self) | Sub-task hierarchy |
| projectId | FK | Optional project grouping |
| goalId | FK | Optional goal alignment |
| assigneeAgentId | FK | Assigned agent |
| assigneeUserId | FK | Assigned human |
| checkoutRunId | FK | Execution lock |
| executionLockedAt | timestamp | When lock acquired |
| billingCode | string | Cost allocation tag |
| requestDepth | int | Nesting depth from root |

#### Heartbeat Run
Each time an agent executes, a heartbeat run is recorded.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| agentId | FK | Which agent ran |
| invocationSource | enum | timer, assignment, on_demand, automation |
| status | enum | queued, running, succeeded, failed, cancelled, timed_out |
| exitCode | int | Process exit code |
| usageJson | JSONB | Token counts, model, provider |
| resultJson | JSONB | Execution output |
| sessionIdBefore | string | Session state input |
| sessionIdAfter | string | Session state output |
| logStore, logRef | string | Log storage reference |
| stdoutExcerpt | text | Captured stdout |
| stderrExcerpt | text | Captured stderr |
| startedAt, finishedAt | timestamp | Execution timing |

#### Approval
Governance gate requiring human decision.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| companyId | FK | Company scope |
| type | enum | hire_agent, approve_ceo_strategy |
| status | enum | pending, revision_requested, approved, rejected, cancelled |
| requestedByAgentId | FK | Who requested |
| payload | JSONB | Request-specific data |
| decisionNote | text | Human's reasoning |
| decidedByUserId | FK | Who decided |
| decidedAt | timestamp | When decided |

#### Cost Event
Immutable record of every token/dollar spent.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| companyId, agentId | FK | Attribution |
| issueId, projectId, goalId | FK | Cost allocation |
| provider | string | anthropic, openai, etc. |
| model | string | claude-opus-4-6, gpt-4, etc. |
| inputTokens | int | Input token count |
| outputTokens | int | Output token count |
| costCents | int | Normalized cost |
| billingCode | string | Custom allocation tag |

#### Activity Log
Immutable audit trail of every action.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| companyId | FK | Company scope |
| actorType | enum | agent, user, system |
| actorId | UUID | Who did it |
| action | string | e.g., "issue.created", "agent.paused" |
| entityType | string | What was affected |
| entityId | UUID | Which entity |
| details | JSONB | Action-specific context |

---

## 5. Core Workflows

### 5.1 Deploy — Agent Lifecycle

#### 5.1.1 One-Command Setup

```bash
./tango run
# 1. Auto-onboard if no config exists (quickstart defaults)
# 2. Extract embedded PostgreSQL, initialize cluster
# 3. Run migrations
# 4. Start API server on :3100
# 5. Serve React UI
# 6. Open browser
```

#### 5.1.2 Agent Creation & Onboarding

```
Board creates company with goal
    ↓
Board/CEO creates agent (or agent requests hire)
    ↓
If requireBoardApprovalForNewAgents:
    → Approval created (status: pending)
    → Board reviews and approves/rejects
    → On approve: agent status → active
    ↓
Agent configured with adapter (Claude, Codex, etc.)
    ↓
Agent receives first assignment
    ↓
Heartbeat triggers → agent wakes and works
```

#### 5.1.3 Heartbeat Execution Protocol

```
1. TRIGGER → timer | assignment | @mention | manual | approval_resolved
2. QUEUE  → wakeup request created (deduplicated, prioritized)
3. INVOKE → adapter spawns agent with JWT + env vars + prompt
4. WORK   → agent calls Tango API:
             GET  /api/agent/me/assignments     (what to work on)
             POST /api/issues/{id}/checkout      (claim task - atomic lock)
             PATCH /api/issues/{id}              (update progress)
             POST /api/issues/{id}/comments      (add notes)
             POST /api/companies/{id}/issues     (create sub-tasks)
             POST /api/issues/{id}/release       (release lock)
5. REPORT → adapter captures: exit code, token usage, session state
6. RECORD → server creates heartbeat run + cost events + activity log
```

#### 5.1.4 Session Persistence

Agents maintain state across heartbeats:

```
Heartbeat Run #1:
  → sessionIdBefore: null
  → agent works, saves progress
  → sessionIdAfter: "session-abc"

Heartbeat Run #2:
  → sessionIdBefore: "session-abc"  (restored)
  → agent resumes from where it left off
  → sessionIdAfter: "session-def"
```

Per-task sessions allow agents to maintain separate context for each issue they work on.

### 5.2 Govern — Human-in-the-Loop

#### 5.2.1 Approval Gates

Every critical action requires human approval:

| Action | Approval Type | Who Decides |
|--------|--------------|-------------|
| Hire new agent | `hire_agent` | Board operator |
| CEO proposes strategy | `approve_ceo_strategy` | Board operator |
| Budget increase (future) | `budget_change` | Board operator |
| Agent termination (future) | `terminate_agent` | Board operator |

**Approval Flow:**
```
Agent requests → pending
    ↓
Board reviews
    ├── approve    → action executed automatically
    ├── reject     → agent notified, no action
    └── request_revision → agent resubmits
```

#### 5.2.2 Board Operator Powers

The board operator (human) has unrestricted control:

1. **Pause/resume any agent** — immediately stops future heartbeats
2. **Pause/resume any task** — freezes work on issue subtree
3. **Override any assignment** — reassign tasks between agents
4. **Override any budget** — increase/decrease agent or company limits
5. **Terminate agents** — permanently stop an agent
6. **Full project management** — create, edit, delete, comment on any issue
7. **Approve/reject all requests** — governance decisions
8. **View all activity** — complete audit trail

#### 5.2.3 Budget Enforcement (Three Tiers)

```
Agent Budget: $100/month
    ├── At $0-$79:   ✅ Normal operation
    ├── At $80:       ⚠️  Soft alert (80% warning)
    └── At $100:      🛑 Hard stop (auto-pause agent)

Company Budget: $1000/month
    └── Same tiers apply at company level
```

Budget tracking is atomic — cost events update agent and company spend in a single transaction.

#### 5.2.4 Atomic Task Checkout

Prevents race conditions when multiple agents compete for work:

```
POST /api/issues/{id}/checkout
{
  "agentId": "agent-uuid",
  "expectedStatuses": ["todo", "backlog"],
  "runId": "run-uuid"
}

Success (200): Task locked to this agent/run
Conflict (409): Another agent owns it → DO NOT RETRY
Already owned (200): Idempotent if you already hold it
```

Only one agent can own a task at a time. This is enforced at the database level.

#### 5.2.5 Permission Model (RBAC)

```
Principal (user or agent)
    ↓
CompanyMembership (links principal to company)
    ↓
PermissionGrant (specific permission + optional scope)
    ├── agents:create       — can hire agents
    ├── tasks:assign        — can assign work
    ├── users:invite        — can invite members
    ├── users:manage_permissions — can grant/revoke permissions
    └── joins:approve       — can approve join requests
```

Implicit permissions:
- CEO has all permissions within their company
- Agent creators have permissions over their reports

#### 5.2.6 Immutable Audit Trail

Every action is logged and can never be modified or deleted:

```json
{
  "action": "issue.status_changed",
  "actorType": "agent",
  "actorId": "agent-uuid",
  "entityType": "issue",
  "entityId": "issue-uuid",
  "details": {
    "from": "todo",
    "to": "in_progress",
    "runId": "run-uuid"
  },
  "createdAt": "2026-03-11T10:30:00Z"
}
```

### 5.3 Share — Templates & Marketplace (v2)

#### 5.3.1 Portable Company Templates

Export an entire company configuration:

```bash
tango export --company my-startup --output template.json
# Exports: agents, hierarchy, projects, goals, workflows
# Scrubs: secrets, API keys, PII
```

Import into another Tango instance:

```bash
tango import --template template.json --company new-startup
# Creates: company, agents, projects, goals
# Prompts: for missing secrets and credentials
```

#### 5.3.2 Agent Marketplace (Future)

```
┌─────────────────────────────────────┐
│  Tango Marketplace                   │
│                                     │
│  📦 "Full-Stack Dev Team"           │
│     CEO + 3 engineers + QA          │
│     ⭐ 4.8 (234 deploys)            │
│                                     │
│  📦 "Content Marketing Squad"       │
│     Content lead + 2 writers        │
│     ⭐ 4.5 (89 deploys)             │
│                                     │
│  📦 "Data Pipeline Builder"         │
│     Single agent, ETL specialist    │
│     ⭐ 4.9 (567 deploys)            │
└─────────────────────────────────────┘
```

---

## 6. API Design

### 6.1 Base URL & Authentication

```
Base: http://localhost:3100/api

Headers:
  Authorization: Bearer <token>
  X-Tango-Run-Id: <run-uuid>          (agent mutations)
  Content-Type: application/json
```

Token types:
- **Agent JWT** — short-lived (48h), issued per heartbeat run
- **Agent API Key** — long-lived, stored as SHA-256 hash
- **User Session** — cookie-based, for board operators

### 6.2 Endpoint Summary

#### Companies
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/companies` | List companies |
| GET | `/api/companies/:id` | Get company |
| POST | `/api/companies` | Create company |
| PATCH | `/api/companies/:id` | Update company |
| PATCH | `/api/companies/:id/budgets` | Update budget |
| POST | `/api/companies/:id/export` | Export template |
| POST | `/api/companies/import` | Import template |

#### Agents
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/agents` | List agents |
| GET | `/api/agents/:id` | Get agent (UUID or shortname) |
| POST | `/api/agents` | Create agent |
| PATCH | `/api/agents/:id` | Update agent |
| POST | `/api/agents/:id/keys` | Create API key |
| POST | `/api/agents/:id/wake` | Manual invoke |
| POST | `/api/agents/:id/reset-session` | Clear session state |
| GET | `/api/agents/:id/config-revisions` | Config history |

#### Issues (Tasks)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/companies/:id/issues` | List issues (filterable) |
| GET | `/api/issues/:id` | Get issue (UUID or "TANGO-39") |
| POST | `/api/companies/:id/issues` | Create issue |
| PATCH | `/api/issues/:id` | Update issue |
| DELETE | `/api/issues/:id` | Delete issue |
| POST | `/api/issues/:id/checkout` | Atomic task claim |
| POST | `/api/issues/:id/release` | Release lock |
| POST | `/api/issues/:id/comments` | Add comment |
| POST | `/api/issues/:id/attachments` | Upload file |

#### Projects & Goals
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/companies/:id/projects` | List projects |
| POST | `/api/companies/:id/projects` | Create project |
| PATCH | `/api/projects/:id` | Update project |
| GET | `/api/companies/:id/goals` | List goals |
| POST | `/api/companies/:id/goals` | Create goal |
| PATCH | `/api/goals/:id` | Update goal |

#### Approvals (Governance)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/companies/:id/approvals` | List approvals |
| POST | `/api/companies/:id/approvals` | Request approval |
| PATCH | `/api/approvals/:id/approve` | Approve |
| PATCH | `/api/approvals/:id/reject` | Reject |
| PATCH | `/api/approvals/:id/request-revision` | Request changes |

#### Costs & Dashboard
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/companies/:id/cost-events` | Report usage |
| GET | `/api/companies/:id/costs/summary` | Spend overview |
| GET | `/api/companies/:id/costs/by-agent` | Agent breakdown |
| GET | `/api/dashboard/:id` | Dashboard metrics |

#### Secrets
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/companies/:id/secrets` | List secrets |
| POST | `/api/companies/:id/secrets` | Create secret |
| POST | `/api/secrets/:id/rotate` | Rotate secret |

#### Activity & Access
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/companies/:id/activity` | Activity log |
| GET | `/api/companies/:id/members` | List members |
| POST | `/api/companies/:id/permissions` | Grant permission |

### 6.3 Error Response Format

```json
{
  "error": "Human-readable error message",
  "code": "CHECKOUT_CONFLICT",
  "details": {}
}
```

| Status | Meaning | Agent Action |
|--------|---------|-------------|
| 400 | Validation error | Fix request |
| 401 | Unauthenticated | Refresh token |
| 403 | Unauthorized | Escalate to manager |
| 404 | Not found | Skip |
| 409 | Conflict (task locked) | DO NOT retry |
| 422 | Invalid state transition | Check current state |
| 500 | Server error | Log and move on |

### 6.4 Real-time Events (SSE)

```
GET /api/companies/:id/events/stream
Accept: text/event-stream
```

Event types:
```
agent.status.changed
heartbeat.run.queued
heartbeat.run.started
heartbeat.run.finished
issue.updated
issue.comment.created
approval.status.changed
activity.appended
cost.threshold.warning
```

---

## 7. Agent Environment Contract

Every agent receives these environment variables when invoked:

| Variable | Description |
|----------|-------------|
| `TANGO_API_URL` | Base URL (e.g., `http://localhost:3100`) |
| `TANGO_API_KEY` | Short-lived JWT for this run |
| `TANGO_AGENT_ID` | Agent's UUID |
| `TANGO_COMPANY_ID` | Company's UUID |
| `TANGO_RUN_ID` | Current heartbeat run ID |
| `TANGO_TASK_ID` | Issue that triggered wake (if any) |
| `TANGO_WAKE_REASON` | Why agent woke (issue_assigned, timer, mention, manual) |

---

## 8. Adapter System

### 8.1 Adapter Interface

```go
type Adapter interface {
    Type() string
    Execute(ctx context.Context, input InvokeInput, hooks Hooks) (InvokeResult, error)
    TestEnvironment(level TestLevel) (TestResult, error)
    Models() []ModelDefinition
}

type InvokeInput struct {
    Agent       AgentContext
    Company     CompanyContext
    Run         RunContext
    Workspace   WorkspaceConfig
    EnvVars     map[string]string
    Prompt      string
}

type InvokeResult struct {
    Status       RunStatus
    ExitCode     int
    Usage        TokenUsage
    SessionState string
    Stdout       string
    Stderr       string
}
```

### 8.2 Built-in Adapters

| Adapter | Runtime | Session Resume | Token Reporting |
|---------|---------|---------------|-----------------|
| `claude_local` | Claude Code CLI | Yes | Yes |
| `codex_local` | OpenAI Codex CLI | Yes | Yes |
| `cursor_local` | Cursor IDE | Yes | Yes |
| `process` | Any shell command | No | Manual |
| `http` | HTTP webhook | Adapter-dependent | Adapter-dependent |
| `openclaw_gateway` | Remote OpenClaw agent | Yes | Yes |

### 8.3 Custom Adapters

Register custom adapters via Go interface implementation:

```go
func init() {
    registry.Register("my_adapter", &MyAdapter{})
}
```

Or use the `http` adapter to integrate any external system via webhook.

---

## 9. Human-in-the-Loop Design

### 9.1 Philosophy

Tango assumes agents are capable but not trustworthy by default. Every critical path has a human checkpoint.

### 9.2 HITL Touchpoints

| Touchpoint | Trigger | Human Action |
|------------|---------|-------------|
| **Agent Hiring** | Agent requests new subordinate | Board approves/rejects |
| **Strategy Approval** | CEO proposes direction | Board approves/rejects |
| **Budget Override** | Agent exceeds budget | Board increases or keeps paused |
| **Task Review** | Agent marks issue `in_review` | Board moves to `done` or returns |
| **Emergency Pause** | Board detects problem | Board pauses agent immediately |
| **Comment Direction** | Agent asks for guidance | Board comments on issue |
| **Escalation** | Agent can't resolve conflict | Board decides priority/assignment |

### 9.3 Notification & Awareness

Board operators see:
- **Dashboard** — real-time agent status, cost trends, pending approvals
- **Inbox** — pending approvals, @-mentions, escalations
- **Activity Feed** — chronological audit trail
- **SSE Events** — real-time push for status changes
- **Cost Alerts** — warnings at 80% budget threshold

### 9.4 Override Mechanics

Board overrides are:
- **Immediate** — take effect on next heartbeat (not current run)
- **Logged** — every override creates an activity log entry
- **Reversible** — pause can be resumed, assignments can be re-assigned
- **Commentable** — board can explain reasoning via comments

---

## 10. Data Invariants & Constraints

### 10.1 Hard Invariants

| Invariant | Enforcement |
|-----------|------------|
| All entities belong to exactly one company | FK + application logic |
| Issue identifiers are unique within company | Unique constraint |
| Agent shortnames are unique within company | Unique constraint |
| Only one agent can hold a task checkout | Atomic DB transaction |
| Cost events are append-only (never updated) | No UPDATE/DELETE routes |
| Activity log is append-only (never updated) | No UPDATE/DELETE routes |
| Agent hierarchy is a strict tree (no cycles) | Application validation |
| Secret values are encrypted at rest | Encryption before storage |
| Budget spend is atomically updated with cost events | DB transaction |

### 10.2 Status Machines

**Agent Status:**
```
pending_approval → active (on board approve)
active → idle (no work)
idle → running (heartbeat triggered)
running → idle (heartbeat complete)
running → error (heartbeat failed)
active/idle/running → paused (board or budget)
paused → active (board resumes)
any → terminated (board terminates)
```

**Issue Status:**
```
backlog → todo → in_progress → in_review → done
                      ↓              ↓
                   blocked      in_progress (returned)
any → cancelled
done/cancelled → todo (reopened via comment)
```

**Approval Status:**
```
pending → approved | rejected | cancelled
pending → revision_requested → pending (resubmit)
```

---

## 11. Configuration & Storage

### 11.1 Instance Layout

```
~/.tango/
├── instances/
│   └── default/
│       ├── db/                    # Embedded PostgreSQL data
│       ├── data/
│       │   ├── backups/           # Database backups
│       │   └── storage/           # File attachments
│       ├── logs/                  # Application logs
│       └── secrets/
│           └── master.key         # Encryption key
├── config.json                    # Instance configuration
└── .env                           # JWT secret, env overrides
```

### 11.2 Configuration Schema

```json
{
  "$meta": { "version": 1, "source": "onboard" },
  "database": {
    "mode": "embedded-postgres | external",
    "connectionString": "postgres://...",
    "embeddedPostgresPort": 54329,
    "backup": {
      "enabled": true,
      "intervalMinutes": 60,
      "retentionDays": 30
    }
  },
  "server": {
    "deploymentMode": "local_trusted | authenticated",
    "exposure": "private | public",
    "host": "127.0.0.1",
    "port": 3100,
    "serveUi": true
  },
  "auth": {
    "baseUrlMode": "auto | explicit",
    "disableSignUp": false
  },
  "storage": {
    "provider": "local_disk | s3"
  },
  "secrets": {
    "provider": "local_encrypted",
    "strictMode": false
  }
}
```

---

## 12. Security Model

### 12.1 Authentication Tiers

| Tier | Method | Audience |
|------|--------|----------|
| Agent Run JWT | HS256 JWT (48h TTL) | Adapters injecting per-run auth |
| Agent API Key | SHA-256 hashed long-lived token | Agent self-service API calls |
| User Session | Cookie-based session | Board operators via UI |
| Local Trusted | Implicit (no auth) | Single-operator local mode |

### 12.2 Security Measures

- Secrets encrypted at rest with AES-256 (master key file)
- Secret values never logged or returned in API responses
- Agent JWT scoped to: agent ID, company ID, run ID
- API keys stored as SHA-256 hashes (never plaintext)
- Company-scoped data isolation (agents can't cross companies)
- Private hostname guard for authenticated deployments
- Activity log for complete audit trail

---

## 13. Metrics & Observability

### 13.1 Dashboard Metrics

| Metric | Description |
|--------|-------------|
| Active agents | Count of agents in active/running status |
| Pending approvals | Count of approvals awaiting decision |
| Monthly spend | Total cost this month (company and per-agent) |
| Budget utilization | Spend / budget as percentage |
| Issues by status | Distribution across backlog/todo/in_progress/done |
| Recent activity | Last N activity log entries |
| Agent health | Last heartbeat time, error rate |
| Cost trend | Daily/weekly spend chart |

### 13.2 Agent Health Signals

| Signal | Healthy | Warning | Critical |
|--------|---------|---------|----------|
| Last heartbeat | < 2x interval | 2-5x interval | > 5x interval |
| Error rate | < 5% | 5-20% | > 20% |
| Budget usage | < 80% | 80-95% | > 95% (auto-pause) |
| Session age | < 24h | 24-72h | > 72h |

---

## 14. Release Plan

### Phase 1: Foundation (v0.1)
- [ ] Go project scaffold (cobra CLI + net/http server)
- [ ] Embedded PostgreSQL integration (embedded-postgres-go)
- [ ] Database schema + sqlc codegen + goose migrations
- [ ] Company CRUD
- [ ] Agent CRUD with hierarchy
- [ ] Issue CRUD with status machine
- [ ] Basic React UI (dashboard, agents, issues)
- [ ] `tango run` one-command startup

### Phase 2: Execution (v0.2)
- [ ] Heartbeat scheduler (goroutine-based)
- [ ] Adapter interface + `process` adapter
- [ ] `claude_local` adapter
- [ ] Agent JWT auth
- [ ] Task checkout/release (atomic locking)
- [ ] Session persistence
- [ ] Wakeup queue with deduplication
- [ ] SSE real-time events

### Phase 3: Governance (v0.3)
- [ ] Approval workflow (hire, strategy)
- [ ] Budget tracking + enforcement (soft/hard)
- [ ] Cost event recording + attribution
- [ ] Activity log (immutable audit trail)
- [ ] Permission grants (RBAC)
- [ ] Board operator override controls
- [ ] Human-in-the-loop review status

### Phase 4: Production (v0.4)
- [ ] Authenticated deployment mode
- [ ] Secret management (encrypted storage + rotation)
- [ ] S3 storage adapter
- [ ] Database backup/restore
- [ ] Docker image
- [ ] CLI onboarding wizard
- [ ] Agent API keys (long-lived)

### Phase 5: Share (v0.5)
- [ ] Company export/import (portable templates)
- [ ] `codex_local` + `cursor_local` adapters
- [ ] HTTP webhook adapter
- [ ] Multi-company support
- [ ] Marketplace foundation

---

## 15. Success Metrics

| Metric | Target |
|--------|--------|
| Time to first agent running | < 5 minutes from `./tango run` |
| Binary size | < 60MB (with embedded PG) |
| Memory usage (idle) | < 50MB |
| Memory usage (10 agents) | < 100MB |
| API response time (p95) | < 50ms |
| Heartbeat overhead | < 100ms per invocation |
| Zero-downgrade migrations | 100% backward compatible |

---

## 16. Competitive Positioning

| Feature | Tango | Paperclip | CrewAI | AutoGen | LangGraph |
|---------|------|-----------|--------|---------|-----------|
| Control plane (not framework) | Yes | Yes | No | No | No |
| Single binary deployment | Yes | No (Node.js) | No (Python) | No | No |
| Embedded database | Yes (Go) | Yes (Node hack) | No | No | No |
| Human-in-the-loop governance | Yes | Partial | No | Basic | No |
| Budget enforcement | Yes | Yes | No | No | No |
| Immutable audit trail | Yes | Yes | No | No | No |
| Agent hierarchy | Yes | Yes | Yes | No | No |
| Adapter-agnostic | Yes | Yes | No | Partial | No |
| Session persistence | Yes | Yes | No | Partial | Yes |
| Atomic task checkout | Yes | Yes | No | No | No |
| Company templates | Yes | Partial | No | No | No |
| Marketplace | Planned | Planned | No | No | No |

---

## Appendix A: Glossary

| Term | Definition |
|------|-----------|
| **Board Operator** | Human user who governs the AI workforce |
| **Agent** | AI employee in the organization |
| **Heartbeat** | Single execution window for an agent |
| **Wakeup** | Signal that triggers a heartbeat |
| **Adapter** | Plugin that connects Tango to an AI runtime |
| **Checkout** | Atomic lock on a task by an agent |
| **Company** | Top-level organizational unit |
| **Issue** | Unit of work (task) |
| **Approval** | Governance gate requiring human decision |
| **Cost Event** | Record of token/dollar spend |
| **Activity Log** | Immutable audit entry |
| **Session** | Persistent state across heartbeats |
| **HITL** | Human-in-the-Loop |

## Appendix B: Name Origin

**Tango** — It takes two to tango. In production AI, the two partners are agents and humans. Neither operates alone — agents execute, humans govern. Tango is the dance floor where they meet: structured, coordinated, and always in sync. Deploy. Govern. Share.

---

*Deploy. Govern. Share.*
*The Control Plane for AI Agents.*
