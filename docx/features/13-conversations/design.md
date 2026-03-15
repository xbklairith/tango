# Design: Conversations (Agent Chat Interface)

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Architecture Overview

Conversations layer on top of existing infrastructure from the Agent Runtime (Feature 11). No new database tables are needed — conversations are issues with `type=conversation`, messages are `issue_comments`, and session state uses `agent_conversation_sessions`. The new code is primarily HTTP handlers, prompt assembly logic, and a React chat UI.

### High-Level Flow

```
User                  ConversationHandler           WakeupService          RunService
  |                         |                            |                      |
  |  POST /conversations    |                            |                      |
  |  /{id}/messages         |                            |                      |
  |  { "body": "..." }      |                            |                      |
  |------------------------>|                            |                      |
  |                         |  1. Create IssueComment    |                      |
  |                         |     (authorType=user)      |                      |
  |                         |                            |                      |
  |                         |  2. Emit SSE:              |                      |
  |                         |  conversation.message      |                      |
  |                         |                            |                      |
  |                         |  3. Enqueue wakeup ------->|                      |
  |                         |     (conversation_message) |                      |
  |  201 { comment }        |                            |                      |
  |<------------------------|                            |                      |
  |                         |                            |  4. Dispatch          |
  |                         |                            |---->|                 |
  |                         |                            |     |                 |
  |  SSE: conversation.     |                            |     | 5. Load context |
  |  agent.typing           |<------ (SSE Hub) <---------|-----|                 |
  |                         |                            |     |                 |
  |                         |                            |     | 6. Execute      |
  |                         |                            |     |    adapter      |
  |                         |                            |     |                 |
  |                         |                            |     | 7. Agent calls  |
  |                     AgentSelfHandler                 |     |    POST /reply  |
  |                         |<----------------------------------|                |
  |                         |  8. Create IssueComment    |     |                 |
  |                         |     (authorType=agent)     |     |                 |
  |                         |  9. Emit SSE:              |     |                 |
  |  SSE: conversation.     |  conversation.agent.replied|     |                 |
  |  agent.replied          |<------- (SSE Hub) ---------|     |                 |
  |                         |                            |     |                 |
  |                         |                            |     | 10. Run done    |
  |  SSE: conversation.     |                            |     | → finalize      |
  |  agent.typing.stopped   |<------ (SSE Hub) <---------|-----|                 |
```

---

## Data Model

No new tables. The feature reuses:

### Issues Table (type=conversation)

A conversation is created as:

```sql
INSERT INTO issues (squad_id, identifier, type, title, status, priority, assignee_agent_id)
VALUES ($1, $2, 'conversation', $3, 'in_progress', 'medium', $4);
```

Key differences from tasks:
- `type = 'conversation'` (vs `'task'`)
- `status` defaults to `'in_progress'` (conversations are active immediately)
- `assignee_agent_id` is required (must have an agent to chat with)

### Issue Comments (messages)

Each message is an `IssueComment`:

| Field | Value (User Message) | Value (Agent Reply) |
|-------|---------------------|---------------------|
| `issue_id` | conversation issue ID | conversation issue ID |
| `author_type` | `user` | `agent` |
| `author_id` | user's UUID | agent's UUID |
| `body` | message text | reply text |

### Agent Conversation Sessions

Session state persists across turns:

```sql
-- Already exists in agent_sessions.sql
INSERT INTO agent_conversation_sessions (agent_id, issue_id, session_state)
VALUES ($1, $2, $3)
ON CONFLICT (agent_id, issue_id) DO UPDATE
    SET session_state = EXCLUDED.session_state, updated_at = now();
```

---

## API Endpoints

### User-Facing Endpoints

#### `POST /api/agents/{id}/conversations` — Start Conversation

Creates a new conversation issue assigned to the specified agent.

**Request:**
```json
{
    "title": "Help me refactor the auth module",
    "message": "I need to split the auth package into separate concerns..."
}
```

**Response (201):**
```json
{
    "conversation": {
        "id": "uuid",
        "identifier": "ARI-42",
        "type": "conversation",
        "title": "Help me refactor the auth module",
        "status": "in_progress",
        "assigneeAgentId": "agent-uuid",
        "createdAt": "2026-03-15T10:00:00Z",
        "updatedAt": "2026-03-15T10:00:00Z"
    },
    "message": {
        "id": "comment-uuid",
        "issueId": "uuid",
        "authorType": "user",
        "authorId": "user-uuid",
        "body": "I need to split the auth package...",
        "createdAt": "2026-03-15T10:00:00Z"
    }
}
```

