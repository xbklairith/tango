# Design: Go Scaffold

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 01-go-scaffold
**Requirements:** [requirements.md](./requirements.md)

---

## 1. Architecture Overview

The Go scaffold is the foundation of the entire Ari system. It delivers a single Go binary that bundles a Cobra CLI, an HTTP server (stdlib `net/http`), and an embedded PostgreSQL instance (for development mode). The binary follows a layered architecture: CLI commands at the outer edge parse flags and configuration, then delegate to internal packages that manage the HTTP server lifecycle, database connections, migrations, and request handling. Every subsequent Ari feature (auth, squads, agents, issues) is built on top of this scaffold.

The architecture enforces strict separation of concerns through Go's `internal/` package boundary. External consumers can only interact through the compiled binary or the HTTP API. Internal packages communicate through well-defined Go interfaces, enabling testability and future substitution (e.g., swapping embedded PG for an external connection).

```
┌──────────────────────────────────────────────────────┐
│  cmd/ari/main.go                                     │
│  ├── root.go    (Cobra root command + global flags)  │
│  ├── run.go     (ari run — full startup sequence)    │
│  └── version.go (ari version)                        │
├──────────────────────────────────────────────────────┤
│  internal/                                           │
│  ├── config/    (configuration loading + validation) │
│  ├── database/  (PG lifecycle, migrations, queries)  │
│  │   ├── migrations/  (goose SQL files)              │
│  │   ├── queries/     (sqlc SQL files)               │
│  │   └── db/          (sqlc generated Go code)       │
│  ├── server/    (HTTP server, router, middleware)     │
│  ├── domain/    (shared domain types)                │
│  └── adapter/   (agent runtime adapters — future)    │
├──────────────────────────────────────────────────────┤
│  PostgreSQL (embedded-postgres-go or external)       │
└──────────────────────────────────────────────────────┘
```

---

## 2. System Context

- **Depends On:** Go 1.24 runtime, PostgreSQL (embedded or external), filesystem for data directory
- **Used By:** Every subsequent Ari feature (auth, squads, agents, issues, UI); React SPA (served via `go:embed` in later phases)
- **External Dependencies:**
  - `github.com/spf13/cobra` — CLI framework
  - `github.com/fergusstrange/embedded-postgres` — embedded PostgreSQL for dev mode
  - `github.com/pressly/goose/v3` — database migrations
  - `github.com/sqlc-dev/sqlc` — SQL code generation (build tool only)

---

## 3. Component Structure

### 3.1 CLI Layer — `cmd/ari/`

#### `cmd/ari/main.go`

**Responsibility:** Program entrypoint. Calls the root command and exits with the appropriate code.

```go
package main

import (
    "os"
)

// version is set via -ldflags at build time.
var version = "dev"

func main() {
    if err := newRootCmd(version).Execute(); err != nil {
        os.Exit(1)
    }
}
```

#### `cmd/ari/root.go`

**Responsibility:** Defines the root Cobra command, global flags, and subcommand registration.

```go
package main

import (
    "github.com/spf13/cobra"
)

func newRootCmd(version string) *cobra.Command {
    root := &cobra.Command{
        Use:   "ari",
        Short: "Ari — The Control Plane for AI Agents",
        Long:  "Deploy, govern, and share AI agent workforces.",
        SilenceUsage:  true,
        SilenceErrors: true,
    }

    root.AddCommand(newRunCmd(version))
    root.AddCommand(newVersionCmd(version))

    return root
}
```

#### `cmd/ari/run.go`

**Responsibility:** Implements the `ari run` command. Orchestrates the full startup sequence: config loading, database lifecycle, migrations, HTTP server, and graceful shutdown.

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "github.com/spf13/cobra"

    "github.com/xb/ari/internal/config"
    "github.com/xb/ari/internal/database"
    "github.com/xb/ari/internal/server"
)

func newRunCmd(version string) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "run",
        Short: "Start the Ari server",
        Long:  "Start the full Ari stack: database, migrations, and HTTP server.",
        RunE: func(cmd *cobra.Command, args []string) error {
            return runServer(cmd.Context(), version)
        },
    }

    cmd.Flags().Int("port", 0, "HTTP server port (overrides ARI_PORT)")

    return cmd
}

func runServer(ctx context.Context, version string) error {
    // 1. Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("loading config: %w", err)
    }

    // 2. Initialize logger
    setupLogger(cfg.LogLevel, cfg.Env)

    slog.Info("starting ari", "version", version, "env", cfg.Env)

    // 3. Start database (embedded or external)
    db, cleanup, err := database.Open(ctx, cfg)
    if err != nil {
        return fmt.Errorf("opening database: %w", err)
    }
    defer cleanup()

    // 4. Run migrations
    if err := database.Migrate(ctx, db); err != nil {
        return fmt.Errorf("running migrations: %w", err)
    }

    // 5. Start HTTP server
    srv := server.New(cfg, db, version)

    // 6. Wait for shutdown signal
    ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
    defer stop()

    return srv.ListenAndServe(ctx)
}
```

#### `cmd/ari/version.go`

**Responsibility:** Prints the build version and exits.

```go
package main

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newVersionCmd(version string) *cobra.Command {
    return &cobra.Command{
        Use:   "version",
        Short: "Print the Ari version",
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Printf("ari version %s\n", version)
        },
    }
}
```

---

### 3.2 Configuration — `internal/config/`

**Responsibility:** Load, validate, and expose application configuration. Single source of truth for all runtime settings.

**Dependencies:** None (leaf package).

```go
package config

