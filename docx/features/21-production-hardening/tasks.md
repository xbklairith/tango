# Tasks: Production Hardening

**Feature:** 21-production-hardening
**Created:** 2026-03-15
**Status:** Pending

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-HARD-001 through REQ-HARD-086, REQ-HARD-NF-001 through REQ-HARD-NF-006

## Implementation Approach

Work through the sub-features in dependency order: config changes first (needed by everything), then rate limiting (simplest, immediate security value), then request body size overrides (small extension), then TLS (builds on config), then OAuth (depends on config + TLS for callback URLs), then backup/restore (independent CLI), then Docker (wraps everything), and finally the onboarding wizard (references all config options). Each task follows the Red-Green-Refactor TDD cycle.

## Progress Summary

- Total Tasks: 12
- Completed: 0/12
- In Progress: None

---

## Tasks (TDD: Red-Green-Refactor)

---

### [ ] Task 01 — Config: Add Production Hardening Fields

**Requirements:** REQ-HARD-018, REQ-HARD-019, REQ-HARD-061, REQ-HARD-086
**Estimated time:** 30 min

#### Context

Extend `config.Config` with all new fields needed by this feature: OAuth provider credentials, TLS settings, rate limit parameters. Add environment variable parsing and validation. This is the foundation for all other tasks.

#### RED — Write Failing Tests

Write `internal/config/config_hardening_test.go`:

1. `TestConfig_OAuthFields` — verify `ARI_OAUTH_GOOGLE_CLIENT_ID`, `ARI_OAUTH_GOOGLE_CLIENT_SECRET`, `ARI_OAUTH_GITHUB_CLIENT_ID`, `ARI_OAUTH_GITHUB_CLIENT_SECRET` are loaded correctly.
2. `TestConfig_TLSFields` — verify `ARI_TLS_CERT`, `ARI_TLS_KEY`, `ARI_DOMAIN`, `ARI_TLS_REDIRECT_PORT` are loaded and validated.
3. `TestConfig_RateLimitFields` — verify `ARI_RATE_LIMIT_RPS` and `ARI_RATE_LIMIT_BURST` parse correctly, default to 100/200.
4. `TestConfig_TLSValidation` — verify error when `ARI_TLS_CERT` is set without `ARI_TLS_KEY` (and vice versa).
5. `TestConfig_RateLimitValidation` — verify error when RPS or burst is zero or negative.
6. `TestConfig_TLSRedirectPortDefault` — verify default is 80 when not set.
7. `TestConfig_TrustedProxies` — verify `ARI_TRUSTED_PROXIES` is parsed correctly (comma-separated CIDRs).
8. `TestConfig_LoadFromYAML` — verify config file (`ari.yaml`) is loaded and env vars override file values.

#### GREEN — Implement

Modify `internal/config/config.go`:

- Add `OAuthProviderConfig` struct with `ClientID`, `ClientSecret` fields
- Add `OAuthGoogle`, `OAuthGitHub` (`OAuthProviderConfig`) to `Config`
- Add `TLSCert`, `TLSKey`, `TLSDomain` (string), `TLSRedirectPort` (int, default 80) to `Config`
- Add `RateLimitRPS` (int, default 100), `RateLimitBurst` (int, default 200) to `Config`
- Add parsing logic in `Load()` for all new env vars
- Add `TrustedProxies` (string, from `ARI_TRUSTED_PROXIES`) to `Config`
- Add cross-field validation: TLS cert and key must both be set or both empty
- Add helper `OAuthGoogleEnabled()`, `OAuthGitHubEnabled()` methods
- Add config file loading: `Load()` reads `ari.yaml` (from CWD or `--config` flag path) using `gopkg.in/yaml.v3`, then overlays environment variables on top. If no config file exists, env-only loading continues as before.

#### Files

- Modify: `internal/config/config.go`
- Create: `internal/config/config_hardening_test.go`

---

### [ ] Task 02 — Rate Limiting Middleware

