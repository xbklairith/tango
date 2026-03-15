# Ari — Roadmap

> Spec-driven development plan. Each feature has requirements, design, and TDD tasks.

---

## What's Built (v0.1 — Foundation + Early Execution)

| # | Feature | Status | Endpoints | Tests |
|---|---------|--------|-----------|-------|
| 01 | Go Scaffold + CLI | Done | 1 | 12 |
| 02 | User Authentication | Done | 4 | 18 |
| 03 | Squad Management | Done | 6 | 14 |
| 04 | Agent Management | Done | 5 | 16 |
| 05 | Issue Tracking | Done | 7 | 22 |
| 06 | Projects & Goals | Done | 8 | 12 |
| 07 | React UI Foundation | Done | — | — |
| 08 | UI Interactivity | Done | — | — |
| 09 | Activity Log | Done | 1 | 6 |
| 10 | Cost Events & Budget | Done | 3 | 8 |
| 11 | Agent Runtime | Done | 6 | 10 |
| — | Golden Agent Journey | Done | — | 1 E2E |
| — | Agent Console (UI) | Done | — | — |
| 12 | Inbox System | Done | 7 | — |
| 13 | Conversations | Done | 5 | — |
| 20 | Claude Adapter | Done | — | 12 |

**Total: 61 API endpoints, 15 DB migrations, 86 React components**

---

## What's Next

### Phase 2: Execution (v0.2)

> Make agents actually useful. Conversations, multi-agent pipelines, richer self-service.

#### Feature 12: Inbox System (Human-in-the-Loop Queue) — DONE
**Priority: HIGH** | Complexity: L | Depends on: 09, 10, 11