import (
    "fmt"
    "os"
    "strconv"
    "time"
)

// Config holds all application configuration values.
type Config struct {
    // Env is the runtime environment: "development" or "production".
    Env string

    // Host is the HTTP server bind address.
    Host string

    // Port is the HTTP server listen port.
    Port int

    // DatabaseURL is the PostgreSQL connection string.
    // When empty, embedded PostgreSQL is used (dev mode).
    DatabaseURL string

    // DataDir is the directory for embedded PG data and local storage.
    DataDir string

    // LogLevel controls logging verbosity: "debug", "info", "warn", "error".
    LogLevel string

    // ShutdownTimeout is the graceful shutdown deadline.
    ShutdownTimeout time.Duration
}

// IsProduction returns true when running in production mode.
func (c *Config) IsProduction() bool {
    return c.Env == "production"
}

// UseEmbeddedPostgres returns true when no external database URL is configured.
func (c *Config) UseEmbeddedPostgres() bool {
    return c.DatabaseURL == ""
}

// Addr returns the "host:port" listen address.
func (c *Config) Addr() string {
    return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
    cfg := &Config{
        Env:             envOrDefault("ARI_ENV", "development"),
        Host:            envOrDefault("ARI_HOST", "0.0.0.0"),
        Port:            3100,
        DatabaseURL:     os.Getenv("ARI_DATABASE_URL"),
        DataDir:         envOrDefault("ARI_DATA_DIR", "./data"),
        LogLevel:        envOrDefault("ARI_LOG_LEVEL", "info"),
        ShutdownTimeout: 30 * time.Second,
    }

    // Parse port
    if v := os.Getenv("ARI_PORT"); v != "" {
        port, err := strconv.Atoi(v)
        if err != nil {
            return nil, fmt.Errorf("invalid ARI_PORT %q: %w", v, err)
        }
        if port < 1 || port > 65535 {
            return nil, fmt.Errorf("ARI_PORT %d out of range (1-65535)", port)
        }
        cfg.Port = port
    }

    // Parse shutdown timeout
    if v := os.Getenv("ARI_SHUTDOWN_TIMEOUT"); v != "" {
        d, err := time.ParseDuration(v)
        if err != nil {
            return nil, fmt.Errorf("invalid ARI_SHUTDOWN_TIMEOUT %q: %w", v, err)
        }
        cfg.ShutdownTimeout = d
    }

    // Validate log level
    switch cfg.LogLevel {
    case "debug", "info", "warn", "error":
        // valid
    default:
        return nil, fmt.Errorf("invalid ARI_LOG_LEVEL %q: must be debug, info, warn, or error", cfg.LogLevel)
    }

    // Validate env
    switch cfg.Env {
    case "development", "production":
        // valid
    default:
        return nil, fmt.Errorf("invalid ARI_ENV %q: must be development or production", cfg.Env)
    }

    return cfg, nil
}

