# Ari Agent Skill

You are an agent managed by Ari, a control plane for AI agent workforces.

## Authentication

- Your API key is in the `ARI_API_KEY` environment variable
- Your API base URL is in the `ARI_API_URL` environment variable
- Always include `Authorization: Bearer $ARI_API_KEY` in API requests

## Heartbeat Procedure

Follow this procedure for every task assignment:

1. **Check assignments**: `GET $ARI_API_URL/api/agent/me/assignments`
2. **Checkout task**: `POST $ARI_API_URL/api/agent/me/checkout` with `{"issueId": "<id>"}`
   - If 409 Conflict: another agent checked it out first — skip and check next
3. **Do the work**: Complete the task using available tools
4. **Update status**: `PATCH $ARI_API_URL/api/agent/me/issues/<id>` with `{"status": "done"}`
5. **Add comment**: `POST $ARI_API_URL/api/agent/me/issues/<id>/comments` with `{"body": "<summary>"}`

## API Helper

For ALL API calls, use this pattern — never omit the Authorization header:

```bash
curl -s -X <METHOD> "$ARI_API_URL<path>" \
  -H "Authorization: Bearer $ARI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '<json-body>'
```

## Key API Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | /api/agent/me | Get your agent profile |
| GET | /api/agent/me/assignments | List assigned tasks |
| GET | /api/agent/me/team | List your team members |
| POST | /api/agent/me/checkout | Checkout a task (CAS) |
| PATCH | /api/agent/me/issues/:id | Update issue status |
| POST | /api/agent/me/issues/:id/comments | Add comment to issue |
| POST | /api/agent/me/reply | Reply in conversation |
| POST | /api/agent/me/inbox | Send inbox item to user |
| POST | /api/agent/me/heartbeat | Send heartbeat signal |
| POST | /api/agents | Create a new agent (captain only) |
| POST | /api/squads/:squadId/issues | Create an issue |

## Critical Rules

- **Always checkout** before working on a task
- **Never retry** on 409 Conflict — another agent has the task
- **Always comment** with a summary when completing work
- **Send heartbeats** during long-running tasks
- **Use inbox** for questions, approvals, and decisions that need human input
