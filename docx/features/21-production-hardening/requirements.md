# Requirements: Production Hardening

**Created:** 2026-03-15
**Status:** Draft
**Feature:** 21-production-hardening
**Dependencies:** 02-user-authentication, 01-go-scaffold, 19-master-key (for ARI_MASTER_KEY; falls back to HKDF from JWT secret if not available)

## Overview

Production Hardening prepares Ari for real-world deployment by adding OAuth/SSO authentication (Google, GitHub), database backup/restore CLI commands, a Docker image with multi-stage build, an interactive CLI onboarding wizard (`ari init`), per-IP rate limiting on all API endpoints, request size limit configuration, and HTTPS/TLS support with optional Let's Encrypt auto-certificates. These capabilities close the gap between the current development-oriented setup and a production-grade self-hosted deployment.

## Scope

**In Scope:**
- OAuth2 login flow for Google and GitHub providers
- `oauth_connections` table linking external identities to Ari users
- CLI commands `ari backup` and `ari restore` for database dump/import
- Multi-stage Dockerfile producing a minimal runtime image
- Docker Compose file for dev with external PostgreSQL option
- Interactive CLI wizard `ari init` generating a config file
- Per-IP token-bucket rate limiting on all API endpoints
- Stricter rate limits on authentication endpoints
- Per-endpoint request body size overrides (extension of existing 1MB middleware)
- TLS termination with user-provided certificates
- Auto-TLS via Let's Encrypt (`autocert`) when a domain is configured
- HSTS headers when TLS is active

**Out of Scope (future):**
- SAML or OpenID Connect beyond OAuth2
- Enterprise SSO (Okta, Azure AD) -- extensible via the same OAuth2 pattern
- Scheduled/automated backup cron
- Kubernetes Helm chart or operator
- Multi-node / HA deployment
- Web Application Firewall (WAF) rules
- Client certificate (mTLS) authentication
- IP allow-list / deny-list

## Definitions

| Term | Definition |
|------|------------|
| OAuth Connection | A row in `oauth_connections` linking an Ari `user_id` to an external provider identity (`provider`, `provider_user_id`). |
| Provider | An OAuth2 identity provider (Google or GitHub). |
| Backup | A logical SQL dump of the PostgreSQL database produced by `pg_dump`. |
| Restore | Importing a previously-created SQL dump into PostgreSQL via `psql` or `pg_restore`. |
| Rate Limiter | A per-IP token-bucket that enforces requests-per-second (RPS) and burst limits. |
| Auto-TLS | Automatic TLS certificate provisioning via ACME / Let's Encrypt using `golang.org/x/crypto/acme/autocert`. |
| Onboarding Wizard | The interactive `ari init` command that generates an initial configuration file and creates the first admin user. |

## Requirements (EARS Format)

### 21.1 — OAuth/SSO Support

#### OAuth Connection Entity

**REQ-HARD-001:** WHEN an OAuth connection is created, the system SHALL assign a UUID as the primary key (`id`).

**REQ-HARD-002:** The system SHALL store the following fields on each OAuth connection: `id` (UUID), `user_id` (FK to users), `provider` (string, one of `google`, `github`), `provider_user_id` (string), `provider_email` (string), `access_token_encrypted` (bytea), `refresh_token_encrypted` (bytea, nullable), `created_at` (timestamp), `updated_at` (timestamp).

**REQ-HARD-003:** The system SHALL enforce a unique constraint on `(provider, provider_user_id)` to prevent duplicate provider links.

**REQ-HARD-004:** The system SHALL enforce a unique constraint on `(user_id, provider)` so a user can have at most one connection per provider.

#### OAuth Flow

**REQ-HARD-010:** The system SHALL expose `GET /api/auth/oauth/{provider}/start` which redirects the user to the provider's authorization URL with a CSRF `state` parameter stored in a short-lived cookie.

**REQ-HARD-011:** The system SHALL expose `GET /api/auth/oauth/{provider}/callback` which exchanges the authorization code for tokens, validates the `state` parameter against the cookie, and resolves the provider user identity.

