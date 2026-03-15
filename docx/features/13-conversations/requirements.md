# Requirements: Conversations (Agent Chat Interface)

**Created:** 2026-03-15
**Status:** Draft

## Overview

Implement real-time conversations between users and agents. A conversation is an issue with `type=conversation` — reusing the existing `issues` and `issue_comments` tables. Users send messages (IssueComments with `authorType=user`), the system auto-wakes the assigned agent, and the agent replies by posting a comment (IssueComments with `authorType=agent`). Session state persists across turns via `agent_conversation_sessions`.

All infrastructure required for conversations already exists from the Agent Runtime feature (Feature 11): wakeup queue, adapter dispatch, session persistence, SSE hub, and Run Token auth. This feature wires those primitives into a cohesive conversation flow and adds the HTTP endpoints, prompt assembly, and React chat UI.

### Requirement ID Format

- Use sequential IDs: `REQ-CONV-001`, `REQ-CONV-002`, etc.
- Numbering is continuous across all categories.

---

## Functional Requirements

### Event-Driven Requirements (WHEN...THEN)

**Conversation Lifecycle**

- [REQ-CONV-001] WHEN a user calls `POST /api/agents/{id}/conversations` with a `title` and optional `message` THEN the system SHALL create an issue with `type=conversation`, `status=in_progress`, `assigneeAgentId` set to the target agent, and (if `message` is provided) an IssueComment with `authorType=user` and `authorId` set to the calling user's ID.

- [REQ-CONV-002] WHEN a conversation is created with an initial message (REQ-CONV-001) THEN the system SHALL immediately enqueue a `WakeupRequest` with `invocationSource=conversation_message`, `ARI_CONVERSATION_ID=<issueId>`, and `ARI_WAKE_REASON=conversation_message` for the assigned agent.

- [REQ-CONV-003] WHEN a user calls `PATCH /api/conversations/{id}/close` THEN the system SHALL transition the conversation issue to `status=done` and emit an `issue.updated` SSE event. No further messages SHALL trigger agent wakeups on a closed conversation.

**User Sends Message**

- [REQ-CONV-004] WHEN a user calls `POST /api/conversations/{id}/messages` with a `body` THEN the system SHALL create an IssueComment with `authorType=user`, `authorId` set to the calling user's ID, and emit a `conversation.message` SSE event containing the comment body, comment ID, and conversation ID.

- [REQ-CONV-005] WHEN a user message is created on a conversation (REQ-CONV-004) THEN the system SHALL enqueue a `WakeupRequest` with `invocationSource=conversation_message` for the conversation's `assigneeAgentId`, passing `ARI_CONVERSATION_ID=<issueId>` in the wakeup context.

- [REQ-CONV-006] WHEN a user message triggers a wakeup (REQ-CONV-005) and the conversation already has an active `HeartbeatRun` in `running` state THEN the system SHALL queue the new `WakeupRequest` (normal dedup applies) and process it after the current run completes, maintaining message ordering.

**Agent Receives Context**

- [REQ-CONV-007] WHEN the system dispatches a conversation-triggered wakeup THEN the system SHALL load the full comment thread for the conversation issue (up to a configurable `maxMessages` limit, default 100), load the agent's conversation session state from `agent_conversation_sessions`, and populate `InvokeInput.Conversation` with `IssueID`, `Messages`, and `SessionState`.

- [REQ-CONV-008] WHEN an agent calls `GET /api/agent/me` and the wakeup reason is `conversation_message` THEN the response SHALL include a `conversation` field containing the conversation issue metadata and the full message thread (up to `maxMessages`).

**Agent Replies**

- [REQ-CONV-009] WHEN an agent calls `POST /api/agent/me/reply` with `{ "conversationId": "<issueId>", "body": "<message>" }` THEN the system SHALL create an IssueComment with `authorType=agent`, `authorId` set to the agent's ID, and emit a `conversation.agent.replied` SSE event containing the comment body, comment ID, conversation ID, and agent ID.

- [REQ-CONV-010] WHEN an agent posts a reply (REQ-CONV-009) THEN the system SHALL verify that the agent is the `assigneeAgentId` of the conversation issue and that the conversation `status` is not `done` or `cancelled`. If validation fails, the system SHALL return HTTP 403 with `code=FORBIDDEN`.

**Typing Indicators**

- [REQ-CONV-011] WHEN the system dispatches a conversation-triggered wakeup and the adapter begins execution THEN the system SHALL emit a `conversation.agent.typing` SSE event with `conversationId` and `agentId` so the UI can display a typing indicator.

