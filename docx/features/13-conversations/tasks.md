# Tasks: Conversations (Agent Chat Interface)

**Created:** 2026-03-15
**Status:** In Progress

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-CONV-001 through REQ-CONV-033

## Implementation Approach

Work from backend to frontend: SQL queries first, then the conversation handler, then agent self-service extensions, then RunService conversation context, and finally the React chat UI. Each task includes its own tests following TDD red-green-refactor.

## Progress Summary

- Total Tasks: 9
- Completed: 8
- In Progress: None
- Remaining: Task 09 (React Chat UI)
- Test Coverage: TBD

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 01 — SQL Queries for Conversations

**Requirements:** REQ-CONV-015, REQ-CONV-016, REQ-CONV-017, REQ-CONV-018
**Estimated time:** 30 min

#### Context

Add sqlc queries for listing conversations by agent and supporting the conversation endpoints. The existing `ListIssuesBySquad` query already supports `type` filtering (REQ-CONV-016), so no changes needed there. New queries: `ListConversationsByAgent`, `CountConversationsByAgent`, `GetLatestComment`.

> **Prerequisite (M7):** After adding new `.sql` query files, run `make sqlc` to regenerate Go code BEFORE writing or running any Go tests. The generated code in `internal/database/db/` must exist for test compilation.

#### RED — Write Failing Tests

Create `internal/database/queries_conversation_test.go` (or extend existing query tests):

1. Insert a conversation issue (`type=conversation`) and a task issue (`type=task`) for the same agent. Call `ListConversationsByAgent` — verify only the conversation is returned.
2. Insert multiple conversations for the same agent. Call `CountConversationsByAgent` — verify count matches.
3. Insert three comments on a conversation. Call `GetLatestComment` — verify the most recent comment is returned.
4. Insert conversations with `status=done` and `status=in_progress`. Call `ListConversationsByAgent` with `filterStatus=in_progress` — verify only active conversations are returned.

Tests fail because the queries do not exist yet.

#### GREEN — Implement Minimum to Pass

1. Add to `internal/database/queries/issues.sql`:
   ```sql
   -- name: ListConversationsByAgent :many
   SELECT * FROM issues
   WHERE type = 'conversation'
     AND assignee_agent_id = @agent_id
     AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
   ORDER BY updated_at DESC
   LIMIT @page_limit
   OFFSET @page_offset;

   -- name: CountConversationsByAgent :one
   SELECT count(*) FROM issues
   WHERE type = 'conversation'
     AND assignee_agent_id = @agent_id
     AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'));
   ```

2. Add to `internal/database/queries/issue_comments.sql`:
   ```sql
   -- name: GetLatestComment :one
   SELECT * FROM issue_comments
   WHERE issue_id = @issue_id
   ORDER BY created_at DESC
   LIMIT 1;
   ```

3. **Run `make sqlc`** to regenerate Go code. This MUST happen before writing or compiling any Go tests that reference the new query functions (`ListConversationsByAgent`, `CountConversationsByAgent`, `GetLatestComment`).
4. Run `make test` — all query tests pass.

#### REFACTOR

- Verify the generated Go types align with existing `db.Issue` and `db.IssueComment` types.
- Confirm `@agent_id` parameter type is `uuid.NullUUID` for nullable FK compatibility.

#### Acceptance Criteria

