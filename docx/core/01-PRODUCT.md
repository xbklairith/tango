# Ari — Product Requirements Document (PRD)

**Product Name:** Ari
**Tagline:** Deploy. Govern. Share. The Control Plane for AI Agents.
**Version:** 1.0
**Date:** 2026-03-11
**Status:** Draft

---

## 1. Executive Summary

### 1.1 Vision

Agents are moving into production. Production requires control.

**Ari** is the control plane for AI agent workforces. It provides the infrastructure to deploy autonomous agents, govern their actions through human-in-the-loop oversight, and share agent capabilities across teams and organizations.

Unlike agent frameworks that help you *build* agents, Ari helps you *run* them — with squad structure, budget enforcement, approval gates, audit trails, and real-time visibility.

### 1.2 Problem Statement

| Problem | Impact |
|---------|--------|
| No organizational structure for AI agents | Agents operate in isolation with no coordination, delegation, or accountability |
| No cost control | Runaway token spend, quota exhaustion, no attribution to projects or goals |
| No governance | Agents make decisions without human oversight; no approval gates for critical actions |
| No audit trail | No record of what agents did, why, or what it cost |
| No persistent context | Agent state lost between sessions; repeated work, lost progress |
| No production readiness | Dev-only tools with no deployment modes, auth, or multi-tenancy |
| No sharing | Agent configurations, squad templates, and workflows locked to individual setups |

### 1.3 Solution

Ari provides three core capabilities:

1. **Deploy** — One-command setup with embedded database, zero-config agent onboarding, pluggable adapters for any AI runtime
2. **Govern** — Human-in-the-loop approval gates, budget enforcement, permission-based access control, immutable audit logs
3. **Share** — Portable squad templates, agent marketplace, cross-organization agent sharing

### 1.4 Target Users

| Persona | Description | Primary Need |
|---------|-------------|-------------|
| **User** | Human who oversees the AI squad | Governance, visibility, cost control |
| **Agent Developer** | Engineer building and configuring agents | Deployment, adapter integration, debugging |
| **Organization Admin** | Manages multiple squads/teams | Multi-tenancy, access control, budgets |
| **Agent Consumer** | Uses pre-built agent templates from marketplace | Quick deployment, trust, sharing |

---

## 2. Product Principles

### 2.1 Core Principles

1. **Control Plane, Not Execution Plane** — Ari orchestrates agents; it does not run them. Agents execute externally and report back via API. This keeps Ari lightweight and adapter-agnostic.

2. **Human-in-the-Loop by Default** — Every critical decision has an approval gate. Humans can override any agent action. The system is designed for trust but verifies through governance.

