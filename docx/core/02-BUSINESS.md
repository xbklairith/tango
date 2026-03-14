# Ari — Business Requirements Document (BRD)

**Product Name:** Ari
**Tagline:** Deploy. Govern. Share. The Control Plane for AI Agents.
**Version:** 1.0
**Date:** 2026-03-11
**Status:** Draft

---

## 1. Business Context

### 1.1 Market Opportunity

AI agents are transitioning from experimental tools to production infrastructure. Organizations deploying multiple agents face a gap: **agent frameworks help build agents, but nothing helps run them at scale with governance.**

The market needs a control plane — not another framework.

### 1.2 Market Landscape

| Category | Examples | Gap |
|----------|---------|-----|
| Agent Frameworks | CrewAI, AutoGen, LangGraph | Build agents, don't manage them in production |
| LLM Platforms | OpenAI, Anthropic, Google | Provide models, not organizational infrastructure |
| DevOps Platforms | Kubernetes, Terraform | Manage containers, not AI agent lifecycles |
| Task Management | Jira, Linear, Asana | Built for humans, not agent workforces |
| Existing Control Planes | Paperclip (open source) | Node.js, heavy dependencies, limited HITL |

**Ari fills the gap:** production-grade agent management with governance, built as a single Go binary.

### 1.3 Business Objectives

| # | Objective | Measure | Target |
|---|-----------|---------|--------|
| BO-1 | Enable zero-config agent deployment | Time from download to first agent running | < 5 minutes |
| BO-2 | Provide human governance over AI agents | % of critical actions gated by approval | 100% |
| BO-3 | Full cost visibility and optional budget enforcement | Cost attribution coverage | 100% of spend tracked; zero overruns when budget is set |
| BO-4 | Full accountability for agent actions | Actions with audit trail coverage | 100% |
| BO-5 | Support any AI runtime | Number of adapter types supported | 6+ at launch |
| BO-6 | Minimize operational overhead | External dependencies required | Zero (single binary) |

---

## 2. Stakeholders

| Stakeholder | Role | Interest |
|-------------|------|----------|
| User | Primary user — human oversight | Governance, visibility, cost control, approvals |
| Agent Developer | Configures and deploys agents | Easy setup, adapter flexibility, debugging tools |
| Organization Admin | Manages squads and access | Multi-tenancy, RBAC, budget allocation |
| Finance/Ops | Tracks AI spending | Cost attribution, budget forecasting, reporting |
| Compliance | Ensures accountability | Audit trails, approval records, data isolation |
| Engineering Leadership | Evaluates platform adoption | Reliability, performance, maintenance cost |

---

## 3. Business Requirements

### BR-1: Deploy — Zero-Friction Agent Deployment

#### BR-1.1: One-Command Setup
**Statement:** The system SHALL provide a single executable that starts a fully operational instance with no external dependencies.

**Acceptance Criteria:**
- [ ] Single binary download (< 60MB)
- [ ] `./ari run` initializes database, runs migrations, starts server, opens UI
- [ ] No Docker, no Node.js, no Python, no external database required
- [ ] Embedded PostgreSQL auto-provisioned on first run
- [ ] Data persists across restarts at `~/.ari/`

**Business Value:** Eliminates deployment friction. Engineers evaluate in 5 minutes instead of 5 hours.

#### BR-1.2: Agent Lifecycle Management
**Statement:** The system SHALL manage the complete lifecycle of AI agents from recruitment through removal.

**Acceptance Criteria:**
- [ ] Create agents with name, role, adapter type, budget, and reporting hierarchy
- [ ] Agent statuses: pending_approval → active → idle ↔ running → paused → terminated
- [ ] Automatic status transitions based on heartbeat results
- [ ] Agent configuration versioned with rollback capability
- [ ] Agents maintain persistent session state across execution windows

#### BR-1.3: Adapter-Agnostic Runtime
**Statement:** The system SHALL support any AI runtime through a pluggable adapter interface.

**Acceptance Criteria:**
- [ ] Built-in adapters: Claude Code, Codex, Cursor, shell process, HTTP webhook
- [ ] Custom adapters via Go interface implementation
- [ ] Each adapter reports: exit code, token usage, session state, stdout/stderr
- [ ] Adapter health testing: `ari doctor` validates adapter availability
- [ ] HTTP adapter enables integration with any external system

