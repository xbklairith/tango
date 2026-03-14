# Requirements: Go Scaffold

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 01-go-scaffold
**PRD Reference:** docx/core/01-PRODUCT.md (Section 5.1.1 — One-Command Setup)

## Overview

This feature establishes the foundational Go project scaffold for Ari. It delivers a single binary that starts an HTTP server with embedded PostgreSQL, runs database migrations, and serves the API — all from one command: `ari run`. Every subsequent feature builds on this foundation.

## Glossary

| Term | Definition |
|------|-----------|
| Embedded PostgreSQL | A PostgreSQL instance bundled via `embedded-postgres-go`, started in-process for development/single-user mode |
| goose | SQL migration tool for PostgreSQL schema versioning |
| sqlc | Code generator that produces type-safe Go code from SQL queries |
| Cobra | Go CLI framework for building command-line applications |
| Graceful shutdown | Server stops accepting new connections and drains in-flight requests before exiting |

## Stakeholders

| Role | Interest |
|------|----------|
| Developer | Needs a working project scaffold to build features on |
| Operator | Needs a single binary with zero-config startup |
| CI/CD Pipeline | Needs reproducible build and test commands |

---

## Requirements

### REQ-SCAFFOLD-001: Go Module Initialization

**[EARS]** The system **shall** be structured as a Go module with the module path matching the project repository.

**Rationale:** Establishes the Go project identity and enables dependency management via `go mod`.

**Acceptance Criteria:**
- A valid `go.mod` file exists at the project root with Go 1.24 as the minimum version.
- A `go.sum` file exists with locked dependency hashes.
- `go build ./...` completes without errors.

---

### REQ-SCAFFOLD-002: Project Directory Structure

**[EARS]** The system **shall** organize source code into the following directory layout:

```
cmd/ari/            # CLI entrypoint (main.go)
internal/
  server/           # HTTP server, router, handlers
  database/         # DB connection, migrations, queries
    migrations/     # goose SQL migration files
    queries/        # sqlc SQL query files
    db/             # sqlc generated Go code
  config/           # Configuration loading and types
  domain/           # Domain models and business types
  adapter/          # Agent runtime adapters (future)
web/                # React SPA (future)
```

**Rationale:** Follows Go project conventions (`cmd/`, `internal/`) and matches the structure defined in the PRD. Using `internal/` enforces package boundaries at the compiler level.

**Acceptance Criteria:**
- All listed directories exist with at least a placeholder file or package declaration.
- Packages under `internal/` are not importable from outside the module.

---

### REQ-SCAFFOLD-003: Cobra CLI Framework

**[EARS]** The system **shall** use Cobra to implement the CLI with a root command and subcommands.

**Rationale:** Cobra provides a standard CLI structure with built-in help, flag parsing, and subcommand routing used by most Go CLI tools (kubectl, Hugo, GitHub CLI).

**Acceptance Criteria:**
- `cmd/ari/main.go` initializes and executes a Cobra root command.
- Running `ari --help` displays usage information.
- Running `ari version` displays the application version string.

---

### REQ-SCAFFOLD-004: `ari run` Command

**[EARS]** The system **shall** provide an `ari run` command that starts the full application stack in a single invocation.

**Rationale:** The PRD specifies one-command startup (`./ari run`) as a core deploy workflow (Section 5.1.1). This is the primary way users interact with Ari.

**[EARS]** When `ari run` is executed, the system **shall** perform the following steps in order:
1. Load configuration from environment variables and/or config file.
2. Start the embedded PostgreSQL instance (dev mode) or connect to an external PostgreSQL instance (prod mode).
3. Run pending database migrations via goose.
4. Start the HTTP server on the configured port (default: 3100).
5. Log the startup URL to stdout.

**Acceptance Criteria:**
- Running `ari run` starts the server and listens on `:3100` by default.
- The `--port` flag overrides the default port.
- The startup sequence logs each step (DB start, migrations, server listen).
- If any step fails, the process exits with a non-zero exit code and a descriptive error message.

---

### REQ-SCAFFOLD-005: Embedded PostgreSQL