3. **Single Binary, Zero Dependencies** — Ship as one Go binary with embedded PostgreSQL. `./ari run` gives you a fully working system. No Docker, no Node.js, no external databases required.

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
| CLI | cobra | Same binary as server, simple CLI |
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
Squad (top-level team)
├── Agents (AI members, tree hierarchy)
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
├── Goals (strategic objectives)
├── Approvals (governance gates)
│   └── ApprovalComments (discussion)
├── CostEvents (financial tracking)
├── SquadSecrets (encrypted credentials)
│   └── SecretVersions (rotation history)
├── ActivityLog (immutable audit trail)
└── Memberships & Permissions (RBAC)
```

### 4.2 Core Entities

> All entities include `createdAt` and `updatedAt` timestamps (omitted from tables for brevity).

#### User
Human who operates the system. Created during onboarding on first `ari run`.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| email | string | Unique login identifier |
| displayName | string | Shown in UI |
| passwordHash | string | bcrypt hash |
| status | enum | active, disabled |
| isAdmin | bool | System-wide administrator flag; first registered user is auto-promoted |

#### Squad
The top-level organizational unit. Everything is squad-scoped for strict data isolation. A single Ari instance supports **multiple squads running simultaneously** — users can create, join, and switch between squads freely. Each squad operates independently with its own agents, issues, budgets, and settings.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| name | string | Squad name |
| description | text | Mission/purpose |
| status | enum | active, paused, archived |
| issuePrefix | string | E.g., "ARI" for ARI-1, ARI-2 |
| issueCounter | int | Auto-incrementing issue number |
| budgetMonthlyCents | int | Monthly spend limit (NULL = unlimited) |
| requireApprovalForNewAgents | bool | Governance flag |
| brandColor | string | UI customization |

> **Cost tracking:** Monthly spend is computed from CostEvents (`SUM(costCents) WHERE createdAt >= month_start_utc`), not stored as a column. This avoids stale data and reset logic.

#### SquadMembership
Links users to squads. A user can belong to **multiple squads** with a different role in each. The (userId, squadId) pair is unique — one membership per user per squad. The UI provides a squad selector to switch between squads; the backend determines squad context from the URL path, not from session state.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| userId | FK | Which user |
| squadId | FK | Which squad |
| role | enum | owner, admin, viewer |

#### Agent
Every AI member on the squad.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| name | string | Display name |
| urlKey | string | Unique shortname (e.g., "alice") |
| role | enum | captain, lead, member |
| title | string | Position title |
| status | enum | active, idle, running, error, paused, terminated, pending_approval |
| reportsTo | FK(self) | Lead in hierarchy |
| capabilities | text | What this agent does |
| adapterType | enum | claude_local, codex_local, cursor, process, http, openclaw_gateway |
| adapterConfig | JSONB | Adapter-specific configuration |
| runtimeConfig | JSONB | Runtime parameters |
| budgetMonthlyCents | int | Per-agent budget (NULL = unlimited) |
| permissions | JSONB | {canCreateAgents: bool} |
| lastHeartbeatAt | timestamp | Last execution time |

> **Cost tracking:** Agent spend is computed from CostEvents, same as squad.

#### Project
Groups related issues under a shared scope.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| name | string | Project name |
| description | text | Purpose/scope |
| status | enum | active, completed, archived |

#### Goal
Strategic objective that issues and projects align to.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| parentId | FK(self) | Goal hierarchy |
| title | string | Goal title |
| description | text | What success looks like |
| status | enum | active, completed, archived |

#### Issue (Task / Conversation)
The unit of work or a conversation thread. Every piece of work is an issue; conversations reuse the same model.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| identifier | string | Human-readable (e.g., "ARI-39") |
| type | enum | task (default), conversation |
| title | string | Task title or conversation subject |
| description | text | Full description (markdown) |
| status | enum | backlog, todo, in_progress, done, blocked, cancelled |
| priority | enum | critical, high, medium, low |
| parentId | FK(self) | Sub-task hierarchy |
| projectId | FK | Optional project grouping |
| goalId | FK | Optional goal alignment |
| assigneeAgentId | FK | Assigned agent |
| assigneeUserId | FK | Assigned human |
| checkoutRunId | FK | Execution lock (Phase 2) |
| executionLockedAt | timestamp | When lock acquired (Phase 2) |
| billingCode | string | Cost allocation tag |
| requestDepth | int | Nesting depth from root |
| pipelineId | FK | Optional stage pipeline (see IssuePipeline) |
| currentStage | string | Current pipeline stage (e.g., "review") |

#### IssuePipeline
A fully custom sequence of stages for an issue. Stage names are freeform strings — not tied to any fixed enum. Any squad can define any workflow. *(Phase 2+)*

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| name | string | Pipeline name (e.g., "dev-flow", "content-pipeline") |
| description | string | What this pipeline is for |
| stages | JSONB | Ordered stage definitions (see below) |

**stages JSONB structure:**
```json
[
  {
    "name": "code",                    // freeform — any name you want
    "assigneeAgentId": "<agent-id>",   // who handles this stage
    "description": "Implement the feature",
    "completionHint": "Code compiles, tests pass"  // guidance for the agent
  },
  {
    "name": "review",
    "assigneeAgentId": "<agent-id>",
    "description": "Review code quality and correctness",
    "completionHint": "No critical issues found"
  }
]
```

**Example pipelines by squad type:**

| Squad | Pipeline | Stages |
|-------|----------|--------|
| **Dev** | `code-review-qa` | code → review → qa |
| **Content** | `publish-flow` | draft → edit → fact_check → approve → publish |
| **Sales** | `outreach` | research → outreach → follow_up → negotiate → closed |
| **Legal** | `contract-flow` | draft_contract → internal_review → legal_review → sign |
| **Support** | `ticket-flow` | triage → investigate → resolve → verify |
| **Research** | `paper-flow` | literature_review → experiment → write → peer_review |
| **Hiring** | `recruit-flow` | source → screen → interview → offer |

**How it works:**
```
1. Issue created with pipelineId → starts at first stage
   → assigneeAgentId = stage[0].agent
   → currentStage = stage[0].name
   → Agent wakes, works on it

2. Agent completes → calls POST /api/issues/{id}/advance
   → Server moves to next stage automatically
   → assigneeAgentId = stage[1].agent
   → currentStage = stage[1].name
   → Next agent wakes (triggered by assignment)

3. Each agent advances until last stage completes → issue status = done

4. Any agent can REJECT back to previous stage:
   → POST /api/issues/{id}/reject-stage  (with comment explaining why)
   → Issue returns to previous stage, previous agent re-assigned
   → e.g., editor sends back to writer with revision notes