#### BR-1.4: Heartbeat Execution Protocol
**Statement:** The system SHALL provide a reliable, observable execution protocol for agent invocations.

**Acceptance Criteria:**
- [ ] Four trigger sources: timer (scheduled), assignment, on-demand, automation
- [ ] Wakeup queue with deduplication and priority ordering
- [ ] Maximum one active run per agent (no concurrent execution)
- [ ] Agent receives: JWT auth, squad context, task context, workspace config
- [ ] Run results recorded: status, usage, cost, logs, session state
- [ ] Session persistence: agents resume from previous state

---

### BR-2: Govern — Human-in-the-Loop Oversight

#### BR-2.1: Unified Inbox
**Statement:** The system SHALL provide a single inbox for all items requiring human attention — approvals, questions, decisions, and alerts.

**Acceptance Criteria:**
- [ ] Agents can create inbox items: approvals, questions, decisions, alerts
- [ ] System can create inbox items: budget warnings, agent errors, task blocks
- [ ] Each item has category, urgency, structured payload, and related issue link
- [ ] User resolves from one UI: approve/reject, answer, select option, dismiss
- [ ] Resolved items wake the requesting agent with the user's response
- [ ] Inbox sorted by urgency (critical → normal → low), filterable by category

**Business Value:** One place for everything. No context-switching between approval queues, alert panels, and message threads.

#### BR-2.2: User Controls
**Statement:** The system SHALL provide users with unrestricted override capability.

**Acceptance Criteria:**
- [ ] Pause/resume any agent (immediate effect)
- [ ] Pause/resume any task or task subtree
- [ ] Override any assignment (reassign between agents)
- [ ] Override any budget (increase/decrease)
- [ ] Terminate agents permanently from the squad
- [ ] Full CRUD on all issues, projects, goals
- [ ] Comment on any entity for direction
- [ ] View complete activity history

**Business Value:** Humans always have the final word. No agent can prevent human intervention.

#### BR-2.3: Cost Tracking & Budget Enforcement
**Statement:** The system SHALL track all AI spending and optionally enforce financial limits at agent and squad levels.

**Acceptance Criteria:**
- [ ] Cost tracking per: agent, issue, project, goal, billing code (always active)
- [ ] Cost events are append-only (immutable financial records)
- [ ] Spend computed from CostEvents via aggregation query (not stored as denormalized column)
- [ ] Monthly budget (UTC calendar month) per agent and per squad — NULL = unlimited (default)
- [ ] When budget is set: soft alert at 80% utilization, hard stop at 100% (agent auto-paused)
- [ ] User can override: set/increase/remove budget, manually resume paused agent
- [ ] Dashboard shows: current spend, budget utilization % (when set), spend trend

**Business Value:** Full cost attribution for financial planning. Optional budget caps prevent runaway spend when desired.

#### BR-2.4: Atomic Task Ownership
**Statement:** The system SHALL prevent multiple agents from working on the same task simultaneously.

**Acceptance Criteria:**
- [ ] Task checkout via atomic database transaction (CAS pattern)
- [ ] Only one agent can hold a task lock at a time
- [ ] Conflict returns HTTP 409 — agent must not retry
- [ ] Lock includes: agentId, runId, timestamp
- [ ] Release endpoint clears lock
- [ ] Stale locks detectable and adoptable by authorized runs

**Business Value:** Eliminates duplicate work. Ensures clear accountability for each task.

#### BR-2.5: Immutable Audit Trail
**Statement:** The system SHALL maintain an immutable, complete record of all actions.

**Acceptance Criteria:**
- [ ] Every create, update, delete, and status change logged
- [ ] Log entry includes: who (actor type + ID), what (action + entity), when, context (run ID, details)
- [ ] Activity log is append-only: no UPDATE or DELETE operations
- [ ] Queryable by: squad, date range, actor, action type, entity
- [ ] Retention: indefinite (no automatic purging)

**Business Value:** Full compliance trail. Debugging capability. Accountability for every decision.

