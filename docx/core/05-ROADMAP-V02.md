# Feature Roadmap: Ari v0.2 — Features 12-17

## Current State

Features 01-11 cover Phase 1 foundation + early Phase 2/3:
- **01-07**: Core scaffold, DB, squads, agents, issues, projects, goals, auth, React SPA
- **08**: UI interactivity (create/edit dialogs, filters, pagination, squad switching)
- **09**: Activity log (append-only audit trail, feed endpoint, dashboard widget)
- **10**: Cost events and budget enforcement (in progress)
- **11**: Agent runtime — adapter, wakeup queue, heartbeat runs, SSE, task checkout (in progress)

---

## Proposed Features

### Feature 12: Inbox System (Unified Human-in-the-Loop Queue)

The single queue through which agents request human attention. Covers `inbox_items` table, CRUD endpoints, resolution workflow (approve/reject/answer/dismiss), and agent-wakeup-on-resolution trigger.

- **Key capabilities**: Inbox CRUD with category/status/urgency filters, resolution state machine, inbox count badge, budget warnings as inbox items, SSE events
- **Dependencies**: 09, 10, 11
- **Complexity**: L
- **Rationale**: Primary governance interface. Agents need a way to ask humans questions and request approvals. Feature 10 already needs inbox items for budget warnings.

### Feature 13: Conversations (Agent Chat Interface)

Real-time chat with agents. A conversation is an issue with `type=conversation`. User messages spawn agent sessions via the adapter; agents reply through the same thread.

- **Key capabilities**: Conversation creation, message posting triggers agent invocation, session continuity per conversation, SSE typing/reply events, chat UI
- **Dependencies**: 11, 12
- **Complexity**: XL
- **Rationale**: Primary user interaction model. Transforms Ari from project management into an interactive control plane.

### Feature 14: Issue Pipelines (Multi-Agent Workflows)

Squad-level workflow templates with sequential stages. Each stage auto-assigns to a designated agent. Agents advance stages; reviewers can reject back.

- **Key capabilities**: Pipeline CRUD, stage advancement/rejection endpoints, pipeline progress UI, wakeup triggers on stage transitions
- **Dependencies**: 11, 13
- **Complexity**: L
- **Rationale**: Enables multi-agent collaboration — code-review-QA flows, content-edit-publish pipelines.

### Feature 15: Agent Self-Service API (`/agent/me/*`)

Full set of endpoints agents call during execution. Run Token JWT scopes responses to the calling agent's identity and role.

- **Key capabilities**: `/me` profile, `/me/assignments`, `/me/squad`, `/me/team`, `/me/goals`, `/me/budget`, `/me/conversations`, `/me/session`, role-based response filtering
- **Dependencies**: 10, 11, 13
- **Complexity**: M
- **Rationale**: Without these endpoints, spawned agents have no way to discover assignments or context.

### Feature 16: Secrets Management

Encrypted secret storage for squads. Secrets injected into agent environments during adapter invocation.

- **Key capabilities**: AES-256-GCM encrypted storage, auto-generated master key, create/list/rotate/delete endpoints, adapter injection, UI for management
- **Dependencies**: 09, 11
- **Complexity**: M
- **Rationale**: Agents need external credentials (API keys, tokens) to do real work. Pulled forward from Phase 4.

### Feature 17: Dashboard Metrics and Observability

Comprehensive single-pane-of-glass dashboard with real-time metrics, cost trends, agent health signals, inbox counts, and status charts.

- **Key capabilities**: Aggregated squad metrics endpoint, cost trend chart, agent health table, inbox badge via SSE, issue velocity widget
- **Dependencies**: 09, 10, 11, 12
- **Complexity**: M
- **Rationale**: "You cannot govern what you cannot see." Read-only aggregation layer — best built last when all data sources exist.

---

## Summary

| # | Feature | Complexity | Dependencies | PRD Phase |
|---|---------|-----------|--------------|-----------|
| 12 | Inbox System | L | 09, 10, 11 | Phase 2-3 |
| 13 | Conversations | XL | 11, 12 | Phase 2 |
| 14 | Issue Pipelines | L | 11, 13 | Phase 2 |
| 15 | Agent Self-Service API | M | 10, 11, 13 | Phase 2 |
| 16 | Secrets Management | M | 09, 11 | Phase 4 (pulled forward) |
| 17 | Dashboard Observability | M | 09, 10, 11, 12 | Phase 2-3 |

## Deferred Beyond v0.2

- Permission grants (RBAC) — Phase 3
- Long-lived agent API keys — Phase 4
- Squad export/import (templates) — Phase 5
- Additional adapters (claude_local, codex_local, cursor, http) — Phase 5
- Database backup/restore — Phase 4
- Docker image — Phase 4
- CLI onboarding wizard (`ari init`) — Phase 4