**Handler logic:**
1. Parse and validate `{id}` as agent UUID; verify agent exists and belongs to user's squad.
2. Begin transaction.
3. Increment squad issue counter → generate identifier.
4. Create issue with `type=conversation`, `status=in_progress`, `assigneeAgentId={id}`.
5. If `message` field is present, create IssueComment with `authorType=user`.
6. Log activity: `conversation.created`.
7. Commit transaction.
8. If message was created, enqueue wakeup with `invocationSource=conversation_message`.
9. Emit `conversation.message` SSE event (if message was created).

#### `POST /api/conversations/{id}/messages` — Send Message

**Request:**
```json
{
    "body": "What about using the strategy pattern for this?"
}
```

**Response (201):**
```json
{
    "id": "comment-uuid",
    "issueId": "conv-uuid",
    "authorType": "user",
    "authorId": "user-uuid",
    "body": "What about using the strategy pattern for this?",
    "createdAt": "2026-03-15T10:01:30Z"
}
```

**Handler logic:**
1. Parse `{id}` as conversation issue UUID.
2. Fetch issue; verify `type=conversation`, `status` is not `done`/`cancelled`.
3. Verify user is a member of the conversation's squad.
4. Verify `assigneeAgentId IS NOT NULL` — reject with 422 if no agent.
5. Begin transaction.
6. Create IssueComment with `authorType=user`, `authorId` from session.
7. Log activity: `conversation.message_sent`.
8. Commit transaction.
9. Emit `conversation.message` SSE event.
10. Enqueue wakeup: `invocationSource=conversation_message`, context includes `ARI_CONVERSATION_ID`.

#### `GET /api/conversations/{id}/messages` — List Messages

Alias for `GET /api/issues/{id}/comments` — same handler, same pagination.

**Response (200):**
```json
{
    "data": [
        {
            "id": "comment-uuid",
            "issueId": "conv-uuid",
            "authorType": "user",
            "authorId": "user-uuid",
            "body": "I need to split the auth package...",
            "createdAt": "2026-03-15T10:00:00Z",
            "updatedAt": "2026-03-15T10:00:00Z"
        },
        {
            "id": "comment-uuid-2",
            "issueId": "conv-uuid",
            "authorType": "agent",
            "authorId": "agent-uuid",
            "body": "I'd suggest separating into three packages...",
            "createdAt": "2026-03-15T10:00:45Z",
            "updatedAt": "2026-03-15T10:00:45Z"
        }
    ],
    "pagination": { "limit": 50, "offset": 0, "total": 2 }
}
```

#### `GET /api/agents/{id}/conversations` — List Agent's Conversations

**Query:** `ListIssuesBySquad` with `filterType=conversation` and `filterAssigneeAgentId={id}`.

**Response (200):**
```json
{
    "data": [
        {
            "id": "conv-uuid",
            "identifier": "ARI-42",
            "type": "conversation",
            "title": "Help me refactor the auth module",
            "status": "in_progress",
            "assigneeAgentId": "agent-uuid",
            "createdAt": "2026-03-15T10:00:00Z",
            "updatedAt": "2026-03-15T10:01:30Z"
        }
    ],
    "pagination": { "limit": 50, "offset": 0, "total": 1 }
}
```

#### `PATCH /api/conversations/{id}/close` — Close Conversation

**Response (200):**
```json
{
    "id": "conv-uuid",
    "type": "conversation",
    "status": "done",
    "...": "..."
}
```

**Handler logic:**
1. Fetch issue; verify `type=conversation`.
2. Validate status transition to `done`.
3. Update issue `status=done`.
4. Log activity: `conversation.closed`.
5. Emit `issue.updated` SSE event.

### Agent Self-Service Endpoints

> **Design Decision (C1):** Agents do NOT use the existing `POST /api/issues/{issueId}/comments` endpoint to reply in conversations. That endpoint requires `UserFromContext` (user session auth) and is designed for human users adding comments to issues. Instead, agents reply via `POST /api/agent/me/reply`, which uses `AgentFromContext` (Run Token auth). This separation ensures proper auth boundaries — agents authenticate with Run Tokens, not user sessions.

#### `POST /api/agent/me/reply` — Agent Posts Reply