**[EARS]** When running in development mode, the system **shall** use `embedded-postgres-go` to start and manage a PostgreSQL instance automatically.

**Rationale:** Eliminates the need for users to install and configure PostgreSQL separately. Supports the zero-config startup promise.

**[EARS]** When running in production mode, the system **shall** connect to an external PostgreSQL instance specified via `ARI_DATABASE_URL` environment variable.

**[EARS]** The system **shall** pin a specific PostgreSQL version in the embedded-postgres-go configuration that is known to work on both amd64 and arm64 (Apple Silicon) architectures. The pinned version **shall** be specified in the database package (not left to the library default).

**[EARS]** When embedded-postgres-go fails to download or start the PostgreSQL binary (e.g., network failure, corrupt download, unsupported platform), the system **shall** return a clear error message that includes:
1. The specific failure reason (download error, checksum mismatch, unsupported architecture, etc.).
2. The attempted PostgreSQL version and target platform.
3. Actionable troubleshooting steps (e.g., "check network connectivity", "set ARI_DATABASE_URL to use an external PostgreSQL instance instead", "ensure platform arm64/darwin is supported").

**Acceptance Criteria:**
- In dev mode, embedded PostgreSQL starts automatically on `ari run` with data stored in a local directory (default: `./data/postgres`).
- In dev mode, the embedded PostgreSQL instance is stopped when the Ari process exits.
- In prod mode, the system connects to the URL specified in `ARI_DATABASE_URL`.
- If `ARI_DATABASE_URL` is set, embedded PostgreSQL is not started regardless of mode.
- The embedded PostgreSQL version is explicitly pinned (not using the library default) and tested on both amd64 and arm64.
- If the PostgreSQL binary download fails, the error message includes the failure reason, target platform, and troubleshooting steps.

---

### REQ-SCAFFOLD-006: Database Migrations with goose

**[EARS]** The system **shall** use goose to manage database schema migrations using SQL files stored in `internal/database/migrations/`.

**Rationale:** goose provides versioned, ordered, reversible migrations that integrate well with Go and PostgreSQL.

**[EARS]** When the application starts, the system **shall** automatically apply all pending migrations before accepting HTTP requests.

**Acceptance Criteria:**
- Migration files follow the goose naming convention: `YYYYMMDDHHMMSS_description.sql`.
- An initial migration exists that creates the `schema_migrations` table (handled by goose automatically).
- Migrations run to completion before the HTTP server starts listening.
- If a migration fails, the application exits with a non-zero exit code and logs the failing migration and error.

---

### REQ-SCAFFOLD-007: sqlc Code Generation

**[EARS]** The system **shall** use sqlc to generate type-safe Go database access code from SQL queries.

**Rationale:** sqlc generates compile-time verified database code, eliminating runtime SQL errors and reducing boilerplate.

**Acceptance Criteria:**
- A valid `sqlc.yaml` configuration file exists at the project root.
- SQL queries are stored in `internal/database/queries/`.
- Generated Go code is output to `internal/database/db/`.
- Running `make sqlc` regenerates the code without errors.
- Generated code is committed to the repository (not gitignored).

---

### REQ-SCAFFOLD-008: HTTP Server with stdlib Router

**[EARS]** The system **shall** implement an HTTP server using Go's standard library `net/http` package with the `http.ServeMux` router.

**Rationale:** Go 1.22+ ServeMux supports method-based routing and path parameters, eliminating the need for third-party routers. Keeps the dependency footprint minimal.

**Acceptance Criteria:**
- The HTTP server uses `net/http.Server` with configurable timeouts (read, write, idle).
- Routes are registered on `http.ServeMux`.
- The server listens on the port specified by configuration (default: 3100).
- All API routes are prefixed with `/api/`.

---

### REQ-SCAFFOLD-009: Health Endpoint

**[EARS]** The system **shall** expose a `GET /api/health` endpoint that returns the application health status.

**Rationale:** Health endpoints are required for monitoring, load balancer probes, and verifying the application is running correctly.