**REQ-HARD-012:** WHEN the OAuth callback resolves a provider identity that matches an existing `oauth_connections` row, the system SHALL issue a JWT session for the linked Ari user and redirect to the SPA.

**REQ-HARD-013:** WHEN the OAuth callback resolves a provider identity with no existing connection BUT the provider email matches an existing Ari user email, the system SHALL create an `oauth_connections` row linking the provider to that user, issue a JWT session, and redirect to the SPA.

**REQ-HARD-014:** WHEN the OAuth callback resolves a provider identity with no existing connection AND no matching email, the system SHALL create a new Ari user (using provider email and display name), create the `oauth_connections` row, issue a JWT session, and redirect to the SPA.

**REQ-HARD-015:** IF `ARI_DISABLE_SIGNUP` is true AND no existing user matches the OAuth identity, THEN the system SHALL reject the login with HTTP 403 and code `SIGNUP_DISABLED`. Note: linking an OAuth provider to an existing user (matched by email) is always allowed regardless of `ARI_DISABLE_SIGNUP` -- only creating a brand new user is blocked.

**REQ-HARD-016:** The OAuth flow SHALL only be available when `ARI_DEPLOYMENT_MODE=authenticated`.

**REQ-HARD-017:** The system SHALL encrypt `access_token` and `refresh_token` at rest using AES-256-GCM with a key derived from `ARI_MASTER_KEY` (via Feature 19's MasterKeyManager). IF `ARI_MASTER_KEY` is not available (Feature 19 not yet implemented), the system SHALL fall back to HKDF-SHA256 from `ARI_JWT_SECRET` with salt `[]byte("ari-oauth-v1")` and info `"ari-oauth-token-encryption"`.

#### OAuth Configuration

**REQ-HARD-018:** The system SHALL read OAuth credentials from environment variables: `ARI_OAUTH_GOOGLE_CLIENT_ID`, `ARI_OAUTH_GOOGLE_CLIENT_SECRET`, `ARI_OAUTH_GITHUB_CLIENT_ID`, `ARI_OAUTH_GITHUB_CLIENT_SECRET`.

**REQ-HARD-019:** IF a provider's client ID and secret are both empty, THEN the system SHALL disable that provider's OAuth endpoints (return HTTP 404).

**REQ-HARD-020:** The system SHALL expose `GET /api/auth/providers` returning a list of enabled OAuth providers (no secrets), so the React UI can render the correct login buttons.

### 21.2 — Database Backup/Restore Commands

**REQ-HARD-030:** The system SHALL expose a `ari backup` CLI command that produces a logical SQL dump of the database.

**REQ-HARD-031:** `ari backup` SHALL accept `--output <path>` (default: `./ari-backup-{timestamp}.sql`) and `--format` (`plain` or `custom`, default `plain`).

**REQ-HARD-032:** WHEN using embedded PostgreSQL, `ari backup` SHALL locate and invoke the embedded `pg_dump` binary.

**REQ-HARD-033:** WHEN using an external PostgreSQL (`ARI_DATABASE_URL` set), `ari backup` SHALL shell out to the system `pg_dump` binary using the connection string.

**REQ-HARD-034:** IF the `pg_dump` binary is not found, THEN the system SHALL return a clear error message with installation instructions.

**REQ-HARD-035:** The system SHALL expose a `ari restore` CLI command that imports a SQL dump into the database.

**REQ-HARD-036:** `ari restore` SHALL accept `--input <path>` (required) and `--confirm` flag (required, to prevent accidental restores).

**REQ-HARD-037:** WHEN `--confirm` is not provided, `ari restore` SHALL print a warning and exit without modifying data.

**REQ-HARD-038:** `ari restore` SHALL run migrations after importing the dump to ensure schema is up to date.

### 21.3 — Docker Image

**REQ-HARD-040:** The project SHALL include a `Dockerfile` that produces a minimal runtime image containing the `ari` binary and embedded PostgreSQL binaries.

**REQ-HARD-041:** The `Dockerfile` SHALL use a multi-stage build: (1) Go build stage, (2) Node.js build stage for the React UI, (3) minimal runtime stage.

**REQ-HARD-042:** The runtime image SHALL expose port 3100 (default) and volume-mount `/data` for persistent storage.

**REQ-HARD-043:** The runtime image SHALL include a `HEALTHCHECK` instruction using `curl http://localhost:3100/api/health`.

**REQ-HARD-044:** The project SHALL include a `docker-compose.yml` with two profiles: `embedded` (default, single container) and `external` (Ari + external PostgreSQL container).

**REQ-HARD-045:** The Docker image size SHALL be under 500MB (compressed).

### 21.4 — CLI Onboarding Wizard (`ari init`)

**REQ-HARD-050:** The system SHALL expose an `ari init` CLI command that interactively configures a new Ari installation.

**REQ-HARD-051:** The wizard SHALL prompt for: deployment mode (`local_trusted` or `authenticated`), HTTP port (default 3100), data directory (default `./data`), and whether to enable TLS.

**REQ-HARD-052:** WHEN `authenticated` mode is selected, the wizard SHALL prompt for admin email and password, then create the initial admin user after running migrations.

**REQ-HARD-053:** The wizard SHALL generate a configuration file at the path specified by `--config` (default `./ari.yaml`).

**REQ-HARD-054:** The wizard SHALL run database migrations as part of initialization.

**REQ-HARD-055:** IF `ari.yaml` already exists, the wizard SHALL prompt for confirmation before overwriting.

**REQ-HARD-056:** The wizard SHALL print a summary of the configuration and a `ari run` command to start the server.

### 21.5 — Rate Limiting

**REQ-HARD-060:** The system SHALL enforce per-IP token-bucket rate limiting on all `/api/` endpoints.

**REQ-HARD-061:** The default rate limit SHALL be configurable via `ARI_RATE_LIMIT_RPS` (default 100 requests/second) and `ARI_RATE_LIMIT_BURST` (default 200).

**REQ-HARD-062:** Authentication endpoints (`/api/auth/login`, `/api/auth/register`, `/api/auth/oauth/*/callback`) SHALL have stricter rate limits: 10 RPS, burst 20.

**REQ-HARD-063:** WHEN a request exceeds the rate limit, the system SHALL return HTTP 429 with a `Retry-After` header indicating seconds until the next available token.

**REQ-HARD-064:** The system SHALL use the `X-Forwarded-For` header (first IP) ONLY when `RemoteAddr` is within the `ARI_TRUSTED_PROXIES` CIDR list, falling back to `RemoteAddr` otherwise. Default: empty (trust direct connection only).

**REQ-HARD-065:** The rate limiter SHALL evict stale entries (no requests in 10 minutes) to prevent memory growth.

**REQ-HARD-066:** The system SHALL add a global per-IP RPS rate limiter as a new middleware layer. The existing brute-force login rate limiter in `auth_handler.go` SHALL be kept as-is for login endpoints, providing defense-in-depth alongside the global limiter.

### 21.6 — Request Size Limits

**REQ-HARD-070:** The system SHALL enforce a default maximum request body size of 1MB on all endpoints (already implemented).

**REQ-HARD-071:** The system SHALL support per-route body size overrides via a middleware configuration map, allowing specific endpoints to accept larger payloads (e.g., file uploads up to 10MB).

**REQ-HARD-072:** WHEN the request body exceeds the configured limit, the system SHALL return HTTP 413 with error code `PAYLOAD_TOO_LARGE`.

### 21.7 — HTTPS/TLS Support

**REQ-HARD-080:** WHEN `ARI_TLS_CERT` and `ARI_TLS_KEY` environment variables are set, the system SHALL start the HTTP server with TLS using the provided certificate and key files.

**REQ-HARD-081:** WHEN `ARI_DOMAIN` is set and `ARI_TLS_CERT` is not set, the system SHALL use `golang.org/x/crypto/acme/autocert` to automatically provision and renew TLS certificates from Let's Encrypt.

**REQ-HARD-082:** WHEN auto-TLS is enabled, the system SHALL store certificates in `{ARI_DATA_DIR}/certs/`.

**REQ-HARD-083:** WHEN TLS is enabled (either mode), the system SHALL add `Strict-Transport-Security: max-age=63072000; includeSubDomains` to all responses.

**REQ-HARD-084:** WHEN TLS is enabled, the system SHALL start an HTTP-to-HTTPS redirect listener on port 80 (configurable via `ARI_TLS_REDIRECT_PORT`).

**REQ-HARD-085:** WHEN no TLS configuration is provided, the system SHALL fall back to plain HTTP (current behavior).

**REQ-HARD-086:** The system SHALL read TLS configuration from environment variables: `ARI_TLS_CERT`, `ARI_TLS_KEY`, `ARI_DOMAIN`, `ARI_TLS_REDIRECT_PORT`.

**REQ-HARD-087:** The system SHALL read `ARI_TRUSTED_PROXIES` (comma-separated CIDR list) to determine which proxy IPs are trusted for `X-Forwarded-For` parsing. Default: empty (trust direct connection only).

---

## Error Handling

| Scenario | HTTP Status | Error Code |
|----------|-------------|------------|
| OAuth provider not configured | 404 | `NOT_FOUND` |
| OAuth state mismatch (CSRF) | 400 | `OAUTH_STATE_INVALID` |
| OAuth code exchange failure | 502 | `OAUTH_EXCHANGE_FAILED` |
| OAuth signup disabled | 403 | `SIGNUP_DISABLED` |
| OAuth provider returns error | 400 | `OAUTH_PROVIDER_ERROR` |
| Rate limit exceeded | 429 | `RATE_LIMITED` |
| Request body too large | 413 | `PAYLOAD_TOO_LARGE` |
| Backup pg_dump not found | CLI error | N/A |
| Restore without --confirm | CLI error | N/A |
| TLS cert file not found | Startup error | N/A |
| TLS key file not found | Startup error | N/A |

---

## Non-Functional Requirements

**REQ-HARD-NF-001:** OAuth login flow (redirect + callback + session creation) SHALL complete within 3 seconds under normal network conditions to the provider.

**REQ-HARD-NF-002:** Rate limiter overhead SHALL add less than 1ms to request latency (in-memory lookup, no database queries).

**REQ-HARD-NF-003:** Rate limiter memory usage SHALL remain bounded (stale entry eviction every 10 minutes, per REQ-HARD-065).

**REQ-HARD-NF-004:** `ari backup` SHALL stream the dump to disk without loading the entire database into memory.

**REQ-HARD-NF-005:** Docker image SHALL build in under 5 minutes on a standard CI runner.

**REQ-HARD-NF-006:** TLS handshake SHALL complete within 500ms for user-provided certificates.

---

## Acceptance Criteria

1. Users can log in via Google OAuth and GitHub OAuth when credentials are configured
2. OAuth login creates or links an Ari user account and issues a valid JWT session
3. `GET /api/auth/providers` returns the list of enabled providers
4. `ari backup` produces a valid SQL dump that can be restored
5. `ari restore --input <file> --confirm` restores the database from a dump
6. `docker build` produces a working image under 500MB
7. `docker-compose up` starts Ari with embedded or external PostgreSQL
8. `ari init` walks through configuration and produces a valid `ari.yaml`
9. Per-IP rate limiting returns 429 with `Retry-After` when exceeded
10. Auth endpoints have stricter rate limits than general API endpoints
11. TLS works with user-provided cert/key files
12. Auto-TLS provisions certificates when `ARI_DOMAIN` is set
13. HSTS header is present on all responses when TLS is active
14. HTTP-to-HTTPS redirect works when TLS is enabled
15. Existing functionality is unaffected when no new configuration is provided

---

## References

- User Authentication: `docx/features/02-user-authentication/`
- Go Scaffold + CLI: `docx/features/01-go-scaffold/`
- Auth middleware: `internal/auth/middleware.go`
- Auth handler: `internal/server/handlers/auth_handler.go`
- Config: `internal/config/config.go`
- Server: `internal/server/server.go`
- CLI entrypoint: `cmd/ari/run.go`
- Existing rate limiter: `internal/auth/rate_limiter.go`
- Existing body size middleware: `internal/server/server.go` (line 64, `maxBodySize(1 << 20)`)