```

**Issue status mapping:**
- Issue `status` stays `in_progress` throughout the pipeline — the `currentStage` field tracks where it actually is
- When last stage advances → status = `done`
- When rejected → status stays `in_progress`, `currentStage` moves back
- User can override: pause, block, or cancel at any stage

**Constraints:**
- Pipeline is optional — issues without pipelineId work as before (single assignee)
- Stage names are freeform strings (not enums) — squads define their own vocabulary
- Stages are sequential (no parallel stages in v1)
- Each stage has exactly one assignee agent
- `advance` and `reject-stage` are atomic operations (like checkout)
- Pipelines are squad-level templates — define once, reuse on many issues
- Pipelines are included in squad export/import templates

#### IssueComment
Discussion thread on an issue.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| issueId | FK | Parent issue |
| authorType | enum | agent, user, system |
| authorId | UUID | Who wrote it |
| body | text | Comment content (markdown) |

#### Conversations (Real-Time Agent Chat)
A conversation is an issue with `type=conversation`. Users chat with agents in real time — each message spawns an agent session that processes and replies immediately.

**Flow:**
```
1. USER SENDS    → User opens conversation with an agent (or starts new one)
                   → IssueComment created (authorType=user)
                   → Server immediately spawns agent session

2. AGENT SPAWNS  → Ari starts a new Claude Code session (via adapter)
                   → Agent receives full conversation history as context
                   → Env: ARI_WAKE_REASON=conversation_message
                   → Env: ARI_CONVERSATION_ID=<issue_id>

3. AGENT WORKS   → Agent processes the message
                   → Agent has full Ari API access during the session:
                       POST /api/squads/{id}/issues    (create tasks for other agents)
                       PATCH /api/issues/{id}           (assign to agents)
                       GET  /api/squads/{id}/issues     (search existing work)
                       POST /api/squads/{id}/approvals  (request approvals)
                   → Agent posts reply: POST /api/issues/{id}/comments

4. USER SEES     → SSE pushes issue.comment.created to UI
                   → User sees agent's response in real time

5. CONTINUE      → User sends another message → back to step 1
                   → Agent session state preserved across turns
                   → Conversation continues until user closes it
```

**Session Continuity:**
- Each conversation maintains its own session state (separate from task sessions)
- Agent resumes from previous session on each new message → maintains context
- Session state stored per-conversation: `AgentConversationSession(agentId, issueId, sessionState)`

**Agent Context Injection:**
When spawned for a conversation, the agent receives a prompt containing:
- The full comment thread (or last N messages if thread is long)
- The agent's role, squad context, and system prompt
- Any referenced issues/projects mentioned in the conversation

**Example — User delegates work via chat:**
```
User:    "Hey Captain, we need to add dark mode to the dashboard.
          Break it into tasks and assign to the frontend team."

Captain: "Got it. I've created 3 issues:
          • ARI-42: Update Tailwind config for dark tokens → assigned to Member-CSS
          • ARI-43: Add theme toggle component → assigned to Member-UI
          • ARI-44: Update all pages to use theme tokens → assigned to Member-UI
          I set priority to medium. Want me to adjust anything?"

User:    "Make ARI-42 high priority, it blocks the others."

Captain: "Done. ARI-42 is now high priority and I've marked ARI-43
          and ARI-44 as blocked by ARI-42."