#### BR-2.6: Permission-Based Access Control
**Statement:** The system SHALL enforce role-based permissions for both humans and agents.

**Acceptance Criteria:**
- [ ] SquadMembership entity links users to squads with roles (owner/admin/viewer)
- [ ] Permission grants: agents:recruit, tasks:assign, users:invite, etc.
- [ ] Captain has implicit full permissions within squad
- [ ] Agent creators have implicit permissions over their direct reports
- [ ] Users can grant/revoke any permission
- [ ] Squad-scoped data isolation: no cross-squad access

---

### BR-3: Share — Templates & Collaboration

#### BR-3.1: Portable Squad Templates
**Statement:** The system SHALL support export and import of squad configurations.

**Acceptance Criteria:**
- [ ] Export: agents, hierarchy, projects, goals, workflows → JSON
- [ ] Automatic scrubbing of secrets, API keys, and PII
- [ ] Import: creates squad with full structure from template
- [ ] Import prompts for missing secrets and credentials
- [ ] Templates are versioned and backward-compatible

**Business Value:** Enables reuse. Teams can share proven agent configurations.

#### BR-3.2: Multi-Squad Isolation
**Statement:** The system SHALL support multiple squads running simultaneously on a single instance with strict isolation. Users SHALL be able to create, join, and switch between squads freely.

**Acceptance Criteria:**
- [ ] All entities scoped to exactly one squad via non-nullable squad_id FK
- [ ] Agents can only see/access their own squad's data
- [ ] Separate budgets, audit trails, and permissions per squad
- [ ] Squad-scoped issue identifiers (e.g., ARI-1 vs ACME-1)
- [ ] Users can be members of multiple squads with independent roles per squad
- [ ] Backend is stateless w.r.t. squad context — squad ID is always in the URL path, no server-side "current squad"
- [ ] UI provides a squad selector to switch between squads instantly (client-side state change, no server round-trip)
- [ ] Multiple browser tabs can operate on different squads concurrently
- [ ] Creating, pausing, or archiving one squad has zero effect on other squads

**Implementation Notes:**
- Backend: Squad ID in URL path (`/api/squads/{id}/...`), membership verified per-request via middleware
- Frontend: Active squad stored in React Context + localStorage for persistence across sessions
- Default squad on login: last-visited (from localStorage), or first in list if no history
- No cross-squad aggregation in v1 — each API call is squad-scoped

#### BR-3.3: Agent Marketplace (Future — v2)
**Statement:** The system SHALL provide a marketplace for discovering and deploying pre-built agent configurations.

**Acceptance Criteria (Future):**
- [ ] Browse agent templates by category
- [ ] One-click deploy of template to local instance
- [ ] Rating and usage statistics
- [ ] Template versioning and updates
- [ ] Community contributions with review process

---

### BR-4: Operational Excellence

#### BR-4.1: Real-Time Visibility
**Statement:** The system SHALL provide real-time visibility into agent operations.

**Acceptance Criteria:**
- [ ] SSE-based event stream for: agent status, run lifecycle, issue updates, cost alerts, conversation replies
- [ ] Dashboard with: active agents, inbox count, spend trend, issue distribution
- [ ] Agent detail view: run history, cost breakdown, session state
- [ ] Issue detail view: full comment thread, attachment support, status history
- [ ] Conversation view: real-time chat with agents (message spawns agent session, agent replies as comment)
- [ ] Unified inbox: approvals, agent questions, decisions, alerts — all in one queue

#### BR-4.2: Database Management
**Statement:** The system SHALL provide reliable data storage with backup and recovery.

**Acceptance Criteria:**
- [ ] Embedded PostgreSQL for zero-config setup
- [ ] External PostgreSQL support for production
- [ ] Automatic scheduled backups (configurable interval, default 60 min)
- [ ] Backup retention policy (configurable, default 30 days)
- [ ] Database restore from backup
- [ ] Migration management with forward-only, backward-compatible schema changes

#### BR-4.3: Secret Management
**Statement:** The system SHALL securely store and manage credentials.

