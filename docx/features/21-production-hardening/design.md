# Design: Production Hardening

**Created:** 2026-03-15
**Status:** Ready for Implementation
**Feature:** 21-production-hardening
**Dependencies:** 02-user-authentication, 01-go-scaffold, 19-master-key (for ARI_MASTER_KEY; falls back to HKDF from JWT secret if not available)

---

## 1. Architecture Overview

Production Hardening adds six cross-cutting capabilities to Ari without changing the core domain model. Each sub-feature is largely independent, allowing parallel implementation. The changes touch the auth layer (OAuth), CLI (backup/restore/init), infrastructure (Docker, TLS), and the HTTP middleware stack (rate limiting, body size).

### Component Relationships

```
cmd/ari/
  main.go
  run.go         ← TLS support in ListenAndServe
  backup.go      ← NEW: ari backup command
  restore.go     ← NEW: ari restore command
  init.go        ← NEW: ari init wizard

internal/
  auth/
    middleware.go ← Rate limiter integration
    oauth.go      ← NEW: OAuth2 flow logic
    oauth_test.go
  config/
    config.go     ← New TLS, OAuth, rate limit fields
  server/
    server.go     ← TLS listener, HSTS middleware, rate limiter wiring
    ratelimit.go  ← NEW: per-IP token-bucket middleware

Dockerfile        ← NEW: multi-stage build
docker-compose.yml ← NEW: embedded + external profiles
```

### Request Flow with Rate Limiting and TLS

```
Client → [TLS Termination] → [Rate Limiter] → [Max Body Size] → [Auth Middleware] → [Router] → [Handler]
                                    |
                                    v
                              429 Retry-After (if exceeded)
```

---

## 2. Database Schema

### Migration: `20260316000020_create_oauth_connections.sql`

```sql
-- +goose Up

CREATE TABLE oauth_connections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('google', 'github')),
    provider_user_id TEXT NOT NULL,
    provider_email  TEXT NOT NULL,
    access_token_encrypted  BYTEA,
    refresh_token_encrypted BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One connection per provider per user
CREATE UNIQUE INDEX uq_oauth_user_provider ON oauth_connections (user_id, provider);

-- One Ari link per provider identity
CREATE UNIQUE INDEX uq_oauth_provider_identity ON oauth_connections (provider, provider_user_id);

-- Lookup by provider email (for auto-link on first OAuth login)
CREATE INDEX idx_oauth_provider_email ON oauth_connections (provider_email);

-- Auto-update updated_at (per-table trigger function, matching existing pattern)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_oauth_connections_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_oauth_connections_updated_at
    BEFORE UPDATE ON oauth_connections
    FOR EACH ROW EXECUTE FUNCTION update_oauth_connections_updated_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_oauth_connections_updated_at ON oauth_connections;
DROP FUNCTION IF EXISTS update_oauth_connections_updated_at;
DROP TABLE IF EXISTS oauth_connections;
```

---

## 3. SQL Queries (sqlc)

### `internal/database/queries/oauth_connections.sql`

```sql
-- name: CreateOAuthConnection :one
INSERT INTO oauth_connections (user_id, provider, provider_user_id, provider_email, access_token_encrypted, refresh_token_encrypted)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetOAuthConnectionByProviderIdentity :one
SELECT * FROM oauth_connections
WHERE provider = $1 AND provider_user_id = $2;

-- name: GetOAuthConnectionsByUserID :many
SELECT * FROM oauth_connections
WHERE user_id = $1
ORDER BY provider;

-- name: DeleteOAuthConnection :exec
DELETE FROM oauth_connections
WHERE id = $1;

-- NOTE: GetUserByEmail already exists in internal/database/queries/users.sql.
-- Do NOT duplicate it here. The OAuth service uses the existing query.
```

---

## 4. OAuth2 Flow Design

### 4.1 Provider Configuration

New fields in `config.Config`:

```go
type OAuthProviderConfig struct {
    ClientID     string
    ClientSecret string
}

type Config struct {
    // ... existing fields ...
    OAuthGoogle  OAuthProviderConfig
    OAuthGitHub  OAuthProviderConfig
    TLSCert      string
    TLSKey       string
    TLSDomain    string
    TLSRedirectPort int
    RateLimitRPS    int
    RateLimitBurst  int
    TrustedProxies  string // comma-separated CIDRs, e.g. "10.0.0.0/8,172.16.0.0/12"
}
```

