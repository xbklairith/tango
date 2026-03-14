# Design: User Authentication

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 02-user-auth
**Dependencies:** 01-go-scaffold

---

## 1. Architecture Overview

User authentication is implemented as a layered auth package (`internal/auth/`) that plugs into the existing Go scaffold HTTP server via middleware. The auth layer supports two deployment modes: `local_trusted` (implicit full access, no credentials required) and `authenticated` (JWT + cookie sessions with bcrypt password verification). All auth state is stored in PostgreSQL via sqlc-generated queries, keeping the system stateless at the HTTP layer. The first registered user is automatically promoted to system admin. Auth endpoints are mounted under `/api/auth/` and the middleware wraps all other `/api/` routes.

## 2. System Context

- **Depends On:** 01-go-scaffold (HTTP server, router, middleware chain, database connection, config system, JSON response helpers, goose migrations, sqlc codegen)
- **Used By:** All future features requiring authenticated access (squad CRUD, agent CRUD, issue CRUD, React UI)
- **External Dependencies:** `golang.org/x/crypto/bcrypt` (password hashing), `github.com/golang-jwt/jwt/v5` (JWT minting/validation)

---

## 3. Component Structure

### 3.1 `internal/auth/jwt.go` — JWT Minting and Validation

**Responsibility:** Create and verify HS256-signed JWTs for user sessions.

**Dependencies:**
- `github.com/golang-jwt/jwt/v5`
- Server configuration (signing secret)

**Public Interface:**

```go
package auth

import (
    "time"

    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
)

// SessionClaims holds the JWT claims for a user session.
type SessionClaims struct {
    jwt.RegisteredClaims
    Email string `json:"email"`
}

// JWTService handles token creation and validation.
type JWTService struct {
    signingKey []byte        // minimum 32 bytes (256 bits)
    ttl        time.Duration // default 24h
}

// NewJWTService creates a JWTService with the given signing key and TTL.
// Returns an error if the key is shorter than 32 bytes.
func NewJWTService(signingKey []byte, ttl time.Duration) (*JWTService, error)

// Mint creates a new signed JWT for the given user.
// Sets sub=userID, email=email, iat=now, exp=now+ttl.
func (s *JWTService) Mint(userID uuid.UUID, email string) (string, error)

// Validate parses and validates a JWT string.
// Returns the claims if the token is valid, or an error indicating:
//   - ErrTokenMalformed: cannot parse
//   - ErrTokenExpired: token past expiration
//   - ErrTokenInvalid: signature mismatch or other validation failure
func (s *JWTService) Validate(tokenString string) (*SessionClaims, error)
```

**Key Behaviors:**
- Signing algorithm is strictly HS256; `Validate` rejects tokens signed with any other algorithm to prevent algorithm-switching attacks.
- `Mint` always sets `iat` and `exp` relative to `time.Now()`.
- `Validate` returns typed sentinel errors for downstream branching.

**Sentinel Errors:**

```go
var (
    ErrTokenMalformed = errors.New("auth: token is malformed")
    ErrTokenExpired   = errors.New("auth: token has expired")
    ErrTokenInvalid   = errors.New("auth: token is invalid")
)
```

---

### 3.2 `internal/auth/password.go` — Password Hashing

**Responsibility:** Hash and verify passwords using bcrypt.

**Dependencies:**
- `golang.org/x/crypto/bcrypt`

**Public Interface:**

```go
package auth

// BcryptCost is the bcrypt work factor. Minimum 10 for production.
const BcryptCost = 10

// HashPassword returns the bcrypt hash of the plaintext password.
func HashPassword(password string) (string, error)

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil on match, bcrypt.ErrMismatchedHashAndPassword on mismatch.
// Comparison is constant-time (guaranteed by bcrypt internals).
func CheckPassword(hash, password string) error

// ValidatePasswordStrength checks that the password meets policy:
//   - At least 8 characters
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
// Returns a slice of human-readable rule violations (empty = valid).
func ValidatePasswordStrength(password string) []string
```

**Key Behaviors:**
- `HashPassword` uses `bcrypt.GenerateFromPassword` with cost = `BcryptCost`.
- `CheckPassword` wraps `bcrypt.CompareHashAndPassword`; constant-time comparison is intrinsic to bcrypt.
- `ValidatePasswordStrength` returns all failing rules at once so the client can display them together.

---

### 3.3 `internal/auth/middleware.go` — Authentication Middleware

**Responsibility:** Intercept HTTP requests, enforce auth in `authenticated` mode, skip auth in `local_trusted` mode, inject user identity into request context.

**Dependencies:**
- `JWTService` (token validation)
- `SessionStore` (session lookup)
- Server configuration (deployment mode)

**Public Interface:**