func envOrDefault(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

**Configuration Reference:**

| Variable | Default | Description |
|----------|---------|-------------|
| `ARI_ENV` | `development` | Runtime environment: `development`, `production` |
| `ARI_HOST` | `0.0.0.0` | HTTP server bind address |
| `ARI_PORT` | `3100` | HTTP server listen port |
| `ARI_DATABASE_URL` | _(empty)_ | External PostgreSQL URL. Empty = embedded PG |
| `ARI_DATA_DIR` | `./data` | Data directory for embedded PG and local storage |
| `ARI_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `ARI_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown deadline (Go duration format) |

---

### 3.3 Database — `internal/database/`

**Responsibility:** Manage the PostgreSQL lifecycle (embedded or external), connection pooling, and schema migrations.

**Dependencies:** `internal/config`, `embedded-postgres-go`, `goose/v3`

#### `internal/database/database.go`

```go
package database

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "path/filepath"
    "runtime"

    embeddedpostgres "github.com/fergusstrange/embedded-postgres"
    _ "github.com/lib/pq"

    "github.com/xb/ari/internal/config"
)

// Open initializes the database connection. In dev mode with no DatabaseURL,
// it starts embedded PostgreSQL first. The returned cleanup function must be
// called on shutdown to release resources (stop embedded PG, close pool).
func Open(ctx context.Context, cfg *config.Config) (*sql.DB, func(), error) {
    var (
        dsn     string
        epg     *embeddedpostgres.EmbeddedPostgres
        cleanup = func() {} // no-op default
    )

    if cfg.UseEmbeddedPostgres() {
        slog.Info("starting embedded postgresql", "data_dir", cfg.DataDir)

        pgDataDir := filepath.Join(cfg.DataDir, "postgres")
        pgPort := uint32(5433) // avoid conflicting with system PG on 5432

        // Pin a specific PG version known to work on both amd64 and arm64 (Apple Silicon).
        // Do NOT rely on the library default — it may not have arm64 binaries.
        pgVersion := embeddedpostgres.V16
        slog.Info("configuring embedded postgresql", "version", pgVersion, "port", pgPort)

        epg = embeddedpostgres.NewDatabase(
            embeddedpostgres.DefaultConfig().
                Version(pgVersion).
                DataPath(pgDataDir).
                Port(pgPort).
                Logger(slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug)),
        )

        if err := epg.Start(); err != nil {
            return nil, nil, fmt.Errorf("starting embedded postgres (version=%s, platform=%s/%s): %w\n\n"+
                "Troubleshooting:\n"+
                "  - Check your network connection (the PG binary is downloaded on first run)\n"+
                "  - If on an unsupported platform, set ARI_DATABASE_URL to use an external PostgreSQL instance\n"+
                "  - Clear cached downloads: rm -rf %s",
                pgVersion, runtime.GOOS, runtime.GOARCH, err, pgDataDir)
        }

        dsn = fmt.Sprintf(
            "host=localhost port=%d user=postgres password=postgres dbname=postgres sslmode=disable",
            pgPort,
        )

        cleanup = func() {
            slog.Info("stopping embedded postgresql")
            if err := epg.Stop(); err != nil {
                slog.Error("failed to stop embedded postgres", "error", err)
            }
        }

        slog.Info("embedded postgresql started", "port", pgPort)
    } else {
        dsn = cfg.DatabaseURL
        slog.Info("connecting to external postgresql")
    }

    db, err := sql.Open("postgres", dsn)
    if err != nil {
        cleanup()
        return nil, nil, fmt.Errorf("opening database connection: %w", err)
    }

    // Configure connection pool
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)

    // Verify connectivity
    if err := db.PingContext(ctx); err != nil {
        cleanup()
        return nil, nil, fmt.Errorf("pinging database: %w", err)
    }

    slog.Info("database connection established")

    // Wrap cleanup to also close the pool
    origCleanup := cleanup
    cleanup = func() {
        slog.Info("closing database connection pool")
        if err := db.Close(); err != nil {
            slog.Error("failed to close database", "error", err)
        }
        origCleanup()
    }

    return db, cleanup, nil
}
```

> **Design Note — Embedded PG Version Pinning & Error Handling:**
> - `embedded-postgres-go` downloads a platform-specific PostgreSQL binary on first run. The library default version may not have arm64/macOS (Apple Silicon) binaries, causing a silent or cryptic failure. We pin `embeddedpostgres.V16` explicitly because it has verified binaries for both `amd64` and `arm64` on Linux and macOS.
> - The `epg.Start()` error path includes `runtime.GOOS`/`runtime.GOARCH` and actionable troubleshooting steps so that users on unsupported platforms get a clear fallback path (`ARI_DATABASE_URL`).
> - If changing the pinned version in the future, verify binary availability at https://repo1.maven.org/maven2/io/zonky/test/postgres/ for all target platforms before merging.

#### `internal/database/migrate.go`

```go
package database

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"

    "github.com/pressly/goose/v3"
)

// Migrate runs all pending goose migrations from the embedded SQL files.
func Migrate(ctx context.Context, db *sql.DB) error {
    slog.Info("running database migrations")

    goose.SetBaseFS(migrationsFS)

    if err := goose.SetDialect("postgres"); err != nil {
        return fmt.Errorf("setting goose dialect: %w", err)
    }

    if err := goose.UpContext(ctx, db, "migrations"); err != nil {
        return fmt.Errorf("running migrations: %w", err)
    }

    version, err := goose.GetDBVersionContext(ctx, db)
    if err != nil {
        return fmt.Errorf("getting migration version: %w", err)
    }

    slog.Info("migrations complete", "version", version)
    return nil
}
```

#### `internal/database/embed.go`

```go
package database

import "embed"

//go:embed migrations/*.sql
var migrationsFS embed.FS
```

#### Initial Migration: `internal/database/migrations/20260314000001_init.sql`

```sql
-- +goose Up
-- Initial migration: create extensions and verify connectivity.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- +goose Down
DROP EXTENSION IF EXISTS "pgcrypto";
```

---

### 3.4 HTTP Server — `internal/server/`

**Responsibility:** HTTP server lifecycle, routing, middleware chain, and request handling.

**Dependencies:** `internal/config`, `database/sql`

#### `internal/server/server.go`

```go
package server

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "net/http"
    "time"

    "github.com/xb/ari/internal/config"
)

// Server wraps the HTTP server with application dependencies.
type Server struct {
    cfg     *config.Config
    db      *sql.DB
    version string
    http    *http.Server
}

// New creates a new Server with routes and middleware configured.
func New(cfg *config.Config, db *sql.DB, version string) *Server {
    s := &Server{
        cfg:     cfg,
        db:      db,
        version: version,
    }

    mux := http.NewServeMux()
    s.registerRoutes(mux)

    handler := s.middleware(mux)

    s.http = &http.Server{
        Addr:         cfg.Addr(),
        Handler:      handler,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    return s
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
// On context cancellation, it performs a graceful shutdown.
func (s *Server) ListenAndServe(ctx context.Context) error {
    errCh := make(chan error, 1)

    go func() {
        slog.Info("http server listening", "addr", s.http.Addr)
        if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errCh <- err
        }
        close(errCh)
    }()

    // Wait for shutdown signal or server error
    select {
    case err := <-errCh:
        return fmt.Errorf("http server error: %w", err)
    case <-ctx.Done():
        slog.Info("shutting down http server", "timeout", s.cfg.ShutdownTimeout)
    }

    // Graceful shutdown
    shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
    defer cancel()

    if err := s.http.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("http server shutdown: %w", err)
    }

    slog.Info("http server stopped")
    return nil
}
```

#### `internal/server/routes.go`

```go
package server

import (
    "net/http"
)

// registerRoutes configures all API routes on the given mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
    // Health check
    mux.HandleFunc("GET /api/health", s.handleHealth)

    // Catch-all for unknown API routes
    mux.HandleFunc("/api/", s.handleNotFound)
}
```

#### `internal/server/middleware.go`

```go
package server

import (
    "fmt"
    "log/slog"
    "net/http"
    "runtime/debug"
    "time"
)

// middleware builds the middleware chain applied to all requests.
func (s *Server) middleware(next http.Handler) http.Handler {
    // Order: outermost runs first
    h := next
    h = s.recoverMiddleware(h)
    h = s.corsMiddleware(h)
    h = s.contentTypeMiddleware(h)
    h = s.loggingMiddleware(h)
    return h
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
    return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs every request with method, path, status, and duration.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rw := newResponseWriter(w)

        next.ServeHTTP(rw, r)

        slog.Info("http request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", rw.statusCode,
            "duration", time.Since(start).String(),
            "remote", r.RemoteAddr,
        )
    })
}

// recoverMiddleware catches panics in handlers and returns HTTP 500.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                stack := debug.Stack()
                slog.Error("panic recovered",
                    "error", fmt.Sprintf("%v", err),
                    "stack", string(stack),
                    "method", r.Method,
                    "path", r.URL.Path,
                )
                writeJSON(w, http.StatusInternalServerError, ErrorResponse{
                    Error: "Internal server error",
                    Code:  "INTERNAL_ERROR",
                })
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// corsMiddleware sets CORS headers for cross-origin requests.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := "http://localhost:5173" // Vite dev server
        if s.cfg.IsProduction() {
            origin = fmt.Sprintf("http://%s", s.cfg.Addr())
        }

        w.Header().Set("Access-Control-Allow-Origin", origin)
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        w.Header().Set("Access-Control-Max-Age", "86400")

        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }

        next.ServeHTTP(w, r)
    })
}

// contentTypeMiddleware sets Content-Type: application/json for all /api/ routes.
func (s *Server) contentTypeMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
            w.Header().Set("Content-Type", "application/json")
        }
        next.ServeHTTP(w, r)
    })
}
```

#### `internal/server/response.go`

```go
package server

import (
    "encoding/json"
    "log/slog"
    "net/http"
)

// ErrorResponse is the standard API error format.
type ErrorResponse struct {
    Error string `json:"error"`
    Code  string `json:"code"`
}

// writeJSON serializes data as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(data); err != nil {
        slog.Error("failed to write json response", "error", err)
    }
}
```

#### `internal/server/handlers.go`

```go
package server

import (
    "net/http"
)

// HealthResponse is returned by the health endpoint.
type HealthResponse struct {
    Status  string `json:"status"`
    Version string `json:"version"`
}

// HealthErrorResponse is returned when a dependency check fails.
type HealthErrorResponse struct {
    Status string `json:"status"`
    Error  string `json:"error"`
}

// handleHealth checks application health including database connectivity.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    if err := s.db.PingContext(r.Context()); err != nil {
        writeJSON(w, http.StatusServiceUnavailable, HealthErrorResponse{
            Status: "unhealthy",
            Error:  "database ping failed",
        })
        return
    }

    writeJSON(w, http.StatusOK, HealthResponse{
        Status:  "ok",
        Version: s.version,
    })
}

// handleNotFound returns a 404 for unknown API routes.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusNotFound, ErrorResponse{
        Error: "Not found",
        Code:  "NOT_FOUND",
    })
}
```

---

### 3.5 Structured Logging — `cmd/ari/logger.go`

**Responsibility:** Initialize `slog` with the appropriate handler based on environment.

```go
package main

import (
    "log/slog"
    "os"
)

func setupLogger(level string, env string) {
    var lvl slog.Level
    switch level {
    case "debug":
        lvl = slog.LevelDebug
    case "warn":
        lvl = slog.LevelWarn
    case "error":
        lvl = slog.LevelError
    default:
        lvl = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{Level: lvl}

    var handler slog.Handler
    if env == "production" {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)
    }

    slog.SetDefault(slog.New(handler))
}
```

---

### 3.6 Domain — `internal/domain/`

**Responsibility:** Shared domain types used across packages. Placeholder for Phase 1; populated with squad, agent, and issue types in later features.

```go
package domain

// Version is set at build time via ldflags.
// Domain models will be added here as features are implemented.
```

---

### 3.7 Adapter — `internal/adapter/`

**Responsibility:** Agent runtime adapter interface. Placeholder for Phase 2.

```go
package adapter

// Adapter defines the interface for agent runtime adapters.
// Implementations are registered in Phase 2 (process, claude_local, etc.).
type Adapter interface {
    // Name returns the adapter identifier (e.g., "process", "claude_local").
    Name() string
}
```

---

## 4. Data Flow

### `ari run` Startup Sequence

```
main.go
  │
  ├──► cobra.Execute()
  │       │
  │       ├──► runServer(ctx, version)
  │       │       │
  │       │       ├── 1. config.Load()
  │       │       │      Parse env vars → validate → return *Config
  │       │       │
  │       │       ├── 2. setupLogger(cfg.LogLevel, cfg.Env)
  │       │       │      Initialize slog (text for dev, JSON for prod)
  │       │       │
  │       │       ├── 3. database.Open(ctx, cfg)
  │       │       │      ├── [dev] Start embedded PG → wait for ready
  │       │       │      └── [prod] Connect to external PG URL
  │       │       │      └── Ping to verify connectivity
  │       │       │
  │       │       ├── 4. database.Migrate(ctx, db)
  │       │       │      goose.UpContext() → apply pending SQL migrations
  │       │       │
  │       │       ├── 5. server.New(cfg, db, version)
  │       │       │      Build mux → register routes → wrap middleware
  │       │       │
  │       │       ├── 6. signal.NotifyContext(SIGINT, SIGTERM)
  │       │       │      Register OS signal handlers
  │       │       │
  │       │       └── 7. srv.ListenAndServe(ctx)
  │       │              ├── Start HTTP listener in goroutine
  │       │              ├── Log "http server listening addr=:3100"
  │       │              └── Block until context cancelled
  │       │
  │       └──► [on signal] Graceful Shutdown
  │               ├── http.Server.Shutdown(timeout=30s)
  │               │    └── Drain in-flight requests
  │               ├── db.Close()
  │               │    └── Close connection pool
  │               └── embeddedPG.Stop()
  │                    └── Stop PG process, flush WAL
  │
  └──► os.Exit(0)
```

### HTTP Request Flow

```
Client Request
     │
     ▼
loggingMiddleware   ── record start time
     │
     ▼
contentTypeMiddleware  ── set Content-Type: application/json (for /api/)
     │
     ▼
corsMiddleware      ── set CORS headers, handle OPTIONS preflight
     │
     ▼
recoverMiddleware   ── defer panic recovery
     │
     ▼
http.ServeMux       ── route to handler
     │
     ├── GET /api/health   → handleHealth
     └── /api/*            → handleNotFound
     │
     ▼
loggingMiddleware   ── log method, path, status, duration
```

---

## 5. API Contracts

### `GET /api/health`

**Purpose:** Application health check with database connectivity verification.

**Request:** No body. No query parameters.

**Response (200 OK):** Application is healthy, database is reachable.

```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

**Response (503 Service Unavailable):** Database is unreachable.

```json
{
  "status": "unhealthy",
  "error": "database ping failed"
}
```

### Error Responses (All Endpoints)

**404 Not Found:** Unknown API route.

```json
{
  "error": "Not found",
  "code": "NOT_FOUND"
}
```

**500 Internal Server Error:** Unhandled panic in a handler.

```json
{
  "error": "Internal server error",
  "code": "INTERNAL_ERROR"
}
```

---

## 6. Directory Structure

Complete file tree to create for the scaffold:

```
ari/
├── cmd/
│   └── ari/
│       ├── main.go          # Entrypoint
│       ├── root.go          # Root cobra command
│       ├── run.go           # ari run command
│       ├── version.go       # ari version command
│       └── logger.go        # slog initialization
├── internal/
│   ├── config/
│   │   ├── config.go        # Config struct + Load()
│   │   └── config_test.go   # Config loading tests
│   ├── database/
│   │   ├── database.go      # Open(), connection lifecycle
│   │   ├── database_test.go # Database lifecycle tests
│   │   ├── migrate.go       # Migrate() with goose
│   │   ├── embed.go         # go:embed for migration files
│   │   ├── migrations/
│   │   │   └── 20260314000001_init.sql
│   │   ├── queries/
│   │   │   └── health.sql   # Placeholder sqlc query
│   │   └── db/
│   │       └── .gitkeep     # sqlc generated code (empty until first generation)
│   ├── server/
│   │   ├── server.go        # Server struct + ListenAndServe
│   │   ├── server_test.go   # Server tests
│   │   ├── routes.go        # Route registration
│   │   ├── middleware.go     # Middleware chain
│   │   ├── handlers.go      # Health + not-found handlers
│   │   └── response.go      # JSON response helpers
│   ├── domain/
│   │   └── domain.go        # Placeholder for domain types
│   └── adapter/
│       └── adapter.go       # Placeholder adapter interface
├── web/
│   └── .gitkeep             # React SPA (future)
├── data/
│   └── .gitkeep             # Runtime data directory (gitignored contents)
├── go.mod
├── go.sum
├── Makefile
├── sqlc.yaml
└── .gitignore
```

---

## 7. Makefile

```makefile
# Ari Makefile
# ============

# Build variables
BINARY     := bin/ari
MODULE     := github.com/xb/ari
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-s -w -X main.version=$(VERSION)"
GO         := go

.PHONY: all build dev test lint sqlc migrate-new clean help

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

---

## 8. Configuration Files

### `sqlc.yaml`

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

### `internal/database/queries/health.sql`

```sql
-- name: Ping :exec
SELECT 1;
```

### `.gitignore` additions

```gitignore
# Build output
bin/

# Runtime data (embedded PG, local storage)
data/postgres/
data/*.db

# OS files
.DS_Store

# IDE
.idea/
.vscode/

# Go
*.test
*.out
```

---

## 9. Graceful Shutdown

The shutdown sequence is critical because embedded PostgreSQL must be stopped cleanly to avoid data corruption. The sequence uses Go's `context` propagation to coordinate all layers.

### Shutdown Sequence

1. **Signal received** (SIGINT or SIGTERM) — the `signal.NotifyContext` cancels the root context.
2. **HTTP server shutdown** — `http.Server.Shutdown(ctx)` stops accepting new connections and waits for in-flight requests up to `ShutdownTimeout` (default: 30s). If the timeout expires, remaining connections are forcibly closed.
3. **Database pool close** — `sql.DB.Close()` closes all pooled connections.
4. **Embedded PostgreSQL stop** — `embeddedpostgres.Stop()` sends SIGTERM to the PG child process, waits for WAL flush, and removes the PID file.
5. **Process exit** — returns to `main()`, exits with code 0.

### Shutdown Timeout Behavior

```
Signal received
     │
     ├── Start shutdown timer (30s default)
     │
     ├── http.Server.Shutdown()
     │   ├── Stop listener (reject new connections)
     │   ├── Wait for in-flight requests...
     │   │   ├── [all done] → continue
     │   │   └── [timeout]  → force close remaining
     │
     ├── db.Close()
     │
     ├── embeddedPG.Stop()
     │
     └── exit(0)
```

### Double-Signal Handling

If the user sends a second SIGINT/SIGTERM during graceful shutdown, the process should exit immediately. This is handled naturally by `signal.NotifyContext` — the first signal cancels the context, subsequent signals terminate the process.

---

## 10. Error Handling

### Strategy

All errors follow three rules:

1. **Wrap with context** — Every error returned to a caller includes context about what operation failed: `fmt.Errorf("starting embedded postgres: %w", err)`.
2. **Log at the boundary** — Errors are logged once at the point where they are handled, not at every level they pass through.
3. **Sanitize for clients** — HTTP responses never expose internal error details. Database errors, file paths, and stack traces are logged server-side but replaced with generic messages in API responses.

### Error Response Format

All API errors use the `ErrorResponse` struct:

```go
type ErrorResponse struct {
    Error string `json:"error"` // Human-readable description
    Code  string `json:"code"`  // Machine-readable code for client logic
}
```

### Error Codes (Phase 1)

| HTTP Status | Code | When |
|-------------|------|------|
| 404 | `NOT_FOUND` | Unknown API route |
| 405 | `METHOD_NOT_ALLOWED` | Wrong HTTP method for route |
| 500 | `INTERNAL_ERROR` | Unhandled panic or unexpected server error |
| 503 | (no code, uses `HealthErrorResponse`) | Database health check failed |

### Startup Errors

Startup errors are fatal. If any step in the startup sequence fails, the process logs the error and exits with code 1. No partial startup is allowed.

| Step | Error Behavior |
|------|---------------|
| Config loading | Exit 1 with "invalid ARI_PORT", "invalid ARI_LOG_LEVEL", etc. |
| Embedded PG start | Exit 1 with "starting embedded postgres: ..." |
| Database ping | Exit 1 with "pinging database: ..." |
| Migration | Exit 1 with "running migrations: ..." and the failing migration name |
| HTTP bind | Exit 1 with "http server error: listen tcp :3100: bind: address already in use" |

---

## 11. Testing Strategy

### Test Categories

#### Unit Tests — `internal/config/config_test.go`

Test configuration loading and validation in isolation using environment variable manipulation.

```go
package config_test

import (
    "os"
    "testing"

    "github.com/xb/ari/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
    // Clear all ARI_ env vars
    clearEnv(t)

    cfg, err := config.Load()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if cfg.Port != 3100 {
        t.Errorf("expected port 3100, got %d", cfg.Port)
    }
    if cfg.Env != "development" {
        t.Errorf("expected env development, got %s", cfg.Env)
    }
    if cfg.Host != "0.0.0.0" {
        t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
    }
    if !cfg.UseEmbeddedPostgres() {
        t.Error("expected embedded postgres when DATABASE_URL is empty")
    }
}

func TestLoad_InvalidPort(t *testing.T) {
    clearEnv(t)
    t.Setenv("ARI_PORT", "not-a-number")

    _, err := config.Load()
    if err == nil {
        t.Fatal("expected error for invalid port")
    }
}

func TestLoad_InvalidLogLevel(t *testing.T) {
    clearEnv(t)
    t.Setenv("ARI_LOG_LEVEL", "verbose")

    _, err := config.Load()
    if err == nil {
        t.Fatal("expected error for invalid log level")
    }
}

func TestLoad_ExternalDatabase(t *testing.T) {
    clearEnv(t)
    t.Setenv("ARI_DATABASE_URL", "postgres://user:pass@host:5432/db")

    cfg, err := config.Load()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.UseEmbeddedPostgres() {
        t.Error("expected external postgres when DATABASE_URL is set")
    }
}

func clearEnv(t *testing.T) {
    t.Helper()
    for _, key := range []string{
        "ARI_ENV", "ARI_HOST", "ARI_PORT", "ARI_DATABASE_URL",
        "ARI_DATA_DIR", "ARI_LOG_LEVEL", "ARI_SHUTDOWN_TIMEOUT",
    } {
        t.Setenv(key, "")
        os.Unsetenv(key)
    }
}
```

#### Unit Tests — `internal/server/server_test.go`

Test HTTP handlers and middleware without a real database by using a mock `*sql.DB` or `httptest`.

```go
package server_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/xb/ari/internal/config"
    "github.com/xb/ari/internal/server"
)

func TestHealthEndpoint_OK(t *testing.T) {
    cfg := &config.Config{
        Env:  "development",
        Host: "localhost",
        Port: 3100,
    }

    // Use a real test database or a stub.
    // For unit tests, we test the handler in isolation via httptest.
    srv := server.New(cfg, testDB(t), "test-version")

    req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
    rec := httptest.NewRecorder()

    srv.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Errorf("expected status 200, got %d", rec.Code)
    }

    var resp map[string]string
    if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
        t.Fatalf("failed to decode response: %v", err)
    }

    if resp["status"] != "ok" {
        t.Errorf("expected status ok, got %s", resp["status"])
    }
    if resp["version"] != "test-version" {
        t.Errorf("expected version test-version, got %s", resp["version"])
    }
}

func TestNotFoundEndpoint(t *testing.T) {
    cfg := &config.Config{
        Env:  "development",
        Host: "localhost",
        Port: 3100,
    }

    srv := server.New(cfg, testDB(t), "test-version")

    req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
    rec := httptest.NewRecorder()

    srv.ServeHTTP(rec, req)

    if rec.Code != http.StatusNotFound {
        t.Errorf("expected status 404, got %d", rec.Code)
    }

    contentType := rec.Header().Get("Content-Type")
    if contentType != "application/json" {
        t.Errorf("expected Content-Type application/json, got %s", contentType)
    }
}

func TestCORSHeaders(t *testing.T) {
    cfg := &config.Config{
        Env:  "development",
        Host: "localhost",
        Port: 3100,
    }

    srv := server.New(cfg, testDB(t), "test-version")

    req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
    rec := httptest.NewRecorder()

    srv.ServeHTTP(rec, req)

    if rec.Header().Get("Access-Control-Allow-Origin") == "" {
        t.Error("expected CORS origin header to be set")
    }
}
```

> Note: `testDB(t)` is a test helper that returns either a real embedded PG `*sql.DB` for integration tests or `nil`/mock for pure handler tests. The exact implementation depends on whether tests are run with `-short` flag.

#### Integration Tests — `internal/database/database_test.go`

Test the full embedded PostgreSQL lifecycle: start, connect, migrate, query, stop.

```go
package database_test

import (
    "context"
    "testing"

    "github.com/xb/ari/internal/config"
    "github.com/xb/ari/internal/database"
)

func TestEmbeddedPostgresLifecycle(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping embedded postgres test in short mode")
    }

    cfg := &config.Config{
        Env:     "development",
        DataDir: t.TempDir(),
    }

    ctx := context.Background()

    db, cleanup, err := database.Open(ctx, cfg)
    if err != nil {
        t.Fatalf("failed to open database: %v", err)
    }
    defer cleanup()

    // Verify connection
    if err := db.PingContext(ctx); err != nil {
        t.Fatalf("failed to ping database: %v", err)
    }

    // Run migrations
    if err := database.Migrate(ctx, db); err != nil {
        t.Fatalf("failed to run migrations: %v", err)
    }

    // Verify migration applied
    var exists bool
    err = db.QueryRowContext(ctx,
        "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pgcrypto')",
    ).Scan(&exists)
    if err != nil {
        t.Fatalf("failed to check extension: %v", err)
    }
    if !exists {
        t.Error("expected pgcrypto extension to exist after migration")
    }
}
```

### Test Execution

| Command | Scope | Duration |
|---------|-------|----------|
| `make test` | All tests with race detection | ~15-30s (includes embedded PG startup) |
| `go test -short ./...` | Unit tests only (skip embedded PG) | ~2s |
| `go test -run TestHealth ./internal/server/` | Single test | <1s |

### Coverage Targets

| Package | Target | Rationale |
|---------|--------|-----------|
| `internal/config` | 90%+ | Pure logic, easy to test exhaustively |
| `internal/server` | 80%+ | Handler and middleware tests via httptest |
| `internal/database` | 60%+ | Integration tests require embedded PG |
| `cmd/ari` | 50%+ | CLI wiring, partially covered by integration |

---

## 12. Performance Considerations

### HTTP Server Timeouts

| Timeout | Value | Rationale |
|---------|-------|-----------|
| ReadTimeout | 15s | Prevent slow-client attacks |
| WriteTimeout | 30s | Allow time for database queries in responses |
| IdleTimeout | 60s | Keep-alive connection reuse |

### Database Connection Pool

| Setting | Value | Rationale |
|---------|-------|-----------|
| MaxOpenConns | 25 | Sufficient for single-server deployment |
| MaxIdleConns | 5 | Reduce connection churn for typical workload |

### Performance Targets (from NFR)

| Metric | Target |
|--------|--------|
| Build time (cold cache) | < 60s |
| Startup to first request | < 10s (including embedded PG) |
| Binary size | < 50 MB (excluding PG assets) |

---

## 13. Security Considerations

### Phase 1 Scope

Phase 1 has no authentication. The scaffold exposes only the health endpoint. Security hardening arrives in Phase 2+ with JWT auth, RBAC, and encrypted secrets. However, the scaffold establishes these baseline practices:

- **No secrets in config** — `ARI_DATABASE_URL` is the only sensitive value, loaded from env vars (never hardcoded).
- **Panic recovery** — Stack traces are logged server-side, never exposed in HTTP responses.
- **CORS restriction** — Only the Vite dev server origin is allowed in development mode.
- **No directory listing** — The server only responds to registered routes.
- **Timeouts on all connections** — Prevents resource exhaustion from slow clients.

---

## 14. Alternatives Considered

### Alternative 1: Chi Router Instead of stdlib ServeMux

**Description:** Use `go-chi/chi` for HTTP routing instead of `http.ServeMux`.

**Pros:**
- More mature middleware ecosystem
- Built-in middleware for logging, recovery, CORS

**Cons:**
- Additional dependency
- Go 1.22+ ServeMux supports method routing and path params natively

**Rejected Because:** The PRD specifies stdlib `net/http` to minimize dependencies. Go 1.24's ServeMux is sufficient for Ari's routing needs. Adding Chi would contradict the "single binary, zero dependencies" principle.

### Alternative 2: TOML/YAML Config File

**Description:** Use a `config.toml` or `config.yaml` file instead of pure environment variables.

**Pros:**
- Easier to manage many settings
- Comments and documentation inline

**Cons:**
- Additional dependency for parsing
- Environment variables are the 12-factor standard for containerized apps
- Config files require file management and discovery logic

**Rejected Because:** Environment variables align with the 12-factor methodology and are the standard for Docker/K8s deployments. A config file can be added later if needed without breaking the env var approach.

### Alternative 3: pgx Instead of lib/pq

**Description:** Use `jackc/pgx` as the PostgreSQL driver instead of `lib/pq`.

**Pros:**
- Actively maintained, better performance
- Native PostgreSQL protocol support

**Cons:**
- `lib/pq` is the most widely used and tested driver
- `embedded-postgres-go` examples typically use `lib/pq`

**Decision:** This is an open question. Either driver works with `database/sql`. The design uses `lib/pq` for simplicity but `pgx` is a viable alternative. The `database/sql` interface abstracts the driver, so switching later is a one-line change.

---

## 15. Open Questions

- [x] Module path: Using `github.com/xb/ari` (matches repo structure)
- [ ] Should `pgx` be preferred over `lib/pq`? (Low risk either way — `database/sql` abstracts it)
- [ ] Should the embedded PG port be configurable? (Currently hardcoded to 5433 to avoid conflicts)
- [ ] Should `make dev` use air/gow for hot reload, or plain `go run`? (Starting with `go run` for simplicity)

---

## 16. Timeline Estimate

- Requirements: 1 day (complete)
- Design: 1 day (this document)
- Implementation: 2-3 days
- Testing: 1 day (included in implementation via TDD)
- Total: 4-5 days

---

## References

- [Requirements](./requirements.md)
- [PRD](../../core/01-PRODUCT.md) — Sections 3 (Architecture), 14 (Release Plan)
- [CLAUDE.md](../../../CLAUDE.md) — Project structure and conventions
- [embedded-postgres-go](https://github.com/fergusstrange/embedded-postgres)
- [goose](https://github.com/pressly/goose)
- [sqlc](https://sqlc.dev)
- [cobra](https://github.com/spf13/cobra)