```

Behind the scenes, each Captain reply is a full agent session where the Captain calls the Ari API to create issues, set assignments, and update priorities.

**Constraints:**
- One active agent session per conversation at a time (new message queued if agent is still processing)
- No streaming of partial agent output (agent posts complete reply as a comment)
- Conversations appear in the issue list, filterable by `type=conversation`
- Cost tracked per conversation via CostEvents linked to the heartbeat run

#### Heartbeat Run
Each time an agent executes, a heartbeat run is recorded. *(Phase 2+)*

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
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
The unified inbox for everything that needs human attention. One model, one queue, one UI.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| category | enum | approval, question, decision, alert |
| type | string | Specific action (see table below) |
| status | enum | pending, resolved, dismissed, expired |
| urgency | enum | critical, normal, low |
| title | string | Short summary for inbox list |
| body | text | Full context (markdown) |
| requestedByAgentId | FK | Which agent created this |
| relatedIssueId | FK | Linked issue (if any) |
| payload | JSONB | Structured data (options, context, attachments) |
| responseNote | text | User's answer/reasoning |
| responsePayload | JSONB | Structured response (selected option, values) |
| resolvedByUserId | FK | Who resolved |
| resolvedAt | timestamp | When resolved |

**Categories:**

| Category | Purpose | Examples |
|----------|---------|---------|
| **approval** | Agent needs permission to act | `recruit_agent`, `captain_request`, `budget_increase`, `terminate_agent` |
| **question** | Agent needs information from user | `clarify_requirement`, `confirm_approach`, `request_credentials`, `missing_context` |
| **decision** | Agent presents options, user picks | `choose_architecture`, `prioritize_tasks`, `resolve_conflict`, `select_vendor` |
| **alert** | FYI that may need action | `budget_warning`, `agent_error`, `task_blocked`, `security_flag` |

**Resolution behavior by category:**

| Category | User action | What happens next |
|----------|-------------|-------------------|
| **approval** | approve / reject / request revision | Agent woken with `ARI_WAKE_REASON=inbox_resolved`, payload contains decision |
| **question** | types answer in responseNote | Agent woken, receives answer in context |
| **decision** | selects from options in payload | Agent woken, receives selected option |
| **alert** | dismiss / acknowledge / take action | Optional agent wake if action taken |

**Agent creates inbox items via API:**
```
POST /api/squads/{id}/inbox
{
  "category": "question",
  "type": "clarify_requirement",
  "title": "Need clarification on auth flow",
  "body": "Should login support SSO or just email/password for v1?",
  "relatedIssueId": "...",
  "urgency": "normal",
  "payload": {
    "options": ["email_only", "email_and_sso", "sso_only"]
  }
}
```

#### Cost Event
Immutable record of every token/dollar spent. Used to compute agent and squad spend.

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| squadId | FK | Squad scope |
| agentId | FK | Which agent |
| heartbeatRunId | FK | Which run generated this cost |
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
| squadId | FK | Squad scope |
| actorType | enum | agent, user, system |
| actorId | UUID | Who did it |
| action | string | e.g., "issue.created", "agent.paused" |
| entityType | string | What was affected |
| entityId | UUID | Which entity |
| details | JSONB | Action-specific context |

### 4.3 Phase Boundaries

| Entity | Phase 1 (v0.1) | Phase 2 (v0.2) | Phase 3 (v0.3) |
|--------|---------------|----------------|----------------|
| User | Yes | | |
| Squad | Yes | | |
| SquadMembership | Yes | | |
| Agent | Yes (basic fields) | + adapter, runtime, session | |
| Project | Yes | | |
| Goal | Yes | | |
| Issue | Yes (no checkout) | + checkout/lock | |
| IssueComment | Yes | | |
| Conversation (issue type) | | Yes (requires adapter) | |
| HeartbeatRun | | Yes | |
| WakeupRequest | | Yes | |
| IssuePipeline | | Yes | |
| AgentAPIKey | | Yes | |
| AgentConfigRevision | | Yes | |
| InboxItem | | Yes (alerts) | Yes (approvals, questions, decisions) |
| CostEvent | | Yes | Yes (+ budget enforcement) |
| ActivityLog | | | Yes |
| SquadSecret | | | | Phase 4 |
| PermissionGrant | | | Yes |

---

## 5. Core Workflows

### 5.1 Deploy — Agent Lifecycle

#### 5.1.1 One-Command Setup

```bash
./ari run
# 1. Auto-onboard if no config exists (quickstart defaults)
# 2. Extract embedded PostgreSQL, initialize cluster
# 3. Run migrations
# 4. Start API server on :3100
# 5. Serve React UI
# 6. Open browser
```

#### 5.1.2 Agent Recruitment & Onboarding

```
User creates squad with goal
    ↓
User/Captain creates agent (or agent requests recruit)
    ↓
If requireApprovalForNewAgents:
    → Approval created (status: pending)
    → User reviews and approves/rejects
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
1. TRIGGER → timer | assignment | @mention | manual | inbox_resolved | conversation_message
2. QUEUE  → wakeup request created (deduplicated, prioritized)
3. INVOKE → adapter spawns agent with JWT + env vars + prompt
4. WORK   → agent calls Ari API:
             GET  /api/agent/me/assignments     (what to work on)
             POST /api/issues/{id}/checkout      (claim task - atomic lock)
             PATCH /api/issues/{id}              (update progress)
             POST /api/issues/{id}/comments      (add notes)
             POST /api/squads/{id}/issues        (create sub-tasks)
             POST /api/squads/{id}/inbox         (ask user a question / request decision)
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

#### 5.2.1 Inbox — Unified Human-in-the-Loop

Everything that needs the user's attention goes through one place: the **Inbox**.

| Category | When | Example |
|----------|------|---------|
| **approval** | Agent needs permission to act | Recruit agent, captain request, budget increase |
| **question** | Agent needs information | "Should login support SSO?", "Where's the API spec?" |
| **decision** | Agent presents options, user picks | "3 architecture options — which one?", "Prioritize these 5 tasks" |
| **alert** | Something happened that may need action | Budget at 80%, agent error, task blocked |

**Flow:**
```
Agent (or system) creates inbox item → pending
    ↓
User sees it in Inbox UI (sorted by urgency)
    ↓
User responds:
    ├── approval  → approve / reject
    ├── question  → types answer
    ├── decision  → selects option
    └── alert     → dismiss / take action
    ↓
Agent woken with ARI_WAKE_REASON=inbox_resolved
    → receives user's response in context
    → continues work
```

#### 5.2.2 User Powers

The user (human) has unrestricted control:

1. **Pause any agent** — immediately stops future heartbeats
2. **Pause/resume any task** — freezes work on issue subtree
3. **Override any assignment** — reassign tasks between agents
4. **Override any budget** — increase/decrease agent or squad limits
5. **Terminate agents** — permanently remove an agent from the squad
6. **Full project management** — create, edit, delete, comment on any issue
7. **Approve/reject all requests** — governance decisions
8. **View all activity** — complete audit trail

#### 5.2.3 Budget Enforcement (Three Tiers)