**Request:**
```json
{
    "conversationId": "conv-uuid",
    "body": "I'd suggest separating into three packages:\n1. auth/session\n2. auth/token\n3. auth/middleware"
}
```

**Response (201):**
```json
{
    "id": "comment-uuid",
    "issueId": "conv-uuid",
    "authorType": "agent",
    "authorId": "agent-uuid",
    "body": "I'd suggest separating into...",
    "createdAt": "2026-03-15T10:00:45Z",
    "updatedAt": "2026-03-15T10:00:45Z"
}
```

**Handler logic:**
1. Extract agent identity from Run Token context (`AgentFromContext`).
2. Parse `conversationId` and `body` from request body.
3. Fetch conversation issue; verify `type=conversation`.
4. Verify agent is the `assigneeAgentId` of the conversation.
5. Verify conversation is not closed (`status != done/cancelled`).
6. Verify squad scope (issue.squadId == token.squadId).
7. Begin transaction.
8. Create IssueComment with `authorType=agent`, `authorId=agentId`.
9. Log activity: `conversation.agent_replied`.
10. Commit transaction.
11. Emit `conversation.agent.replied` SSE event.

#### `GET /api/agent/me/conversations` — Agent's Conversations

Returns conversation issues assigned to the authenticated agent.

**Response (200):**
```json
{
    "conversations": [
        {
            "id": "conv-uuid",
            "identifier": "ARI-42",
            "title": "Help me refactor the auth module",
            "status": "in_progress",
            "messageCount": 5,
            "lastMessageAt": "2026-03-15T10:01:30Z"
        }
    ]
}
```

### Run Token Extension: `conv_id` Claim

> **Important (M5/M6):** The PRD specifies that the Run Token JWT includes a `conv_id` claim (see PRD line: `"conv_id": "<issue_id|null>"`). This requires modifications to `internal/auth/run_token.go`:

1. Add `ConversationID string json:"conv_id,omitempty"` to `RunTokenClaims`.
2. Add `ConversationID uuid.UUID` (or `*uuid.UUID`) to `AgentIdentity`.
3. Modify `RunTokenService.Mint()` to accept an optional `conversationID` parameter (use variadic `opts ...MintOption` or add a `MintParams` struct to avoid breaking existing callers).
4. Update `Validate()` to populate `AgentIdentity.ConversationID` from the `conv_id` claim.

This allows the `GetMe` handler to read the conversation ID directly from the token identity instead of performing a multi-step DB lookup (find active run → load wakeup → parse context JSON). The `Invoke()` method must pass the conversation ID when calling `Mint()`.

#### `GET /api/agent/me` — Extended Response

When `ARI_WAKE_REASON=conversation_message`, the existing `/api/agent/me` response is extended. The conversation ID is read from `AgentIdentity.ConversationID` (populated from the `conv_id` Run Token claim):

```json
{
    "agent": { "id": "...", "name": "...", "..." : "..." },
    "squad": { "id": "...", "name": "...", "slug": "..." },
    "tasks": [ "..." ],
    "conversation": {
        "id": "conv-uuid",
        "identifier": "ARI-42",
        "title": "Help me refactor the auth module",
        "messages": [
            {
                "id": "comment-uuid",
                "authorType": "user",
                "authorId": "user-uuid",
                "body": "I need to split the auth package...",
                "createdAt": "2026-03-15T10:00:00Z"
            }
        ],
        "sessionState": "previous-session-blob-if-any"
    }
}
```

The `conversation` field is only present when the wakeup reason is `conversation_message` and `ARI_CONVERSATION_ID` is set in the wakeup context.

> **Note (ISSUE-10):** The `tasks` array in the `GET /api/agent/me` response uses `ListIssuesByAssigneeAgent`. This query should filter `type != 'conversation'` so that conversation issues do not appear in the `tasks` array. Alternatively, rename the field to `issues` to be type-agnostic. For v1, filtering out conversations from the `tasks` array is the simpler approach.

---

## Wakeup Flow (Conversation Message)

The conversation wakeup uses the same pipeline as any other wakeup (Feature 11), with conversation-specific context injection:

### 1. Enqueue

> **Design Decision (ISSUE-09):** Only set `ARI_CONVERSATION_ID` in the wakeup context map, NOT `ARI_TASK_ID`. Setting `ARI_TASK_ID` would cause `buildInvokeInput` to generate a task-oriented prompt ("Your current task: ...") which is wrong for conversations. The conversation has its own session lookup path via `GetConversationSession`.