- [x] `ListConversationsByAgent` query returns only conversation-type issues for the specified agent
- [x] `CountConversationsByAgent` query returns correct count with optional status filter
- [x] `GetLatestComment` query returns the most recent comment for an issue
- [x] `make sqlc` succeeds
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/database/queries/issues.sql`
- **Modify:** `internal/database/queries/issue_comments.sql`

---

### [x] Task 02 — ConversationHandler: Start Conversation

**Requirements:** REQ-CONV-001, REQ-CONV-002, REQ-CONV-022, REQ-CONV-024
**Estimated time:** 60 min

#### Context

Create `ConversationHandler` with the `StartConversation` endpoint (`POST /api/agents/{id}/conversations`). This creates a conversation issue, optionally creates the first message, and enqueues a wakeup if a message is provided.

#### RED — Write Failing Tests

Create `internal/server/handlers/conversation_handler_test.go`:

1. `TestStartConversation_WithMessage` — POST with `title` and `message`, verify: issue created with `type=conversation`, `status=in_progress`, `assigneeAgentId` set; IssueComment created with `authorType=user`; response includes both `conversation` and `message`; wakeup enqueued with `invocationSource=conversation_message`.
2. `TestStartConversation_WithoutMessage` — POST with only `title`, verify: issue created; no comment; no wakeup enqueued.
3. `TestStartConversation_InvalidAgent` — POST with non-existent agent ID, verify 404.
4. `TestStartConversation_WrongSquad` — agent belongs to a different squad than the user, verify 403.
5. `TestStartConversation_MissingTitle` — POST without `title`, verify 400 validation error.

Tests fail because `ConversationHandler` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/server/handlers/conversation_handler.go`:
   - `ConversationHandler` struct with `queries`, `dbConn`, `wakeupSvc`, `sseHub`.
   - `NewConversationHandler` constructor.
   - `RegisterRoutes` registering `POST /api/agents/{id}/conversations`.
   - `StartConversation` handler implementing the logic from the design.
2. Run all tests — pass.

#### REFACTOR

- Extract `verifySquadMembership` usage from the existing `IssueHandler` pattern.
- Ensure activity logging follows the same `logActivity` pattern.
- Add doc comments to all exported functions.

#### Acceptance Criteria

- [x] `POST /api/agents/{id}/conversations` creates a conversation issue
- [x] Optional `message` field creates first IssueComment and enqueues wakeup
- [x] Squad membership is verified
- [x] Activity logged as `conversation.created`
- [x] Response includes conversation and optional message
- [x] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/server/handlers/conversation_handler.go`
- **Create:** `internal/server/handlers/conversation_handler_test.go`

---

### [x] Task 03 — ConversationHandler: Send Message + Auto-Wake

**Requirements:** REQ-CONV-004, REQ-CONV-005, REQ-CONV-006, REQ-CONV-020, REQ-CONV-025, REQ-CONV-027
**Estimated time:** 60 min

#### Context

Implement `POST /api/conversations/{id}/messages` — the core conversation flow. User sends a message, comment is created, SSE event emitted, and agent is auto-woken.

#### RED — Write Failing Tests

Add to `conversation_handler_test.go`:

1. `TestSendMessage_Success` — POST message on active conversation, verify: comment created with `authorType=user`; `conversation.message` SSE event emitted; wakeup enqueued with `conversation_message` source.
2. `TestSendMessage_ClosedConversation` — conversation has `status=done`, verify 422 with `CONVERSATION_CLOSED`.
3. `TestSendMessage_CancelledConversation` — conversation has `status=cancelled`, verify 422 with `CONVERSATION_CLOSED`.
4. `TestSendMessage_NoAgent` — conversation has `assigneeAgentId=NULL`, verify 422 with `NO_AGENT_ASSIGNED`.
5. `TestSendMessage_NotConversation` — issue is `type=task`, verify 422 with `NOT_A_CONVERSATION`.
6. `TestSendMessage_EmptyBody` — empty body, verify 400 validation error.
7. `TestSendMessage_AgentPaused` — agent has `status=paused`, verify comment is created but wakeup is not enqueued (WakeupService returns nil).

Tests fail because `SendMessage` is not implemented.

#### GREEN — Implement Minimum to Pass

1. Add `SendMessage` handler to `ConversationHandler`.
2. Register route: `POST /api/conversations/{id}/messages`.
3. Implement validation, comment creation, SSE emission, and wakeup enqueue per design.
4. Run all tests — pass.

#### REFACTOR

- Ensure error codes match the design table (`CONVERSATION_CLOSED`, `NOT_A_CONVERSATION`, `NO_AGENT_ASSIGNED`).
- Verify SSE event payload matches the documented shape.

#### Acceptance Criteria

- [x] `POST /api/conversations/{id}/messages` creates a user comment
- [x] `conversation.message` SSE event emitted
- [x] Wakeup enqueued with `conversation_message` source
- [x] Closed conversations rejected with 422
- [x] Non-conversation issues rejected with 422
- [x] Agent paused: message saved, wakeup silently skipped
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/conversation_handler.go`
- **Modify:** `internal/server/handlers/conversation_handler_test.go`