```
Agent Budget: $100/month
    ├── At $0-$79:   ✅ Normal operation
    ├── At $80:       ⚠️  Soft alert (80% warning)
    └── At $100:      🛑 Hard stop (auto-pause agent)

Squad Budget: $1000/month
    └── Same tiers apply at squad level
```

Budget tracking is atomic — cost events update agent and squad spend in a single transaction.

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
SquadMembership (links principal to squad)
    ↓
 PermissionGrant (specific permission + optional scope)
    ├── agents:recruit     — can add agents to squad
    ├── tasks:assign       — can assign work
    ├── users:invite       — can invite members
    ├── users:manage_permissions — can grant/revoke permissions
    └── joins:approve      — can approve join requests
```

Implicit permissions:
- Captain has all permissions within their squad
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

#### 5.3.1 Portable Squad Templates

Export an entire squad configuration:

```bash
ari export --squad my-dev-team --output template.json
# Exports: agents, hierarchy, projects, goals, workflows
# Scrubs: secrets, API keys, PII
```

Import into another Ari instance:

```bash
ari import --template template.json --squad new-team
# Creates: squad, agents, projects, goals
# Prompts: for missing secrets and credentials
```

#### 5.3.2 Agent Marketplace (Future)

```
┌─────────────────────────────────────┐
│  Ari Marketplace                     │
│                                     │
│  📦 "Full-Stack Dev Squad"          │
│     Captain + 3 engineers + QA      │
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
  X-Ari-Run-Id: <run-uuid>          (agent mutations)
  Content-Type: application/json
```

Token types:
- **Agent JWT** — short-lived (48h), issued per heartbeat run
- **Agent API Key** — long-lived, stored as SHA-256 hash
- **User Session** — cookie-based, for users

### 6.2 Endpoint Summary

#### Squads
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/squads` | List squads |
| GET | `/api/squads/:id` | Get squad |
| POST | `/api/squads` | Create squad |
| PATCH | `/api/squads/:id` | Update squad |
| PATCH | `/api/squads/:id/budgets` | Update budget |
| POST | `/api/squads/:id/export` | Export template |
| POST | `/api/squads/import` | Import template |

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
| GET | `/api/squads/:id/issues` | List issues (filterable) |
| GET | `/api/issues/:id` | Get issue (UUID or "ARI-39") |
| POST | `/api/squads/:id/issues` | Create issue |
| PATCH | `/api/issues/:id` | Update issue |
| DELETE | `/api/issues/:id` | Delete issue |
| POST | `/api/issues/:id/checkout` | Atomic task claim |
| POST | `/api/issues/:id/release` | Release lock |
| POST | `/api/issues/:id/advance` | Advance to next pipeline stage |
| POST | `/api/issues/:id/reject-stage` | Reject back to previous stage |
| POST | `/api/issues/:id/comments` | Add comment |
| POST | `/api/issues/:id/attachments` | Upload file |

#### Conversations
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/agents/:id/conversations` | Start new conversation with agent |
| GET | `/api/agents/:id/conversations` | List agent's conversations |
| POST | `/api/conversations/:id/messages` | Send message (creates comment + spawns agent) |
| GET | `/api/conversations/:id/messages` | Get conversation messages |
| PATCH | `/api/conversations/:id/close` | Close conversation |

> Note: Conversations are issues with `type=conversation`. The `/conversations` routes are convenience aliases — `POST /api/conversations/:id/messages` is equivalent to `POST /api/issues/:id/comments` but also triggers immediate agent invocation.

#### Agent Self-Service (token-scoped)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/agent/me` | Agent's own profile |
| GET | `/api/agent/me/assignments` | Issues assigned to this agent |
| GET | `/api/agent/me/squad` | Squad context (name, goals, projects) |
| GET | `/api/agent/me/team` | Direct reports (Captain/Lead) or siblings + lead |
| GET | `/api/agent/me/goals` | Goals relevant to this agent |
| GET | `/api/agent/me/budget` | Agent's spend and budget status |
| GET | `/api/agent/me/conversations` | Conversations assigned to this agent |
| GET | `/api/agent/me/session` | Persisted session state |
| PUT | `/api/agent/me/session` | Save session state |

> All `/agent/me/*` responses are scoped by the Run Token JWT — the agent only sees its own data based on its role. See section 7.3.

#### Projects & Goals
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/squads/:id/projects` | List projects |
| POST | `/api/squads/:id/projects` | Create project |
| PATCH | `/api/projects/:id` | Update project |
| GET | `/api/squads/:id/goals` | List goals |
| POST | `/api/squads/:id/goals` | Create goal |
| PATCH | `/api/goals/:id` | Update goal |

#### Pipelines (Issue Stage Workflows)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/squads/:id/pipelines` | List squad pipelines |
| POST | `/api/squads/:id/pipelines` | Create pipeline template |
| GET | `/api/pipelines/:id` | Get pipeline detail |
| PATCH | `/api/pipelines/:id` | Update pipeline stages |
| DELETE | `/api/pipelines/:id` | Delete pipeline |