```go
package auth

import (
    "context"
    "net/http"

    "github.com/google/uuid"
)

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "auth_user"

// Identity represents the authenticated user injected into the request context.
type Identity struct {
    UserID uuid.UUID
    Email  string
}

// UserFromContext extracts the authenticated Identity from the request context.
// Returns the Identity and true if present, or zero-value and false if not.
func UserFromContext(ctx context.Context) (Identity, bool)

// Middleware returns an http.Handler that enforces authentication.
//
// Behavior by deployment mode:
//   - local_trusted: passes all requests through, injecting a synthetic
//     local operator Identity (zero UUID, email "local@ari.local").
//   - authenticated: extracts JWT from cookie or Authorization header,
//     validates it, verifies the session exists in the database,
//     and injects the Identity. Returns 401 on failure.
//
// Public endpoints (register, login, health) are skipped regardless of mode.
func Middleware(
    mode     DeploymentMode,
    jwtSvc   *JWTService,
    sessions SessionStore,
) func(next http.Handler) http.Handler
```

**Public Endpoints (skip list):**

```go
// publicEndpoints are paths that never require authentication.
var publicEndpoints = map[string]map[string]bool{
    "/api/auth/register": {"POST": true},
    "/api/auth/login":    {"POST": true},
    "/api/health":        {"GET": true},
}
```

**Token Extraction Order:**
1. Check `ari_session` cookie.
2. If no cookie, check `Authorization: Bearer <token>` header.
3. If neither, return 401 `UNAUTHENTICATED`.

**Key Behaviors:**
- In `local_trusted` mode, a synthetic `Identity{UserID: uuid.Nil, Email: "local@ari.local"}` is injected into every request context.
- In `authenticated` mode, after JWT validation, the middleware queries the sessions table to confirm the session has not been revoked.
- If the JWT is expired, returns `TOKEN_EXPIRED`; if malformed or revoked, returns `INVALID_TOKEN`.

---

### 3.4 `internal/auth/session.go` — Session Management

**Responsibility:** Create, look up, and revoke database-backed sessions. Session records tie a JWT to a user and enable server-side revocation.

**Dependencies:**
- sqlc-generated queries (`internal/database/db`)
- `crypto/sha256` for token hashing

**Public Interface:**

```go
package auth

import (
    "context"
    "time"

    "github.com/google/uuid"
)

// SessionStore defines the interface for session persistence.
// Backed by PostgreSQL via sqlc queries.
type SessionStore interface {
    // Create persists a new session. tokenHash is SHA-256(JWT).
    Create(ctx context.Context, params CreateSessionParams) (Session, error)

    // FindByTokenHash retrieves an active (non-expired) session by its token hash.
    // Returns ErrSessionNotFound if no matching active session exists.
    FindByTokenHash(ctx context.Context, tokenHash string) (Session, error)

    // DeleteByID removes a single session (logout).
    DeleteByID(ctx context.Context, id uuid.UUID) error

    // DeleteByUserID removes all sessions for a user (force logout).
    DeleteByUserID(ctx context.Context, userID uuid.UUID) error

    // DeleteExpired removes all sessions past their expiresAt (cleanup).
    DeleteExpired(ctx context.Context) (int64, error)
}

// CreateSessionParams holds the data needed to create a session.
type CreateSessionParams struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    TokenHash string
    ExpiresAt time.Time
}

// Session represents a persisted session record.
type Session struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    TokenHash string
    ExpiresAt time.Time
    CreatedAt time.Time
}

// HashToken returns the hex-encoded SHA-256 hash of a raw JWT string.
// Used to store and look up sessions without persisting the raw token.
func HashToken(rawToken string) string

var ErrSessionNotFound = errors.New("auth: session not found")
```

**Key Behaviors:**
- Raw JWT tokens are never stored in the database; only their SHA-256 hash is persisted.
- `FindByTokenHash` only returns sessions where `expires_at > NOW()`.
- `DeleteExpired` is called by a background goroutine on a configurable interval (default: 1 hour).
- Multiple concurrent sessions per user are supported (different browsers/devices).

---

### 3.5 `internal/auth/ratelimit.go` — Login Rate Limiter

**Responsibility:** Limit failed login attempts per IP to prevent brute-force attacks.

**Public Interface:**

```go
package auth

import (
    "net/http"
    "sync"
    "time"
)

// RateLimiter tracks failed login attempts per IP address.
// Uses an in-memory sliding window (no external dependencies).
type RateLimiter struct {
    mu       sync.Mutex
    attempts map[string][]time.Time // IP -> timestamps of failed attempts
    limit    int                    // max failures per window (default: 10)
    window   time.Duration          // sliding window size (default: 1 minute)
}

// NewRateLimiter creates a rate limiter with the given limit and window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter

// Allow checks whether the given IP is within the rate limit.
// Returns true if the request should proceed, false if rate-limited.
func (rl *RateLimiter) Allow(ip string) bool

// CheckAndRecord atomically checks the rate limit and records a failed attempt
// if within the limit. Acquires the mutex once to prevent TOCTOU races between
// separate Allow + Record calls. Returns true if the request should proceed
// (attempt was recorded), false if rate-limited.
func (rl *RateLimiter) CheckAndRecord(ip string) bool

// Record adds a failed attempt for the given IP.
func (rl *RateLimiter) Record(ip string)

// Reset clears the failed attempt history for the given IP (called on successful login).
func (rl *RateLimiter) Reset(ip string)
```