**Requirements:** REQ-HARD-060, REQ-HARD-061, REQ-HARD-062, REQ-HARD-063, REQ-HARD-064, REQ-HARD-065, REQ-HARD-066, REQ-HARD-NF-002, REQ-HARD-NF-003
**Estimated time:** 60 min

#### Context

Implement per-IP token-bucket rate limiting as HTTP middleware using `golang.org/x/time/rate`. The middleware sits before auth in the chain, protecting against brute-force attacks. Auth endpoints get stricter limits. This is a **new global middleware** that coexists with the existing `auth.NewRateLimiter()` in `AuthHandler` (which is kept for defense-in-depth on login endpoints).

#### RED — Write Failing Tests

Write `internal/server/ratelimit_test.go`:

1. `TestRateLimiter_AllowsNormalTraffic` — 100 requests from same IP within burst, all succeed.
2. `TestRateLimiter_BlocksExcessTraffic` — burst+1 request returns 429 with `Retry-After` header and `RATE_LIMITED` error code.
3. `TestRateLimiter_PerIPIsolation` — different IPs have independent limits; IP-A exhausted does not affect IP-B.
4. `TestRateLimiter_AuthEndpointStricterLimits` — `/api/auth/login` returns 429 after 20 requests (auth burst), while `/api/squads` still succeeds at the same count.
5. `TestRateLimiter_XForwardedFor_TrustedProxy` — uses first IP from `X-Forwarded-For` header only when RemoteAddr is in `ARI_TRUSTED_PROXIES` CIDR list.
6. `TestRateLimiter_XForwardedFor_UntrustedProxy` — ignores `X-Forwarded-For` when RemoteAddr is NOT in trusted list, falls back to RemoteAddr.
7. `TestRateLimiter_FallbackToRemoteAddr` — uses `RemoteAddr` when no `X-Forwarded-For` or no trusted proxies configured.
8. `TestRateLimiter_StaleEviction` — after cleanup interval, stale entries are removed (mock time or short interval).
9. `TestRateLimiter_OAuthCallbackStricterLimits` — `/api/auth/oauth/google/callback` uses auth-tier limits.

#### GREEN — Implement

Create `internal/server/ratelimit.go`:

- `RateLimitConfig` struct with `GeneralRPS`, `GeneralBurst`, `AuthRPS`, `AuthBurst`, `CleanupAge`
- `RateLimitMiddleware` struct with `sync.Mutex`, `map[string]*ipLimiter`, config
- `NewRateLimitMiddleware(cfg RateLimitConfig) *RateLimitMiddleware`
- `Middleware() func(http.Handler) http.Handler` — extracts IP, selects limiter tier, checks `Allow()`, returns 429 with `Retry-After`
- `StartCleanup(ctx context.Context)` — goroutine that evicts stale entries
- `extractIP(r *http.Request, trustedProxies []*net.IPNet) string` — X-Forwarded-For (only if RemoteAddr in trusted proxies) or RemoteAddr
- `parseTrustedProxies(cidrs string) ([]*net.IPNet, error)` — parse comma-separated CIDRs from config
- `isAuthEndpoint(path string) bool` — detects auth-tier paths

Wire into `server.go`:

- Add `rateLimiter *RateLimitMiddleware` to `New()` parameters
- Insert `rateLimiter.Middleware()` into middleware chain before `maxBodySize`
- **Keep** existing `rateLimiter` parameter on `NewAuthHandler` (old login rate limiter retained for defense-in-depth)

#### Files

- Create: `internal/server/ratelimit.go`
- Create: `internal/server/ratelimit_test.go`
- Modify: `internal/server/server.go` (middleware chain: add global RPS limiter as separate middleware before maxBodySize)
- Modify: `cmd/ari/run.go` (create RateLimitMiddleware, pass to server.New; keep existing login rate limiter for AuthHandler unchanged)
- **Do NOT modify:** `internal/server/handlers/auth_handler.go` — existing brute-force login rate limiter stays as defense-in-depth

---

### [ ] Task 03 — Request Body Size Overrides

**Requirements:** REQ-HARD-070, REQ-HARD-071, REQ-HARD-072
**Estimated time:** 30 min

#### Context