| Task | Description |
|------|-------------|
| 12.1 | DB migration: `inbox_items` table (category, urgency, status, resolution) |
| 12.2 | InboxItem domain model + status machine (pending → acknowledged → resolved) |
| 12.3 | CRUD endpoints: `POST/GET /api/squads/{id}/inbox`, `PATCH /api/inbox/{id}` |
| 12.4 | Resolution actions: approve, reject, answer, dismiss |
| 12.5 | Budget warning auto-creation (integrate with BudgetEnforcementService) |
| 12.6 | Agent error auto-creation (integrate with RunService finalize) |
| 12.7 | SSE events: `inbox.created`, `inbox.resolved` |
| 12.8 | React UI: Inbox page with filters, badge count in sidebar |
| 12.9 | Agent self-service: `GET /api/agent/me/inbox` (agent's pending items) |
| 12.10 | Integration tests + E2E test |

---

#### Feature 13: Conversations (Agent Chat Interface) — DONE
**Priority: HIGH** | Complexity: XL | Depends on: 11, 12

| Task | Description |
|------|-------------|
| 13.1 | Conversation = Issue with `type=conversation` (reuse existing table) |
| 13.2 | `POST /api/conversations/{id}/messages` — user sends message |
| 13.3 | Message → wakeup agent with conversation context |
| 13.4 | Agent replies via `POST /api/agent/me/reply` |
| 13.5 | Session continuity: load/save conversation session per agent+issue |
| 13.6 | SSE events: `conversation.message`, `conversation.typing` |
| 13.7 | React UI: Chat component with message bubbles, typing indicator |
| 13.8 | Conversation list page (filter by agent, status) |
| 13.9 | Agent console integration (show conversation activity) |
| 13.10 | Integration tests + E2E test |

---

#### Feature 14: Issue Pipelines (Multi-Agent Workflows)
**Priority: MEDIUM** | Complexity: L | Depends on: 11, 13 | **Status: Spec'd**

Enable multi-agent collaboration. An issue flows through stages, each handled by a different agent.

```
Spec ready:
  docx/features/14-issue-pipelines/requirements.md  ✓
  docx/features/14-issue-pipelines/design.md         ✓
  docx/features/14-issue-pipelines/tasks.md          (pending)
```

| Task | Description |
|------|-------------|
| 14.1 | DB migration: `pipelines` + `pipeline_stages` tables |
| 14.2 | Pipeline CRUD: `POST/GET /api/squads/{id}/pipelines` |
| 14.3 | Stage definition: name, assigned agent, order |
| 14.4 | Issue attachment: `PATCH /api/issues/{id}` with `pipelineId` |
| 14.5 | Stage advancement: `POST /api/issues/{id}/advance` |
| 14.6 | Auto-wake next agent on stage transition |
| 14.7 | Stage rejection: `POST /api/issues/{id}/reject` (back to previous) |
| 14.8 | React UI: Pipeline builder, issue stage indicator |
| 14.9 | Integration tests |

---

#### Feature 15: Agent Self-Service API (Extended)
**Priority: MEDIUM** | Complexity: M | Depends on: 11, 12, 13 | **Status: Spec'd**

Expand what agents can query about themselves and their environment.

```
Spec ready:
  docx/features/15-agent-self-service-extended/requirements.md  ✓
  docx/features/15-agent-self-service-extended/design.md         ✓
  docx/features/15-agent-self-service-extended/tasks.md          (pending)
```

| Task | Description |
|------|-------------|
| 15.1 | `GET /api/agent/me/assignments` — all assigned issues (not just active) |
| 15.2 | `GET /api/agent/me/team` — sibling agents in hierarchy |
| 15.3 | `GET /api/agent/me/budget` — remaining budget for current billing period |
| 15.4 | `GET /api/agent/me/goals` — goals linked to assigned issues |
| 15.5 | `POST /api/agent/me/inbox` — agent creates inbox item (ask human) |
| 15.6 | `POST /api/agent/me/reply` — agent replies to conversation |
| 15.7 | `POST /api/agent/me/cost` — agent reports its own cost event |
| 15.8 | Role-based response filtering (captain sees more than member) |
| 15.9 | Integration tests |

---

### Phase 3: Governance (v0.3)

> Make agents trustworthy. Full approval gates, permissions, override controls.

#### Feature 16: Approval Gates — **Status: Spec'd**
**Priority: HIGH** | Complexity: M | Depends on: 12

Agents must request approval before taking critical actions. Configurable per squad.

```
Spec ready:
  docx/features/16-approval-gates/requirements.md  ✓
  docx/features/16-approval-gates/design.md         ✓
  docx/features/16-approval-gates/tasks.md          (pending)
```

| Task | Description |
|------|-------------|
| 16.1 | Squad settings: `approvalRequired` for actions (deploy, delete, spend > $X) |
| 16.2 | Agent creates approval request → inbox item |
| 16.3 | Approval resolution → agent receives callback |
| 16.4 | Timeout handling (auto-reject after N hours) |
| 16.5 | React UI: Approval queue with approve/reject buttons |

---

#### Feature 17: Permission Grants (RBAC) — **Status: Spec'd**
**Priority: MEDIUM** | Complexity: M | Depends on: 12

Fine-grained role-based access control beyond owner/admin/viewer.

| Task | Description |
|------|-------------|
| 17.1 | Permission model: resource + action + role matrix |
| 17.2 | API enforcement middleware |
| 17.3 | Agent role scoping (captain can create sub-issues, member cannot) |
| 17.4 | React UI: Role management page |

---

#### Feature 18: Dashboard Observability — **Status: Spec'd**
**Priority: MEDIUM** | Complexity: M | Depends on: 09, 10, 11, 12

Single-pane-of-glass metrics dashboard.

| Task | Description |
|------|-------------|
| 18.1 | Aggregated metrics endpoint: `GET /api/squads/{id}/metrics` |
| 18.2 | Cost trend chart (daily/weekly/monthly) |
| 18.3 | Agent health table (uptime, error rate, last run) |
| 18.4 | Issue velocity widget (created vs closed over time) |
| 18.5 | Inbox badge count via SSE |
| 18.6 | React UI: Dashboard widgets with charts |

---

### Phase 4: Production (v0.4)

> Make it deployable. Security hardening, secrets, backups, Docker.

#### Feature 19: Secrets Management — **Status: Spec'd**
**Priority: HIGH** | Complexity: M | Depends on: 11

Agents need API keys, tokens, credentials. Store them securely.

| Task | Description |
|------|-------------|
| 19.1 | DB migration: `squad_secrets` (AES-256-GCM encrypted) |
| 19.2 | Master key auto-generation + storage |
| 19.3 | CRUD endpoints: `POST/GET/DELETE /api/squads/{id}/secrets` |
| 19.4 | Key rotation: `POST /api/squads/{id}/secrets/{name}/rotate` |
| 19.5 | Adapter injection: secrets passed as env vars to spawned agents |
| 19.6 | React UI: Secret management page (masked values) |

---

#### Feature 20: Claude Adapter
**Priority: HIGH** | Complexity: L | Depends on: 11, 19

First-class Claude Code integration. The flagship adapter.

| Task | Description |
|------|-------------|
| 20.1 | `claude_local` adapter: spawn Claude Code CLI as subprocess |
| 20.2 | Structured log parsing (tool calls → console entries) |
| 20.3 | Session continuity via Claude's session files |
| 20.4 | Model selection (opus/sonnet/haiku) via agent config |
| 20.5 | Cost extraction from Claude's usage output |
| 20.6 | System prompt injection via CLAUDE.md |
| 20.7 | E2E test: Claude agent resolves issue |

---

#### Feature 21: Production Hardening — **Status: Spec'd**
**Priority: MEDIUM** | Complexity: L

| Task | Description |
|------|-------------|
| 21.1 | OAuth/SSO support (Google, GitHub) |
| 21.2 | Database backup/restore commands (`ari backup`, `ari restore`) |
| 21.3 | Docker image + Dockerfile |
| 21.4 | CLI onboarding wizard (`ari init`) |
| 21.5 | Rate limiting on all endpoints |
| 21.6 | Request size limits |
| 21.7 | HTTPS/TLS support |

---

### Phase 5: Share (v0.5)

> Make it viral. Templates, marketplace, multi-org.

#### Feature 22: Squad Templates
| Task | Description |
|------|-------------|
| 22.1 | `ari export` — export squad as portable YAML |
| 22.2 | `ari import` — import squad template |
| 22.3 | Template includes: agents, hierarchy, pipelines, system prompts |

#### Feature 23: Additional Adapters
| Task | Description |
|------|-------------|
| 23.1 | HTTP webhook adapter (call external API) |
| 23.2 | `cursor_local` adapter |
| 23.3 | `codex_local` adapter |
| 23.4 | Docker container adapter |

#### Feature 24: Marketplace
| Task | Description |
|------|-------------|
| 24.1 | Template registry |
| 24.2 | Publish/discover/install flow |
| 24.3 | Version management |

---

## Execution Order (Recommended)

```
Now          ──→  v0.2 Execution  ──→  v0.3 Governance  ──→  v0.4 Production

 12: Inbox         14: Pipelines       16: Approval Gates     19: Secrets
 13: Conversations 15: Self-Service    17: RBAC               20: Claude Adapter
                                       18: Dashboard          21: Hardening
```

**Immediate next (pick 1-2):**

| Feature | Impact | Effort | Why first |
|---------|--------|--------|-----------|
| **14: Issue Pipelines** | High | 3-4 days | Multi-agent workflows — the core differentiator |
| **15: Agent Self-Service** | Medium | 2-3 days | Smarter agents — they can query their own context |
| **16: Approval Gates** | High | 2-3 days | Governance — agents must ask before critical actions |

**My recommendation:** Start with **Feature 14 (Issue Pipelines)** — multi-agent workflows are the flagship feature that distinguishes Ari. Then **Feature 16 (Approval Gates)** for governance, then **Feature 15 (Agent Self-Service)** for agent intelligence.

---

## Spec-Driven Process

For each feature:

```
1. Write  docx/features/{NN}-{name}/requirements.md   ← EARS requirements
2. Write  docx/features/{NN}-{name}/design.md          ← Technical design
3. Write  docx/features/{NN}-{name}/tasks.md           ← TDD task checklist
4. Write tests first (red)
5. Implement (green)
6. Write  docx/features/{NN}-{name}/task_N_completed.md ← Summary
```