- [REQ-CONV-012] WHEN the agent's `HeartbeatRun` finishes (regardless of status) for a conversation-triggered wakeup THEN the system SHALL emit a `conversation.agent.typing.stopped` SSE event so the UI can remove the typing indicator.

**Session Continuity**

- [REQ-CONV-013] WHEN an adapter returns a non-empty `sessionIdAfter` for a conversation-triggered wakeup THEN the system SHALL persist the session state in `agent_conversation_sessions` keyed by `(agentId, issueId)`, overwriting any previous session state for that conversation.

- [REQ-CONV-014] WHEN a subsequent conversation-triggered wakeup is dispatched for the same agent and conversation THEN the system SHALL populate `InvokeInput.Run.SessionState` and `InvokeInput.Conversation.SessionState` from the most recent `agent_conversation_sessions` record so the agent can resume context.

**Conversation Listing**

- [REQ-CONV-015] WHEN a user calls `GET /api/agents/{id}/conversations` THEN the system SHALL return all issues with `type=conversation` and `assigneeAgentId` equal to the specified agent, sorted by `updated_at DESC`, with pagination support.

- [REQ-CONV-016] WHEN a user calls `GET /api/squads/{squadId}/issues?type=conversation` THEN the system SHALL return only conversation issues for that squad, leveraging the existing `ListIssuesBySquad` query with the `type` filter.

- [REQ-CONV-017] WHEN an agent calls `GET /api/agent/me/conversations` THEN the system SHALL return all issues with `type=conversation` and `assigneeAgentId` matching the agent's Run Token identity, sorted by `updated_at DESC`.

**Message History**

- [REQ-CONV-018] WHEN a user calls `GET /api/conversations/{id}/messages` THEN the system SHALL return all IssueComments for the conversation issue, ordered by `created_at ASC`, with pagination support. Each comment SHALL include `authorType`, `authorId`, `body`, and `createdAt`.

---

### State-Driven Requirements (WHILE...the system SHALL)

- [REQ-CONV-019] WHILE an agent has a `HeartbeatRun` in `running` state for a conversation-triggered wakeup, the system SHALL maintain the `conversation.agent.typing` indicator for that conversation on the SSE stream.

- [REQ-CONV-020] WHILE a conversation issue has `status=done` or `status=cancelled`, the system SHALL reject any new messages via `POST /api/conversations/{id}/messages` with HTTP 422 and `code=CONVERSATION_CLOSED`.

---

### Ubiquitous Requirements (The system SHALL always)

- [REQ-CONV-021] The system SHALL store all conversation messages as `IssueComment` records — no separate messages table. The `authorType` field (`user`, `agent`, `system`) distinguishes the sender.

- [REQ-CONV-022] The system SHALL enforce squad-level data isolation for all conversation endpoints: users MUST be members of the conversation's squad, and agents MUST belong to the conversation's squad (validated via Run Token `squad_id` claim).

- [REQ-CONV-023] The system SHALL include the conversation's `assigneeAgentId` in every conversation-related SSE event so the UI can filter events by agent.

- [REQ-CONV-024] The system SHALL log an activity entry (`action=conversation.created`, `action=conversation.message_sent`, `action=conversation.agent_replied`, `action=conversation.closed`) for every conversation lifecycle event.

---

### Conditional Requirements (IF...THEN)

- [REQ-CONV-025] IF the conversation's assigned agent has `status=paused` or `status=terminated` THEN the system SHALL still accept the user's message (creating the IssueComment) but SHALL NOT enqueue a wakeup. The UI SHOULD display a warning that the agent is unavailable.

- [REQ-CONV-026] IF the agent's monthly spend has reached 100% of its `budgetMonthlyCents` THEN the system SHALL reject the wakeup for the conversation message (existing budget enforcement applies) and create an inbox alert of `type=budget_warning`.

- [REQ-CONV-027] IF a conversation has no assigned agent (`assigneeAgentId IS NULL`) THEN the system SHALL reject `POST /api/conversations/{id}/messages` with HTTP 422 and `code=NO_AGENT_ASSIGNED`.

- [REQ-CONV-028] IF the user sends a message while the agent is already processing a previous message (REQ-CONV-006) THEN the UI SHALL display the message immediately in the thread (optimistic rendering) and show a "processing" indicator until the agent replies.

---

## Non-Functional Requirements

### Performance

- [REQ-CONV-029] The system SHALL deliver a `conversation.agent.replied` SSE event to the UI within 500ms of the IssueComment being persisted to the database.

- [REQ-CONV-030] The system SHALL load and return conversation message history (up to 100 messages) within 200ms for 99% of requests.

### Security