Extend the existing `maxBodySize` middleware to support per-route overrides via a configuration map. The current 1MB default remains. Return proper HTTP 413 with `PAYLOAD_TOO_LARGE` error code. This is mostly documenting and testing existing behavior with a small extension.

#### RED — Write Failing Tests

Write `internal/server/bodysize_test.go`:

1. `TestMaxBodySize_DefaultLimit` — request with body > 1MB returns 413.
2. `TestMaxBodySize_UnderLimit` — request with body < 1MB succeeds.
3. `TestMaxBodySize_RouteOverride` — configure override for `/api/uploads` at 10MB, verify 10MB body succeeds on that path but fails on other paths.
4. `TestMaxBodySize_ErrorResponse` — verify 413 response has `PAYLOAD_TOO_LARGE` error code in JSON body.

#### GREEN — Implement

Modify `internal/server/server.go`:

- Replace `maxBodySize(maxBytes int64)` with `maxBodySizeWithOverrides(cfg BodySizeConfig)`
- `BodySizeConfig` struct: `Default int64`, `Overrides map[string]int64`
- Match override by path prefix
- Wrap `http.MaxBytesReader` error detection to return proper 413 JSON response

#### Files

- Modify: `internal/server/server.go`
- Create: `internal/server/bodysize_test.go`

---

### [ ] Task 04 — HTTPS/TLS Support

**Requirements:** REQ-HARD-080, REQ-HARD-081, REQ-HARD-082, REQ-HARD-083, REQ-HARD-084, REQ-HARD-085, REQ-HARD-086, REQ-HARD-NF-006
**Estimated time:** 60 min

#### Context

Add TLS support to the HTTP server with three modes: user-provided cert/key, auto-TLS via Let's Encrypt, or plain HTTP (current default). When TLS is active, add HSTS headers and start an HTTP-to-HTTPS redirect listener. Uses `golang.org/x/crypto/acme/autocert`.

#### RED — Write Failing Tests

Write `internal/server/tls_test.go`:

1. `TestResolveTLSConfig_UserCerts` — verify TLS config created from cert/key files (use self-signed test certs).
2. `TestResolveTLSConfig_AutoTLS` — verify autocert manager created when domain is set (mock, verify config shape).
3. `TestResolveTLSConfig_NoTLS` — verify nil returned when no TLS config.
4. `TestResolveTLSConfig_MissingKey` — verify error when cert set but key missing.
5. `TestResolveTLSConfig_MissingCert` — verify error when key set but cert missing.
6. `TestHSTSMiddleware_Enabled` — verify `Strict-Transport-Security` header present.
7. `TestHSTSMiddleware_Disabled` — verify no HSTS header when TLS not active.
8. `TestHTTPRedirect` — verify HTTP request gets 301 redirect to HTTPS.

#### GREEN — Implement

Create `internal/server/tls.go`:

- `resolveTLSConfig(cfg *config.Config) (*tls.Config, error)` — handles all three modes
- `hstsMiddleware(enabled bool) func(http.Handler) http.Handler`
- `startRedirectServer(ctx context.Context, port int)` — HTTP-to-HTTPS redirect listener; uses `context.WithTimeout(context.Background(), 5*time.Second)` for graceful shutdown

Modify `internal/server/server.go`:

- Accept `tlsConfig *tls.Config` in `New()` (or resolve inside based on config)
- In `ListenAndServe()`, use `ListenAndServeTLS` when TLS config present
- Add HSTS middleware to chain when TLS active
- Start redirect server goroutine when TLS active

Modify `cmd/ari/run.go`:

- Call `resolveTLSConfig(cfg)` and pass to `server.New()`

#### Files

- Create: `internal/server/tls.go`
- Create: `internal/server/tls_test.go`
- Modify: `internal/server/server.go`
- Modify: `cmd/ari/run.go`

---

### [ ] Task 05 — Database Migration: OAuth Connections Table

**Requirements:** REQ-HARD-001, REQ-HARD-002, REQ-HARD-003, REQ-HARD-004
**Estimated time:** 20 min

#### Context