#### Inbox (Unified Human Actions)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/squads/:id/inbox` | List inbox items (filterable by category, status, urgency) |
| GET | `/api/inbox/:id` | Get inbox item detail |
| POST | `/api/squads/:id/inbox` | Create inbox item (agent or system) |
| PATCH | `/api/inbox/:id/resolve` | Resolve: approve, answer, select, dismiss |
| PATCH | `/api/inbox/:id/dismiss` | Dismiss (alerts only) |

#### Costs & Dashboard
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/squads/:id/cost-events` | Report usage |
| GET | `/api/squads/:id/costs/summary` | Spend overview |
| GET | `/api/squads/:id/costs/by-agent` | Agent breakdown |
| GET | `/api/dashboard/:id` | Dashboard metrics |

#### Secrets
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/squads/:id/secrets` | List secrets |
| POST | `/api/squads/:id/secrets` | Create secret |
| POST | `/api/secrets/:id/rotate` | Rotate secret |

#### Activity & Access
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/squads/:id/activity` | Activity log |
| GET | `/api/squads/:id/members` | List members |
| POST | `/api/squads/:id/permissions` | Grant permission |

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
| 403 | Unauthorized | Escalate to lead |
| 404 | Not found | Skip |
| 409 | Conflict (task locked) | DO NOT retry |
| 422 | Invalid state transition | Check current state |
| 500 | Server error | Log and move on |

### 6.4 Real-time Events (SSE)

```
GET /api/squads/:id/events/stream
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
conversation.agent.typing        # agent session started, processing message
conversation.agent.replied       # agent posted reply comment
inbox.item.created              # new item needs attention
inbox.item.resolved             # user resolved an item
activity.appended
cost.threshold.warning
```

---

## 7. Agent Environment Contract

Every agent spawn receives a **unique, short-lived Run Token** (JWT). This token IS the agent's identity — the server extracts agent ID, squad ID, and run ID from it. No other credentials are needed.

#### 7.1 Run Token (Per-Spawn JWT)

Every time Ari spawns an agent (heartbeat, conversation, manual), it mints a new JWT:

```
JWT Claims:
{
  "sub":       "<agent_id>",         // who this agent is
  "squad_id":  "<squad_id>",         // which squad
  "run_id":    "<heartbeat_run_id>", // this specific execution
  "role":      "captain|lead|member",// agent's role in the squad
  "conv_id":   "<issue_id|null>",    // conversation context (if any)
  "iat":       1710000000,
  "exp":       1710172800            // 48h TTL
}
```

**Security guarantees:**
- One token per spawn — never reused across runs
- Server verifies signature on every API call — agent cannot forge identity
- Token scopes all API responses to this agent's data (see 7.3)
- Token invalidated when run completes or agent is paused/terminated
- Stolen token is limited: short TTL, scoped to one agent in one squad

#### 7.2 Environment Variables

| Variable | Description |
|----------|-------------|
| `ARI_API_URL` | Base URL (e.g., `http://localhost:3100`) |
| `ARI_API_KEY` | Run Token (JWT) — the ONLY credential the agent needs |
| `ARI_AGENT_ID` | Agent's UUID (convenience — also in JWT) |
| `ARI_SQUAD_ID` | Squad's UUID (convenience — also in JWT) |
| `ARI_RUN_ID` | Current heartbeat run ID (convenience — also in JWT) |
| `ARI_TASK_ID` | Issue that triggered wake (if any) |
| `ARI_CONVERSATION_ID` | Conversation issue ID (if wake_reason=conversation_message) |
| `ARI_WAKE_REASON` | Why agent woke (issue_assigned, timer, mention, manual, inbox_resolved, conversation_message) |

#### 7.3 Scoped API Responses (Agent Identity → Agent Context)

The server uses the Run Token to identify the calling agent and scopes all `/agent/me/*` responses to **only data relevant to that agent**:

| Endpoint | Returns (scoped by token) |
|----------|--------------------------|
| `GET /api/agent/me` | Agent's own profile: name, role, status, adapter, config |
| `GET /api/agent/me/assignments` | Issues assigned to this agent only |
| `GET /api/agent/me/squad` | Squad info: name, goals, projects (read-only context) |
| `GET /api/agent/me/team` | Direct reports (if Captain/Lead) or siblings + lead |
| `GET /api/agent/me/goals` | Goals relevant to agent's projects/assignments |
| `GET /api/agent/me/budget` | Agent's own spend and budget status |
| `GET /api/agent/me/conversations` | Conversations assigned to this agent |
| `GET /api/agent/me/session` | Agent's persisted session state |

**How role shapes the response:**

| Role | Can see | Can do |
|------|---------|--------|
| **Captain** | All squad issues, all agents, all projects/goals | Create issues, assign to any agent, propose strategy |
| **Lead** | Own sub-team's issues, own direct reports | Create issues, assign to direct reports |
| **Member** | Only own assigned issues | Update own issues, post comments, create sub-tasks |

