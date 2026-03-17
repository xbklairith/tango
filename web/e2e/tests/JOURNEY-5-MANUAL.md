# Journey 5: Agent Team Creation — Manual Testing

## Prerequisites

- Server running on `http://localhost:3199`
- A registered user with a squad and captain agent

## Quick Start (automated setup + manual exploration)

```bash
# 1. Clean everything
pkill -9 -f "ari-bin" 2>/dev/null
pkill -9 -f "pg-runtime" 2>/dev/null
lsof -ti:3199 2>/dev/null | xargs kill -9 2>/dev/null
sleep 2
rm -rf ~/.ari/realms/testing/db 2>/dev/null

# 2. Build and run Journey 5 (keeps server alive after test)
go build -o /tmp/ari-bin ./cmd/ari
cd web
KEEP_SERVER=1 npx playwright test e2e/tests/journey-5-agent-team-creation.spec.ts --timeout 300000

# 3. Get credentials
cat /tmp/j5-credentials.json

# 4. Server is at http://localhost:3199 — login with the j5 credentials
```

## What to Expect After Journey 5

Login and you should see:

| Page | What's there |
|------|-------------|
| **Dashboard** | Research Team squad with stat cards |
| **Agents** | Research Captain (captain, idle or running) |
| **Conversations** | "Create investment research team" — captain's reply visible |
| **Inbox** | "Approve: Create Investment Research Team" — resolved/approved |
| **Agent Detail** | 2 runs: `conversation_message` (succeeded) + `inbox_resolved` (succeeded or running) |

## Manual Exploration Steps

### 1. Check the Conversation

Navigate to **Conversations** > "Create investment research team"

- You should see the user message asking to create a team
- The captain's reply confirming an approval request was sent

### 2. Check the Inbox

Navigate to **Inbox**

- The approval item should show as **Resolved** (approved)
- Click into it to see the details: team member names, response note

### 3. Check Agent Runs

Navigate to **Agents** > **Research Captain** > scroll to Runs

- **Run 1** (`conversation_message`): Agent processed the conversation, created inbox approval
- **Run 2** (`inbox_resolved`): Agent woke up after approval, attempted to create team agents

### 4. Start a New Conversation (manual test)

Try talking to the captain yourself:

1. Go to **Conversations** > **New Conversation**
2. Select **Research Captain**
3. Send a message like: "Create a bug tracker agent named Bug Hunter"
4. Wait for the agent to respond (check the agent detail page for run status)
5. If the agent sends an approval request, go to **Inbox** and approve it
6. Watch the `inbox_resolved` run fire — the agent should attempt to create the agent

### 5. Verify the inbox_resolved Prompt

When you approve an inbox item from an agent:

1. The server enqueues an `inbox_resolved` wakeup
2. The agent gets a prompt with:
   - The original inbox item (title, body, category)
   - The resolution (approved/rejected)
   - Your response note
   - Full curl examples for creating agents, issues, replying, and inbox
3. The agent acts on the decision

## Troubleshooting

### "Session expired" on login
- The server was restarted with a fresh DB — your old cookies are stale
- Clear cookies for `localhost:3199` and login again with the j5 credentials

### Agent run stuck on "running"
- The agent (Claude CLI) may be retrying failed API calls
- Check the run logs on the agent detail page
- The agent has a timeout — it will eventually finish

### No agents created after approval
- The LLM agent's curl calls may fail intermittently (auth header issues)
- This is a known limitation — the inbox_resolved wakeup and prompt work correctly
- Check the run logs to see what the agent attempted

### Server died
- Embedded PostgreSQL can crash if ports conflict
- Re-run the quick start commands above