**Key Behaviors:**
- Sliding window: only attempts within the last `window` duration count.
- `CheckAndRecord` should be preferred over separate `Allow` + `Record` calls to avoid TOCTOU races under concurrent requests from the same IP.
- On successful login, the counter for that IP is reset.
- A background cleanup goroutine prunes stale entries every 5 minutes to prevent memory growth.
- In-memory storage is acceptable for Phase 1 (single-instance deployment).

---

### 3.6 `internal/server/handlers/auth_handler.go` — Auth HTTP Handlers

**Responsibility:** Handle HTTP requests for registration, login, logout, and current-user endpoints.

**Dependencies:**
- `auth.JWTService`, `auth.SessionStore`, `auth.RateLimiter`
- sqlc-generated user queries
- Server configuration (deployment mode, `disableSignUp`)

**Public Interface:**

```go
package handlers

import (
    "net/http"

    "github.com/your-org/ari/internal/auth"
    "github.com/your-org/ari/internal/database/db"
)

// AuthHandler groups the auth-related HTTP handlers.
type AuthHandler struct {
    queries     *db.Queries
    jwt         *auth.JWTService
    sessions    auth.SessionStore
    rateLimiter *auth.RateLimiter
    mode        auth.DeploymentMode
    disableSignUp bool
    isSecure    bool           // true when not localhost (controls Secure cookie flag)
    sessionTTL  time.Duration  // configurable session/cookie TTL (default 24h from AuthConfig)
}

// NewAuthHandler creates an AuthHandler with all dependencies.
func NewAuthHandler(
    queries     *db.Queries,
    jwt         *auth.JWTService,
    sessions    auth.SessionStore,
    rateLimiter *auth.RateLimiter,
    mode        auth.DeploymentMode,
    disableSignUp bool,
    isSecure    bool,
    sessionTTL  time.Duration,
) *AuthHandler

// Register handles POST /api/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request)

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request)

// Logout handles POST /api/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request)

// Me handles GET /api/auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request)

// RegisterRoutes mounts auth handlers onto the provided mux.
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux)
```

**Route Registration:**

```go
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /api/auth/register", h.Register)
    mux.HandleFunc("POST /api/auth/login", h.Login)
    mux.HandleFunc("POST /api/auth/logout", h.Logout)
    mux.HandleFunc("GET /api/auth/me", h.Me)
}
```

---

### 3.7 `internal/auth/types.go` — Shared Types

```go
package auth

// DeploymentMode determines the authentication behavior of the server.
type DeploymentMode string

const (
    ModeLocalTrusted DeploymentMode = "local_trusted"
    ModeAuthenticated DeploymentMode = "authenticated"
)
```

---

## 4. Database Schema

### 4.1 Migration: `YYYYMMDDHHMMSS_create_users_and_sessions.sql`

```sql
-- +goose Up

CREATE TABLE users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email          TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    is_admin       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Case-insensitive unique index for email.
CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email));

CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
```

### 4.2 sqlc Queries: `internal/database/queries/users.sql`

```sql
-- name: CreateUser :one
INSERT INTO users (id, email, display_name, password_hash, status, is_admin)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, email, display_name, status, is_admin, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, display_name, password_hash, status, is_admin, created_at, updated_at
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
SELECT id, email, display_name, status, is_admin, created_at, updated_at
FROM users
WHERE id = $1;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2, updated_at = now() WHERE id = $1;
```

### 4.3 sqlc Queries: `internal/database/queries/sessions.sql`

```sql
-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, token_hash, expires_at, created_at;

-- name: GetSessionByTokenHash :one
SELECT id, user_id, token_hash, expires_at, created_at
FROM sessions
WHERE token_hash = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
```

---

## 5. API Contracts

### 5.1 POST /api/auth/register

**Purpose:** Create a new user account. The first user becomes system admin.

**Request:**

```go
type RegisterRequest struct {
    Email       string `json:"email"`
    DisplayName string `json:"displayName"`
    Password    string `json:"password"`
}
```

```json
{
    "email": "alice@example.com",
    "displayName": "Alice",
    "password": "SecurePass1"
}
```

**Response (201 Created):**

```json
{
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "alice@example.com",
    "displayName": "Alice",
    "status": "active",
    "isAdmin": true,
    "createdAt": "2026-03-14T12:00:00Z"
}
```