```go
// In ConversationHandler.SendMessage()
ctxMap := map[string]any{
    "ARI_CONVERSATION_ID": conversationIssueID.String(),
    // NOTE: Do NOT set ARI_TASK_ID here. Conversations use a separate
    // session lookup (GetConversationSession) and prompt assembly path.
}
wakeupSvc.Enqueue(ctx, agentID, squadID, "conversation_message", ctxMap)
```

### 2. Dispatch (RunService.Invoke) — Session Loading for Conversations

> **Critical (C3):** The `Invoke()` method (around line 78-89 in `run_handler.go`) currently loads session state ONLY via `GetTaskSession` keyed by `taskID`. For conversations, this method **must be modified** to:
> 1. Extract `convID` from the wakeup context via `extractConversationID(wakeup.ContextJson)`.
> 2. If `convID != nil`, call `GetConversationSession(agentID, convID)` instead of `GetTaskSession`.
> 3. If `convID == nil` and `taskID != nil`, use the existing `GetTaskSession` path.
>
> This is critical for session continuity — without this change, conversation session state will never be loaded on subsequent turns.

```go
// In RunService.Invoke(), replace the session loading block (lines 78-89):
var sessionBefore string
taskID := extractTaskID(wakeup.ContextJson)
convID := extractConversationID(wakeup.ContextJson)

if convID != nil {
    // Conversation session — separate table from task sessions
    ss, err := s.queries.GetConversationSession(ctx, db.GetConversationSessionParams{
        AgentID: agent.ID,
        IssueID: *convID,
    })
    if err == nil {
        sessionBefore = ss
    }
} else if taskID != nil {
    // Task session — existing path
    ss, err := s.queries.GetTaskSession(ctx, db.GetTaskSessionParams{
        AgentID: agent.ID,
        IssueID: *taskID,
    })
    if err == nil {
        sessionBefore = ss
    }
}
```

### 3. Build Invoke Input (New Conversation Code)

> **Critical (C2):** The conversation context loading in `buildInvokeInput` is **entirely new functionality** that must be written. This includes: extracting the conversation ID from wakeup context, loading the conversation issue, fetching the message thread (up to `maxMessages`), assembling the conversation prompt, loading conversation session state, and populating `InvokeInput.Conversation`. None of this code exists today.
>
> **Guard (ISSUE-09):** The task-prompt block (`if taskID != nil { ... }` around line 368-408) must be guarded with `if taskID != nil && convID == nil` to prevent generating a task-oriented prompt when processing a conversation wakeup.

Add new conversation context loading to `RunService.buildInvokeInput` when `ARI_CONVERSATION_ID` is present:

```go
if convID != nil {
    // Load conversation issue
    convIssue, err := s.queries.GetIssueByID(ctx, *convID)
    if err == nil {
        // Load message thread (up to maxMessages)
        comments, err := s.queries.ListIssueComments(ctx, db.ListIssueCommentsParams{
            IssueID:    *convID,
            PageLimit:  100,
            PageOffset: 0,
        })
        if err == nil {
            messages := make([]adapter.CommentEntry, 0, len(comments))
            for _, c := range comments {
                messages = append(messages, adapter.CommentEntry{
                    ID:         c.ID,
                    AuthorType: string(c.AuthorType),
                    AuthorID:   c.AuthorID,
                    Body:       c.Body,
                    CreatedAt:  c.CreatedAt,
                })
            }

            // Load conversation session state
            var sessionState string
            ss, err := s.queries.GetConversationSession(ctx, db.GetConversationSessionParams{
                AgentID: agent.ID,
                IssueID: *convID,
            })
            if err == nil {
                sessionState = ss
            }

            input.Conversation = &adapter.ConversationContext{
                IssueID:      convID.String(),
                Messages:     messages,
                SessionState: sessionState,
            }
        }
    }
}
```

### 4. Prompt Assembly (Conversation)

When `wake_reason=conversation_message`, build a conversation-specific prompt:

```go
prompt := fmt.Sprintf(`You are %s, a %s in squad %s.

%s

You are in a conversation: %s (%s)

Message thread:
%s

Reply to the user's latest message. Use POST /api/agent/me/reply to send your response:
POST %s/api/agent/me/reply
Body: {"conversationId": "%s", "body": "<your reply>"}`,
    agent.Name, string(agent.Role), squadName,
    systemPrompt,
    convIssue.Title, convIssue.Identifier,
    formatMessageThread(comments),
    s.apiURL, convID.String(),
)
```