Create the `oauth_connections` table with unique constraints, indexes, and a per-table `update_oauth_connections_updated_at()` trigger function (matching the existing pattern used by agents, issues, projects, goals, pipelines). This migration must run after all existing migrations (current latest: 000018).

#### RED — Write Failing Tests

Add assertions to migration smoke tests:

1. After `RunMigrations()`, the table `oauth_connections` exists with expected columns.
2. Unique constraint `uq_oauth_user_provider` on `(user_id, provider)` exists.
3. Unique constraint `uq_oauth_provider_identity` on `(provider, provider_user_id)` exists.
4. Index `idx_oauth_provider_email` exists.
5. CHECK constraint on `provider` column allows only `google` and `github`.

#### GREEN — Implement

Create `internal/database/migrations/20260316000020_create_oauth_connections.sql` with schema from design.md section 2.

#### Files

- Create: `internal/database/migrations/20260316000020_create_oauth_connections.sql`
- Modify: `internal/database/database_test.go` (add migration assertions)

---

### [ ] Task 06 — SQL Queries and sqlc: OAuth Connections

**Requirements:** REQ-HARD-002, REQ-HARD-012, REQ-HARD-013, REQ-HARD-014
**Estimated time:** 30 min

#### Context

Write sqlc query definitions for OAuth connection CRUD: create, get by provider identity, list by user, delete. **Note:** `GetUserByEmail` already exists in `internal/database/queries/users.sql` -- do NOT duplicate it. The OAuth service will use the existing query. Run `make sqlc` to generate Go code.

#### RED — Write Failing Tests

Write `internal/database/db/oauth_connections_test.go`:

1. `TestCreateOAuthConnection` — insert, verify all fields.
2. `TestGetOAuthConnectionByProviderIdentity` — insert and retrieve by provider + provider_user_id.
3. `TestGetOAuthConnectionsByUserID` — insert multiple providers for one user, verify list.
4. `TestDeleteOAuthConnection` — insert and delete, verify gone.
5. `TestUniqueConstraint_UserProvider` — second insert for same user+provider fails.
6. `TestUniqueConstraint_ProviderIdentity` — second insert for same provider+provider_user_id fails.

#### GREEN — Implement

Create `internal/database/queries/oauth_connections.sql` with queries from design.md section 3. Run `make sqlc`.

#### Files

- Create: `internal/database/queries/oauth_connections.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/oauth_connections_test.go`

---

### [ ] Task 07 — OAuth Service: Provider Configuration and Flow Logic

**Requirements:** REQ-HARD-010, REQ-HARD-011, REQ-HARD-012, REQ-HARD-013, REQ-HARD-014, REQ-HARD-015, REQ-HARD-016, REQ-HARD-017, REQ-HARD-NF-001
**Estimated time:** 90 min

#### Context

Implement the OAuth2 service that orchestrates the login flow: generating authorization URLs, handling callbacks, exchanging codes for tokens, resolving provider identities, finding or creating Ari users, encrypting tokens, and issuing JWT sessions. Uses `golang.org/x/oauth2` for the protocol flow.

#### RED — Write Failing Tests

Write `internal/auth/oauth_test.go`:

1. `TestOAuthService_StartFlow_Google` — verify redirect URL contains correct scopes, state param, and callback URL.
2. `TestOAuthService_StartFlow_GitHub` — verify redirect URL for GitHub provider.
3. `TestOAuthService_StartFlow_DisabledProvider` — verify error when provider not configured.
4. `TestOAuthService_Callback_ExistingConnection` — mock token exchange, verify user found via oauth_connections, JWT issued.
5. `TestOAuthService_Callback_EmailMatch` — mock token exchange, no connection exists but email matches existing user, verify connection created and JWT issued.
6. `TestOAuthService_Callback_NewUser` — mock token exchange, no connection and no email match, verify user and connection created, JWT issued.
7. `TestOAuthService_Callback_SignupDisabled_NewUser` — no existing match + signup disabled, verify 403 error.
8. `TestOAuthService_Callback_SignupDisabled_EmailMatch` — existing user email match + signup disabled, verify connection created and login succeeds (linking is always allowed).
9. `TestOAuthService_Callback_StateMismatch` — verify 400 error when state param doesn't match cookie.
10. `TestOAuthService_TokenEncryption` — verify tokens encrypted at rest and can be decrypted with same key.
11. `TestOAuthService_KeyDerivation` — verify HKDF produces deterministic key from JWT secret (fallback) and from master key (preferred).