**Possible Errors:**
- `400 VALIDATION_ERROR` — Missing required field, invalid email format, or display name out of range (1-255 chars).
- `400 INVALID_PASSWORD` — Password does not meet strength requirements. Body includes `"details"` with list of failing rules.
- `403 REGISTRATION_DISABLED` — Sign-up is disabled and at least one user already exists.
- `409 EMAIL_EXISTS` — A user with this email already exists.

---

### 5.2 POST /api/auth/login

**Purpose:** Authenticate a user and issue a session.

**Request:**

```go
type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}
```

```json
{
    "email": "alice@example.com",
    "password": "SecurePass1"
}
```

**Response (200 OK):**

```json
{
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "email": "alice@example.com",
        "displayName": "Alice",
        "status": "active",
        "isAdmin": true
    }
}
```

Also sets the `ari_session` cookie (see Section 7 for cookie details).

**Possible Errors:**
- `400 VALIDATION_ERROR` — Missing email or password.
- `401 INVALID_CREDENTIALS` — Email not found or password mismatch (same error for both to prevent enumeration).
- `403 ACCOUNT_DISABLED` — User account is disabled.
- `429 RATE_LIMITED` — Too many failed login attempts from this IP.

---

### 5.3 POST /api/auth/logout

**Purpose:** Invalidate the current session and clear the cookie.

**Request:** No body required. Session identified by cookie or Authorization header.

**Response (200 OK):**

```json
{
    "message": "Logged out successfully"
}
```

Also clears the `ari_session` cookie by setting `Max-Age=0`.

**Possible Errors:**
- `401 UNAUTHENTICATED` — No valid session (still clears cookie as a best-effort).

---

### 5.4 GET /api/auth/me

**Purpose:** Return the profile of the currently authenticated user.

**Response (200 OK) — Authenticated Mode:**

```json
{
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "alice@example.com",
    "displayName": "Alice",
    "status": "active",
    "isAdmin": true,
    "createdAt": "2026-03-14T12:00:00Z"
}
```

**Response (200 OK) — Local Trusted Mode:**

```json
{
    "id": "00000000-0000-0000-0000-000000000000",
    "email": "local@ari.local",
    "displayName": "Local Operator",
    "status": "active",
    "isAdmin": true,
    "createdAt": "0001-01-01T00:00:00Z"
}
```

**Possible Errors:**
- `401 UNAUTHENTICATED` — No valid session (authenticated mode only).

---

## 6. Data Flow

### 6.1 Registration Flow

```
Client                         Server                              Database
  |                              |                                    |
  |-- POST /api/auth/register -->|                                    |
  |                              |-- Validate input (email, name, pw) |
  |                              |-- ValidatePasswordStrength(pw)     |
  |                              |-- HashPassword(pw)                 |
  |                              |-- BEGIN TX ----------------------->|
  |                              |-- CountUsers() ------------------>|
  |                              |<--- count -------------------------|
  |                              |   (check disableSignUp gate)       |
  |                              |   (if count=0, isAdmin=true)       |
  |                              |-- CreateUser(params) ------------>|
  |                              |<--- user (or EMAIL_EXISTS err) ----|
  |                              |-- COMMIT TX --------------------->|
  |<-- 201 {user} --------------|                                    |
```

**Detailed Steps:**

1. **Input Validation:** Parse JSON body. Validate email format (regexp), displayName length (1-255), and password strength rules.
2. **Hash Password:** Call `HashPassword(password)` with bcrypt cost 10 (done before the transaction to avoid holding the TX during bcrypt).
3. **Atomic First-User Check + Insert (single transaction):** Begin a database transaction wrapping `CountUsers` + `CreateUser` to prevent a race condition where two concurrent requests both see count=0 and both become admin.
   - **Sign-Up Gate:** If `disableSignUp` is true and count > 0, rollback and return `403 REGISTRATION_DISABLED`. If count = 0, allow (first user setup).
   - **First User Check:** If count = 0, set `isAdmin = true`.
   - **Insert User:** Call `CreateUser` with generated UUID. If the unique email index is violated, rollback and return `409 EMAIL_EXISTS`.
   - Commit transaction.
4. **Return:** Respond with 201 and the user object (no password hash).

---

### 6.2 Login Flow

```
Client                         Server                              Database
  |                              |                                    |
  |-- POST /api/auth/login ----->|                                    |
  |                              |-- RateLimiter.Allow(ip) ?          |
  |                              |-- Validate input                   |
  |                              |-- GetUserByEmail(email) --------->|
  |                              |<--- user (or not found) ----------|
  |                              |-- Check user.status != disabled    |
  |                              |-- CheckPassword(hash, pw)         |
  |                              |   (constant-time bcrypt compare)  |
  |                              |-- JWTService.Mint(userID, email)  |
  |                              |-- HashToken(jwt)                   |
  |                              |-- CreateSession(hash) ----------->|
  |                              |<--- session ----------------------|
  |                              |-- Set ari_session cookie           |
  |                              |-- RateLimiter.Reset(ip)           |
  |<-- 200 {token, user} -------|                                    |
```