---

### [x] Task 04 — ConversationHandler: List, Messages, Close

**Requirements:** REQ-CONV-003, REQ-CONV-015, REQ-CONV-018, REQ-CONV-024
**Estimated time:** 45 min

#### Context

Implement the remaining user-facing conversation endpoints: listing agent conversations, listing messages, and closing a conversation.

#### RED — Write Failing Tests

Add to `conversation_handler_test.go`:

1. `TestListAgentConversations` — create two conversations and one task for an agent, call `GET /api/agents/{id}/conversations`, verify only conversations returned with correct pagination.
2. `TestListMessages` — create three comments on a conversation, call `GET /api/conversations/{id}/messages`, verify all returned in chronological order with author attribution.
3. `TestListMessages_Pagination` — create 10 comments, request with `limit=3&offset=2`, verify correct subset returned.
4. `TestCloseConversation` — close an active conversation, verify `status=done` and `issue.updated` SSE event emitted.
5. `TestCloseConversation_AlreadyClosed` — close a conversation with `status=done`, verify 422 invalid status transition.
6. `TestCloseConversation_NotConversation` — attempt to close a task issue, verify 422.

Tests fail because handlers are not implemented.

#### GREEN — Implement Minimum to Pass

1. Add `ListAgentConversations` handler — uses `ListConversationsByAgent` query from Task 01.
2. Add `ListMessages` handler — uses existing `ListIssueComments` and `CountIssueComments` queries.
3. Add `CloseConversation` handler — validates type, transitions status, logs activity.
4. Register routes: `GET /api/agents/{id}/conversations`, `GET /api/conversations/{id}/messages`, `PATCH /api/conversations/{id}/close`.
5. Run all tests — pass.

#### REFACTOR

- Reuse `dbCommentToResponse` from `issue_handler.go` for consistent comment serialization.
- Ensure pagination response format matches existing patterns (`data` + `pagination`).

#### Acceptance Criteria

