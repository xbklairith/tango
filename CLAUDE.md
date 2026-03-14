# Ari — The Control Plane for AI Agents

## Project Overview
Ari is a self-hosted control plane for deploying, governing, and sharing AI agent workforces. Single Go binary with embedded PostgreSQL and React UI.

## Tech Stack
- **Backend:** Go 1.24 + stdlib `net/http` + Cobra CLI
- **Database:** PostgreSQL (embedded-postgres-go for dev, external for prod) + sqlc + goose
- **Frontend:** React 19 + Vite + Tailwind CSS + shadcn/ui
- **Auth:** JWT + bcrypt + sessions
- **Real-time:** Server-Sent Events (SSE)

## Project Structure
```
cmd/ari/            # CLI entrypoint (cobra)
internal/
  server/           # HTTP server, router, handlers
  database/         # DB connection, migrations, queries
    migrations/     # goose SQL migrations
    queries/        # sqlc SQL queries
    db/             # sqlc generated Go code
  config/           # Configuration types
  domain/           # Domain models
  adapter/          # Agent runtime adapters
web/                # React SPA (Vite)
docx/               # Documentation
  core/             # PRD, BRD, tech stack, codebase guide
  features/         # Feature specs
```

## Commands
```bash
make dev            # Run server in dev mode
make build          # Build binary to bin/ari
make test           # Run Go tests
make sqlc           # Regenerate sqlc code
make ui-dev         # Run frontend dev server
make ui-build       # Build frontend for production
```

## Key Patterns
- All entities are squad-scoped (strict data isolation)
- Issue identifiers: "{prefix}-{counter}" (e.g., ARI-1)
- Agent hierarchy is a strict tree (captain → lead → member, no cycles)
- Activity log and cost events are append-only (immutable)
- Atomic task checkout via CAS pattern
- Budget enforcement: soft alert at 80%, hard stop (auto-pause) at 100%
- Roles: Captain = lead agent, Lead = sub-team manager, Member = standard agent

## API
- Base: `http://localhost:3100/api`
- All responses: `Content-Type: application/json`
- Error format: `{"error": "message", "code": "CODE"}`

## Phase 1 Focus (v0.1)
- Go scaffold with CLI + HTTP server
- Database schema + migrations
- Squad/Agent/Issue CRUD
- Basic React dashboard
- `ari run` one-command startup

# currentDate
Today's date is 2026-03-12.