### 5. SSE Events During Conversation

```
event: conversation.agent.typing
data: {"conversationId": "conv-uuid", "agentId": "agent-uuid"}

event: conversation.agent.replied
data: {"conversationId": "conv-uuid", "agentId": "agent-uuid", "commentId": "comment-uuid", "body": "..."}

event: conversation.agent.typing.stopped
data: {"conversationId": "conv-uuid", "agentId": "agent-uuid"}

event: conversation.message
data: {"conversationId": "conv-uuid", "commentId": "comment-uuid", "authorType": "user", "authorId": "user-uuid", "body": "..."}
```

---

## SSE Event Shapes

| Event Type | Trigger | Payload Fields |
|---|---|---|
| `conversation.message` | User sends message | `conversationId`, `commentId`, `authorType`, `authorId`, `body` |
| `conversation.agent.typing` | Agent run starts for conversation | `conversationId`, `agentId` |
| `conversation.agent.replied` | Agent posts reply comment | `conversationId`, `agentId`, `commentId`, `body` |
| `conversation.agent.typing.stopped` | Agent run finishes for conversation | `conversationId`, `agentId` |

---

## Handler Structure

### New Handler: `ConversationHandler`

```go
// internal/server/handlers/conversation_handler.go

type ConversationHandler struct {
    queries   *db.Queries
    dbConn    *sql.DB
    wakeupSvc *WakeupService
    sseHub    *sse.Hub
}

func NewConversationHandler(q *db.Queries, dbConn *sql.DB, ws *WakeupService, hub *sse.Hub) *ConversationHandler

func (h *ConversationHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /api/agents/{id}/conversations", h.StartConversation)
    mux.HandleFunc("GET /api/agents/{id}/conversations", h.ListAgentConversations)
    mux.HandleFunc("POST /api/conversations/{id}/messages", h.SendMessage)
    mux.HandleFunc("GET /api/conversations/{id}/messages", h.ListMessages)
    mux.HandleFunc("PATCH /api/conversations/{id}/close", h.CloseConversation)
}
```

### Extended: `AgentSelfHandler`

Add two new routes:

```go
func (h *AgentSelfHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/agent/me", h.GetMe)           // extended
    mux.HandleFunc("PATCH /api/agent/me/task", h.UpdateTask)
    mux.HandleFunc("POST /api/agent/me/reply", h.Reply)     // new
    mux.HandleFunc("GET /api/agent/me/conversations", h.ListConversations) // new
}
```

### Extended: `RunService.buildInvokeInput`

Add conversation context loading when `ARI_CONVERSATION_ID` is present in the wakeup context.

### Extended: `RunService.Invoke`

Add typing indicator SSE events:

> **Important (M4):** The `conversationId` must be extracted from the wakeup context (`extractConversationID`) BEFORE the `heartbeat.run.started` SSE emission, so the typing event can include the conversation ID. The extraction should happen alongside the existing `extractTaskID` call in the session-loading block (see section 2 above).

- Emit `conversation.agent.typing` immediately after `heartbeat.run.started` when `invocationSource=conversation_message` and `convID != nil`.
- Emit `conversation.agent.typing.stopped` in `finalize()` when the wakeup source is `conversation_message`.

---

## SQL Queries (New)

### `internal/database/queries/issues.sql` — Additions

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

### `internal/database/queries/issue_comments.sql` — Additions

```sql
-- name: GetLatestComment :one
SELECT * FROM issue_comments
WHERE issue_id = @issue_id
ORDER BY created_at DESC
LIMIT 1;
```

---

## React Chat UI

### Component Tree

```
ConversationPage
├── ConversationSidebar          # List of conversations
│   ├── ConversationListItem     # Individual conversation preview
│   └── NewConversationButton    # Start new conversation
├── ChatPanel                    # Active conversation
│   ├── ChatHeader               # Title, agent info, close button
│   ├── MessageList              # Scrollable message thread
│   │   ├── MessageBubble        # Individual message (user/agent)
│   │   └── TypingIndicator      # Shown while agent is processing
│   └── MessageInput             # Text input + send button
└── AgentPicker (modal)          # Select agent for new conversation
```

### State Management