- [x] `GET /api/agents/{id}/conversations` returns only conversation issues for the agent
- [x] `GET /api/conversations/{id}/messages` returns comments with pagination
- [x] `PATCH /api/conversations/{id}/close` transitions to `done`
- [x] Activity logged for close: `conversation.closed`
- [x] Non-conversation issues rejected
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/conversation_handler.go`
- **Modify:** `internal/server/handlers/conversation_handler_test.go`

---

### [x] Task 05 — AgentSelfHandler: Reply + List Conversations

**Requirements:** REQ-CONV-009, REQ-CONV-010, REQ-CONV-017, REQ-CONV-023, REQ-CONV-031
**Estimated time:** 60 min

#### Context

Extend `AgentSelfHandler` with two new endpoints: `POST /api/agent/me/reply` (agent posts a conversation reply) and `GET /api/agent/me/conversations` (agent lists its conversations). These use Run Token auth.

#### RED — Write Failing Tests

Add to `internal/server/handlers/agent_self_handler_test.go`:

1. `TestAgentReply_Success` — agent posts reply with valid Run Token, verify: IssueComment created with `authorType=agent`; `conversation.agent.replied` SSE event emitted with correct payload.
2. `TestAgentReply_NotAssignee` — agent is not the conversation's assignee, verify 403.
3. `TestAgentReply_ClosedConversation` — conversation is `done`, verify 403.
4. `TestAgentReply_NotConversation` — issue is `type=task`, verify 403.
5. `TestAgentReply_EmptyBody` — empty body, verify 400.
6. `TestAgentReply_SquadMismatch` — conversation belongs to different squad than token, verify 403.
7. `TestAgentReply_NoAuth` — no Run Token, verify 401.
8. `TestAgentListConversations` — agent has two conversations and one task, verify only conversations returned.

Tests fail because `Reply` and `ListConversations` are not implemented.

#### GREEN — Implement Minimum to Pass

1. Add `Reply` handler to `AgentSelfHandler`:
   - Extract agent identity from Run Token context.
   - Validate `conversationId` and `body`.
   - Verify conversation type, assignee, squad scope, and status.
   - Create IssueComment with `authorType=agent`.
   - Log activity: `conversation.agent_replied`.
   - Emit `conversation.agent.replied` SSE event.
2. Add `ListConversations` handler:
   - Extract agent identity from Run Token context.
   - Query `ListConversationsByAgent` with agent ID.
   - Return conversation list.
3. Register routes in `RegisterRoutes`.
4. Run all tests — pass.

#### REFACTOR

- Ensure `Reply` response matches `commentResponse` struct format.
- Add doc comments to new methods.

#### Acceptance Criteria

- [x] `POST /api/agent/me/reply` creates agent comment on conversation
- [x] `conversation.agent.replied` SSE event emitted
- [x] Agent must be the conversation's assignee
- [x] Closed conversations reject replies
- [x] Squad scope enforced via Run Token
- [x] `GET /api/agent/me/conversations` returns agent's conversations
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/agent_self_handler.go`
- **Modify:** `internal/server/handlers/agent_self_handler_test.go`

---

### [x] Task 06 — Run Token: Add `conv_id` Claim

**Requirements:** REQ-CONV-008, REQ-CONV-031
**Estimated time:** 30 min

#### Context

The PRD specifies that the Run Token JWT includes a `conv_id` claim (`"conv_id": "<issue_id|null>"`). This allows the `GetMe` handler to read the conversation ID directly from the token identity instead of performing a multi-step DB lookup. Requires modifications to `internal/auth/run_token.go`.

#### RED — Write Failing Tests

Add to `internal/auth/run_token_test.go`:

1. `TestMint_WithConversationID` — mint a token with a conversation ID, validate it, verify `ConversationID` claim is present and correct.
2. `TestMint_WithoutConversationID` — mint a token without conversation ID, validate it, verify `ConversationID` claim is empty/omitted.
3. `TestValidate_ConversationID` — validate a token with `conv_id` claim, verify `AgentIdentity.ConversationID` is populated.

Tests fail because `ConversationID` field does not exist on `RunTokenClaims` or `AgentIdentity`.

#### GREEN — Implement Minimum to Pass

1. Add `ConversationID string \`json:"conv_id,omitempty"\`` to `RunTokenClaims` in `internal/auth/run_token.go`.
2. Add `ConversationID uuid.UUID` to `AgentIdentity` (use `uuid.Nil` when not set).
3. Modify `RunTokenService.Mint()` signature to accept optional conversation ID. Use a variadic option pattern to avoid breaking existing callers:
   ```go
   type MintOption func(*RunTokenClaims)

   func WithConversationID(convID uuid.UUID) MintOption {
       return func(c *RunTokenClaims) {
           c.ConversationID = convID.String()
       }
   }

   func (s *RunTokenService) Mint(agentID, squadID, runID uuid.UUID, role string, opts ...MintOption) (string, error)
   ```
4. Update `Validate()` to parse `conv_id` claim into `AgentIdentity.ConversationID`.
5. Update the `RunService.Invoke()` call to `Mint()` to pass `WithConversationID(convID)` when `convID != nil`.
6. Run all tests — pass.

#### REFACTOR