**Acceptance Criteria:**
- [ ] Secrets encrypted at rest (AES-256)
- [ ] Master key file stored outside database
- [ ] Secret versioning with rotation support
- [ ] Secrets never logged or returned in API responses
- [ ] Agent configs can reference secrets via `secret_ref` bindings
- [ ] Support for external secret managers (AWS, GCP, Vault) in future

#### BR-4.4: Health & Diagnostics
**Statement:** The system SHALL provide diagnostic tools for troubleshooting.

**Acceptance Criteria:**
- [ ] `ari doctor` validates: config, database, adapters, auth, storage
- [ ] Auto-repair mode for common issues
- [ ] Health endpoint: `GET /api/health`
- [ ] Structured logging (JSON) to file
- [ ] Run log storage with compression and SHA-256 integrity

---

## 4. Non-Functional Requirements

### NFR-1: Performance

| Metric | Requirement |
|--------|------------|
| API response time (p95) | < 50ms for CRUD operations |
| API response time (p99) | < 200ms |
| Heartbeat scheduling overhead | < 100ms per invocation |
| Concurrent agents supported | 100+ per instance |
| Database query time | < 10ms for indexed queries |
| SSE event delivery latency | < 500ms from event to client |

### NFR-2: Resource Efficiency

| Metric | Requirement |
|--------|------------|
| Binary size | < 60MB (including embedded PG extractor) |
| Memory (idle, no agents) | < 50MB |
| Memory (10 active agents) | < 100MB |
| Memory (100 active agents) | < 500MB |
| CPU (idle) | < 1% |
| Disk (base install) | < 200MB (including PG data dir) |

### NFR-3: Reliability

| Metric | Requirement |
|--------|------------|
| Data durability | PostgreSQL WAL + scheduled backups |
| Graceful shutdown | Clean PG stop on SIGTERM/SIGINT |
| Stale lock detection | Detect and recover from abandoned checkouts |
| Process reuse | Detect existing PG instance, reuse instead of duplicate |
| Migration safety | Forward-only, backward-compatible, auto-applied |

### NFR-4: Security

| Requirement | Implementation |
|------------|---------------|
| Auth in production | JWT + session-based, no anonymous access |
| Secret storage | AES-256 encrypted at rest |
| API key storage | SHA-256 hashed, never stored in plaintext |
| Data isolation | Squad-scoped, no cross-tenant access |
| Audit compliance | Immutable activity log, no deletion |
| Transport | HTTPS recommended for authenticated mode |

### NFR-5: Compatibility

| Requirement | Specification |
|------------|--------------|
| OS support | macOS (arm64, x64), Linux (arm64, x64) |
| Go version | 1.22+ |
| PostgreSQL | 15+ (embedded) or 14+ (external) |
| Browser support | Chrome, Firefox, Safari (latest 2 versions) |
| Node.js (UI build only) | 20+ |

---

## 5. Constraints & Assumptions

### 5.1 Constraints

| # | Constraint | Rationale |
|---|-----------|-----------|
| C-1 | Single binary distribution | Core value prop — zero dependencies |
| C-2 | PostgreSQL only (no MySQL, SQLite) | Full PG compatibility for production migration |
| C-3 | Self-hosted first | Control and privacy are primary concerns |
| C-4 | Go backend | Performance, single binary, native concurrency |
| C-5 | React frontend | Ecosystem maturity, component availability |
| C-6 | REST API (not GraphQL) | Simplicity, cacheability, agent-friendly |

### 5.2 Assumptions

| # | Assumption | Risk if Wrong |
|---|-----------|---------------|
| A-1 | Users have macOS or Linux | Windows support deferred; may need WSL docs |
| A-2 | Agents can make HTTP calls | Core interaction model; no alternative path |
| A-3 | Monthly budget cycle is sufficient | Some users may need weekly or per-project budgets |
| A-4 | Single user per deployment (v1) | Multi-user support needed for teams |
| A-5 | Embedded PG is sufficient for small-medium scale | Large deployments will use external PG |
| A-6 | SSE is sufficient for real-time needs | WebSocket upgrade path available if needed |

---

## 6. Risk Assessment