**Detailed Steps:**

1. **Rate Limit Check:** Call `RateLimiter.Allow(ip)`. If denied, return `429 RATE_LIMITED`.
2. **Input Validation:** Parse JSON body. Validate email format and password presence.
3. **Lookup User:** Call `GetUserByEmail`. If not found, perform a dummy bcrypt compare (constant-time anti-enumeration), record failed attempt in rate limiter, then return `401 INVALID_CREDENTIALS`.
4. **Check Status:** If `user.Status == "disabled"`, return `403 ACCOUNT_DISABLED`. A disabled user always gets 403 regardless of password correctness.
5. **Verify Password:** Call `CheckPassword(user.PasswordHash, password)`. If mismatch, record failed attempt in rate limiter, return `401 INVALID_CREDENTIALS`.
6. **Mint JWT:** Call `JWTService.Mint(user.ID, user.Email)`. Token contains `sub`, `email`, `iat`, `exp` claims.
7. **Create Session:** Compute `HashToken(jwt)`, insert session record with `expiresAt = now + h.sessionTTL` (from `AuthConfig.SessionTTL`, default 24h).
8. **Set Cookie:** Set `ari_session` cookie with the raw JWT (see Section 7).
9. **Reset Rate Limiter:** Clear failed attempts for this IP.
10. **Return:** Respond with 200, the JWT token (for `Authorization` header use), and user profile.

---

### 6.3 Request Authentication Flow

```
Client                         Middleware                           Database
  |                              |                                    |
  |-- GET /api/squads ---------->|                                    |
  |                              |-- Check if public endpoint (no)    |
  |                              |-- Check deployment mode            |
  |                              |                                    |
  |  [local_trusted]             |                                    |
  |                              |-- Inject synthetic Identity        |
  |                              |-- next.ServeHTTP(w, r)             |
  |                              |                                    |
  |  [authenticated]             |                                    |
  |                              |-- Extract token from cookie/header |
  |                              |-- JWTService.Validate(token)       |
  |                              |-- HashToken(token)                 |
  |                              |-- FindByTokenHash(hash) --------->|
  |                              |<--- session (or not found) -------|
  |                              |-- Inject Identity into context     |
  |                              |-- next.ServeHTTP(w, r)             |
  |<-- 200 response ------------|                                    |
```

---

### 6.4 Logout Flow

```
Client                         Server                              Database
  |                              |                                    |
  |-- POST /api/auth/logout ---->|                                    |
  |                              |-- Extract Identity from context    |
  |                              |-- Extract token from cookie/header |
  |                              |-- HashToken(token)                 |
  |                              |-- FindByTokenHash(hash) --------->|
  |                              |<--- session ----------------------|
  |                              |-- DeleteByID(session.ID) -------->|
  |                              |-- Clear ari_session cookie         |
  |<-- 200 {message} -----------|                                    |
```

---

## 7. Security

### 7.1 Password Security

- **Hashing:** bcrypt with cost factor 10 (REQ-AUTH-033). Upgradeable by changing `BcryptCost` constant.
- **Constant-time comparison:** Guaranteed by `bcrypt.CompareHashAndPassword` internals (REQ-AUTH-120).
- **No plaintext storage:** Passwords exist in memory only during the hash operation; never logged, never returned in responses (REQ-AUTH-003, REQ-AUTH-004).
- **Strength validation:** Minimum 8 chars, at least one uppercase, one lowercase, one digit (REQ-AUTH-030, REQ-AUTH-031).

### 7.2 Anti-Enumeration

- Login returns the identical `INVALID_CREDENTIALS` error for both "email not found" and "wrong password" (REQ-AUTH-044).
- When email is not found, the server performs a dummy `bcrypt.CompareHashAndPassword` against a pre-computed hash to equalize response times:

```go
// dummyHash is a pre-computed bcrypt hash used when the user is not found,
// ensuring constant-time response regardless of whether the email exists.
var dummyHash []byte

func init() {
    var err error
    dummyHash, err = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), BcryptCost)
    if err != nil {
        panic(fmt.Sprintf("auth: failed to generate dummy hash: %v", err))
    }
}

func antiEnumerationCompare() {
    _ = bcrypt.CompareHashAndPassword(dummyHash, []byte("not-a-real-password"))
}
```

### 7.3 Cookie Flags

The `ari_session` cookie is configured as follows:

```go
http.Cookie{
    Name:     "ari_session",
    Value:    rawJWT,
    Path:     "/",                              // REQ-AUTH-073
    HttpOnly: true,                             // REQ-AUTH-070 — not accessible to JavaScript
    Secure:   h.isSecure,                       // REQ-AUTH-071 — true when not localhost
    SameSite: http.SameSiteLaxMode,             // REQ-AUTH-072
    MaxAge:   int(h.sessionTTL.Seconds()),      // dynamic, matches configured session TTL
}
```