**Server enforcement:**
- Every API call is verified against the JWT — no trust of env vars or headers
- An agent calling `GET /api/squads/:id/issues` only sees issues it has access to based on role
- Write operations (POST, PATCH) are checked: member can't assign to agents outside its scope
- Cross-squad calls return 403 — token is squad-scoped
- Terminated/paused agent tokens are rejected with 401

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
    Agent          AgentContext
    Squad          SquadContext
    Run            RunContext
    Workspace      WorkspaceConfig
    EnvVars        map[string]string
    Prompt         string
    Conversation   *ConversationContext  // set when wake_reason=conversation_message
}

type ConversationContext struct {
    IssueID        string           // conversation issue ID
    Messages       []CommentEntry   // full thread (or last N messages)
    SessionState   string           // previous session state for this conversation
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

Ari assumes agents are capable but not trustworthy by default. Every critical path has a human checkpoint.

### 9.2 HITL Touchpoints

| Touchpoint | Trigger | Human Action |
|------------|---------|-------------|
| **Agent Recruitment** | Agent requests new teammate | User approves/rejects |
| **Strategy Approval** | Captain proposes direction | User approves/rejects |
| **Budget Override** | Agent exceeds budget | User increases or keeps paused |
| **Task Review** | Agent marks issue `in_review` | User moves to `done` or returns |
| **Emergency Pause** | User detects problem | User pauses agent immediately |
| **Comment Direction** | Agent asks for guidance | User comments on issue |
| **Escalation** | Agent can't resolve conflict | User decides priority/assignment |

### 9.3 Notification & Awareness

Users see:
- **Dashboard** — real-time agent status, cost trends, inbox count
- **Inbox** — unified queue: approvals, questions, decisions, alerts (sorted by urgency)
- **Conversations** — chat with any agent via conversation issues
- **Activity Feed** — chronological audit trail
- **SSE Events** — real-time push for status changes
- **Cost Alerts** — warnings at 80% budget threshold

### 9.4 Override Mechanics

User overrides are:
- **Immediate** — take effect on next heartbeat (not current run)
- **Logged** — every override creates an activity log entry
- **Reversible** — pause can be resumed, assignments can be re-assigned
- **Commentable** — user can explain reasoning via comments

---

## 10. Data Invariants & Constraints

### 10.1 Hard Invariants

| Invariant | Enforcement |
|-----------|------------|
| All entities belong to exactly one squad | FK + application logic |
| Issue identifiers are unique within squad | Unique constraint |
| Agent shortnames are unique within squad | Unique constraint |
| Only one agent can hold a task checkout | Atomic DB transaction |
| Cost events are append-only (never updated) | No UPDATE/DELETE routes |
| Activity log is append-only (never updated) | No UPDATE/DELETE routes |
| Agent hierarchy is a strict tree (no cycles) | Application validation |
| Secret values are encrypted at rest | Encryption before storage |
| Budget spend is atomically updated with cost events | DB transaction |

### 10.2 Status Machines

**Agent Status:**
```
pending_approval → active (user approves)
active → idle (no work)
idle → running (heartbeat triggered)
running → idle (heartbeat complete)
running → error (heartbeat failed)
active/idle/running → paused (user or budget)
paused → active (user resumes)
any → terminated (user removes from squad)
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
~/.ari/
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

| Tier | Method | Audience | Scope |
|------|--------|----------|-------|
| **Run Token** | HS256 JWT (48h TTL) | Per-spawn agent auth | agent + squad + run + role |
| **Agent API Key** | SHA-256 hashed long-lived token | Agent self-service (between runs) | agent + squad |
| **User Session** | Cookie-based session | Users via UI | user + squad memberships |
| **Local Trusted** | Implicit (no auth) | Single-operator local mode | full access |

**Run Token lifecycle:**
1. Ari mints a new JWT before every agent spawn (heartbeat, conversation, manual)
2. JWT injected as `ARI_API_KEY` env var — the only credential the agent receives
3. Server verifies JWT on every API call → extracts agent identity + role
4. Token expires after 48h or when run completes (whichever is first)
5. Paused/terminated agents: existing tokens rejected immediately (server-side revocation list)

### 12.2 Security Measures

- **Per-spawn isolation**: every agent session gets its own unique token — no shared credentials
- **Identity from token, not env vars**: server trusts only the JWT signature, not `ARI_AGENT_ID` etc.
- **Role-based response scoping**: API responses filtered by agent role (Captain sees all, Member sees own)
- **Write permission checks**: agent can only modify entities within its role scope
- Secrets encrypted at rest with AES-256 (master key file)
- Secret values never logged or returned in API responses
- API keys stored as SHA-256 hashes (never plaintext)
- Squad-scoped data isolation (agents can't cross squads)
- Private hostname guard for authenticated deployments
- Activity log for complete audit trail

---

## 13. Metrics & Observability

### 13.1 Dashboard Metrics

| Metric | Description |
|--------|-------------|
| Active agents | Count of agents in active/running status |
| Pending approvals | Count of approvals awaiting decision |
| Monthly spend | Total cost this month (squad and per-agent) |
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
- [ ] Go project scaffold (Cobra CLI + net/http server + `ari run`)
- [ ] Embedded PostgreSQL integration (embedded-postgres-go)
- [ ] Database schema + sqlc codegen + goose migrations
- [ ] User + auth (JWT + bcrypt + cookie sessions + rate limiting)
- [ ] Squad CRUD + SquadMembership (owner/admin/viewer)
- [ ] Agent CRUD with hierarchy (captain/lead/member)
- [ ] Issue CRUD with status machine + sub-tasks
- [ ] IssueComment (discussion threads)
- [ ] Project + Goal CRUD
- [ ] Basic React UI (dashboard, squads, agents, issues, projects)
- [ ] REST API with error handling + validation

### Phase 2: Execution (v0.2)
- [ ] Per-spawn Run Token (JWT) — unique per agent session
- [ ] Agent self-service API (`/agent/me/*` — scoped by token + role)
- [ ] Adapter interface + `process` adapter
- [ ] `claude_local` adapter (Claude Code CLI)
- [ ] Heartbeat scheduler (goroutine-based)
- [ ] Wakeup queue with deduplication + priority
- [ ] Task checkout/release (atomic CAS locking)
- [ ] Session persistence (per-agent + per-task)
- [ ] Conversations (real-time agent chat via issue type)
- [ ] IssuePipeline (custom stage workflows — code/review/qa etc.)
- [ ] InboxItem (alerts: budget warnings, agent errors)
- [ ] Cost event recording + attribution (always-on tracking)
- [ ] SSE real-time events

### Phase 3: Governance (v0.3)
- [ ] InboxItem full (approvals, questions, decisions)
- [ ] Budget enforcement (soft alert 80%, hard stop 100% when set)
- [ ] Activity log (immutable audit trail)
- [ ] Permission grants (RBAC — role-based API scoping)
- [ ] User override controls (pause/resume/terminate/reassign)

### Phase 4: Production (v0.4)
- [ ] Advanced auth features (OAuth, SSO, 2FA)
- [ ] Secret management (AES-256 encrypted storage + rotation)
- [ ] Agent API keys (long-lived, SHA-256 hashed)
- [ ] Database backup/restore
- [ ] S3 storage adapter
- [ ] Docker image
- [ ] CLI onboarding wizard (`ari init`)

### Phase 5: Share (v0.5)
- [ ] Squad export/import (portable templates with pipelines)
- [ ] `codex_local` + `cursor_local` adapters
- [ ] HTTP webhook adapter
- [ ] Multi-squad support
- [ ] Marketplace foundation

---

## 15. Success Metrics

| Metric | Target |
|--------|--------|
| Time to first agent running | < 5 minutes from `./ari run` |
| Binary size | < 60MB (with embedded PG) |
| Memory usage (idle) | < 50MB |
| Memory usage (10 agents) | < 100MB |
| API response time (p95) | < 50ms |
| Heartbeat overhead | < 100ms per invocation |
| Zero-downgrade migrations | 100% backward compatible |

---

## 16. Competitive Positioning

| Feature | Ari | Paperclip | CrewAI | AutoGen | LangGraph |
|---------|-----|-----------|--------|---------|-----------|
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
| Squad templates | Yes | Partial | No | No | No |
| Marketplace | Planned | Planned | No | No | No |

---

## Appendix A: Glossary

| Term | Definition |
|------|-----------|
| **User** | Human who governs the AI squad |
| **Agent** | AI member on the squad |
| **Captain** | Lead agent with full squad permissions |
| **Lead** | Agent who manages a sub-team of members |
| **Member** | Standard agent executing tasks |
| **Heartbeat** | Single execution window for an agent |
| **Wakeup** | Signal that triggers a heartbeat |
| **Adapter** | Plugin that connects Ari to an AI runtime |
| **Checkout** | Atomic lock on a task by an agent |
| **Squad** | Top-level organizational unit (team of agents) |
| **Issue** | Unit of work (task) |
| **Approval** | Governance gate requiring human decision |
| **Cost Event** | Record of token/dollar spend |
| **Activity Log** | Immutable audit entry |
| **Session** | Persistent state across heartbeats |
| **Paused** | Agent temporarily stopped by user or budget |
| **Terminated** | Agent permanently removed from squad |
| **HITL** | Human-in-the-Loop |

## Appendix B: Name Origin

**Ari** — Means "lion" in multiple languages — symbolizing strength, leadership, and vigilance. In production AI, the control plane must be the steadfast guardian: watching over agents, enforcing governance, and ensuring every action is accountable. Deploy. Govern. Share.

---

*Deploy. Govern. Share.*
*The Control Plane for AI Agents.*