- [REQ-CONV-031] The `POST /api/agent/me/reply` endpoint SHALL only accept requests authenticated with a valid Run Token JWT. The `agentId` from the token MUST match the conversation's `assigneeAgentId`.

- [REQ-CONV-032] The system SHALL validate that users calling conversation endpoints are members of the conversation's squad, using the same `verifySquadMembership` pattern as existing issue handlers.

### Reliability

- [REQ-CONV-033] The system SHALL handle agent crashes during conversation processing by emitting a `conversation.agent.typing.stopped` SSE event and transitioning the agent to `error` status (existing RunService.finalize handles this).

---

## Constraints

- Conversations are issues with `type=conversation` — no separate conversations table.
- Messages are `IssueComment` records — no separate messages table.
- One active agent session per conversation at a time (new messages queue additional wakeups).
- No streaming of partial agent output (agent posts complete reply as a comment).
- Session state is per-conversation, not per-message.
- The `/conversations` routes are convenience aliases — they operate on the same underlying `issues` and `issue_comments` tables.

---

## Acceptance Criteria

- [ ] `POST /api/agents/{id}/conversations` creates a conversation issue and optionally sends the first message + wakes the agent.
- [ ] `POST /api/conversations/{id}/messages` creates a user comment, emits SSE, and auto-wakes the assigned agent.
- [ ] `GET /api/conversations/{id}/messages` returns the full message thread with author attribution.
- [ ] `POST /api/agent/me/reply` creates an agent comment and emits `conversation.agent.replied` SSE.
- [ ] `GET /api/agent/me` includes conversation context when `wake_reason=conversation_message`.
- [ ] `GET /api/agent/me/conversations` returns conversations assigned to the agent.
- [ ] `PATCH /api/conversations/{id}/close` transitions to `done` and prevents further wakeups.
- [ ] Typing indicator SSE events (`conversation.agent.typing`, `conversation.agent.typing.stopped`) fire on run start/finish.
- [ ] Session state persists across conversation turns via `agent_conversation_sessions`.
- [ ] Queued wakeups process in order when messages arrive while agent is running.
- [ ] React chat UI displays messages in real time via SSE.
- [ ] Closed conversations reject new messages with HTTP 422.

---

## Out of Scope

- Streaming partial agent output mid-turn (agent posts complete reply as a comment).
- Multi-agent conversations (one agent per conversation for v1).
- File/image attachments in conversation messages (text only for v1).
- Read receipts or message delivery confirmation.
- Message editing or deletion.
- Conversation transfer (reassigning to a different agent mid-conversation).
- Threaded replies within a conversation (flat message list only).

---

## Dependencies

- Agent Runtime (Feature 11): WakeupService, RunService, SSE Hub, adapter dispatch, session persistence.
- Issue handler: `internal/server/handlers/issue_handler.go` (existing CRUD and comment endpoints).
- Agent self-service handler: `internal/server/handlers/agent_self_handler.go` (to be extended with `/reply` and `/conversations`).
- Issue domain model: `internal/domain/issue.go` (IssueType, CommentAuthorType constants).
- Session queries: `internal/database/queries/agent_sessions.sql` (UpsertConversationSession, GetConversationSession).
- Comment queries: `internal/database/queries/issue_comments.sql` (CreateIssueComment, ListIssueComments).

---

## Risks & Assumptions

**Assumptions:**
- The existing `issue_comments` table and `IssueComment` model are sufficient for conversation messages (no additional metadata needed for v1).
- SSE is sufficient for real-time message delivery; WebSocket is not needed.
- Agents post complete replies as single comments (no streaming or multi-part responses).
- The wakeup queue's deduplication and priority system correctly handles rapid sequential messages.

**Risks:**
- Long agent response times (minutes) may frustrate users waiting for replies; the typing indicator mitigates perceived latency but does not reduce actual latency.
- High message volume on a single conversation could create a large comment thread that slows down context loading; the `maxMessages` limit (REQ-CONV-007) mitigates this.
- Conversation session state blobs may grow large if agents store extensive context; no size limit is enforced for v1.

---

## References

- PRD: `docx/core/01-PRODUCT.md` (sections: "Conversations (Real-Time Agent Chat)", "Agent Self-Service", SSE event types)
- Agent Runtime requirements: `docx/features/11-agent-runtime/requirements.md` (REQ-005, REQ-022, REQ-023, REQ-026, REQ-027, REQ-048)
- Issue domain model: `internal/domain/issue.go`
- Issue handler: `internal/server/handlers/issue_handler.go`
- Agent self-service handler: `internal/server/handlers/agent_self_handler.go`
- Wakeup handler: `internal/server/handlers/wakeup_handler.go`
- Run handler: `internal/server/handlers/run_handler.go`