- `isSecure` is determined at startup: `true` when the configured host is not `127.0.0.1` or `localhost`.

### 7.4 Rate Limiting

- In-memory sliding window: 10 failed login attempts per IP per 60-second window (REQ-AUTH-123).
- On rate limit exceeded: HTTP 429 with `RATE_LIMITED` error code (REQ-AUTH-124).
- On successful login: counter is reset for that IP.
- Stale entries pruned every 5 minutes.

### 7.5 Token Storage

- Raw JWTs are never stored in the database.
- Sessions store `SHA-256(JWT)` as the `token_hash` column.
- Lookup by hash enables server-side revocation without storing sensitive token material.

### 7.6 JWT Secret Key

- Minimum 256 bits (32 bytes) as required by REQ-AUTH-NFR-004.
- Loaded from `ARI_JWT_SECRET` environment variable.
- If not set at startup in `authenticated` mode, the server generates a cryptographically random 32-byte key and persists it to `{data_dir}/secrets/jwt.key`. The `secrets/` directory must be created with `os.MkdirAll(dir, 0700)` and the key file written with `os.WriteFile(path, key, 0600)` to restrict access to the owner only.
- The `JWTService` constructor rejects keys shorter than 32 bytes.

### 7.7 SQL Injection Prevention

- All database queries use sqlc-generated parameterized statements (REQ-AUTH-NFR-003).
- No raw string concatenation in SQL.

---

## 8. Deployment Modes

### 8.1 `local_trusted` (Default)

| Aspect | Behavior |
|--------|----------|
| Auth middleware | Passes all requests, injects synthetic Identity |
| Bind address | `127.0.0.1` only (REQ-AUTH-012) |
| Register/Login | Endpoints exist but are not required |
| `/api/auth/me` | Returns synthetic local operator (REQ-AUTH-101) |
| Sessions table | Not used |
| JWT minting | Not used |

**Synthetic Identity:**

```go
var LocalOperatorIdentity = Identity{
    UserID: uuid.Nil,
    Email:  "local@ari.local",
}
```

### 8.2 `authenticated`

| Aspect | Behavior |
|--------|----------|
| Auth middleware | Validates JWT + session on every protected request |
| Bind address | Configurable (default `0.0.0.0`) |
| Register/Login | Required; first user becomes admin |
| `/api/auth/me` | Returns real user profile from database |
| Sessions table | Active; tracks all sessions |
| Rate limiting | Active on login endpoint |

### 8.3 Configuration

Extends the existing scaffold configuration:

```go
// Added to internal/config/config.go

type AuthConfig struct {
    DeploymentMode auth.DeploymentMode `env:"ARI_DEPLOYMENT_MODE" default:"local_trusted"`
    JWTSecret      string              `env:"ARI_JWT_SECRET"`       // auto-generated if empty
    SessionTTL     time.Duration       `env:"ARI_SESSION_TTL" default:"24h"`
    DisableSignUp  bool                `env:"ARI_DISABLE_SIGNUP" default:"false"`
}
```

| Variable | Default | Description |
|----------|---------|-------------|
| `ARI_DEPLOYMENT_MODE` | `local_trusted` | `local_trusted` or `authenticated` |
| `ARI_JWT_SECRET` | (auto-generated) | HS256 signing key (hex-encoded, min 32 bytes) |
| `ARI_SESSION_TTL` | `24h` | JWT and session expiration duration |
| `ARI_DISABLE_SIGNUP` | `false` | Disable new user registration (except first user) |

---

## 9. Error Handling

### 9.1 Error Codes

| HTTP | Code | When |
|------|------|------|
| 400 | `VALIDATION_ERROR` | Missing/invalid field in request body |
| 400 | `INVALID_PASSWORD` | Password fails strength requirements |
| 401 | `UNAUTHENTICATED` | No token provided on protected endpoint |
| 401 | `INVALID_CREDENTIALS` | Wrong email or password on login |
| 401 | `INVALID_TOKEN` | Malformed JWT or revoked session |
| 401 | `TOKEN_EXPIRED` | JWT past expiration |
| 403 | `ACCOUNT_DISABLED` | Disabled user attempts login |
| 403 | `REGISTRATION_DISABLED` | Sign-up blocked by config |
| 409 | `EMAIL_EXISTS` | Duplicate email on registration |
| 429 | `RATE_LIMITED` | Too many failed login attempts |

### 9.2 Error Response Format

All errors follow the scaffold convention (REQ-SCAFFOLD-014):

```json
{
    "error": "Human-readable error message",
    "code": "MACHINE_READABLE_CODE"
}
```

For `INVALID_PASSWORD`, an additional `details` field lists failing rules:

```json
{
    "error": "Password does not meet requirements",
    "code": "INVALID_PASSWORD",
    "details": [
        "must be at least 8 characters",
        "must contain at least one uppercase letter"
    ]
}
```

---

## 10. Performance Considerations

### 10.1 Targets

| Operation | Target | Notes |
|-----------|--------|-------|
| Login (p95) | < 500ms | Dominated by bcrypt hash time (~100ms at cost 10) |
| Register (p95) | < 500ms | Same bcrypt bottleneck |
| Middleware (JWT validation) | < 5ms | Pure in-memory HMAC verification |
| Session lookup | < 10ms | Indexed query on `token_hash` |

### 10.2 Optimizations

- **JWT validation is stateless HMAC** — no database hit for the cryptographic check.
- **Session lookup is indexed** — `idx_sessions_token_hash` ensures O(log n) lookup.
- **Rate limiter is in-memory** — zero database overhead for rate checking.
- **Expired session cleanup** runs in background, not in the request path.

---

## 11. Testing Strategy

### 11.1 Unit Tests

**`internal/auth/jwt_test.go`:**

```go
func TestMint_ProducesValidToken(t *testing.T)
func TestMint_SetsCorrectClaims(t *testing.T)
func TestValidate_AcceptsValidToken(t *testing.T)
func TestValidate_RejectsExpiredToken(t *testing.T)
func TestValidate_RejectsMalformedToken(t *testing.T)
func TestValidate_RejectsWrongSigningKey(t *testing.T)
func TestValidate_RejectsNoneAlgorithm(t *testing.T)
func TestNewJWTService_RejectsShortKey(t *testing.T)
```

**`internal/auth/password_test.go`:**

```go
func TestHashPassword_ProducesBcryptHash(t *testing.T)
func TestCheckPassword_MatchesCorrectPassword(t *testing.T)
func TestCheckPassword_RejectsWrongPassword(t *testing.T)
func TestValidatePasswordStrength_AcceptsValid(t *testing.T)
func TestValidatePasswordStrength_RejectsTooShort(t *testing.T)
func TestValidatePasswordStrength_RejectsNoUppercase(t *testing.T)
func TestValidatePasswordStrength_RejectsNoLowercase(t *testing.T)
func TestValidatePasswordStrength_RejectsNoDigit(t *testing.T)
func TestValidatePasswordStrength_ReturnsAllViolations(t *testing.T)
```

**`internal/auth/middleware_test.go`:**

```go
func TestMiddleware_LocalTrusted_PassesAllRequests(t *testing.T)
func TestMiddleware_LocalTrusted_InjectsSyntheticIdentity(t *testing.T)
func TestMiddleware_Authenticated_RejectsNoToken(t *testing.T)
func TestMiddleware_Authenticated_AcceptsValidCookie(t *testing.T)
func TestMiddleware_Authenticated_AcceptsBearerHeader(t *testing.T)
func TestMiddleware_Authenticated_RejectsExpiredToken(t *testing.T)
func TestMiddleware_Authenticated_RejectsRevokedSession(t *testing.T)
func TestMiddleware_SkipsPublicEndpoints(t *testing.T)
func TestUserFromContext_ReturnsIdentity(t *testing.T)
func TestUserFromContext_ReturnsFalseWhenMissing(t *testing.T)
```

**`internal/auth/ratelimit_test.go`:**

```go
func TestRateLimiter_AllowsUnderLimit(t *testing.T)
func TestRateLimiter_BlocksOverLimit(t *testing.T)
func TestRateLimiter_SlidingWindowExpires(t *testing.T)
func TestRateLimiter_ResetClearsHistory(t *testing.T)
func TestRateLimiter_IndependentPerIP(t *testing.T)
```

**`internal/auth/session_test.go`:**

```go
func TestHashToken_DeterministicOutput(t *testing.T)
func TestHashToken_DifferentInputsDifferentHashes(t *testing.T)
```

### 11.2 Integration Tests (require database)

**`internal/server/handlers/auth_handler_test.go`:**

These tests use an embedded PostgreSQL instance (from the scaffold) with migrations applied.

```go
func TestRegister_Success_FirstUser_IsAdmin(t *testing.T)
func TestRegister_Success_SecondUser_NotAdmin(t *testing.T)
func TestRegister_DuplicateEmail_Returns409(t *testing.T)
func TestRegister_InvalidEmail_Returns400(t *testing.T)
func TestRegister_WeakPassword_Returns400(t *testing.T)
func TestRegister_MissingFields_Returns400(t *testing.T)
func TestRegister_DisabledSignUp_FirstUser_Allowed(t *testing.T)
func TestRegister_DisabledSignUp_SecondUser_Returns403(t *testing.T)

func TestLogin_Success_ReturnsTokenAndCookie(t *testing.T)
func TestLogin_WrongEmail_Returns401(t *testing.T)
func TestLogin_WrongPassword_Returns401(t *testing.T)
func TestLogin_SameErrorForBothWrongEmailAndPassword(t *testing.T)
func TestLogin_DisabledAccount_Returns403(t *testing.T)
func TestLogin_RateLimited_Returns429(t *testing.T)

func TestLogout_ClearsSessionAndCookie(t *testing.T)
func TestLogout_NoAuth_Returns401(t *testing.T)

func TestMe_Authenticated_ReturnsProfile(t *testing.T)
func TestMe_LocalTrusted_ReturnsSyntheticIdentity(t *testing.T)
func TestMe_NoAuth_Returns401(t *testing.T)

func TestFullFlow_RegisterLoginMeLogout(t *testing.T)
func TestMultipleSessions_IndependentLogout(t *testing.T)
```