#### GREEN — Implement

Create `internal/auth/oauth.go`:

- `OAuthService` struct with providers map, queries, db, jwtSvc, sessions, encKey
- `NewOAuthService(...)` constructor — configures `oauth2.Config` for each enabled provider
- `StartFlow(provider string) (redirectURL, state string, err error)` — generates state, builds auth URL
- `HandleCallback(ctx, provider, code, state, expectedState string) (token string, err error)` — exchanges code, fetches userinfo, finds/creates user, creates connection, issues JWT
- `fetchGoogleUserInfo(ctx, token) (email, name, sub string, err error)` — calls Google userinfo API
- `fetchGitHubUserInfo(ctx, token) (email, name, sub string, err error)` — calls GitHub user + emails API
- `encryptToken(plaintext []byte) ([]byte, error)` — AES-256-GCM encrypt
- `decryptToken(ciphertext []byte) ([]byte, error)` — AES-256-GCM decrypt
- `deriveEncryptionKey(masterKey, jwtSecret []byte) ([]byte, error)` — Uses `ARI_MASTER_KEY` (Feature 19) if available; falls back to HKDF-SHA256 from JWT secret with salt `[]byte("ari-oauth-v1")` and info `"ari-oauth-token-encryption"`

#### Files

- Create: `internal/auth/oauth.go`
- Create: `internal/auth/oauth_test.go`

---

### [ ] Task 08 — OAuth Handler: HTTP Endpoints and Registration

**Requirements:** REQ-HARD-010, REQ-HARD-011, REQ-HARD-016, REQ-HARD-019, REQ-HARD-020
**Estimated time:** 45 min

#### Context

Create the HTTP handler that exposes OAuth endpoints: start flow, callback, and providers list. Wire into the server router. Add OAuth callback and providers to `publicEndpoints` in middleware. Set and validate the CSRF state cookie.

#### RED — Write Failing Tests

Write `internal/auth/oauth_handler_test.go` (or extend `internal/server/handlers/auth_handler_test.go`):

1. `TestOAuthStart_Google` — GET `/api/auth/oauth/google/start`, verify 302 redirect to Google, state cookie set.
2. `TestOAuthStart_GitHub` — GET `/api/auth/oauth/github/start`, verify 302 redirect to GitHub.
3. `TestOAuthStart_DisabledProvider` — GET `/api/auth/oauth/google/start` when Google not configured, verify 404.
4. `TestOAuthStart_InvalidProvider` — GET `/api/auth/oauth/invalid/start`, verify 404.
5. `TestOAuthCallback_Success` — GET `/api/auth/oauth/google/callback?code=X&state=Y` with matching state cookie, verify 302 redirect to SPA with session cookie.
6. `TestOAuthCallback_StateMismatch` — verify 400 with `OAUTH_STATE_INVALID`.
7. `TestOAuthCallback_NoCode` — verify 400.
8. `TestOAuthProviders_Endpoint` — GET `/api/auth/providers`, verify JSON response with enabled providers list.
9. `TestOAuth_RequiresAuthenticatedMode` — verify OAuth endpoints return 404 in `local_trusted` mode.

#### GREEN — Implement

Create OAuth handler (in `internal/auth/oauth_handler.go` or extend `internal/server/handlers/auth_handler.go`):

- `HandleOAuthStart(w, r)` — extract provider from URL, call `oauthSvc.StartFlow()`, set state cookie (Secure flag only when TLS is active: check `r.TLS != nil` or config), redirect
- `HandleOAuthCallback(w, r)` — extract provider/code/state, read state cookie, call `oauthSvc.HandleCallback()`, set session cookie, redirect to SPA (URL derived from: `ARI_DOMAIN` if set, else request Host header, else `localhost:3100`)
- `HandleProviders(w, r)` — return list of enabled providers

