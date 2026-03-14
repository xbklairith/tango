# Tasks: Go Scaffold

**Created:** 2026-03-14
**Status:** Complete
**Feature:** 01-go-scaffold
**Methodology:** TDD Red-Green-Refactor
**Requirements:** [requirements.md](./requirements.md)
**Design:** [design.md](./design.md)

---

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- All REQ-SCAFFOLD-001 through REQ-SCAFFOLD-015 are covered
- All REQ-NFR-001 through REQ-NFR-004 are covered
- See [Traceability Matrix](#traceability-matrix) at the bottom for full mapping

## Implementation Approach

Components are built bottom-up following dependency order: module init, then configuration (leaf package), then database layer, then HTTP server, then CLI orchestration, and finally integration verification. Each task uses TDD with explicit RED/GREEN/REFACTOR phases. Tasks within the same group can occasionally be parallelized, but cross-group dependencies must be respected (see dependency graph at bottom).

## Progress Summary

- Total Tasks: 18
- Completed: 18/18
- In Progress: None
- Test Coverage: 0%

---

## Tasks (TDD: Red-Green-Refactor)

---

### Group 1: Go Module & Directory Structure

#### Task 1.1 — Initialize Go Module and Create Directory Layout

**Linked Requirements:** REQ-SCAFFOLD-001, REQ-SCAFFOLD-002

**RED Phase:**
- [x] Verify `go build ./...` fails (no Go source files exist yet)
- [x] Verify `go.mod` does not yet have the correct module path and Go version

**GREEN Phase:**
- [x] Run `go mod init github.com/xb/ari` at the project root
- [x] Set Go version to 1.24 in `go.mod`
- [x] Create all directories from the design
- [x] Add placeholder `.go` files with package declarations
- [x] Create/update `.gitignore`

**REFACTOR Phase:**
- [x] Verify `go build ./...` succeeds with no errors
- [x] Verify all directories exist and contain at least a placeholder file
- [x] Verify packages under `internal/` are not importable from outside the module

**Acceptance Criteria:**
- [x] `go.mod` exists with `module github.com/xb/ari` and `go 1.24`
- [x] `go.sum` will be created once dependencies are added
- [x] `go build ./...` completes without errors
- [x] All directories from REQ-SCAFFOLD-002 exist with package declarations
- [x] `.gitignore` covers build artifacts and runtime data

---

### Group 2: Configuration

#### Task 2.1 — Config Struct and Defaults

**Linked Requirements:** REQ-SCAFFOLD-011

**RED Phase:**
- [ ] Create `internal/config/config_test.go`
- [ ] Write `TestLoad_Defaults` — clear all `ARI_*` env vars, call `config.Load()`, assert:
  - `Port == 3100`
  - `Host == "0.0.0.0"`
  - `Env == "development"`
  - `DatabaseURL == ""`
  - `DataDir == "./data"`
  - `LogLevel == "info"`
  - `ShutdownTimeout == 30s`
  - `UseEmbeddedPostgres() == true`
  - `IsProduction() == false`
  - `Addr() == "0.0.0.0:3100"`
- [ ] Run `go test ./internal/config/` — expect compilation failure (no source files)

**GREEN Phase:**
- [ ] Create `internal/config/config.go` with the `Config` struct
- [ ] Implement helper methods: `IsProduction()`, `UseEmbeddedPostgres()`, `Addr()`
- [ ] Implement `Load()` function with default values
- [ ] Implement `envOrDefault()` helper for reading env vars with fallbacks
- [ ] Run tests — expect `TestLoad_Defaults` to pass

**REFACTOR Phase:**
- [ ] Ensure all struct fields have doc comments
- [ ] Verify no unused imports or variables
- [ ] Run `go vet ./internal/config/`

**Acceptance Criteria:**
- [ ] `config.Load()` returns correct defaults when no env vars are set
- [ ] `Config` helper methods return expected values
- [ ] Tests pass: `go test -race ./internal/config/`

---

#### Task 2.2 — Config Validation and Environment Variable Overrides

**Linked Requirements:** REQ-SCAFFOLD-011

**RED Phase:**
- [ ] Write `TestLoad_EnvOverrides` — set `ARI_PORT=8080`, `ARI_HOST=127.0.0.1`, `ARI_ENV=production`, `ARI_DATABASE_URL=postgres://user:pass@host:5432/db`, `ARI_DATA_DIR=/tmp/ari`, `ARI_LOG_LEVEL=debug`, `ARI_SHUTDOWN_TIMEOUT=10s`, assert all values are correctly parsed
- [ ] Write `TestLoad_InvalidPort` — set `ARI_PORT=not-a-number`, expect error
- [ ] Write `TestLoad_PortOutOfRange` — set `ARI_PORT=99999`, expect error
- [ ] Write `TestLoad_InvalidLogLevel` — set `ARI_LOG_LEVEL=verbose`, expect error
- [ ] Write `TestLoad_InvalidEnv` — set `ARI_ENV=staging`, expect error
- [ ] Write `TestLoad_InvalidShutdownTimeout` — set `ARI_SHUTDOWN_TIMEOUT=notaduration`, expect error
- [ ] Write `TestLoad_ExternalDatabase` — set `ARI_DATABASE_URL`, assert `UseEmbeddedPostgres() == false`
- [ ] Run tests — expect failures for new test cases

**GREEN Phase:**
- [ ] Implement port parsing from `ARI_PORT` with `strconv.Atoi` and range validation (1-65535)
- [ ] Implement `ARI_SHUTDOWN_TIMEOUT` parsing via `time.ParseDuration`
- [ ] Implement `ARI_LOG_LEVEL` validation (must be `debug`, `info`, `warn`, `error`)
- [ ] Implement `ARI_ENV` validation (must be `development`, `production`)
- [ ] Return descriptive `fmt.Errorf` messages for each validation failure
- [ ] Run tests — all pass

**REFACTOR Phase:**
- [ ] Verify error messages include the invalid value and accepted range/options
- [ ] Ensure `t.Setenv()` is used in tests (auto-restores after test)
- [ ] Add a `clearEnv` test helper that unsets all `ARI_*` variables
- [ ] Run `go vet ./internal/config/`

**Acceptance Criteria:**
- [ ] All `ARI_*` env vars override defaults correctly
- [ ] Invalid `ARI_PORT` (non-numeric or out of range 1-65535) returns descriptive error
- [ ] Invalid `ARI_LOG_LEVEL` returns descriptive error listing valid options
- [ ] Invalid `ARI_ENV` returns descriptive error listing valid options
- [ ] Invalid `ARI_SHUTDOWN_TIMEOUT` returns descriptive error
- [ ] `go test -race ./internal/config/` passes

---

### Group 3: Embedded PostgreSQL

#### Task 3.1 — Database Open with Embedded PostgreSQL

**Linked Requirements:** REQ-SCAFFOLD-005

**RED Phase:**
- [ ] Create `internal/database/database_test.go`
- [ ] Write `TestOpen_EmbeddedPostgres`:
  - Create a config with `UseEmbeddedPostgres() == true` and a temp `DataDir` via `t.TempDir()`
  - Call `database.Open(ctx, cfg)`
  - Assert no error returned
  - Assert `*sql.DB` is not nil
  - Assert `db.PingContext(ctx)` succeeds
  - Assert cleanup function is not nil
  - Call cleanup, assert subsequent ping fails or resources are released
- [ ] Run test — expect compilation failure

**GREEN Phase:**
- [ ] Create `internal/database/database.go` with `Open(ctx context.Context, cfg *config.Config) (*sql.DB, func(), error)`
- [ ] Implement embedded PG startup path:
  - Pin a specific PostgreSQL version via `embeddedpostgres.DefaultConfig().Version(embeddedpostgres.V16)` — do NOT use the library default (it may lack arm64 binaries)
  - Create `embeddedpostgres.NewDatabase()` with `Version`, `DataPath`, `Port(5433)`, and logger
  - Call `epg.Start()`; on failure, return an error that includes: PG version, `runtime.GOOS`/`runtime.GOARCH`, the underlying error, and actionable troubleshooting steps (check network, use `ARI_DATABASE_URL` instead, clear cached downloads)
  - Build DSN: `host=localhost port=5433 user=postgres password=postgres dbname=postgres sslmode=disable`
- [ ] Implement external DB path: use `cfg.DatabaseURL` as DSN directly
- [ ] Open `sql.DB` via `sql.Open("postgres", dsn)`
- [ ] Configure connection pool: `SetMaxOpenConns(25)`, `SetMaxIdleConns(5)`
- [ ] Ping to verify connectivity
- [ ] Build cleanup function that closes the pool and stops embedded PG (in that order)
- [ ] Add `github.com/lib/pq` and `github.com/fergusstrange/embedded-postgres` to `go.mod`
- [ ] Run `go mod tidy`
- [ ] Run test — passes

**REFACTOR Phase:**
- [ ] Ensure cleanup is called on all error paths (if PG started but ping fails, cleanup stops PG)
- [ ] Verify logging calls use `slog` with appropriate context fields (`data_dir`, `port`)
- [ ] Verify the original cleanup is wrapped to also close the DB pool

**Acceptance Criteria:**
- [ ] `database.Open()` starts embedded PG when `UseEmbeddedPostgres()` is true
- [ ] `database.Open()` uses `DatabaseURL` directly when set
- [ ] Cleanup function stops embedded PG and closes the connection pool
- [ ] Connection pool is configured with `MaxOpenConns=25` and `MaxIdleConns=5`
- [ ] `db.PingContext()` succeeds after Open
- [ ] Embedded PG version is explicitly pinned (e.g., `embeddedpostgres.V16`), not using library default
- [ ] If embedded PG fails to start/download, error message includes: PG version, OS/arch, and troubleshooting steps
- [ ] Test passes: `go test -race -count=1 ./internal/database/`

**Notes:**
- This test requires network I/O and takes ~5-8 seconds (embedded PG startup). Use `if testing.Short() { t.Skip("skipping embedded PG test") }` for fast test runs.
- Embedded PG uses port 5433 to avoid conflicts with any system PostgreSQL on 5432.
- The pinned PG version must be tested on both amd64 and arm64/macOS (Apple Silicon). If CI only covers one architecture, add a manual verification step for the other.

---

### Group 4: Database Migrations

#### Task 4.1 — Goose Migration Runner with Embedded SQL

**Linked Requirements:** REQ-SCAFFOLD-006

**RED Phase:**
- [ ] Create `internal/database/migrations/20260314000001_init.sql`:
  ```sql
  -- +goose Up
  CREATE EXTENSION IF NOT EXISTS "pgcrypto";

  -- +goose Down
  DROP EXTENSION IF EXISTS "pgcrypto";
  ```
- [ ] Write `TestMigrate_AppliesPendingMigrations` in `database_test.go`:
  - Open embedded PG via `database.Open()`
  - Call `database.Migrate(ctx, db)`
  - Assert no error
  - Query `goose_db_version` table to confirm migration version > 0
  - Clean up
- [ ] Run test — expect compilation failure

**GREEN Phase:**
- [ ] Create `internal/database/embed.go` with `//go:embed migrations/*.sql` directive exposing `migrationsFS`
- [ ] Create `internal/database/migrate.go` with `Migrate(ctx context.Context, db *sql.DB) error`
- [ ] Set `goose.SetBaseFS(migrationsFS)` and `goose.SetDialect("postgres")`
- [ ] Call `goose.UpContext(ctx, db, "migrations")` to apply pending migrations
- [ ] Log migration version after completion via `goose.GetDBVersionContext()`
- [ ] Add `github.com/pressly/goose/v3` to `go.mod`
- [ ] Run `go mod tidy`
- [ ] Run test — passes

**REFACTOR Phase:**
- [ ] Wrap all goose errors with context: `fmt.Errorf("setting goose dialect: %w", err)`, `fmt.Errorf("running migrations: %w", err)`
- [ ] Verify migration file follows goose naming convention: `YYYYMMDDHHMMSS_description.sql`
- [ ] Verify embedded FS correctly includes the `.sql` files

**Acceptance Criteria:**
- [ ] `database.Migrate()` applies all pending SQL migrations
- [ ] Migration files are embedded via `go:embed`
- [ ] Initial migration creates the `pgcrypto` extension
- [ ] Goose dialect is set to `postgres`
- [ ] Migration failure returns a descriptive wrapped error
- [ ] Test passes: `go test -race -count=1 ./internal/database/`

---

#### Task 4.2 — sqlc Configuration and Placeholder Query

**Linked Requirements:** REQ-SCAFFOLD-007

**RED Phase:**
- [ ] Verify `sqlc generate` fails (no config file exists)

**GREEN Phase:**
- [ ] Create `sqlc.yaml` at project root:
  ```yaml
  version: "2"
  sql:
    - engine: "postgresql"
      queries: "internal/database/queries/"
      schema: "internal/database/migrations/"
      gen:
        go:
          package: "db"
          out: "internal/database/db"
          sql_package: "database/sql"
          emit_json_tags: true
          emit_empty_slices: true
  ```
- [ ] Create `internal/database/queries/health.sql`:
  ```sql
  -- name: Ping :exec
  SELECT 1;
  ```
- [ ] Run `sqlc generate` — verify it produces Go files in `internal/database/db/`

**REFACTOR Phase:**
- [ ] Verify generated code compiles: `go build ./internal/database/db/`
- [ ] Commit generated code (not gitignored, per REQ-SCAFFOLD-007)
- [ ] Verify `go vet ./internal/database/db/` passes

**Acceptance Criteria:**
- [ ] `sqlc.yaml` exists and is valid
- [ ] `sqlc generate` produces Go code in `internal/database/db/` without errors
- [ ] Generated code compiles successfully
- [ ] Placeholder `health.sql` query exists in `internal/database/queries/`
- [ ] Generated code is committed to the repository

**Notes:**
- sqlc must be installed as a build tool (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).

---

### Group 5: HTTP Server

#### Task 5.1 — Server Struct and JSON Response Helpers

**Linked Requirements:** REQ-SCAFFOLD-008, REQ-SCAFFOLD-014

**RED Phase:**
- [ ] Create `internal/server/server_test.go`
- [ ] Write `TestWriteJSON_Success` — call `writeJSON` with an `httptest.ResponseRecorder` and a test struct, assert:
  - Response body is valid JSON matching the struct
  - `Content-Type` header is `application/json`
  - Status code matches the provided code
- [ ] Write `TestWriteJSON_ErrorResponse` — call `writeJSON` with `ErrorResponse{Error: "test", Code: "TEST"}`, assert JSON body has `"error":"test"` and `"code":"TEST"` fields
- [ ] Run tests — expect compilation failure

**GREEN Phase:**
- [ ] Create `internal/server/response.go`:
  - `ErrorResponse` struct with `Error string` and `Code string` JSON-tagged fields
  - `writeJSON(w http.ResponseWriter, status int, data any)` function
- [ ] `writeJSON` sets `Content-Type: application/json`, writes the status code, encodes data as JSON via `json.NewEncoder`
- [ ] Create `internal/server/server.go` with the `Server` struct: `cfg *config.Config`, `db *sql.DB`, `version string`, `http *http.Server`
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Ensure `writeJSON` logs encoding errors via `slog.Error`
- [ ] Verify `ErrorResponse` JSON tags match the API spec: `json:"error"` and `json:"code"`
- [ ] Run `go vet ./internal/server/`

**Acceptance Criteria:**
- [ ] `writeJSON` produces valid JSON with correct `Content-Type` and status code
- [ ] `ErrorResponse` serializes to `{"error":"...","code":"..."}`
- [ ] `Server` struct compiles with all required fields
- [ ] Tests pass: `go test -race ./internal/server/`

---

#### Task 5.2 — Middleware Chain (Logging, Recovery, CORS, Content-Type)

**Linked Requirements:** REQ-SCAFFOLD-015

**RED Phase:**
- [ ] Write `TestRecoverMiddleware_CatchesPanic` — create a handler that panics, wrap with recovery middleware, send a request via `httptest`, assert:
  - Response status is 500
  - Response body contains `{"error":"Internal server error","code":"INTERNAL_ERROR"}`
  - Server does not crash
- [ ] Write `TestCorsMiddleware_SetsHeaders` — send a request through CORS middleware (dev config), assert `Access-Control-Allow-Origin` is `http://localhost:5173`
- [ ] Write `TestCorsMiddleware_HandlesPreflight` — send an OPTIONS request, assert 204 status and all CORS headers present (`Allow-Origin`, `Allow-Methods`, `Allow-Headers`, `Max-Age`)
- [ ] Write `TestContentTypeMiddleware_SetsJSON` — send a request to `/api/test`, assert `Content-Type: application/json`
- [ ] Write `TestContentTypeMiddleware_SkipsNonAPI` — send a request to `/other`, assert `Content-Type` is NOT `application/json`
- [ ] Write `TestLoggingMiddleware_CapturesStatus` — send a request through logging middleware with a handler that writes 201, verify the `responseWriter` captured status 201
- [ ] Run tests — expect compilation failure

**GREEN Phase:**
- [ ] Create `internal/server/middleware.go`
- [ ] Implement `responseWriter` wrapper that captures status code via `WriteHeader` override
- [ ] Implement `loggingMiddleware` — records start time, wraps writer, logs method/path/status/duration via `slog.Info`
- [ ] Implement `recoverMiddleware` — defers `recover()`, logs stack trace via `slog.Error`, returns 500 with `ErrorResponse`
- [ ] Implement `corsMiddleware` — sets CORS headers (`Allow-Origin`, `Allow-Methods`, `Allow-Headers`, `Max-Age`), handles OPTIONS preflight with 204
- [ ] Implement `contentTypeMiddleware` — sets `Content-Type: application/json` for `/api/` prefixed paths
- [ ] Implement `middleware()` method that chains: logging -> contentType -> cors -> recover -> handler
- [ ] Run tests — all pass

**REFACTOR Phase:**
- [ ] Verify middleware order matches the design (outermost runs first)
- [ ] Ensure `responseWriter.WriteHeader` correctly delegates only once
- [ ] Ensure `newResponseWriter` defaults status to 200
- [ ] Run `go vet ./internal/server/`

**Acceptance Criteria:**
- [ ] Panics in handlers return 500 with `{"error":"Internal server error","code":"INTERNAL_ERROR"}`; server stays up
- [ ] CORS headers allow `http://localhost:5173` in dev mode
- [ ] CORS headers use server's own address in production mode
- [ ] OPTIONS preflight returns 204 with CORS headers
- [ ] All `/api/` responses get `Content-Type: application/json`
- [ ] Non-API paths do not get forced `Content-Type`
- [ ] Every request is logged with method, path, status, and duration
- [ ] Tests pass: `go test -race ./internal/server/`

---

#### Task 5.3 — Route Registration and Not-Found Handler

**Linked Requirements:** REQ-SCAFFOLD-008, REQ-SCAFFOLD-014

**RED Phase:**
- [ ] Write `TestHandleNotFound_ReturnsJSON404` — create a server, send `GET /api/unknown` via `httptest`, assert:
  - Status 404
  - Body: `{"error":"Not found","code":"NOT_FOUND"}`
  - `Content-Type: application/json`
- [ ] Run test — expect compilation failure

**GREEN Phase:**
- [ ] Create `internal/server/routes.go` with `registerRoutes(mux *http.ServeMux)`
- [ ] Register `"GET /api/health"` route (handler implemented in next task, stub for now)
- [ ] Register `/api/` catch-all route pointing to `handleNotFound`
- [ ] Create `internal/server/handlers.go` with `handleNotFound(w, r)`:
  - Call `writeJSON(w, 404, ErrorResponse{Error: "Not found", Code: "NOT_FOUND"})`
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Verify route patterns use Go 1.22+ method-based routing syntax (e.g., `"GET /api/health"`)
- [ ] Verify catch-all `/api/` correctly matches unregistered sub-paths
- [ ] Run `go vet ./internal/server/`

**Acceptance Criteria:**
- [ ] Unknown `/api/*` routes return 404 with `{"error":"Not found","code":"NOT_FOUND"}`
- [ ] Routes are registered on `http.ServeMux`
- [ ] Route registration uses Go 1.22+ method-based syntax
- [ ] Tests pass: `go test -race ./internal/server/`

---

### Group 6: Health Endpoint

#### Task 6.1 — Health Handler with Database Ping

**Linked Requirements:** REQ-SCAFFOLD-009

**RED Phase:**
- [ ] Write `TestHandleHealth_Healthy` — create a server with a real or mock `*sql.DB` that responds to ping, send `GET /api/health` via `httptest`, assert:
  - Status 200
  - Body: `{"status":"ok","version":"test"}`
  - `Content-Type: application/json`
- [ ] Write `TestHandleHealth_Unhealthy` — create a server with a closed `*sql.DB` (call `db.Close()` before the request), send `GET /api/health`, assert:
  - Status 503
  - Body contains `"status":"unhealthy"` and `"error"`
- [ ] Run tests — expect failures

**GREEN Phase:**
- [ ] Implement `handleHealth(w, r)` in `handlers.go`:
  - Call `s.db.PingContext(r.Context())`
  - On success: `writeJSON(w, 200, HealthResponse{Status: "ok", Version: s.version})`
  - On failure: `writeJSON(w, 503, HealthErrorResponse{Status: "unhealthy", Error: "database ping failed"})`
- [ ] Define `HealthResponse` struct: `Status string`, `Version string`
- [ ] Define `HealthErrorResponse` struct: `Status string`, `Error string`
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Verify response structs use correct JSON tags: `json:"status"`, `json:"version"`, `json:"error"`
- [ ] Ensure the health handler does not leak internal error details (only returns "database ping failed", not the actual DB error)
- [ ] Run `go vet ./internal/server/`

**Acceptance Criteria:**
- [ ] `GET /api/health` returns 200 with `{"status":"ok","version":"<version>"}` when DB is reachable
- [ ] `GET /api/health` returns 503 with `{"status":"unhealthy","error":"database ping failed"}` when DB is unreachable
- [ ] Response `Content-Type` is `application/json`
- [ ] Version string in response matches what was passed to the server
- [ ] Tests pass: `go test -race ./internal/server/`

---

#### Task 6.2 — Server Constructor and ListenAndServe with Graceful Shutdown

**Linked Requirements:** REQ-SCAFFOLD-008

**RED Phase:**
- [ ] Write `TestNew_ConfiguresTimeouts` — create a server via `New()`, assert (via reflection or exported fields):
  - Read timeout is 15s
  - Write timeout is 30s
  - Idle timeout is 60s
  - Addr matches `cfg.Addr()`
- [ ] Write `TestListenAndServe_StartsAndStops` — start the server in a goroutine on a random port, send a request to verify it is listening, cancel the context, verify the function returns without error
- [ ] Run tests — expect failures

**GREEN Phase:**
- [ ] Implement `New(cfg *config.Config, db *sql.DB, version string) *Server`:
  - Create `http.NewServeMux()`, call `s.registerRoutes(mux)`, wrap with `s.middleware(mux)`
  - Configure `http.Server` with `Addr: cfg.Addr()`, `ReadTimeout: 15s`, `WriteTimeout: 30s`, `IdleTimeout: 60s`
- [ ] Implement `ListenAndServe(ctx context.Context) error`:
  - Start `s.http.ListenAndServe()` in a goroutine, send errors to a buffered channel
  - Wait for context cancellation or server error via `select`
  - On cancellation: call `s.http.Shutdown()` with `cfg.ShutdownTimeout` deadline
  - Log startup and shutdown events via `slog.Info`
  - Ignore `http.ErrServerClosed` (normal shutdown)
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Verify the error channel is correctly sized (buffered with cap 1) and closed
- [ ] Ensure `http.ErrServerClosed` is filtered out and not returned
- [ ] Verify shutdown context is created with `context.WithTimeout(context.Background(), ...)`
- [ ] Run `go vet ./internal/server/`

**Acceptance Criteria:**
- [ ] `server.New()` configures HTTP server with read=15s, write=30s, idle=60s timeouts
- [ ] Server listens on `cfg.Addr()`
- [ ] `ListenAndServe()` starts the server and blocks until context cancellation
- [ ] Context cancellation triggers graceful shutdown with configurable timeout
- [ ] `http.ErrServerClosed` is handled silently
- [ ] Tests pass: `go test -race ./internal/server/`

---

### Group 7: CLI (Cobra)

#### Task 7.1 — Root Command and Version Command

**Linked Requirements:** REQ-SCAFFOLD-003

**RED Phase:**
- [ ] Create `cmd/ari/root_test.go`
- [ ] Write `TestRootCmd_Help` — create root command via `newRootCmd("test")`, execute with `--help` flag, assert output contains "Ari" and "Control Plane"
- [ ] Write `TestVersionCmd_PrintsVersion` — create root command, execute `["version"]` args, assert output contains `ari version test`
- [ ] Run tests — expect compilation failure

**GREEN Phase:**
- [ ] Create `cmd/ari/main.go`:
  - `var version = "dev"` (set via ldflags at build time)
  - `func main()` calls `newRootCmd(version).Execute()`, exits with code 1 on error
- [ ] Create `cmd/ari/root.go` with `newRootCmd(version string) *cobra.Command`:
  - `Use: "ari"`, `Short: "Ari — The Control Plane for AI Agents"`, `Long: "Deploy, govern, and share AI agent workforces."`
  - `SilenceUsage: true`, `SilenceErrors: true`
  - Register `newRunCmd(version)` and `newVersionCmd(version)` as subcommands
- [ ] Create `cmd/ari/version.go` with `newVersionCmd(version string) *cobra.Command`:
  - `Use: "version"`, `Short: "Print the Ari version"`
  - `Run` prints `ari version <version>` via `fmt.Printf`
- [ ] Add `github.com/spf13/cobra` to `go.mod`, run `go mod tidy`
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Verify command descriptions are clear and match the design
- [ ] Ensure `SilenceUsage` and `SilenceErrors` are set to prevent Cobra from double-printing errors
- [ ] Run `go vet ./cmd/ari/`

**Acceptance Criteria:**
- [ ] `ari --help` displays usage information including "Ari" and "Control Plane"
- [ ] `ari version` prints `ari version dev` (or the injected version string)
- [ ] Root command has `SilenceUsage: true` and `SilenceErrors: true`
- [ ] Tests pass: `go test -race ./cmd/ari/`

---

#### Task 7.2 — Run Command (Startup Orchestration)

**Linked Requirements:** REQ-SCAFFOLD-004

**RED Phase:**
- [ ] Write `TestRunCmd_HasPortFlag` — create the run command via `newRunCmd("test")`, verify the `--port` flag is registered
- [ ] Write `TestRunCmd_FailsOnBadConfig` — set `ARI_PORT=invalid`, execute run command, assert error is returned
- [ ] Run tests — expect compilation failure

**GREEN Phase:**
- [ ] Create `cmd/ari/run.go` with `newRunCmd(version string) *cobra.Command`:
  - `Use: "run"`, `Short: "Start the Ari server"`, `Long: "Start the full Ari stack: database, migrations, and HTTP server."`
  - Register `--port` flag (int, default 0, description "HTTP server port (overrides ARI_PORT)")
  - `RunE` calls `runServer(cmd.Context(), version)`
- [ ] Implement `runServer(ctx context.Context, version string) error`:
  1. `cfg, err := config.Load()` — return `fmt.Errorf("loading config: %w", err)` on failure
  2. `setupLogger(cfg.LogLevel, cfg.Env)` — initialize slog
  3. `slog.Info("starting ari", "version", version, "env", cfg.Env)` — log startup
  4. `db, cleanup, err := database.Open(ctx, cfg)` — return `fmt.Errorf("opening database: %w", err)` on failure; `defer cleanup()`
  5. `database.Migrate(ctx, db)` — return `fmt.Errorf("running migrations: %w", err)` on failure
  6. `srv := server.New(cfg, db, version)` — create HTTP server
  7. `ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` — register signal handlers; `defer stop()`
  8. `return srv.ListenAndServe(ctx)` — block until shutdown
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Ensure all errors are wrapped with descriptive context
- [ ] Verify `defer cleanup()` is called for database resources
- [ ] Verify `defer stop()` is called for signal context
- [ ] Run `go vet ./cmd/ari/`

**Acceptance Criteria:**
- [ ] `ari run` orchestrates the full startup sequence: config -> logger -> DB -> migrations -> HTTP server
- [ ] `--port` flag is available on the run command
- [ ] If any startup step fails, a descriptive wrapped error is returned
- [ ] Each startup step is logged at info level
- [ ] Database resources are cleaned up on exit via defer
- [ ] Tests pass: `go test -race ./cmd/ari/`

---

#### Task 7.3 — Structured Logging Setup

**Linked Requirements:** REQ-SCAFFOLD-013

**RED Phase:**
- [ ] Create `cmd/ari/logger_test.go`
- [ ] Write `TestSetupLogger_DevUsesTextHandler` — call `setupLogger("info", "development")`, capture log output by writing to a buffer, verify output is in text format (contains `level=INFO`, not JSON)
- [ ] Write `TestSetupLogger_ProdUsesJSONHandler` — call `setupLogger("info", "production")`, verify output is in JSON format (valid JSON with `"level"` key)
- [ ] Write `TestSetupLogger_RespectsLevel` — call `setupLogger("error", "development")`, emit a debug-level message, verify it is NOT present in output; emit an error-level message, verify it IS present
- [ ] Run tests — expect compilation failure

**GREEN Phase:**
- [ ] Create `cmd/ari/logger.go` with `setupLogger(level string, env string)`
- [ ] Parse log level string to `slog.Level`: `"debug"` -> `LevelDebug`, `"warn"` -> `LevelWarn`, `"error"` -> `LevelError`, default -> `LevelInfo`
- [ ] Create `slog.TextHandler` for development, `slog.JSONHandler` for production, both writing to `os.Stdout`
- [ ] Set as the default logger via `slog.SetDefault(slog.New(handler))`
- [ ] Run tests — pass

**REFACTOR Phase:**
- [ ] Ensure unknown log levels default to `slog.LevelInfo` without error
- [ ] Verify handler options use `&slog.HandlerOptions{Level: lvl}`
- [ ] Run `go vet ./cmd/ari/`

**Acceptance Criteria:**
- [ ] Development mode uses `slog.TextHandler` output
- [ ] Production mode uses `slog.JSONHandler` output
- [ ] Log level is respected (lower severity messages are suppressed)
- [ ] Logger is set as the global default via `slog.SetDefault`
- [ ] Tests pass: `go test -race ./cmd/ari/`

---

### Group 8: Graceful Shutdown

#### Task 8.1 — Signal Handling and Ordered Cleanup Verification

**Linked Requirements:** REQ-SCAFFOLD-010

**RED Phase:**
- [x] Write `TestGracefulShutdown_OrderedCleanup` (in `internal/server/server_test.go` or `cmd/ari/`):
  - Start the full server with a cancellable context on a random port
  - Send `GET /api/health` to confirm it is running and returns 200
  - Cancel the context (simulates SIGTERM)
  - Assert `ListenAndServe` returns without error (nil)
  - Assert the server no longer accepts connections (subsequent request fails)
- [x] Write `TestGracefulShutdown_DrainsInFlight`:
  - Register a test handler that sleeps for 500ms before responding
  - Start the server, begin the slow request in a goroutine
  - Cancel the context 100ms after the request starts
  - Assert the slow request completes successfully (receives 200, not connection reset)
  - Assert the server shuts down after the request completes
- [x] Run tests — both pass

**GREEN Phase:**
- [x] The `ListenAndServe` implementation from Task 6.2 handles graceful shutdown via `http.Server.Shutdown()`
- [x] Verify `Shutdown()` uses `context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)` to enforce the deadline
- [x] The `runServer` function ensures cleanup order via defer stack: HTTP shutdown returns first, then `cleanup()` runs (DB close, then PG stop)
- [x] Run tests — pass

**REFACTOR Phase:**
- [x] Verify shutdown timeout is configurable via `ARI_SHUTDOWN_TIMEOUT` (change config, verify behavior)
- [x] Verify logging at each shutdown step: "shutting down http server", "closing database connection pool", "stopping embedded postgresql"
- [x] Add a note in test about double-signal behavior (second signal kills process, handled by `signal.NotifyContext`)
- [x] Run `go vet ./...`

**Acceptance Criteria:**
- [ ] Context cancellation (simulating SIGTERM/SIGINT) triggers graceful shutdown
- [ ] In-flight requests complete within the shutdown timeout before server exits
- [ ] HTTP server stops accepting new connections immediately on shutdown signal
- [ ] Resources are released in correct order: HTTP drain -> DB pool close -> embedded PG stop
- [ ] Shutdown timeout is configurable (default: 30s)
- [ ] Process returns nil error on clean shutdown
- [ ] Tests pass: `go test -race ./...`

---

### Group 9: Makefile

#### Task 9.1 — Makefile with All Required Targets

**Linked Requirements:** REQ-SCAFFOLD-012

**RED Phase:**
- [ ] Verify `make build` fails (no Makefile or incomplete Makefile)
- [ ] Verify `make test` fails

**GREEN Phase:**
- [ ] Create `Makefile` at project root with all targets:
  ```makefile
  BINARY     := bin/ari
  MODULE     := github.com/xb/ari
  VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
  LDFLAGS    := -ldflags "-s -w -X main.version=$(VERSION)"
  GO         := go

  .PHONY: all build dev test lint sqlc migrate-new ui-dev ui-build clean help

  ## all: Build the binary (default target)
  all: build

  ## build: Compile the binary to bin/ari
  build:
  	$(GO) build $(LDFLAGS) -o $(BINARY) ./cmd/ari

  ## dev: Run the server in development mode
  dev:
  	ARI_ENV=development $(GO) run $(LDFLAGS) ./cmd/ari run

  ## test: Run all Go tests with race detection
  test:
  	$(GO) test -race -count=1 ./...

  ## lint: Run go vet and staticcheck
  lint:
  	$(GO) vet ./...
  	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

  ## sqlc: Regenerate sqlc code from SQL queries
  sqlc:
  	sqlc generate

  ## migrate-new: Create a new goose migration (usage: make migrate-new NAME=create_users)
  migrate-new:
  	@test -n "$(NAME)" || (echo "Usage: make migrate-new NAME=description" && exit 1)
  	goose -dir internal/database/migrations create $(NAME) sql

  ## ui-dev: Run the frontend dev server
  ui-dev:
  	cd web && npm run dev

  ## ui-build: Build the frontend for production
  ui-build:
  	cd web && npm run build

  ## clean: Remove build artifacts and data directory
  clean:
  	rm -rf bin/ data/

  ## help: Show this help message
  help:
  	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
  ```
- [ ] Verify tabs are used for Makefile recipe indentation (not spaces)

**REFACTOR Phase:**
- [ ] Run `make build` — verify binary produced at `bin/ari`
- [ ] Run `make test` — verify all tests pass
- [ ] Run `make clean` — verify `bin/` and `data/` are removed
- [ ] Run `make help` — verify help output lists all targets with descriptions
- [ ] Run `make lint` — verify no `go vet` issues
- [ ] Run `bin/ari version` — verify version output

**Acceptance Criteria:**
- [ ] `make build` produces a binary at `bin/ari` with version injected via ldflags
- [ ] `make test` runs all Go tests with `-race -count=1` flags
- [ ] `make dev` starts the server in development mode via `go run`
- [ ] `make sqlc` runs `sqlc generate`
- [ ] `make migrate-new NAME=foo` creates a new migration file (requires goose installed)
- [ ] `make lint` runs `go vet` (and `staticcheck` if available)
- [ ] `make clean` removes `bin/` and `data/`
- [ ] `make help` lists all targets with descriptions
- [ ] All targets have `.PHONY` declarations
- [ ] All targets are documented with `##` comments

---

### Group 10: Integration

#### Task 10.1 — Full `ari run` Startup/Shutdown Integration Test

**Linked Requirements:** REQ-SCAFFOLD-004, REQ-SCAFFOLD-005, REQ-SCAFFOLD-006, REQ-SCAFFOLD-009, REQ-SCAFFOLD-010, REQ-NFR-002, REQ-NFR-004

**RED Phase:**
- [ ] Create integration test file (e.g., `cmd/ari/integration_test.go`)
- [ ] Write `TestIntegration_FullLifecycle`:
  - Set `ARI_PORT` to a random available port (use `net.Listen` to find one, then close it)
  - Set `ARI_DATA_DIR` to `t.TempDir()`
  - Clear `ARI_DATABASE_URL` to trigger embedded PG mode
  - Launch `runServer(ctx, "test-version")` in a goroutine with a cancellable context
  - Wait for server ready: poll `GET http://localhost:<port>/api/health` with retries, timeout after 10s (REQ-NFR-002)
  - Assert health response is 200 with `{"status":"ok","version":"test-version"}`
  - Cancel the context
  - Assert the goroutine returns nil error (clean shutdown)
- [ ] Write `TestIntegration_HealthEndpoint_FullStack`:
  - Same server setup
  - Send `GET /api/health`, verify 200 with valid JSON
  - Verify `Content-Type: application/json` header
- [ ] Write `TestIntegration_UnknownRoute_Returns404`:
  - Same server setup
  - Send `GET /api/nonexistent`
  - Assert 404 with `{"error":"Not found","code":"NOT_FOUND"}`
- [ ] Run tests — expect failures until all components are wired together

**GREEN Phase:**
- [ ] Ensure all components from Groups 1-8 are properly integrated and compile
- [ ] Wire `runServer()` to call config -> logger -> database -> migration -> server in sequence
- [ ] Run tests — all pass

**REFACTOR Phase:**
- [ ] Add `if testing.Short() { t.Skip("skipping integration test (requires embedded PG)") }` at the top of each integration test
- [ ] Ensure temp directories are cleaned up via `t.TempDir()` (automatic)
- [ ] Measure and log startup-to-healthy time; verify it is under 10 seconds (REQ-NFR-002)
- [ ] Extract a `startTestServer(t) (baseURL string, cancel func())` helper to reduce duplication across integration tests
- [ ] Run `go vet ./...`

**Acceptance Criteria:**
- [ ] Full `ari run` lifecycle works end-to-end: start embedded PG, run migrations, start HTTP server, serve health endpoint, shut down cleanly
- [ ] `GET /api/health` returns 200 with `{"status":"ok","version":"..."}` during operation
- [ ] Unknown API routes return 404 with `{"error":"Not found","code":"NOT_FOUND"}`
- [ ] Startup to first healthy response is under 10 seconds (REQ-NFR-002)
- [ ] Graceful shutdown completes without errors
- [ ] All tests pass: `go test -race -count=1 ./...`

---

#### Task 10.2 — Build Verification and Non-Functional Checks

**Linked Requirements:** REQ-SCAFFOLD-001, REQ-SCAFFOLD-012, REQ-NFR-001, REQ-NFR-003, REQ-NFR-004

**RED Phase:**
- [ ] Verify `make build` produces `bin/ari`
- [ ] Verify `bin/ari version` prints the version string
- [ ] Verify `bin/ari --help` shows usage information

**GREEN Phase:**
- [ ] Run `make build` and confirm binary at `bin/ari`
- [ ] Run `bin/ari version` — assert output contains "ari version"
- [ ] Run `bin/ari --help` — assert output contains "Ari" and "Control Plane"
- [ ] Check binary size: `ls -lh bin/ari` — verify under 50 MB (REQ-NFR-003)
- [ ] Time the build: `time make build` — verify under 60 seconds (REQ-NFR-001)
- [ ] Run `make test` — verify all tests pass, each non-generated package has at least one test (REQ-NFR-004)

**REFACTOR Phase:**
- [ ] Verify version is injected via ldflags (build on a tagged commit shows the tag, not "dev")
- [ ] Verify binary is stripped (`-s -w` ldflags reduce size)
- [ ] Run `make lint` — verify clean output

**Acceptance Criteria:**
- [ ] `make build` produces `bin/ari` in under 60 seconds (REQ-NFR-001)
- [ ] Binary size is under 50 MB (REQ-NFR-003)
- [ ] `bin/ari version` prints the injected version
- [ ] `bin/ari --help` displays correct usage information
- [ ] `make test` discovers and runs at least one test per non-generated package (REQ-NFR-004)
- [ ] All tests pass with zero failures and race detection enabled

---

### Final Verification

#### Task 10.3 — Pre-Merge Checklist

**Final Checks:**

- [ ] All 18 tasks above completed
- [ ] All tests passing: `make test`
- [ ] No linter errors: `make lint`
- [ ] No type errors: `go vet ./...`
- [ ] `go build ./...` succeeds
- [ ] Test coverage reviewed (at least one test per non-generated package)
- [ ] No debug code or `fmt.Println` left in source
- [ ] No commented-out code
- [ ] All environment variables documented in `internal/config/config.go`
- [ ] Database migration tested (initial migration applies cleanly)
- [ ] `.gitignore` covers all generated/runtime artifacts
- [ ] `sqlc.yaml` is valid and `make sqlc` succeeds
- [ ] Binary runs: `make build && bin/ari version`
- [ ] All requirement IDs covered (see traceability matrix)

**Acceptance Criteria:**
- [ ] Feature is production-ready for Phase 1
- [ ] All quality gates passed
- [ ] Ready for PR/merge

---

## Task Dependency Graph

```
Task 1.1 (Module & Dirs)
  │
  ├──► Task 2.1 (Config Defaults)
  │      │
  │      └──► Task 2.2 (Config Validation)
  │             │
  │             ├──► Task 3.1 (Embedded PG)
  │             │      │
  │             │      └──► Task 4.1 (Migrations)
  │             │             │
  │             │             └──► Task 4.2 (sqlc Config)
  │             │
  │             └──► Task 5.1 (Server Struct + JSON Helpers)
  │                    │
  │                    ├──► Task 5.2 (Middleware)
  │                    │
  │                    └──► Task 5.3 (Routes + Not-Found)
  │                           │
  │                           └──► Task 6.1 (Health Handler)
  │                                  │
  │                                  └──► Task 6.2 (ListenAndServe)
  │
  ├──► Task 7.1 (Root + Version Cmd)
  │      │
  │      └──► Task 7.2 (Run Cmd) ◄── requires 3.1 + 4.1 + 6.2
  │             │
  │             └──► Task 7.3 (Logger)
  │
  ├──► Task 8.1 (Graceful Shutdown) ◄── requires 6.2 + 7.2
  │
  ├──► Task 9.1 (Makefile) ◄── requires 7.1
  │
  └──► Task 10.1 (Integration) ◄── requires ALL Groups 1-8
         │
         ├──► Task 10.2 (Build Verification) ◄── requires 9.1
         │
         └──► Task 10.3 (Pre-Merge Checklist) ◄── requires ALL
```

---

## Traceability Matrix

| Task | Requirements Covered |
|------|---------------------|
| 1.1  | REQ-SCAFFOLD-001, REQ-SCAFFOLD-002 |
| 2.1  | REQ-SCAFFOLD-011 |
| 2.2  | REQ-SCAFFOLD-011 |
| 3.1  | REQ-SCAFFOLD-005 |
| 4.1  | REQ-SCAFFOLD-006 |
| 4.2  | REQ-SCAFFOLD-007 |
| 5.1  | REQ-SCAFFOLD-008, REQ-SCAFFOLD-014 |
| 5.2  | REQ-SCAFFOLD-015 |
| 5.3  | REQ-SCAFFOLD-008, REQ-SCAFFOLD-014 |
| 6.1  | REQ-SCAFFOLD-009 |
| 6.2  | REQ-SCAFFOLD-008 |
| 7.1  | REQ-SCAFFOLD-003 |
| 7.2  | REQ-SCAFFOLD-004 |
| 7.3  | REQ-SCAFFOLD-013 |
| 8.1  | REQ-SCAFFOLD-010 |
| 9.1  | REQ-SCAFFOLD-012 |
| 10.1 | REQ-SCAFFOLD-004, REQ-SCAFFOLD-005, REQ-SCAFFOLD-006, REQ-SCAFFOLD-009, REQ-SCAFFOLD-010, REQ-NFR-002, REQ-NFR-004 |
| 10.2 | REQ-SCAFFOLD-001, REQ-SCAFFOLD-012, REQ-NFR-001, REQ-NFR-003, REQ-NFR-004 |
| 10.3 | All REQ-SCAFFOLD-*, All REQ-NFR-* |

---

## Estimated Effort

| Group | Tasks | Estimated Time |
|-------|-------|---------------|
| 1. Module & Dirs | 1 | 15-30 min |
| 2. Configuration | 2 | 45-60 min |
| 3. Embedded PG | 1 | 30-45 min |
| 4. Migrations | 2 | 30-45 min |
| 5. HTTP Server | 3 | 60-90 min |
| 6. Health Endpoint | 2 | 30-45 min |
| 7. CLI (Cobra) | 3 | 45-60 min |
| 8. Graceful Shutdown | 1 | 30-45 min |
| 9. Makefile | 1 | 15-30 min |
| 10. Integration | 3 | 60-90 min |
| **Total** | **19 tasks** | **~6-8 hours** |

---

## Task Tracking Legend

- `[ ]` — Not started
- `[~]` — In progress
- `[x]` — Completed

## Commit Strategy

After each completed task, commit using present tense imperative mood:

```bash
# After RED phase
git add -A && git commit -m "test: Add failing tests for [component]"

# After GREEN phase
git add -A && git commit -m "feat: Implement [component]"

# After REFACTOR phase
git add -A && git commit -m "refactor: Clean up [component]"
```

## Notes

### Implementation Notes

- Embedded PostgreSQL tests take ~5-8 seconds; use `testing.Short()` skip for fast feedback loops
- Embedded PG uses port 5433 to avoid conflicts with system PostgreSQL on 5432
- sqlc is a build-time tool only; install via `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- goose CLI needed for `make migrate-new`; install via `go install github.com/pressly/goose/v3/cmd/goose@latest`

### Blockers

(none yet)

### Future Improvements

- Hot-reload for `make dev` (e.g., `air` or `watchexec`)
- Configurable embedded PG port via env var
- Structured request ID middleware for tracing
- Health endpoint with detailed dependency checks (DB latency, migration version)

### Lessons Learned

(to be filled during implementation)