```typescript
interface ConversationState {
    conversations: Conversation[]
    activeConversationId: string | null
    messages: Record<string, Message[]>   // conversationId → messages
    typingAgents: Set<string>             // conversationIds where agent is typing
    sending: boolean                       // optimistic send in progress
}

interface Message {
    id: string
    issueId: string
    authorType: 'user' | 'agent' | 'system'
    authorId: string
    body: string
    createdAt: string
}
```

### SSE Integration

The chat UI subscribes to the squad's SSE stream and filters for conversation events:

```typescript
// In useSSE hook
eventSource.addEventListener('conversation.message', (e) => {
    const data = JSON.parse(e.data)
    addMessage(data.conversationId, data)
})

eventSource.addEventListener('conversation.agent.replied', (e) => {
    const data = JSON.parse(e.data)
    addMessage(data.conversationId, {
        id: data.commentId,
        authorType: 'agent',
        authorId: data.agentId,
        body: data.body,
        createdAt: new Date().toISOString(),
    })
})

eventSource.addEventListener('conversation.agent.typing', (e) => {
    const data = JSON.parse(e.data)
    setTyping(data.conversationId, true)
})

eventSource.addEventListener('conversation.agent.typing.stopped', (e) => {
    const data = JSON.parse(e.data)
    setTyping(data.conversationId, false)
})
```

### API Client

```typescript
// web/src/api/conversations.ts

export async function startConversation(agentId: string, title: string, message?: string) {
    return post(`/api/agents/${agentId}/conversations`, { title, message })
}

export async function sendMessage(conversationId: string, body: string) {
    return post(`/api/conversations/${conversationId}/messages`, { body })
}

export async function listMessages(conversationId: string, limit = 50, offset = 0) {
    return get(`/api/conversations/${conversationId}/messages?limit=${limit}&offset=${offset}`)
}

export async function listAgentConversations(agentId: string) {
    return get(`/api/agents/${agentId}/conversations`)
}

export async function closeConversation(conversationId: string) {
    return patch(`/api/conversations/${conversationId}/close`)
}
```

---

## Error Handling

| Condition | HTTP Status | Error Code | Message |
|---|---|---|---|
| Conversation not found | 404 | `NOT_FOUND` | "Conversation not found" |
| Issue is not a conversation | 422 | `NOT_A_CONVERSATION` | "Issue is not a conversation" |
| Conversation is closed | 422 | `CONVERSATION_CLOSED` | "Conversation is closed" |
| No agent assigned | 422 | `NO_AGENT_ASSIGNED` | "No agent assigned to conversation" |
| Agent not assignee | 403 | `FORBIDDEN` | "Agent is not assigned to this conversation" |
| Squad mismatch | 403 | `FORBIDDEN` | "Not a member of this squad" |
| Invalid message body | 400 | `VALIDATION_ERROR` | "body is required" |
| Agent paused/terminated | 200 | (none) | Message saved; wakeup silently skipped |

---

## File Organization

```
internal/server/handlers/
├── conversation_handler.go       # NEW — user-facing conversation endpoints
├── conversation_handler_test.go  # NEW — handler tests
├── agent_self_handler.go         # MODIFIED — add Reply, ListConversations
├── agent_self_handler_test.go    # MODIFIED — add tests for new endpoints
├── run_handler.go                # MODIFIED — extend buildInvokeInput, typing SSE
web/src/
├── pages/
│   └── ConversationsPage.tsx     # NEW — main conversation page
├── components/
│   ├── ChatPanel.tsx             # NEW — message thread + input
│   ├── ConversationSidebar.tsx   # NEW — conversation list sidebar
│   ├── MessageBubble.tsx         # NEW — individual message rendering
│   └── TypingIndicator.tsx       # NEW — typing dots animation
├── api/
│   └── conversations.ts          # NEW — API client functions
└── hooks/
    └── useConversation.ts        # NEW — state management hook
```

---

## Testing Strategy

1. **Unit tests** for `ConversationHandler` — mock DB queries, verify HTTP responses, SSE events, and wakeup enqueue calls.
2. **Unit tests** for `AgentSelfHandler.Reply` — verify agent auth, conversation ownership, comment creation.
3. **Unit tests** for extended `buildInvokeInput` — verify conversation context is loaded and prompt is assembled correctly.
4. **Integration tests** — full conversation flow: start → send message → agent wakes → agent replies → SSE events received.
5. **React component tests** — `ChatPanel` renders messages, handles SSE events, sends messages.