Modify `internal/auth/middleware.go`:

- Add `/api/auth/providers` to `publicEndpoints`
- Add `/api/auth/oauth/` prefix handling for start and callback endpoints (all OAuth routes are unauthenticated by design)

Wire in `cmd/ari/run.go`:

- Create `OAuthService` if any provider is configured
- Register OAuth routes

#### Files

- Create: `internal/auth/oauth_handler.go` (or modify auth_handler.go)
- Create: `internal/auth/oauth_handler_test.go`
- Modify: `internal/auth/middleware.go` (public endpoints)
- Modify: `cmd/ari/run.go` (wiring)

---

### [ ] Task 09 — CLI: Backup and Restore Commands

**Requirements:** REQ-HARD-030, REQ-HARD-031, REQ-HARD-032, REQ-HARD-033, REQ-HARD-034, REQ-HARD-035, REQ-HARD-036, REQ-HARD-037, REQ-HARD-038, REQ-HARD-NF-004
**Estimated time:** 60 min

#### Context

Add `ari backup` and `ari restore` Cobra commands. Both commands detect embedded vs external PostgreSQL and locate the appropriate `pg_dump`/`psql` binaries. Backup streams output to file. Restore requires `--confirm` flag and runs migrations afterward.

#### RED — Write Failing Tests

Write `cmd/ari/backup_test.go` and `cmd/ari/restore_test.go`:

1. `TestBackupCmd_Flags` — verify `--output` and `--format` flags are registered and have correct defaults.
2. `TestBackupCmd_DefaultOutputFilename` — verify default filename includes timestamp pattern.
3. `TestBackupCmd_FindPgDump_Embedded` — mock embedded PG binary discovery (verify path resolution logic).
4. `TestBackupCmd_FindPgDump_External` — verify uses `exec.LookPath("pg_dump")`.
5. `TestBackupCmd_PgDumpNotFound` — verify clear error message.
6. `TestRestoreCmd_RequiresInput` — verify error when `--input` not provided.
7. `TestRestoreCmd_RequiresConfirm` — verify warning printed and exit when `--confirm` not provided.
8. `TestRestoreCmd_Flags` — verify `--input` and `--confirm` flags registered.

#### GREEN — Implement

Create `cmd/ari/backup.go`:

- `newBackupCmd() *cobra.Command` with flags
- `runBackup(cmd, args) error` — load config, find pg_dump, execute with connection args, stream to file
- `findPgDump(cfg *config.Config) (string, error)` — embedded or system binary
- `buildPgDumpArgs(cfg *config.Config, output, format string) []string` — includes `--clean` flag to add DROP statements, so `ari restore` works without manual cleanup

Create `cmd/ari/restore.go`:

- `newRestoreCmd() *cobra.Command` with flags
- `runRestore(cmd, args) error` — validate confirm flag, load config, find psql, execute import, run migrations
- `findPsql(cfg *config.Config) (string, error)`

Modify `cmd/ari/main.go`:

- Register `backupCmd` and `restoreCmd` on root command

#### Files

- Create: `cmd/ari/backup.go`
- Create: `cmd/ari/restore.go`
- Create: `cmd/ari/backup_test.go`
- Create: `cmd/ari/restore_test.go`
- Modify: `cmd/ari/main.go` (register commands)

---

### [ ] Task 10 — Docker Image and Compose

**Requirements:** REQ-HARD-040, REQ-HARD-041, REQ-HARD-042, REQ-HARD-043, REQ-HARD-044, REQ-HARD-045, REQ-HARD-NF-005
**Estimated time:** 45 min

#### Context

Create a multi-stage Dockerfile and Docker Compose configuration. The Dockerfile builds Go binary and React UI in separate stages, then copies into a minimal Alpine runtime. Runs as non-root user (`ari`, UID 1000). Docker Compose defaults to external PostgreSQL profile (recommended for Docker). The embedded profile is available but requires internet access (or pre-downloaded binaries) since embedded-postgres-go downloads PG binaries on first run.

#### RED — Write Failing Tests