Environment variables:
- `ARI_OAUTH_GOOGLE_CLIENT_ID`, `ARI_OAUTH_GOOGLE_CLIENT_SECRET`
- `ARI_OAUTH_GITHUB_CLIENT_ID`, `ARI_OAUTH_GITHUB_CLIENT_SECRET`
- `ARI_TLS_CERT`, `ARI_TLS_KEY`, `ARI_DOMAIN`, `ARI_TLS_REDIRECT_PORT`
- `ARI_RATE_LIMIT_RPS`, `ARI_RATE_LIMIT_BURST`, `ARI_TRUSTED_PROXIES`

### 4.2 OAuth Service

`internal/auth/oauth.go`:

```go
type OAuthService struct {
    providers map[string]*oauth2.Config // "google", "github"
    queries   *db.Queries
    dbConn    *sql.DB
    jwtSvc    *JWTService
    sessions  SessionStore
    encKey    []byte // derived from ARI_MASTER_KEY (preferred) or HKDF from JWT secret (fallback)
}

func NewOAuthService(
    queries *db.Queries,
    dbConn *sql.DB,
    jwtSvc *JWTService,
    sessions SessionStore,
    masterKey []byte,   // from ARI_MASTER_KEY (Feature 19); may be nil
    jwtSecret []byte,   // fallback key material when masterKey is nil
    baseURL string,
    google, github OAuthProviderConfig,
) *OAuthService
```

### 4.3 Login Flow Sequence

```
Browser                    Ari Server                  Google/GitHub
  |                           |                            |
  |-- GET /api/auth/oauth/google/start -->                 |
  |                           |                            |
  |   Set state cookie        |                            |
  |<-- 302 Redirect to Google auth URL ------------------>|
  |                           |                            |
  |   User authorizes         |                            |
  |<-- 302 Redirect to callback with code + state --------|
  |                           |                            |
  |-- GET /api/auth/oauth/google/callback?code=X&state=Y  |
  |                           |                            |
  |   Validate state cookie   |                            |
  |                           |-- Exchange code for token ->|
  |                           |<-- Access + refresh token --|
  |                           |-- GET /userinfo ----------->|
  |                           |<-- email, name, sub --------|
  |                           |                            |
  |   Find/create user        |                            |
  |   Create oauth_connection |                            |
  |   Issue JWT session       |                            |
  |                           |                            |
  |<-- 302 Redirect to SPA with session cookie             |
```

**SPA redirect URL resolution:** The callback handler redirects to the SPA after successful authentication. The SPA URL is determined in this order:
1. `ARI_DOMAIN` if set (e.g., `https://ari.example.com/`)
2. Request `Host` header (e.g., `https://{Host}/` or `http://{Host}/` based on TLS)
3. Fallback: `http://localhost:3100/`

### 4.4 Provider Endpoints

Google:
- Auth URL: `https://accounts.google.com/o/oauth2/v2/auth`
- Token URL: `https://oauth2.googleapis.com/token`
- Userinfo: `https://www.googleapis.com/oauth2/v3/userinfo`
- Scopes: `openid`, `email`, `profile`

GitHub:
- Auth URL: `https://github.com/login/oauth/authorize`
- Token URL: `https://github.com/login/oauth/access_token`
- Userinfo: `https://api.github.com/user` + `https://api.github.com/user/emails`
- Scopes: `user:email`

### 4.5 Token Encryption