### 11.3 Test Helpers

```go
// testutil/auth.go — shared test utilities

// SetupTestDB creates an embedded PostgreSQL instance with migrations applied.
// Returns db.Queries and a cleanup function.
func SetupTestDB(t *testing.T) (*db.Queries, func())

// MintTestToken creates a valid JWT for use in test requests.
func MintTestToken(t *testing.T, jwtSvc *auth.JWTService, userID uuid.UUID, email string) string

// NewAuthenticatedRequest creates an http.Request with a valid session cookie.
func NewAuthenticatedRequest(t *testing.T, method, path string, body io.Reader, token string) *http.Request
```

### 11.4 Coverage Target

80% code coverage across `internal/auth/` and `internal/server/handlers/auth_handler.go`.

---

## 12. Session Cleanup

A background goroutine runs `DeleteExpiredSessions` on a configurable interval:

```go
// StartSessionCleanup launches a goroutine that periodically deletes expired sessions.
// It respects context cancellation for graceful shutdown.
func StartSessionCleanup(ctx context.Context, store SessionStore, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            deleted, err := store.DeleteExpired(ctx)
            if err != nil {
                slog.Error("session cleanup failed", "error", err)
                continue
            }
            if deleted > 0 {
                slog.Info("cleaned up expired sessions", "count", deleted)
            }
        }
    }
}
```

- Default interval: 1 hour.
- Launched during server startup in `ari run`, cancelled on graceful shutdown.

---

## 13. Alternatives Considered

### Alternative 1: Opaque Session Tokens (no JWT)

**Description:** Use random opaque tokens stored in the database; every request hits the DB for validation.

**Pros:**
- Simpler implementation; no JWT library needed.
- Instant revocation without a session table lookup.

**Cons:**
- Every authenticated request requires a database query (no stateless path).
- Cannot support `Authorization: Bearer` tokens for API consumers without DB.

**Rejected Because:** JWT provides a stateless validation fast path for the common case, with the session table providing revocation capability. This hybrid approach balances performance and security.

### Alternative 2: Third-Party Auth Provider (e.g., Authelia, Keycloak)

**Description:** Delegate auth entirely to an external identity provider.

**Pros:**
- Rich feature set (MFA, social login, SAML).
- Less code to maintain.

**Cons:**
- Violates the "single binary, zero dependencies" principle (PRD 2.1.3).
- Adds operational complexity for self-hosted users.

**Rejected Because:** Ari's core value proposition is zero-dependency single-binary deployment. Built-in auth keeps the stack simple. External providers can be considered for a future enterprise tier.

### Alternative 3: Argon2id Instead of bcrypt

**Description:** Use Argon2id for password hashing (memory-hard, GPU-resistant).

**Pros:**
- Stronger resistance to GPU-based brute force.
- Configurable memory/parallelism parameters.

**Cons:**
- Less mature Go library ecosystem.
- More complex tuning (memory, iterations, parallelism).
- bcrypt is still considered secure and is the industry default.

**Rejected Because:** bcrypt at cost 10 meets security requirements and has battle-tested Go support via `golang.org/x/crypto/bcrypt`. Argon2id can be adopted later if threat model changes.

---

## 14. Timeline Estimate

- Requirements: 1 day (complete)
- Design: 1 day (this document)
- Implementation: 3-4 days
  - Day 1: Database migration + sqlc queries + password/JWT packages
  - Day 2: Session store + middleware + rate limiter
  - Day 3: Auth handlers + route registration + integration with scaffold
  - Day 4: Testing + edge cases + cleanup
- Testing: 1 day (integration tests, manual verification)
- Total: 6-7 days

---

## 15. References

- [Requirements](./requirements.md) — EARS requirements for 02-user-auth
- [Go Scaffold Requirements](../01-go-scaffold/requirements.md) — Foundation dependency
- [PRD Section 4.2](../../core/01-PRODUCT.md) — User entity definition
- [PRD Section 12.1](../../core/01-PRODUCT.md) — Authentication tiers
- [PRD Section 12.2](../../core/01-PRODUCT.md) — Security measures
- [PRD Section 11.2](../../core/01-PRODUCT.md) — Configuration schema