- Verify existing `Mint()` callers still compile without changes (variadic opts are backward-compatible).
- Ensure `conv_id` is omitted from JWT when not set (JSON `omitempty` handles this).

#### Acceptance Criteria

- [x] `RunTokenClaims` has `ConversationID` field with `json:"conv_id,omitempty"`
- [x] `AgentIdentity` has `ConversationID` field
- [x] `Mint()` accepts optional conversation ID without breaking existing callers
- [x] `Validate()` populates `AgentIdentity.ConversationID` from `conv_id` claim
- [x] `RunService.Invoke()` passes conversation ID to `Mint()` when applicable
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/auth/run_token.go`
- **Modify:** `internal/auth/run_token_test.go`
- **Modify:** `internal/server/handlers/run_handler.go` (Mint call)

---

### [x] Task 07 — RunService: Conversation Context + Typing SSE

**Requirements:** REQ-CONV-007, REQ-CONV-008, REQ-CONV-011, REQ-CONV-012, REQ-CONV-013, REQ-CONV-014
**Estimated time:** 60 min

#### Context

Extend `RunService` to support conversation wakeups. This involves three critical changes:

1. **Session loading in `Invoke()` (C3):** The `Invoke()` method (around line 78-89 in `run_handler.go`) must be modified to check for `ARI_CONVERSATION_ID` in the wakeup context. When present, use `GetConversationSession(agentID, convID)` instead of `GetTaskSession(agentID, taskID)`. Without this, conversation session state will never be restored across turns.

2. **Conversation context in `buildInvokeInput()` (C2):** This is entirely new code — load conversation issue, fetch message thread (up to 100 messages), assemble conversation-specific prompt, and populate `InvokeInput.Conversation`. Add a guard: `if taskID != nil && convID == nil` around the existing task-prompt block to prevent generating task prompts for conversation wakeups (ISSUE-09).

3. **Typing indicator SSE events (M4):** Extract `conversationId` from wakeup context BEFORE the `heartbeat.run.started` SSE emission so typing events can include the conversation ID.

Also extend `AgentSelfHandler.GetMe` to include conversation data when the wake reason is `conversation_message`.

#### RED — Write Failing Tests

Add to `internal/server/handlers/run_handler_test.go`:

1. `TestInvoke_ConversationSessionLoading` — create a conversation with existing session state in `agent_conversation_sessions`, invoke with `ARI_CONVERSATION_ID` set (and NO `ARI_TASK_ID`), verify `GetConversationSession` is called (not `GetTaskSession`) and `sessionBefore` is populated correctly.
2. `TestBuildInvokeInput_ConversationContext` — create a conversation wakeup with `ARI_CONVERSATION_ID`, verify `InvokeInput.Conversation` is populated with messages and session state.
3. `TestBuildInvokeInput_ConversationPrompt` — verify the assembled prompt includes conversation title, message thread, and reply instructions (NOT task-oriented prompt).
4. `TestBuildInvokeInput_TaskPromptGuard` — create a wakeup with both `ARI_TASK_ID` and `ARI_CONVERSATION_ID`, verify the task-prompt block is skipped (no "Your current task:" in prompt).
5. `TestBuildInvokeInput_NoConversation` — wakeup without `ARI_CONVERSATION_ID`, verify `InvokeInput.Conversation` is nil.
6. `TestInvoke_ConversationTypingSSE` — invoke with `conversation_message` source, verify `conversation.agent.typing` SSE event emitted after `heartbeat.run.started` and includes `conversationId`.
7. `TestFinalize_ConversationTypingStoppedSSE` — finalize a conversation run, verify `conversation.agent.typing.stopped` SSE event emitted.

Add to `internal/server/handlers/agent_self_handler_test.go`:

6. `TestGetMe_WithConversation` — agent's wakeup has `ARI_CONVERSATION_ID` set, verify response includes `conversation` field with messages and session state.

Tests fail because conversation context loading and typing SSE are not implemented.

#### GREEN — Implement Minimum to Pass

1. **Modify `RunService.Invoke()` session loading (C3):**
   - Extract `convID` via `extractConversationID(wakeup.ContextJson)` alongside the existing `extractTaskID` call (around line 78-89).
   - If `convID != nil`, call `GetConversationSession(agentID, convID)` to load session state.
   - Else if `taskID != nil`, use existing `GetTaskSession` path.
   - Extract `convID` BEFORE `heartbeat.run.started` SSE emission so it is available for the typing event (M4).
2. **Extend `RunService.Invoke()` with typing SSE:**
   - Immediately after `heartbeat.run.started` SSE, if `invocationSource=conversation_message` and `convID != nil`, emit `conversation.agent.typing` with `conversationId` and `agentId`.
3. **Extend `RunService.buildInvokeInput()` (C2 — all new code):**
   - Guard the existing task-prompt block: change `if taskID != nil {` to `if taskID != nil && convID == nil {` (ISSUE-09).
   - When `convID != nil`, load conversation issue, message thread (up to 100 messages), and conversation session state.
   - Assemble conversation-specific prompt with message thread and reply instructions.
   - Populate `adapter.ConversationContext` on `InvokeInput`.
4. **Extend `RunService.finalize()`:**
   - If wakeup source is `conversation_message`, emit `conversation.agent.typing.stopped`.
5. **Extend `AgentSelfHandler.GetMe`:**
   - Read conversation ID from `AgentIdentity.ConversationID` (populated from `conv_id` Run Token claim — see M5/M6).
   - If present, load conversation issue and messages, include in response.
6. Run all tests — pass.

#### REFACTOR

- Extract conversation prompt assembly into a separate `buildConversationPrompt` function.
- Extract message thread formatting into `formatMessageThread(comments []db.IssueComment) string`.
- Ensure `adapter.CommentEntry` type is defined (may already exist from Feature 11).

#### Acceptance Criteria

- [x] `Invoke()` uses `GetConversationSession` (not `GetTaskSession`) when `ARI_CONVERSATION_ID` is present
- [x] `buildInvokeInput` populates `ConversationContext` when `ARI_CONVERSATION_ID` is present
- [x] Task-prompt block is guarded with `if taskID != nil && convID == nil` (no task prompt for conversations)
- [x] Conversation prompt includes message thread and reply instructions
- [x] `conversation.agent.typing` SSE emitted when conversation run starts, includes `conversationId`
- [x] `conversation.agent.typing.stopped` SSE emitted when conversation run finishes
- [x] Session state loaded from `agent_conversation_sessions`
- [x] `GET /api/agent/me` includes conversation field when wake reason is `conversation_message`
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/run_handler.go`
- **Modify:** `internal/server/handlers/run_handler_test.go`
- **Modify:** `internal/server/handlers/agent_self_handler.go`
- **Modify:** `internal/server/handlers/agent_self_handler_test.go`

---

### [x] Task 08 — Server Wiring + Route Registration

**Requirements:** REQ-CONV-022, REQ-CONV-024
**Estimated time:** 30 min

#### Context

Wire the `ConversationHandler` into the server initialization in `cmd/ari/run.go` (or wherever handlers are registered). Ensure all conversation routes are registered with the HTTP mux and that the handler has access to required dependencies (queries, dbConn, wakeupSvc, sseHub).

#### RED — Write Failing Tests

1. `TestServerRoutes_ConversationEndpoints` — start the server and verify that conversation routes are registered (send requests, expect non-404 responses for all conversation paths).
2. Verify that `POST /api/agent/me/reply` is registered (non-404 for agent-auth'd request).

Tests fail because conversation routes are not wired.

#### GREEN — Implement Minimum to Pass

1. In server initialization:
   - Create `ConversationHandler` with dependencies.
   - Call `conversationHandler.RegisterRoutes(mux)`.
   - The `AgentSelfHandler` routes already include the new `Reply` and `ListConversations` from Task 05.
   - The Run Token `conv_id` claim from Task 06 must be wired through.
2. Run full test suite — pass.

#### REFACTOR

- Verify handler creation order (ConversationHandler needs WakeupService, which needs to be created first).
- Ensure no route conflicts with existing issue endpoints.

#### Acceptance Criteria

- [x] `ConversationHandler` created and routes registered in server startup
- [x] All conversation endpoints return non-404 responses
- [x] Agent self-service reply and conversations endpoints registered
- [x] `make test` passes
- [x] `make build` succeeds

#### Files to Create / Modify

- **Modify:** `cmd/ari/run.go` (or server initialization file)

---

### [ ] Task 09 — React Chat UI

**Requirements:** REQ-CONV-028, REQ-CONV-029
**Estimated time:** 90 min

#### Context

Build the React chat interface: a conversation list sidebar, a message thread panel with real-time SSE updates, and a message input. Uses the existing SSE subscription infrastructure from the dashboard.

#### RED — Write Failing Tests

Create `web/src/pages/__tests__/ConversationsPage.test.tsx`:

1. `renders conversation list` — mock API response with two conversations, verify both appear in sidebar.
2. `sends message` — type in input, click send, verify API called with correct body, verify optimistic message appears.
3. `receives agent reply via SSE` — simulate `conversation.agent.replied` SSE event, verify message appears in thread.
4. `shows typing indicator` — simulate `conversation.agent.typing` SSE event, verify typing indicator visible.
5. `hides typing indicator` — simulate `conversation.agent.typing.stopped`, verify indicator removed.
6. `starts new conversation` — click new conversation button, select agent, enter title, verify API called.
7. `closes conversation` — click close button, verify API called, verify status updates.

Tests fail because components do not exist.

#### GREEN — Implement Minimum to Pass

1. Create `web/src/api/conversations.ts` — API client functions.
2. Create `web/src/hooks/useConversation.ts` — state management with SSE event handlers.
3. Create `web/src/components/ChatPanel.tsx` — message list + input.
4. Create `web/src/components/ConversationSidebar.tsx` — conversation list.
5. Create `web/src/components/MessageBubble.tsx` — individual message rendering.
6. Create `web/src/components/TypingIndicator.tsx` — animated typing dots.
7. Create `web/src/pages/ConversationsPage.tsx` — main page assembling sidebar + chat panel.
8. Add route to `web/src/App.tsx` or router config.
9. Add "Conversations" link to sidebar navigation.
10. Run tests — pass.

#### REFACTOR

- Use shadcn/ui components for buttons, inputs, and scrollable areas.
- Apply Tailwind styling consistent with existing dashboard.
- Ensure messages auto-scroll to bottom on new message.
- Add empty state for no conversations / no messages.
- Ensure mobile-responsive layout (sidebar collapses on small screens).

#### Acceptance Criteria

- [ ] Conversation list displays in sidebar
- [ ] Clicking a conversation loads its message thread
- [ ] Messages display with correct author attribution (user vs agent)
- [ ] Sending a message shows optimistic update and calls API
- [ ] SSE events update the thread in real time
- [ ] Typing indicator shows/hides based on SSE events
- [ ] New conversation flow works (agent picker, title, first message)
- [ ] Close conversation button works
- [ ] `make ui-build` succeeds
- [ ] Component tests pass

#### Files to Create / Modify

- **Create:** `web/src/api/conversations.ts`
- **Create:** `web/src/hooks/useConversation.ts`
- **Create:** `web/src/components/ChatPanel.tsx`
- **Create:** `web/src/components/ConversationSidebar.tsx`
- **Create:** `web/src/components/MessageBubble.tsx`
- **Create:** `web/src/components/TypingIndicator.tsx`
- **Create:** `web/src/pages/ConversationsPage.tsx`
- **Modify:** `web/src/App.tsx` (or router config — add route)
- **Modify:** Sidebar navigation component (add Conversations link)