| # | Risk | Probability | Impact | Mitigation |
|---|------|------------|--------|-----------|
| R-1 | Embedded PG binary compatibility issues on edge Linux distros | Medium | High | Test on Ubuntu, Debian, Alpine, Amazon Linux; document requirements |
| R-2 | Agent adapters break with runtime updates (Claude CLI, Codex CLI) | High | Medium | Version pinning, adapter abstraction, fast patch releases |
| R-3 | Users outgrow single-instance architecture | Medium | Medium | External PG migration path; horizontal scaling in roadmap |
| R-4 | Cost tracking accuracy varies by adapter | Medium | Medium | Standardized usage reporting contract; manual correction API |
| R-5 | Security vulnerabilities in self-hosted auth | Low | High | Minimal auth surface; security audit before v1; HTTPS enforcement |
| R-6 | Competitive response from agent frameworks | High | Low | Control plane is a different category; complement, not compete |

---

## 7. Success Criteria

### 7.1 Launch Criteria (v1.0)

- [ ] Single binary runs on macOS arm64/x64 and Linux x64
- [ ] `./ari run` → working instance in < 5 minutes
- [ ] Create squad → recruit agent → assign task → agent executes → results visible
- [ ] Approval workflow: recruit request → user approves → agent activated
- [ ] Cost tracking: all agent spend attributed and visible; budget enforcement when limits are set
- [ ] Audit trail: every action logged and queryable
- [ ] Dashboard: real-time agent status, cost tracking, pending approvals
- [ ] At least 3 adapters working: claude_local, process, http

### 7.2 Post-Launch Metrics (90 days)

| Metric | Target |
|--------|--------|
| GitHub stars | 1,000+ |
| Active instances (telemetry opt-in) | 500+ |
| Community adapters contributed | 3+ |
| Average setup time (reported) | < 10 minutes |
| Critical bugs reported | < 5 |
| Documentation completeness | 90%+ of features documented |

---

## 8. Differentiation Summary

### Why Ari Over Alternatives

```
┌─────────────────────────────────────────────────────┐
│                                                     │
│  "I need to BUILD an agent"                         │
│   → Use CrewAI, AutoGen, LangGraph                  │
│                                                     │
│  "I need to RUN agents in production"               │
│   → Use Ari                                          │
│                                                     │
│  "I need GOVERNANCE over my AI workforce"           │
│   → Use Ari                                          │
│                                                     │
│  "I need to SHARE agent configurations"             │
│   → Use Ari                                          │
│                                                     │
└─────────────────────────────────────────────────────┘
```

### The Three Pillars

| Pillar | What It Means | What Users Get |
|--------|--------------|----------------|
| **Deploy** | One binary, zero deps, 5-minute setup | From download to running agents in minutes |
| **Govern** | HITL approvals, budgets, audit trails | Sleep at night knowing agents are controlled |
| **Share** | Templates, marketplace, portability | Reuse proven agent setups across teams |

---

## Appendix A: Reference Architecture (Paperclip)

Ari draws architectural inspiration from Paperclip (MIT-licensed), an open-source control plane for AI agents. Key concepts adapted:

| Concept | Origin (Paperclip) | Evolution (Ari) |
|---------|-------------------|-------------------|
| Company model | Multi-company isolation | Squad-based isolation |
| Agent hierarchy | Tree with roles | Captain → Lead → Member |
| Heartbeat protocol | Timer/assignment/mention triggers | Same + automation triggers |
| Atomic checkout | CAS-pattern task locking | Same |
| Cost tracking | Per-agent/issue/project | Same + billing codes |
| Approval gates | hire_agent, ceo_strategy | recruit_agent, captain_request |
| Session persistence | Per-agent and per-task | Same |
| Adapter system | Node.js adapters | Go interfaces |
| Embedded database | embedded-postgres (npm) | embedded-postgres-go |
| Activity log | Append-only audit | Same |

**Key differentiators from Paperclip:**
1. Single Go binary (vs. Node.js + pnpm + 500 deps)
2. Stronger HITL emphasis (review status, escalation protocol)
3. SSE instead of WebSocket (simpler, proxy-friendly)
4. sqlc instead of ORM (zero runtime overhead)
5. Squad templates & marketplace vision

---

*Deploy. Govern. Share.*
*Ari — The Control Plane for AI Agents.*
*Agents are moving into production. Production requires control.*