1. Verify `Dockerfile` builds successfully: `docker build -t ari:test .`
2. Verify image size is under 500MB: `docker image inspect ari:test --format '{{.Size}}'`
3. Verify health check works: `docker run -d --name ari-test ari:test && sleep 10 && docker inspect --format='{{.State.Health.Status}}' ari-test`
4. Verify `docker-compose --profile embedded up` starts successfully.
5. Verify `docker-compose --profile external up` starts Ari + PostgreSQL.

(These are manual/CI verification steps, not Go unit tests.)

#### GREEN — Implement

Create project root files:

- `Dockerfile` — multi-stage build per design.md section 6.1
- `docker-compose.yml` — per design.md section 6.2
- `.dockerignore` — per design.md section 6.3

#### Files

- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`

---

### [ ] Task 11 — CLI: Onboarding Wizard (`ari init`)

**Requirements:** REQ-HARD-050, REQ-HARD-051, REQ-HARD-052, REQ-HARD-053, REQ-HARD-054, REQ-HARD-055, REQ-HARD-056
**Estimated time:** 60 min

#### Context

Implement the interactive `ari init` wizard that guides users through initial setup. Prompts for deployment mode, port, data directory, TLS preference, and (in authenticated mode) admin credentials. Generates `ari.yaml` config file, runs migrations, and creates the admin user.

#### RED — Write Failing Tests

Write `cmd/ari/init_test.go`:

1. `TestInitCmd_Flags` — verify `--config` flag registered with default `./ari.yaml`.
2. `TestInitCmd_GenerateConfig_LocalTrusted` — provide mock stdin, verify generated YAML has correct deployment mode, port, data dir.
3. `TestInitCmd_GenerateConfig_Authenticated` — verify generated YAML includes admin email (not password).
4. `TestInitCmd_OverwriteProtection` — verify prompt when config file exists.
5. `TestConfigFileFormat` — verify generated YAML can be parsed back into a valid config struct.
6. `TestInitCmd_NonInteractive` — verify `--non-interactive` flag with all required flags works without prompts (for CI/scripting).

#### GREEN — Implement

Create `cmd/ari/init.go`:

- `newInitCmd() *cobra.Command` with `--config`, `--non-interactive` flags
- `runInit(cmd, args) error` — interactive flow using `bufio.Scanner` on stdin
- `promptChoice(prompt string, options []string) string`
- `promptString(prompt, defaultVal string) string`
- `promptPassword(prompt string) string`
- `generateConfigYAML(opts InitOptions) ([]byte, error)`
- `InitOptions` struct holding all wizard answers

Uses `gopkg.in/yaml.v3` for config file generation.

Modify `cmd/ari/main.go`:

- Register `initCmd` on root command

#### Files

- Create: `cmd/ari/init.go`
- Create: `cmd/ari/init_test.go`
- Modify: `cmd/ari/main.go` (register command)

---

### [ ] Task 12 — Integration Tests: Full Production Hardening Flow

**Requirements:** All requirements (end-to-end coverage)
**Estimated time:** 60 min

#### Context

Write comprehensive integration tests covering the full production hardening feature set: OAuth login flow (with mocked providers), rate limiting behavior under load, TLS server startup, backup/restore round-trip, and config file generation. These tests verify all components work together.

#### RED — Write Failing Tests

Create `internal/server/hardening_integration_test.go`:

1. `TestOAuthFlow_EndToEnd` — start server in authenticated mode with mock OAuth provider, complete full login flow (start → callback → session), verify user created with oauth_connection.
2. `TestOAuthFlow_ExistingUser_EmailLink` — OAuth callback with email matching existing user, verify connection created and linked.
3. `TestOAuthFlow_SignupDisabled` — OAuth callback with unknown email + signup disabled, verify 403.
4. `TestRateLimiting_EndToEnd` — send requests above burst limit from same IP, verify 429 with Retry-After. Verify different IP not affected.
5. `TestRateLimiting_AuthStricter` — verify auth endpoints hit limit before general endpoints.
6. `TestProvidersEndpoint` — verify `/api/auth/providers` returns correct enabled state.
7. `TestBodySizeLimit_Default` — verify 1MB+ body returns 413.
8. `TestHSTSHeader` — verify HSTS header present when TLS config provided.

Create `cmd/ari/backup_integration_test.go`:

9. `TestBackupRestore_RoundTrip` — start embedded PG, create data, run backup, wipe DB, run restore, verify data intact.
10. `TestBackup_OutputFile` — verify backup file created at specified path with valid SQL content.

Create `cmd/ari/init_integration_test.go`:

11. `TestInit_GeneratesValidConfig` — run wizard with piped input, verify config file created and parseable.

#### GREEN — Implement

Run all tests and verify they pass against implementations from Tasks 01-11.

#### REFACTOR

Review test coverage, ensure all error paths are exercised, verify requirement traceability.

#### Files

- Create: `internal/server/hardening_integration_test.go`
- Create: `cmd/ari/backup_integration_test.go`
- Create: `cmd/ari/init_integration_test.go`

---

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-HARD-001 | Task 05 |
| REQ-HARD-002 | Task 05, Task 06 |
| REQ-HARD-003 | Task 05 |
| REQ-HARD-004 | Task 05 |
| REQ-HARD-010 | Task 07, Task 08 |
| REQ-HARD-011 | Task 07, Task 08 |
| REQ-HARD-012 | Task 06, Task 07, Task 12 |
| REQ-HARD-013 | Task 06, Task 07, Task 12 |
| REQ-HARD-014 | Task 06, Task 07, Task 12 |
| REQ-HARD-015 | Task 07, Task 12 |
| REQ-HARD-016 | Task 08 |
| REQ-HARD-017 | Task 07 |
| REQ-HARD-018 | Task 01 |
| REQ-HARD-019 | Task 01, Task 08 |
| REQ-HARD-020 | Task 08, Task 12 |
| REQ-HARD-030 | Task 09 |
| REQ-HARD-031 | Task 09 |
| REQ-HARD-032 | Task 09 |
| REQ-HARD-033 | Task 09 |
| REQ-HARD-034 | Task 09 |
| REQ-HARD-035 | Task 09 |
| REQ-HARD-036 | Task 09 |
| REQ-HARD-037 | Task 09 |
| REQ-HARD-038 | Task 09 |
| REQ-HARD-040 | Task 10 |
| REQ-HARD-041 | Task 10 |
| REQ-HARD-042 | Task 10 |
| REQ-HARD-043 | Task 10 |
| REQ-HARD-044 | Task 10 |
| REQ-HARD-045 | Task 10 |
| REQ-HARD-050 | Task 11 |
| REQ-HARD-051 | Task 11 |
| REQ-HARD-052 | Task 11 |
| REQ-HARD-053 | Task 11 |
| REQ-HARD-054 | Task 11 |
| REQ-HARD-055 | Task 11 |
| REQ-HARD-056 | Task 11 |
| REQ-HARD-060 | Task 02 |
| REQ-HARD-061 | Task 01, Task 02 |
| REQ-HARD-062 | Task 02 |
| REQ-HARD-063 | Task 02 |
| REQ-HARD-064 | Task 02 |
| REQ-HARD-065 | Task 02 |
| REQ-HARD-066 | Task 02 |
| REQ-HARD-070 | Task 03 |
| REQ-HARD-071 | Task 03 |
| REQ-HARD-072 | Task 03 |
| REQ-HARD-080 | Task 04 |
| REQ-HARD-081 | Task 04 |
| REQ-HARD-082 | Task 04 |
| REQ-HARD-083 | Task 04, Task 12 |
| REQ-HARD-084 | Task 04 |
| REQ-HARD-085 | Task 04 |
| REQ-HARD-086 | Task 01, Task 04 |
| REQ-HARD-NF-001 | Task 07 |
| REQ-HARD-NF-002 | Task 02 |
| REQ-HARD-NF-003 | Task 02 |
| REQ-HARD-NF-004 | Task 09 |
| REQ-HARD-NF-005 | Task 10 |
| REQ-HARD-NF-006 | Task 04 |