**[EARS]** The health endpoint **shall** return:
- HTTP 200 with `{"status": "ok", "version": "<version>"}` when the application is healthy.
- HTTP 503 with `{"status": "unhealthy", "error": "<reason>"}` when a critical dependency (database) is unavailable.

**Acceptance Criteria:**
- `GET /api/health` returns 200 with JSON body when the server and database are operational.
- `GET /api/health` returns 503 when the database connection fails a ping check.
- The response includes the application version string.
- The response `Content-Type` is `application/json`.

---

### REQ-SCAFFOLD-010: Graceful Shutdown

**[EARS]** When the process receives SIGTERM or SIGINT, the system **shall** perform a graceful shutdown.

**Rationale:** Graceful shutdown prevents dropped requests and data corruption during deployments or restarts.

**[EARS]** The graceful shutdown sequence **shall**:
1. Stop accepting new HTTP connections.
2. Wait for in-flight requests to complete (up to a configurable timeout, default: 30 seconds).
3. Close the database connection pool.
4. Stop the embedded PostgreSQL instance (if running).
5. Exit with code 0.

**[EARS]** If in-flight requests do not complete within the shutdown timeout, the system **shall** force-close remaining connections and exit with code 0.

**Acceptance Criteria:**
- Sending SIGTERM triggers graceful shutdown (logged to stdout).
- Sending SIGINT (Ctrl+C) triggers graceful shutdown.
- In-flight requests complete before the server exits (within timeout).
- The shutdown timeout is configurable via `ARI_SHUTDOWN_TIMEOUT` (default: 30s).
- All resources (DB connections, embedded PG) are released before exit.

---

### REQ-SCAFFOLD-011: Configuration System

**[EARS]** The system **shall** load configuration from environment variables with an `ARI_` prefix, with sensible defaults for development.

**Rationale:** Environment variables are the standard mechanism for configuring containerized applications (12-factor app methodology).

**[EARS]** The system **shall** support the following configuration values:

| Variable | Default | Description |
|----------|---------|-------------|
| `ARI_PORT` | `3100` | HTTP server listen port |
| `ARI_HOST` | `0.0.0.0` | HTTP server listen address |
| `ARI_DATABASE_URL` | (empty, triggers embedded PG) | External PostgreSQL connection string |
| `ARI_DATA_DIR` | `./data` | Directory for embedded PG data and local storage |
| `ARI_LOG_LEVEL` | `info` | Logging verbosity: debug, info, warn, error |
| `ARI_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown deadline |
| `ARI_ENV` | `development` | Runtime environment: development, production |

**Acceptance Criteria:**
- All configuration values have sensible defaults that work for local development.
- Environment variables with the `ARI_` prefix override defaults.
- Invalid configuration values (e.g., non-numeric port) cause the application to exit with a descriptive error at startup.
- The configuration struct is defined in `internal/config/`.

---

### REQ-SCAFFOLD-012: Makefile Build System

**[EARS]** The system **shall** provide a Makefile with standard development commands.

**Rationale:** A Makefile provides a consistent interface for building, testing, and managing the project regardless of the developer's environment.

**[EARS]** The Makefile **shall** include the following targets:

| Target | Description |
|--------|-------------|
| `make dev` | Run the server in development mode (with hot-reload if feasible, otherwise plain `go run`) |
| `make build` | Compile the binary to `bin/ari` |
| `make test` | Run all Go tests with race detection enabled |
| `make sqlc` | Regenerate sqlc code from SQL queries |
| `make migrate-new NAME=<name>` | Create a new goose migration file |
| `make lint` | Run `go vet` and staticcheck (if available) |
| `make clean` | Remove build artifacts and data directory |

**Acceptance Criteria:**
- `make build` produces a statically-linked binary at `bin/ari`.
- `make test` runs all tests and reports results.
- `make sqlc` regenerates database code without errors.
- `make dev` starts the application in development mode.
- All targets are documented in the Makefile with comments.

---

### REQ-SCAFFOLD-013: Structured Logging

**[EARS]** The system **shall** use Go's standard library `log/slog` package for structured, leveled logging.

**Rationale:** `log/slog` (Go 1.21+) provides structured logging without third-party dependencies. Structured logs are parseable by log aggregation tools.

**Acceptance Criteria:**
- All application logs use `slog` with JSON output in production mode and text output in development mode.
- Log entries include timestamp, level, message, and relevant context fields.
- The log level is configurable via `ARI_LOG_LEVEL`.
- Startup and shutdown events are logged at `info` level.
- Errors are logged at `error` level with relevant context.

---

### REQ-SCAFFOLD-014: JSON Response Conventions

**[EARS]** The system **shall** return all API responses as JSON with consistent error formatting.

**Rationale:** Consistent response formats reduce client-side complexity and match the API design specified in the PRD (Section 6.1).

**[EARS]** Error responses **shall** follow the format:
```json
{
  "error": "Human-readable error message",
  "code": "MACHINE_READABLE_CODE"
}
```

**Acceptance Criteria:**
- All API responses include `Content-Type: application/json`.
- Success responses return appropriate HTTP status codes (200, 201, 204).
- Error responses use the standard error format with `error` and `code` fields.
- 404 responses for unknown routes return `{"error": "Not found", "code": "NOT_FOUND"}`.
- 405 responses for wrong HTTP methods return the standard error format.

---

### REQ-SCAFFOLD-015: Request/Response Middleware

**[EARS]** The system **shall** implement HTTP middleware for cross-cutting concerns.

**Rationale:** Middleware centralizes logging, recovery, and header management, keeping handler code focused on business logic.

**[EARS]** The following middleware **shall** be applied to all API routes:
1. **Request logging** — logs method, path, status code, and duration for every request.
2. **Panic recovery** — catches panics in handlers, logs the stack trace, and returns HTTP 500.
3. **CORS headers** — sets appropriate headers for local development (configurable origin).
4. **Content-Type** — sets `Content-Type: application/json` on all API responses.

**Acceptance Criteria:**
- Every HTTP request is logged with method, path, status, and duration.
- A panic in a handler does not crash the server; it returns 500 and logs the stack trace.
- CORS headers allow the React dev server origin (`http://localhost:5173`) in development mode.
- All `/api/` responses have `Content-Type: application/json`.