Encrypt OAuth tokens at rest using AES-256-GCM. The encryption key is sourced from `ARI_MASTER_KEY` (via Feature 19's MasterKeyManager) when available. If Feature 19 is not yet implemented, the key is derived from `ARI_JWT_SECRET` using HKDF-SHA256 with a fixed salt `[]byte("ari-oauth-v1")` and info string `"ari-oauth-token-encryption"`.

**Dependency:** Feature 19 (MasterKeyManager) provides `ARI_MASTER_KEY`. If unavailable, the HKDF fallback is used. When Feature 19 is implemented, migration to `ARI_MASTER_KEY` is transparent (re-encrypt tokens on first access or provide a migration script).

```go
func deriveEncryptionKey(masterKey []byte, jwtSecret []byte) ([]byte, error) {
    // Prefer ARI_MASTER_KEY from Feature 19's MasterKeyManager
    if len(masterKey) > 0 {
        h := hkdf.New(sha256.New, masterKey, []byte("ari-oauth-v1"), []byte("ari-oauth-token-encryption"))
        key := make([]byte, 32)
        _, err := io.ReadFull(h, key)
        return key, err
    }
    // Fallback: derive from JWT secret with fixed salt (never nil)
    h := hkdf.New(sha256.New, jwtSecret, []byte("ari-oauth-v1"), []byte("ari-oauth-token-encryption"))
    key := make([]byte, 32)
    _, err := io.ReadFull(h, key)
    return key, err
}
```

### 4.6 Public Endpoint: Enabled Providers

`GET /api/auth/providers` returns:

```json
{
  "providers": [
    {"name": "google", "enabled": true},
    {"name": "github", "enabled": false}
  ]
}
```

Add `/api/auth/providers` to `publicEndpoints` in `middleware.go`.
Add `/api/auth/oauth/` prefix to `publicEndpoints` in `middleware.go` (start and callback endpoints are unauthenticated).

**OAuth vs DisableSignUp behavior:** When `ARI_DISABLE_SIGNUP` is true, linking an OAuth provider to an existing user (matched by email) is always allowed. Only creating a brand new user (no existing email match) is blocked with HTTP 403 `SIGNUP_DISABLED`.

---

## 5. Backup/Restore CLI Design

### 5.1 `ari backup`

```go
// cmd/ari/backup.go

func newBackupCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "backup",
        Short: "Create a database backup",
        RunE:  runBackup,
    }
    cmd.Flags().StringP("output", "o", "", "Output file path (default: ari-backup-{timestamp}.sql)")
    cmd.Flags().String("format", "plain", "Dump format: plain or custom")
    return cmd
}
```

**Strategy:**
1. Load config to determine embedded vs external PostgreSQL.
2. For embedded PG: locate `pg_dump` in the embedded-postgres cache directory (`~/.embedded-postgres-go/`).
3. For external PG: use `exec.LookPath("pg_dump")` to find system binary.
4. Execute `pg_dump --clean` (includes DROP statements so `ari restore` works without manual cleanup) with appropriate connection args.
5. Stream output to the specified file.

### 5.2 `ari restore`

```go
// cmd/ari/restore.go

func newRestoreCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "restore",
        Short: "Restore database from backup",
        RunE:  runRestore,
    }
    cmd.Flags().StringP("input", "i", "", "Input backup file path (required)")
    cmd.Flags().Bool("confirm", false, "Confirm destructive restore operation")
    cmd.MarkFlagRequired("input")
    return cmd
}
```

**Strategy:**
1. Require `--confirm` flag -- exit with warning if missing.
2. Load config, connect to database.
3. For plain format: pipe file contents to `psql`.
4. For custom format: use `pg_restore`.
5. Run `database.Migrate()` after restore to apply any newer migrations.

### 5.3 Embedded PG Binary Discovery

```go
func findEmbeddedPgDump(dataDir string) (string, error) {
    // embedded-postgres-go caches binaries under ~/.embedded-postgres-go/
    // Walk known paths to find pg_dump
    candidates := []string{
        filepath.Join(os.UserHomeDir(), ".embedded-postgres-go", "extracted", "*", "bin", "pg_dump"),
    }
    // ... glob and return first match
}
```

---

## 6. Docker Design

### 6.1 Dockerfile

```dockerfile
# Stage 1: Build Go binary
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ari ./cmd/ari

# Stage 2: Build React UI
FROM node:22-alpine AS ui-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 3: Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=go-builder /ari /usr/local/bin/ari
COPY --from=ui-builder /app/web/dist /app/web/dist

# Run as non-root user
RUN adduser -D -u 1000 ari && chown -R ari:ari /app

# Data volume (must be writable by ari user)
VOLUME /data
RUN mkdir -p /data && chown -R ari:ari /data
ENV ARI_DATA_DIR=/data
EXPOSE 3100

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:3100/api/health || exit 1

USER ari
ENTRYPOINT ["ari"]
CMD ["run"]
```

### 6.2 Docker Compose

```yaml
# Note: No top-level "version" key (deprecated in modern Docker Compose).
# Default profile uses external PostgreSQL (recommended for Docker).
# Embedded mode requires internet access (or pre-downloaded binaries) to fetch
# embedded-postgres-go binaries on first run -- use "embedded" profile only for
# quick local testing outside Docker.

services:
  # Default: Ari with external PostgreSQL (recommended for Docker deployments)
  ari:
    build: .
    ports:
      - "3100:3100"
    environment:
      - ARI_ENV=production
      - ARI_DEPLOYMENT_MODE=authenticated
      - ARI_DATABASE_URL=postgres://ari:ari@postgres:5432/ari?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    profiles: ["external", "default"]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ari
      POSTGRES_PASSWORD: ari
      POSTGRES_DB: ari
    volumes:
      - pg-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ari"]
      interval: 5s
      timeout: 5s
      retries: 5
    profiles: ["external", "default"]

  # Embedded mode: single container with embedded PG.
  # WARNING: Requires internet to download PG binaries on first start,
  # or pre-downloaded binaries volume-mounted into the container.
  ari-embedded:
    build: .
    ports:
      - "3100:3100"
    volumes:
      - ari-data:/data
    environment:
      - ARI_ENV=production
      - ARI_DEPLOYMENT_MODE=authenticated
    profiles: ["embedded"]

volumes:
  ari-data:
  pg-data:
```

### 6.3 .dockerignore

```
.git
data/
node_modules/
web/node_modules/
bin/
*.md
docx/
test-results/
.claude/
```

---

## 7. CLI Onboarding Wizard Design

### 7.1 `ari init`

```go
// cmd/ari/init.go

func newInitCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "init",
        Short: "Initialize a new Ari installation",
        RunE:  runInit,
    }
    cmd.Flags().String("config", "./ari.yaml", "Config file output path")
    return cmd
}
```

**Interactive Flow:**

```
$ ari init

Welcome to Ari! Let's set up your installation.

? Deployment mode:
  > local_trusted (no authentication, localhost only)
    authenticated (JWT auth, for remote/shared access)

? HTTP port [3100]:

? Data directory [./data]:

? Enable TLS?
  > No
    Yes, with my own certificates
    Yes, with Let's Encrypt (requires a domain)

[If authenticated mode:]
? Admin email: admin@example.com
? Admin password: ********

Writing configuration to ./ari.yaml ...
Running database migrations ...
Creating admin user ...

Done! Start Ari with:
  ari run --config ./ari.yaml
```

### 7.2 Config File Format

```yaml
# ari.yaml
deployment_mode: authenticated
host: 0.0.0.0
port: 3100
data_dir: ./data
log_level: info

# TLS (optional)
# tls_cert: /path/to/cert.pem
# tls_key: /path/to/key.pem
# domain: ari.example.com

# OAuth (optional)
# oauth:
#   google:
#     client_id: "..."
#     client_secret: "..."
#   github:
#     client_id: "..."
#     client_secret: "..."
```

**Config Loading Priority:** Environment variables > config file > defaults. The `config.Load()` function is extended to optionally read from a YAML file first (`ari.yaml` in CWD or path from `--config` flag), then overlay environment variables. Uses `gopkg.in/yaml.v3` for parsing. This config file loading is implemented in Task 01 as part of the config foundation work.

---

## 8. Rate Limiting Design

### 8.1 Middleware

`internal/server/ratelimit.go`:

```go
type RateLimitConfig struct {
    GeneralRPS   int // default 100
    GeneralBurst int // default 200
    AuthRPS      int // default 10
    AuthBurst    int // default 20
    CleanupAge   time.Duration // default 10m
}

type ipLimiter struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}

type RateLimitMiddleware struct {
    mu       sync.Mutex
    limiters map[string]*ipLimiter
    config   RateLimitConfig
}

func NewRateLimitMiddleware(cfg RateLimitConfig) *RateLimitMiddleware

func (rl *RateLimitMiddleware) Middleware() func(http.Handler) http.Handler
```

### 8.2 IP Extraction

The `X-Forwarded-For` header is only trusted when `RemoteAddr` falls within the configured `ARI_TRUSTED_PROXIES` CIDR list. Default: empty (trust direct connection only, ignore `X-Forwarded-For`).

```go
type RateLimitMiddleware struct {
    // ... (existing fields)
    trustedProxies []*net.IPNet // parsed from ARI_TRUSTED_PROXIES
}

func extractIP(r *http.Request, trustedProxies []*net.IPNet) string {
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    remoteIP := net.ParseIP(host)

    // Only parse X-Forwarded-For if RemoteAddr is a trusted proxy
    if remoteIP != nil && len(trustedProxies) > 0 {
        for _, cidr := range trustedProxies {
            if cidr.Contains(remoteIP) {
                if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
                    parts := strings.SplitN(xff, ",", 2)
                    return strings.TrimSpace(parts[0])
                }
                break
            }
        }
    }
    // Fall back to RemoteAddr
    return host
}
```

### 8.3 Auth Endpoint Detection

```go
var authPaths = map[string]bool{
    "/api/auth/login":    true,
    "/api/auth/register": true,
}

func isAuthEndpoint(path string) bool {
    if authPaths[path] { return true }
    return strings.Contains(path, "/api/auth/oauth/") && strings.HasSuffix(path, "/callback")
}
```

### 8.4 Middleware Chain Update

In `server.go`, the middleware chain becomes:

```go
var handler http.Handler = mux
handler = auth.Middleware(mode, jwtSvc, sessions, runTokenSvc)(handler)
handler = maxBodySize(1 << 20)(handler)
handler = rateLimiter.Middleware()(handler)   // NEW: before body size
handler = hstsMiddleware(tlsEnabled)(handler) // NEW: HSTS when TLS
handler = s.middleware(handler)
```

### 8.5 Coexistence with Existing Login Rate Limiter

The existing `auth.NewRateLimiter()` brute-force login rate limiter in `auth_handler.go` is **kept as-is**. It provides defense-in-depth: the global `RateLimitMiddleware` enforces per-IP RPS across all endpoints (with stricter auth-tier limits), while the existing login limiter provides additional brute-force protection specifically for login attempts (e.g., failed-attempt counting, lockout). Both layers operate independently.

**Changes to `auth_handler.go`:** None -- keep the existing `rateLimiter` field and logic.

**Changes to `server.go`:** Add the new global `RateLimitMiddleware` as a separate middleware layer in the chain (before `maxBodySize`). The `AuthHandler` constructor signature remains unchanged.

---

## 9. TLS Design

### 9.1 Server Startup

Modify `Server.ListenAndServe()` in `internal/server/server.go`:

```go
func (s *Server) ListenAndServe(ctx context.Context) error {
    if s.tlsConfig != nil {
        // TLS mode
        s.http.TLSConfig = s.tlsConfig
        go s.startRedirectServer(ctx) // HTTP→HTTPS redirect
        slog.Info("https server listening", "addr", s.http.Addr)
        err = s.http.ListenAndServeTLS("", "") // certs in TLSConfig
    } else {
        slog.Info("http server listening", "addr", s.http.Addr)
        err = s.http.ListenAndServe()
    }
    // ... shutdown logic unchanged
}
```

### 9.2 TLS Configuration Resolution

```go
func resolveTLSConfig(cfg *config.Config) (*tls.Config, error) {
    // Case 1: User-provided cert/key
    if cfg.TLSCert != "" && cfg.TLSKey != "" {
        cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
        return &tls.Config{Certificates: []tls.Certificate{cert}}, err
    }

    // Case 2: Auto-TLS via Let's Encrypt
    if cfg.TLSDomain != "" {
        certDir := filepath.Join(cfg.DataDir, "certs")
        m := &autocert.Manager{
            Cache:      autocert.DirCache(certDir),
            Prompt:     autocert.AcceptTOS,
            HostPolicy: autocert.HostWhitelist(cfg.TLSDomain),
        }
        return m.TLSConfig(), nil
    }

    // Case 3: No TLS
    return nil, nil
}
```

### 9.3 HSTS Middleware

```go
func hstsMiddleware(enabled bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        if !enabled { return next }
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
            next.ServeHTTP(w, r)
        })
    }
}
```

### 9.4 HTTP-to-HTTPS Redirect

```go
func (s *Server) startRedirectServer(ctx context.Context) {
    port := s.cfg.TLSRedirectPort
    if port == 0 { port = 80 }
    redirect := &http.Server{
        Addr: fmt.Sprintf(":%d", port),
        Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            target := "https://" + r.Host + r.URL.RequestURI()
            http.Redirect(w, r, target, http.StatusMovedPermanently)
        }),
    }
    go redirect.ListenAndServe()
    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    redirect.Shutdown(shutdownCtx)
}
```

---

## 10. Request Body Size Override Design

### 10.1 Per-Route Configuration

Extend the existing `maxBodySize` middleware to support per-route overrides:

```go
type BodySizeConfig struct {
    Default   int64            // 1MB
    Overrides map[string]int64 // path prefix → max bytes
}

type bodySizeOverride struct {
    Prefix string
    Size   int64
}

func maxBodySizeWithOverrides(cfg BodySizeConfig) func(http.Handler) http.Handler {
    // Pre-sort overrides by prefix length (longest first) to ensure
    // the most specific prefix matches before shorter ones.
    sorted := make([]bodySizeOverride, 0, len(cfg.Overrides))
    for prefix, size := range cfg.Overrides {
        sorted = append(sorted, bodySizeOverride{Prefix: prefix, Size: size})
    }
    sort.Slice(sorted, func(i, j int) bool {
        return len(sorted[i].Prefix) > len(sorted[j].Prefix)
    })

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            maxBytes := cfg.Default
            for _, override := range sorted {
                if strings.HasPrefix(r.URL.Path, override.Prefix) {
                    maxBytes = override.Size
                    break
                }
            }
            if r.Body != nil {
                r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Initially, no overrides are configured. This is a hook point for future file upload endpoints.

---

## 11. Config Changes Summary

New fields added to `config.Config`:

| Field | Env Var | Default | Type |
|-------|---------|---------|------|
| `OAuthGoogle.ClientID` | `ARI_OAUTH_GOOGLE_CLIENT_ID` | `""` | `string` |
| `OAuthGoogle.ClientSecret` | `ARI_OAUTH_GOOGLE_CLIENT_SECRET` | `""` | `string` |
| `OAuthGitHub.ClientID` | `ARI_OAUTH_GITHUB_CLIENT_ID` | `""` | `string` |
| `OAuthGitHub.ClientSecret` | `ARI_OAUTH_GITHUB_CLIENT_SECRET` | `""` | `string` |
| `TLSCert` | `ARI_TLS_CERT` | `""` | `string` |
| `TLSKey` | `ARI_TLS_KEY` | `""` | `string` |
| `TLSDomain` | `ARI_DOMAIN` | `""` | `string` |
| `TLSRedirectPort` | `ARI_TLS_REDIRECT_PORT` | `80` | `int` |
| `RateLimitRPS` | `ARI_RATE_LIMIT_RPS` | `100` | `int` |
| `RateLimitBurst` | `ARI_RATE_LIMIT_BURST` | `200` | `int` |
| `TrustedProxies` | `ARI_TRUSTED_PROXIES` | `""` | `string` (comma-separated CIDRs) |

---

## 12. Dependencies (Go Modules)

New dependencies:

| Package | Purpose |
|---------|---------|
| `golang.org/x/oauth2` | OAuth2 client flow |
| `golang.org/x/oauth2/google` | Google provider endpoints |
| `golang.org/x/oauth2/github` | GitHub provider endpoints (or manual config) |
| `golang.org/x/crypto/acme/autocert` | Let's Encrypt auto-TLS |
| `golang.org/x/crypto/hkdf` | Key derivation for token encryption |
| `golang.org/x/time/rate` | Token-bucket rate limiter |
| `gopkg.in/yaml.v3` | Config file parsing for `ari init` |

---

## 13. Security Considerations

1. **OAuth state parameter:** Use `crypto/rand` to generate 32-byte state, stored in a short-lived (5-minute) HttpOnly, SameSite=Lax cookie. The `Secure` flag is set only when TLS is active (check `r.TLS != nil` or config `TLSCert`/`TLSDomain` is set). Validated on callback.
2. **Token encryption:** OAuth access/refresh tokens encrypted at rest with AES-256-GCM. Key derived from JWT secret via HKDF so no additional secret management.
3. **Rate limiting:** Applied before auth middleware to protect against brute-force attacks on login endpoints.
4. **HSTS:** Prevents protocol downgrade attacks when TLS is active.
5. **HTTP redirect:** Ensures clients always reach the HTTPS endpoint.
6. **Backup files:** Contain full database contents including password hashes. Users are responsible for securing backup files.
7. **Docker:** Runs as non-root user in production image. Secrets passed via environment variables, not baked into image.