---

## Non-Functional Requirements

### REQ-NFR-001: Build Time

**[EARS]** The system **shall** compile from source in under 60 seconds on a standard development machine.

**Acceptance Criteria:**
- `make build` completes in under 60 seconds with a cold module cache.

---

### REQ-NFR-002: Startup Time

**[EARS]** The system **shall** be ready to accept HTTP requests within 10 seconds of `ari run` being executed (including embedded PostgreSQL startup and migrations).

**Acceptance Criteria:**
- From process start to first successful `GET /api/health` response is under 10 seconds.

---

### REQ-NFR-003: Binary Size

**[EARS]** The compiled binary (excluding embedded PostgreSQL assets) **should** be under 50 MB.

**Acceptance Criteria:**
- `ls -lh bin/ari` reports a size under 50 MB.

---

### REQ-NFR-004: Test Foundation

**[EARS]** The scaffold **shall** include at least one test per package to establish the testing pattern.

**Acceptance Criteria:**
- `make test` discovers and runs at least one test in each non-generated package.
- Tests pass with zero failures.
- Race detection is enabled (`-race` flag).

---

## Dependency Matrix

| Dependency | Version | Purpose |
|-----------|---------|---------|
| Go | 1.24+ | Language runtime |
| cobra | latest | CLI framework |
| embedded-postgres-go | latest | Development database |
| goose | v3 | Schema migrations |
| sqlc | latest | Query code generation |

## Traceability

| Requirement | PRD Section |
|-------------|-------------|
| REQ-SCAFFOLD-004 | 5.1.1 One-Command Setup |
| REQ-SCAFFOLD-005 | 5.1.1 One-Command Setup (embedded PG) |
| REQ-SCAFFOLD-008 | 6.1 Base URL (port 3100) |
| REQ-SCAFFOLD-009 | 6.1 Base URL (health check implied) |
| REQ-SCAFFOLD-011 | 5.1.1 One-Command Setup (zero-config) |
| REQ-SCAFFOLD-014 | 6.1 API response format |
