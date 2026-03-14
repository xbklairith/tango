# Tasks: User Authentication

**Created:** 2026-03-14
**Status:** Not Started
**Feature:** 02-user-auth
**Dependencies:** 01-go-scaffold

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Technical design: [design.md](./design.md)
- Requirement coverage: REQ-AUTH-001 through REQ-AUTH-124, REQ-AUTH-NFR-001 through NFR-004
- Missing coverage: None — all requirements mapped below

## Implementation Approach

Components are built bottom-up: database schema first (foundation), then pure-logic packages (password, JWT, rate limiter), then session store (depends on DB), then middleware (depends on JWT + sessions), then HTTP handlers (depends on everything), and finally integration tests that exercise the full stack. Each task follows TDD Red-Green-Refactor. Dependencies between tasks are strictly ordered — later tasks build on earlier ones.

## Progress Summary

- Total Tasks: 28
- Completed: 0/28
- In Progress: None
- Test Coverage: 0%

---

## Tasks (TDD: Red-Green-Refactor)

### Component 1: Database Schema

#### Task 1.1: Create Users Table Migration

**Linked Requirements:** REQ-AUTH-001, REQ-AUTH-002, REQ-AUTH-NFR-003

**RED Phase:**
- [ ] Write a Go test that runs goose migrations and asserts the `users` table exists with expected columns (`id`, `email`, `display_name`, `password_hash`, `status`, `is_admin`, `created_at`, `updated_at`)
  - Test case: Query `information_schema.columns` for the `users` table after migration
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Create goose migration file `internal/database/migrations/YYYYMMDDHHMMSS_create_users.sql`
  - `users` table with UUID PK, `status` CHECK constraint (`active`, `disabled`), `is_admin` BOOLEAN, timestamps
  - Case-insensitive unique index: `CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email))`

**REFACTOR Phase:**
- [ ] Verify `+goose Down` correctly drops table and index
- [ ] Confirm migration is idempotent (up/down/up succeeds)

**Acceptance Criteria:**
- [ ] `users` table created with all columns from design Section 4.1
- [ ] Case-insensitive unique index on `lower(email)` prevents duplicate emails
- [ ] `status` column constrained to `active` or `disabled`
- [ ] `is_admin` defaults to `FALSE`
- [ ] Migration rolls back cleanly

---

#### Task 1.2: Create Sessions Table Migration

**Linked Requirements:** REQ-AUTH-090, REQ-AUTH-NFR-003

**RED Phase:**
- [ ] Write a test asserting the `sessions` table exists with columns (`id`, `user_id`, `token_hash`, `expires_at`, `created_at`) and foreign key to `users`
  - Expected failure: Table does not exist

**GREEN Phase:**
- [ ] Add sessions table to the same or a new migration file
  - `sessions` table with UUID PK, FK to `users(id)` with `ON DELETE CASCADE`
  - Indexes: `idx_sessions_token_hash`, `idx_sessions_user_id`, `idx_sessions_expires_at`

**REFACTOR Phase:**
- [ ] Verify cascade delete works (deleting a user removes their sessions)
- [ ] Confirm all three indexes exist

**Acceptance Criteria:**
- [ ] `sessions` table created with all columns from design Section 4.1
- [ ] Foreign key constraint with `ON DELETE CASCADE` enforced
- [ ] Three indexes created for token_hash, user_id, and expires_at
- [ ] Migration rolls back cleanly (sessions dropped before users)

---

#### Task 1.3: Write sqlc Queries for Users

**Linked Requirements:** REQ-AUTH-001, REQ-AUTH-002, REQ-AUTH-020, REQ-AUTH-NFR-003

**RED Phase:**
- [ ] Write tests for generated query functions: `CreateUser`, `GetUserByEmail`, `GetUserByID`, `CountUsers`, `UpdateUserStatus`
  - Test: Insert a user, retrieve by email (case-insensitive), retrieve by ID, count users, update status
  - Expected failure: Generated code does not exist yet

**GREEN Phase:**
- [ ] Create `internal/database/queries/users.sql` with all five queries (per design Section 4.2)
- [ ] Run `make sqlc` to generate Go code
- [ ] `CreateUser` returns all fields except `password_hash`
- [ ] `GetUserByEmail` returns `password_hash` (needed for login verification)
- [ ] `GetUserByID` excludes `password_hash` (safe for API responses)

**REFACTOR Phase:**
- [ ] Ensure query naming is consistent with project conventions
- [ ] Verify parameterized statements (no raw SQL concatenation)

**Acceptance Criteria:**
- [ ] `make sqlc` succeeds without errors
- [ ] All five query functions generated and callable
- [ ] `GetUserByEmail` uses `lower(email) = lower($1)` for case-insensitive lookup
- [ ] `CreateUser` RETURNING clause excludes `password_hash` from API-facing fields
- [ ] Tests pass against embedded PostgreSQL

---

#### Task 1.4: Write sqlc Queries for Sessions

**Linked Requirements:** REQ-AUTH-090, REQ-AUTH-091, REQ-AUTH-092, REQ-AUTH-094, REQ-AUTH-095

**RED Phase:**
- [ ] Write tests for: `CreateSession`, `GetSessionByTokenHash`, `DeleteSession`, `DeleteSessionsByUserID`, `DeleteExpiredSessions`
  - Test: Create session, find by token hash (only non-expired), delete single, delete all for user, delete expired
  - Expected failure: Generated code does not exist

**GREEN Phase:**
- [ ] Create `internal/database/queries/sessions.sql` with all five queries (per design Section 4.3)
- [ ] Run `make sqlc` to generate Go code
- [ ] `GetSessionByTokenHash` includes `WHERE expires_at > now()` filter
- [ ] `DeleteExpiredSessions` uses `:execrows` to return count of deleted rows

**REFACTOR Phase:**
- [ ] Confirm query performance with EXPLAIN on indexed columns
- [ ] Ensure consistent naming with users queries

**Acceptance Criteria:**
- [ ] All five session query functions generated and callable
- [ ] `GetSessionByTokenHash` only returns non-expired sessions
- [ ] `DeleteExpiredSessions` returns the count of deleted rows
- [ ] `DeleteSessionsByUserID` removes all sessions for a given user
- [ ] Cascade delete verified: deleting user removes associated sessions

---

### Component 2: Password Module

#### Task 2.1: Implement Password Strength Validation

**Linked Requirements:** REQ-AUTH-030, REQ-AUTH-031, REQ-AUTH-032

**RED Phase:**
- [ ] Write `internal/auth/password_test.go` with tests:
  - `TestValidatePasswordStrength_AcceptsValid` — "SecurePass1" returns empty slice
  - `TestValidatePasswordStrength_RejectsTooShort` — "Short1A" (7 chars) returns violation
  - `TestValidatePasswordStrength_RejectsNoUppercase` — "lowercase1" returns violation
  - `TestValidatePasswordStrength_RejectsNoLowercase` — "UPPERCASE1" returns violation
  - `TestValidatePasswordStrength_RejectsNoDigit` — "NoDigitHere" returns violation
  - `TestValidatePasswordStrength_ReturnsAllViolations` — "ab" returns all four violations
  - Expected failure: `ValidatePasswordStrength` function does not exist

**GREEN Phase:**
- [ ] Create `internal/auth/password.go`
- [ ] Implement `ValidatePasswordStrength(password string) []string`
  - Check length >= 8, has uppercase, has lowercase, has digit
  - Return all failing rules at once (not short-circuit)

**REFACTOR Phase:**
- [ ] Extract rule checks into a table-driven structure for extensibility
- [ ] Ensure error messages are human-readable and match design Section 9.2

**Acceptance Criteria:**
- [ ] Passwords under 8 characters rejected with descriptive message
- [ ] Missing uppercase, lowercase, or digit each produce specific messages
- [ ] All violations returned simultaneously (not one at a time)
- [ ] Valid passwords return empty slice
- [ ] All tests pass

---

#### Task 2.2: Implement Password Hashing and Verification

**Linked Requirements:** REQ-AUTH-003, REQ-AUTH-033, REQ-AUTH-120

**RED Phase:**
- [ ] Add tests to `internal/auth/password_test.go`:
  - `TestHashPassword_ProducesBcryptHash` — output starts with `$2a$` or `$2b$`
  - `TestHashPassword_CostFactor` — bcrypt cost is at least 10
  - `TestCheckPassword_MatchesCorrectPassword` — hash of "Test1234" matches "Test1234"
  - `TestCheckPassword_RejectsWrongPassword` — hash of "Test1234" does not match "Wrong999"
  - Expected failure: `HashPassword` and `CheckPassword` do not exist

**GREEN Phase:**
- [ ] Implement `HashPassword(password string) (string, error)` using `bcrypt.GenerateFromPassword` with `BcryptCost = 10`
- [ ] Implement `CheckPassword(hash, password string) error` wrapping `bcrypt.CompareHashAndPassword`

**REFACTOR Phase:**
- [ ] Confirm `BcryptCost` is exported as a constant for future adjustability
- [ ] Add doc comments explaining constant-time comparison guarantee

**Acceptance Criteria:**
- [ ] `HashPassword` produces valid bcrypt hash with cost >= 10
- [ ] `CheckPassword` returns nil for correct password, error for incorrect
- [ ] Constant-time comparison guaranteed by bcrypt internals
- [ ] No plaintext password stored or logged
- [ ] All tests pass

---

### Component 3: JWT Module

#### Task 3.1: Implement JWT Service Constructor and Key Validation

**Linked Requirements:** REQ-AUTH-063, REQ-AUTH-NFR-004

**RED Phase:**
- [ ] Write `internal/auth/jwt_test.go` with tests:
  - `TestNewJWTService_AcceptsValidKey` — 32-byte key accepted, no error
  - `TestNewJWTService_RejectsShortKey` — 31-byte key returns error
  - `TestNewJWTService_AcceptsLongerKey` — 64-byte key accepted
  - Expected failure: `NewJWTService` does not exist

**GREEN Phase:**
- [ ] Create `internal/auth/jwt.go`
- [ ] Define `JWTService` struct with `signingKey []byte` and `ttl time.Duration`
- [ ] Implement `NewJWTService(signingKey []byte, ttl time.Duration) (*JWTService, error)`
  - Reject keys shorter than 32 bytes

**REFACTOR Phase:**
- [ ] Define sentinel errors: `ErrTokenMalformed`, `ErrTokenExpired`, `ErrTokenInvalid`
- [ ] Add `SessionClaims` struct extending `jwt.RegisteredClaims` with `Email` field

**Acceptance Criteria:**
- [ ] Constructor rejects keys under 32 bytes with clear error message
- [ ] Constructor accepts keys of 32+ bytes
- [ ] Sentinel errors defined and exported
- [ ] `SessionClaims` struct defined with `Email` field

---

#### Task 3.2: Implement JWT Minting

**Linked Requirements:** REQ-AUTH-060, REQ-AUTH-061, REQ-AUTH-062

**RED Phase:**
- [ ] Add tests to `internal/auth/jwt_test.go`:
  - `TestMint_ProducesValidToken` — returns non-empty string, no error
  - `TestMint_SetsCorrectClaims` — parse token and verify `sub`, `email`, `iat`, `exp`
  - `TestMint_ExpirationMatchesTTL` — `exp - iat` equals configured TTL (24h)
  - Expected failure: `Mint` method does not exist

**GREEN Phase:**
- [ ] Implement `(s *JWTService) Mint(userID uuid.UUID, email string) (string, error)`
  - Create `SessionClaims` with `sub=userID.String()`, `email`, `iat=now`, `exp=now+ttl`
  - Sign with HS256 using `s.signingKey`

**REFACTOR Phase:**
- [ ] Verify token structure with manual decode (header.payload.signature)
- [ ] Ensure no unnecessary allocations

**Acceptance Criteria:**
- [ ] Minted token is a valid three-part JWT string
- [ ] `sub` claim contains user UUID as string
- [ ] `email` claim matches input
- [ ] `exp` is exactly `iat + ttl`
- [ ] Signing algorithm is HS256

---

#### Task 3.3: Implement JWT Validation

**Linked Requirements:** REQ-AUTH-064, REQ-AUTH-065

**RED Phase:**
- [ ] Add tests to `internal/auth/jwt_test.go`:
  - `TestValidate_AcceptsValidToken` — mint then validate returns claims, no error
  - `TestValidate_RejectsExpiredToken` — token with past expiration returns `ErrTokenExpired`
  - `TestValidate_RejectsMalformedToken` — garbage string returns `ErrTokenMalformed`
  - `TestValidate_RejectsWrongSigningKey` — token signed with different key returns `ErrTokenInvalid`
  - `TestValidate_RejectsNoneAlgorithm` — crafted `alg: none` token returns `ErrTokenInvalid`
  - Expected failure: `Validate` method does not exist

**GREEN Phase:**
- [ ] Implement `(s *JWTService) Validate(tokenString string) (*SessionClaims, error)`
  - Parse with `jwt.ParseWithClaims`, enforce `jwt.WithValidMethods([]string{"HS256"})`
  - Map parse errors to sentinel errors (`ErrTokenExpired`, `ErrTokenMalformed`, `ErrTokenInvalid`)

**REFACTOR Phase:**
- [ ] Consolidate error mapping logic into a helper function
- [ ] Ensure algorithm-switching attack is blocked (alg: none, RS256, etc.)

**Acceptance Criteria:**
- [ ] Valid tokens return correct `SessionClaims` with `UserID` and `Email`
- [ ] Expired tokens return `ErrTokenExpired`
- [ ] Malformed tokens return `ErrTokenMalformed`
- [ ] Wrong-key tokens return `ErrTokenInvalid`
- [ ] `alg: none` tokens rejected
- [ ] Only HS256 algorithm accepted

---

### Component 4: Session Store

#### Task 4.1: Implement Token Hashing Utility

**Linked Requirements:** REQ-AUTH-090 (token_hash field)

**RED Phase:**
- [ ] Write `internal/auth/session_test.go` with tests:
  - `TestHashToken_DeterministicOutput` — same input always produces same hash
  - `TestHashToken_DifferentInputsDifferentHashes` — different tokens produce different hashes
  - `TestHashToken_HexEncoded` — output is hex-encoded string of expected length (64 chars for SHA-256)
  - Expected failure: `HashToken` function does not exist

**GREEN Phase:**
- [ ] Create `internal/auth/session.go`
- [ ] Implement `HashToken(rawToken string) string` using `crypto/sha256` + `encoding/hex`

**REFACTOR Phase:**
- [ ] Add doc comment explaining why raw tokens are never stored

**Acceptance Criteria:**
- [ ] Output is deterministic for same input
- [ ] Output is 64-character hex string (SHA-256)
- [ ] Different inputs produce different outputs
- [ ] All tests pass

---

#### Task 4.2: Implement SessionStore Interface and PostgreSQL Backend

**Linked Requirements:** REQ-AUTH-090, REQ-AUTH-091, REQ-AUTH-092, REQ-AUTH-093, REQ-AUTH-094, REQ-AUTH-095

**RED Phase:**
- [ ] Write tests (require embedded DB) for a `PgSessionStore`:
  - `TestPgSessionStore_Create` — creates session, returns populated struct
  - `TestPgSessionStore_FindByTokenHash_Active` — finds non-expired session
  - `TestPgSessionStore_FindByTokenHash_Expired` — returns `ErrSessionNotFound` for expired
  - `TestPgSessionStore_DeleteByID` — removes specific session
  - `TestPgSessionStore_DeleteByUserID` — removes all sessions for a user
  - `TestPgSessionStore_DeleteExpired` — removes expired sessions, returns count
  - `TestPgSessionStore_MultipleSessions` — multiple concurrent sessions per user supported
  - Expected failure: `PgSessionStore` does not exist

**GREEN Phase:**
- [ ] Define `SessionStore` interface (per design Section 3.4)
- [ ] Implement `PgSessionStore` struct wrapping sqlc `*db.Queries`
- [ ] Implement all five interface methods delegating to sqlc queries
- [ ] Define `ErrSessionNotFound` sentinel error

**REFACTOR Phase:**
- [ ] Ensure `FindByTokenHash` maps "no rows" to `ErrSessionNotFound` cleanly
- [ ] Verify type mappings between sqlc models and auth domain types

**Acceptance Criteria:**
- [ ] `SessionStore` interface defined with all five methods
- [ ] `PgSessionStore` implements the interface correctly
- [ ] Expired sessions not returned by `FindByTokenHash`
- [ ] `DeleteExpired` returns accurate count
- [ ] Multiple concurrent sessions per user supported
- [ ] `ErrSessionNotFound` returned when session not found

---

#### Task 4.3: Implement Session Cleanup Background Goroutine

**Linked Requirements:** REQ-AUTH-095

**RED Phase:**
- [ ] Write test for `StartSessionCleanup`:
  - Test: Create expired sessions, start cleanup with short interval, verify sessions are deleted
  - Test: Verify cleanup respects context cancellation (graceful shutdown)
  - Expected failure: `StartSessionCleanup` function does not exist

**GREEN Phase:**
- [ ] Implement `StartSessionCleanup(ctx context.Context, store SessionStore, interval time.Duration)`
  - Ticker-based loop calling `store.DeleteExpired` periodically
  - Respects `ctx.Done()` for shutdown
  - Logs via `slog` on errors and successful cleanups

**REFACTOR Phase:**
- [ ] Ensure ticker is properly stopped on context cancellation
- [ ] Verify no goroutine leaks in tests

**Acceptance Criteria:**
- [ ] Expired sessions cleaned up on configured interval
- [ ] Goroutine exits cleanly when context is cancelled
- [ ] Errors logged without crashing
- [ ] Successful cleanup count logged at Info level
- [ ] No goroutine leaks

---

### Component 6.1 (Prerequisite): Configuration

> **Note:** AuthConfig is implemented before middleware because the middleware depends on deployment mode and session TTL from config.

#### Task 6.1: Implement Configuration Extensions

**Linked Requirements:** REQ-AUTH-010, REQ-AUTH-015

**RED Phase:**
- [ ] Write test for `AuthConfig` parsing:
  - Test: Default values are `local_trusted`, empty JWT secret, 24h TTL, `disableSignUp=false`
  - Test: Environment variables override defaults
  - Expected failure: `AuthConfig` struct does not exist

**GREEN Phase:**
- [ ] Add `AuthConfig` struct to `internal/config/config.go` (per design Section 8.3)
  - `DeploymentMode`, `JWTSecret`, `SessionTTL`, `DisableSignUp`
- [ ] Integrate into existing config loading

**REFACTOR Phase:**
- [ ] Ensure config validation rejects invalid deployment mode values
- [ ] Add default value documentation in struct tags or comments

**Acceptance Criteria:**
- [ ] `AuthConfig` populated from environment variables
- [ ] Defaults: `local_trusted` mode, empty JWT secret (auto-generated), 24h TTL, signup enabled
- [ ] Invalid deployment mode rejected at startup
- [ ] Config accessible from server initialization

---

### Component 5: Auth Middleware

#### Task 5.1: Implement Shared Types and Context Helpers

**Linked Requirements:** REQ-AUTH-010, REQ-AUTH-083

**RED Phase:**
- [ ] Write `internal/auth/middleware_test.go` with tests:
  - `TestUserFromContext_ReturnsIdentity` — identity injected into context is retrievable
  - `TestUserFromContext_ReturnsFalseWhenMissing` — empty context returns false
  - Expected failure: Types and functions do not exist

**GREEN Phase:**
- [ ] Create `internal/auth/types.go` with `DeploymentMode` type and constants (`ModeLocalTrusted`, `ModeAuthenticated`)
- [ ] Create `internal/auth/middleware.go` with `Identity` struct, `contextKey` type, `userContextKey` constant
- [ ] Implement `UserFromContext(ctx context.Context) (Identity, bool)`
- [ ] Implement unexported `contextWithUser(ctx context.Context, identity Identity) context.Context`

**REFACTOR Phase:**
- [ ] Ensure context key type is unexported to prevent collisions
- [ ] Add doc comments for public API

**Acceptance Criteria:**
- [ ] `DeploymentMode` type with `ModeLocalTrusted` and `ModeAuthenticated` constants
- [ ] `Identity` struct with `UserID` (uuid.UUID) and `Email` (string)
- [ ] `UserFromContext` correctly retrieves identity from context
- [ ] `UserFromContext` returns false for context without identity
- [ ] Context key is collision-safe (unexported type)

---

#### Task 5.2: Implement local_trusted Mode Middleware

**Linked Requirements:** REQ-AUTH-011, REQ-AUTH-081, REQ-AUTH-083, REQ-AUTH-101

**RED Phase:**
- [ ] Add tests to `internal/auth/middleware_test.go`:
  - `TestMiddleware_LocalTrusted_PassesAllRequests` — all requests pass through without auth
  - `TestMiddleware_LocalTrusted_InjectsSyntheticIdentity` — downstream handler sees `Identity{UserID: uuid.Nil, Email: "local@ari.local"}`
  - Expected failure: `Middleware` function does not exist

**GREEN Phase:**
- [ ] Implement `Middleware(mode DeploymentMode, jwtSvc *JWTService, sessions SessionStore) func(next http.Handler) http.Handler`
  - When mode is `local_trusted`: inject `LocalOperatorIdentity` into context, call `next.ServeHTTP`
- [ ] Define `LocalOperatorIdentity` as package-level variable

**REFACTOR Phase:**
- [ ] Ensure middleware signature is compatible with stdlib `http.Handler` chaining

**Acceptance Criteria:**
- [ ] All requests pass through in `local_trusted` mode regardless of auth headers
- [ ] Synthetic identity injected with `uuid.Nil` and `"local@ari.local"`
- [ ] No database or JWT service calls made in `local_trusted` mode
- [ ] Downstream handlers can extract identity via `UserFromContext`

---

#### Task 5.3: Implement authenticated Mode Middleware

**Linked Requirements:** REQ-AUTH-013, REQ-AUTH-014, REQ-AUTH-074, REQ-AUTH-080, REQ-AUTH-082, REQ-AUTH-083, REQ-AUTH-084, REQ-AUTH-091

**RED Phase:**
- [ ] Add tests to `internal/auth/middleware_test.go`:
  - `TestMiddleware_Authenticated_RejectsNoToken` — returns 401 `UNAUTHENTICATED`
  - `TestMiddleware_Authenticated_AcceptsValidCookie` — `ari_session` cookie with valid JWT passes
  - `TestMiddleware_Authenticated_AcceptsBearerHeader` — `Authorization: Bearer <token>` passes
  - `TestMiddleware_Authenticated_RejectsExpiredToken` — returns 401 `TOKEN_EXPIRED`
  - `TestMiddleware_Authenticated_RejectsRevokedSession` — valid JWT but deleted session returns 401 `INVALID_TOKEN`
  - `TestMiddleware_SkipsPublicEndpoints` — `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/health` pass without auth
  - `TestMiddleware_Authenticated_InjectsIdentity` — downstream handler receives correct `UserID` and `Email`
  - Expected failure: Authenticated branch of middleware not implemented

**GREEN Phase:**
- [ ] Extend `Middleware` function with `authenticated` mode branch:
  1. Check if request matches public endpoints skip list — if so, call `next` directly
  2. Extract token from `ari_session` cookie first, then `Authorization: Bearer` header
  3. Call `jwtSvc.Validate(token)` — map errors to HTTP 401 responses
  4. Call `sessions.FindByTokenHash(HashToken(token))` — if not found, return 401 `INVALID_TOKEN`
  5. Inject `Identity{UserID, Email}` from claims into request context
  6. Call `next.ServeHTTP`
- [ ] Define `publicEndpoints` map per design Section 3.3

**REFACTOR Phase:**
- [ ] Extract token extraction logic into `extractToken(r *http.Request) string` helper
- [ ] Extract public endpoint check into `isPublicEndpoint(path, method string) bool` helper
- [ ] Ensure error responses use scaffold JSON error format

**Acceptance Criteria:**
- [ ] Requests without token receive 401 `UNAUTHENTICATED`
- [ ] Cookie-based auth works with `ari_session` cookie
- [ ] Bearer token auth works with `Authorization` header
- [ ] Cookie takes precedence over header when both present
- [ ] Expired tokens return 401 `TOKEN_EXPIRED`
- [ ] Revoked sessions return 401 `INVALID_TOKEN`
- [ ] Public endpoints bypass all auth checks
- [ ] Identity correctly injected into context for downstream handlers

---

### Component 6: Register Endpoint

#### Task 6.2: Implement POST /api/auth/register Handler

**Linked Requirements:** REQ-AUTH-020, REQ-AUTH-021, REQ-AUTH-022, REQ-AUTH-023, REQ-AUTH-024, REQ-AUTH-110, REQ-AUTH-111, REQ-AUTH-112

**RED Phase:**
- [ ] Write `internal/server/handlers/auth_handler_test.go` with tests (require embedded DB):
  - `TestRegister_Success_FirstUser_IsAdmin` — first user gets `isAdmin=true`, HTTP 201
  - `TestRegister_Success_SecondUser_NotAdmin` — second user gets `isAdmin=false`
  - `TestRegister_DuplicateEmail_Returns409` — same email returns 409 `EMAIL_EXISTS`
  - `TestRegister_DuplicateEmailCaseInsensitive_Returns409` — "Alice@X.com" then "alice@x.com" returns 409
  - `TestRegister_InvalidEmail_Returns400` — "not-an-email" returns 400 `VALIDATION_ERROR`
  - `TestRegister_WeakPassword_Returns400` — "weak" returns 400 `INVALID_PASSWORD` with details array
  - `TestRegister_MissingFields_Returns400` — empty body returns 400 `VALIDATION_ERROR`
  - `TestRegister_DisplayNameTooLong_Returns400` — 256-char name returns 400 `VALIDATION_ERROR`
  - `TestRegister_DisplayNameEmpty_Returns400` — empty displayName returns 400 `VALIDATION_ERROR`
  - `TestRegister_DisabledSignUp_FirstUser_Allowed` — first user allowed even with `disableSignUp=true`
  - `TestRegister_DisabledSignUp_SecondUser_Returns403` — second user blocked with 403 `REGISTRATION_DISABLED`
  - `TestRegister_ResponseExcludesPasswordHash` — response JSON has no `passwordHash` field
  - Expected failure: `AuthHandler` and `Register` method do not exist

**GREEN Phase:**
- [ ] Create `internal/server/handlers/auth_handler.go`
- [ ] Define `AuthHandler` struct and `NewAuthHandler` constructor (per design Section 3.6)
- [ ] Implement `Register(w http.ResponseWriter, r *http.Request)`:
  1. Parse and validate `RegisterRequest` (email format, displayName length 1-255, password strength)
  2. Hash password with `HashPassword` (before transaction to avoid holding TX during bcrypt)
  3. Begin database transaction wrapping `CountUsers` + `CreateUser` to prevent concurrent first-user admin race
  4. Check `disableSignUp` gate: if true and count > 0, rollback and return 403
  5. Check first-user: if count == 0, set `isAdmin = true`
  6. Insert user with `CreateUser`; catch unique violation for 409
  7. Commit transaction
  8. Return 201 with user JSON (no `passwordHash`)
- [ ] Implement `RegisterRoutes(mux *http.ServeMux)` to mount all auth routes

**REFACTOR Phase:**
- [ ] Extract input validation into a `validateRegisterRequest` helper
- [ ] Extract email format check into a reusable `isValidEmail` function
- [ ] Ensure all error responses match scaffold JSON format

**Acceptance Criteria:**
- [ ] First registered user is admin, subsequent users are not
- [ ] Duplicate email (case-insensitive) returns 409
- [ ] Invalid email format returns 400
- [ ] Weak password returns 400 with all violation details
- [ ] Missing required fields return 400
- [ ] `displayName` length enforced (1-255)
- [ ] `disableSignUp` blocks registration unless zero users exist
- [ ] `CountUsers` + `CreateUser` wrapped in a single transaction to prevent concurrent first-user admin race
- [ ] Response never contains `passwordHash`
- [ ] HTTP 201 on success

---

### Component 10: Rate Limiting

> **Note:** Rate limiting is implemented before the login endpoint because the login handler depends on the rate limiter.

#### Task 10.1: Implement Sliding Window Rate Limiter

**Linked Requirements:** REQ-AUTH-123, REQ-AUTH-124

**RED Phase:**
- [ ] Write `internal/auth/ratelimit_test.go` with tests:
  - `TestRateLimiter_AllowsUnderLimit` — 9 attempts with limit 10 all return `true`
  - `TestRateLimiter_BlocksOverLimit` — 11th attempt with limit 10 returns `false`
  - `TestRateLimiter_SlidingWindowExpires` — record 10 attempts, advance time past window, next attempt allowed
  - `TestRateLimiter_ResetClearsHistory` — record attempts, call `Reset(ip)`, next attempt allowed
  - `TestRateLimiter_IndependentPerIP` — IP "1.1.1.1" at limit does not block "2.2.2.2"
  - Expected failure: `RateLimiter` struct does not exist

**GREEN Phase:**
- [ ] Create `internal/auth/ratelimit.go`
- [ ] Implement `RateLimiter` struct with `sync.Mutex`, `attempts map[string][]time.Time`, `limit int`, `window time.Duration`
- [ ] Implement `NewRateLimiter(limit int, window time.Duration) *RateLimiter`
- [ ] Implement `Allow(ip string) bool` — count attempts within window, return `count < limit`
- [ ] Implement `Record(ip string)` — append `time.Now()` to attempts list
- [ ] Implement `CheckAndRecord(ip string) bool` — atomically check limit and record attempt in a single mutex acquisition; returns true if allowed (attempt recorded), false if rate-limited
- [ ] Implement `Reset(ip string)` — delete IP entry from map

**REFACTOR Phase:**
- [ ] Prune expired entries in `Allow` method to prevent unbounded memory growth
- [ ] Consider adding a background cleanup goroutine (5-minute interval per design)
- [ ] Ensure thread-safety under concurrent access

**Acceptance Criteria:**
- [ ] Requests under limit (10/min) are allowed
- [ ] Requests over limit are blocked
- [ ] Sliding window expires old entries correctly
- [ ] `Reset` clears history for specific IP
- [ ] IPs are tracked independently
- [ ] Thread-safe under concurrent access
- [ ] Stale entries cleaned up to prevent memory growth

---

#### Task 10.2: Integrate Rate Limiter with Login Handler

**Linked Requirements:** REQ-AUTH-123, REQ-AUTH-124

**RED Phase:**
- [ ] Add test to `internal/server/handlers/auth_handler_test.go`:
  - `TestLogin_RateLimited_Returns429` — send 10 failed logins, 11th returns 429 `RATE_LIMITED`
  - `TestLogin_RateLimitResetOnSuccess` — send 9 failed logins, 1 success, then failed login is allowed (counter reset)
  - Expected failure: Rate limiter not wired into login handler

**GREEN Phase:**
- [ ] Wire `RateLimiter` into `AuthHandler` struct
- [ ] In `Login` handler: check `rateLimiter.Allow(ip)` before processing
- [ ] On failed login: call `rateLimiter.Record(ip)`
- [ ] On successful login: call `rateLimiter.Reset(ip)`
- [ ] Implement IP extraction helper: In Phase 1, always use `r.RemoteAddr` (ignoring `X-Forwarded-For`) with a documented limitation. `X-Forwarded-For` should only be trusted from configured trusted proxies, which is deferred to a future phase.

**REFACTOR Phase:**
- [ ] Ensure rate limit check is the first operation in login flow (before any DB queries)

**Acceptance Criteria:**
- [ ] 429 `RATE_LIMITED` returned after 10 failed attempts per minute per IP
- [ ] Counter resets on successful login
- [ ] Rate limit checked before any expensive operations (DB queries, bcrypt)
- [ ] Different IPs tracked independently

---

### Component 7: Login Endpoint

#### Task 7.1: Implement Anti-Enumeration Dummy Hash

**Linked Requirements:** REQ-AUTH-044, REQ-AUTH-121

**RED Phase:**
- [ ] Write test in `internal/auth/password_test.go`:
  - `TestAntiEnumerationCompare_DoesNotPanic` — calling the dummy compare function completes without error or panic
  - Expected failure: `dummyHash` and `antiEnumerationCompare` do not exist

**GREEN Phase:**
- [ ] Add package-level `dummyHash` variable (pre-computed bcrypt hash)
- [ ] Add `AntiEnumerationCompare()` function that performs a dummy bcrypt compare

**REFACTOR Phase:**
- [ ] Ensure dummy hash is computed at init time, not per-request

**Acceptance Criteria:**
- [ ] Dummy bcrypt compare executes in similar time to real compare
- [ ] Function does not panic or return a visible error to callers
- [ ] Pre-computed hash avoids per-request cost

---

#### Task 7.2: Implement POST /api/auth/login Handler

**Linked Requirements:** REQ-AUTH-040, REQ-AUTH-041, REQ-AUTH-042, REQ-AUTH-043, REQ-AUTH-044, REQ-AUTH-045, REQ-AUTH-070, REQ-AUTH-071, REQ-AUTH-072, REQ-AUTH-073, REQ-AUTH-074, REQ-AUTH-110, REQ-AUTH-112, REQ-AUTH-120, REQ-AUTH-121, REQ-AUTH-122

**RED Phase:**
- [ ] Add tests to `internal/server/handlers/auth_handler_test.go`:
  - `TestLogin_Success_ReturnsTokenAndCookie` — valid credentials return 200, token in body, `ari_session` cookie set
  - `TestLogin_Success_CookieFlags` — cookie has `HttpOnly=true`, `SameSite=Lax`, `Path=/`, `MaxAge=int(sessionTTL.Seconds())`
  - `TestLogin_WrongEmail_Returns401` — unknown email returns 401 `INVALID_CREDENTIALS`
  - `TestLogin_WrongPassword_Returns401` — wrong password returns 401 `INVALID_CREDENTIALS`
  - `TestLogin_SameErrorForBothWrongEmailAndPassword` — verify error responses are identical (anti-enumeration)
  - `TestLogin_DisabledAccount_Returns403` — disabled user returns 403 `ACCOUNT_DISABLED`
  - `TestLogin_MissingFields_Returns400` — empty email or password returns 400 `VALIDATION_ERROR`
  - `TestLogin_CreatesSessionInDatabase` — after login, session record exists in DB
  - Expected failure: `Login` method does not exist

**GREEN Phase:**
- [ ] Implement `Login(w http.ResponseWriter, r *http.Request)`:
  1. Check rate limiter: `Allow(ip)` — if denied, return 429
  2. Parse and validate `LoginRequest` (email format, password not empty)
  3. `GetUserByEmail(email)` — if not found, call `AntiEnumerationCompare()`, record failed attempt in rate limiter, return 401 `INVALID_CREDENTIALS`
  4. Check `user.Status != "disabled"` — if disabled, return 403 `ACCOUNT_DISABLED`
  5. `CheckPassword(user.PasswordHash, password)` — if mismatch, record failed attempt in rate limiter, return 401 `INVALID_CREDENTIALS`
  6. `jwtSvc.Mint(user.ID, user.Email)` — generate JWT
  7. `HashToken(jwt)` — compute session hash
  8. `sessions.Create(...)` — persist session with `expiresAt = now + h.sessionTTL` (from config, default 24h)
  9. Set `ari_session` cookie with correct flags
  10. Reset rate limiter for IP
  11. Return 200 with token and user profile (no `passwordHash`)

**REFACTOR Phase:**
- [ ] Extract cookie construction into `newSessionCookie(token string, isSecure bool, ttl time.Duration) *http.Cookie` helper
- [ ] Ensure response timing is constant regardless of email existence

**Acceptance Criteria:**
- [ ] Valid login returns 200 with JWT token and user profile
- [ ] `ari_session` cookie set with `HttpOnly`, `SameSite=Lax`, `Path=/`
- [ ] `Secure` flag set when not localhost
- [ ] Wrong email and wrong password produce identical 401 responses
- [ ] Disabled accounts receive 403 `ACCOUNT_DISABLED`
- [ ] Session record persisted in database
- [ ] Rate limiter counter reset on successful login
- [ ] Response never contains `passwordHash`

---

#### Task 7.3: Implement Activity Log Writes for Auth Events

**Linked Requirements:** REQ-AUTH-122

**RED Phase:**
- [ ] Write tests in `internal/server/handlers/auth_handler_test.go`:
  - `TestLogin_Success_LogsActivity` — successful login writes an activity log entry with event type "login_success" and user ID
  - `TestLogin_Failure_LogsActivity` — failed login writes an activity log entry with event type "login_failure" and attempted email (no password)
  - `TestRegister_Success_LogsActivity` — successful registration writes an activity log entry with event type "registration"
  - `TestLogout_Success_LogsActivity` — successful logout writes an activity log entry with event type "logout"
  - Expected failure: No activity log writes implemented in auth handlers

**GREEN Phase:**
- [ ] Add activity log writes to `Register`, `Login`, and `Logout` handlers
  - Log login success with user ID and email
  - Log login failure with attempted email (never include password or token)
  - Log registration with new user ID
  - Log logout with user ID
- [ ] Use existing activity log infrastructure from the scaffold (or define an interface if not yet available)

**REFACTOR Phase:**
- [ ] Ensure no sensitive data (passwords, tokens, hashes) is included in log entries
- [ ] Extract activity log event types into constants

**Acceptance Criteria:**
- [ ] Login success, login failure, registration, and logout all produce activity log entries
- [ ] No sensitive data (passwords, tokens) in activity log entries
- [ ] Activity log entries include relevant identifiers (user ID, email) for auditing

---

### Component 8: Logout Endpoint

#### Task 8.1: Implement POST /api/auth/logout Handler

**Linked Requirements:** REQ-AUTH-050, REQ-AUTH-051, REQ-AUTH-092

**RED Phase:**
- [ ] Add tests to `internal/server/handlers/auth_handler_test.go`:
  - `TestLogout_ClearsSessionAndCookie` — after login+logout, session deleted from DB, cookie cleared (`Max-Age=0`)
  - `TestLogout_NoAuth_Returns401` — request without auth returns 401
  - `TestLogout_InvalidatesOnlyCurrentSession` — user with two sessions, logout on one preserves the other
  - Expected failure: `Logout` method does not exist

**GREEN Phase:**
- [ ] Implement `Logout(w http.ResponseWriter, r *http.Request)`:
  1. Extract token from cookie or header
  2. Compute `HashToken(token)`
  3. `sessions.FindByTokenHash(hash)` — get session ID
  4. `sessions.DeleteByID(session.ID)` — remove session
  5. Clear `ari_session` cookie by setting `MaxAge: 0`
  6. Return 200 with `{"message": "Logged out successfully"}`

**REFACTOR Phase:**
- [ ] Ensure cookie is cleared even if session deletion fails (best-effort)
- [ ] Handle edge case where session already expired/deleted gracefully

**Acceptance Criteria:**
- [ ] Session record deleted from database on logout
- [ ] `ari_session` cookie cleared with `MaxAge: 0`
- [ ] Other sessions for the same user remain active
- [ ] Unauthenticated requests return 401
- [ ] Response body contains success message

---

### Component 9: Me Endpoint

#### Task 9.1: Implement GET /api/auth/me Handler

**Linked Requirements:** REQ-AUTH-100, REQ-AUTH-101, REQ-AUTH-004

**RED Phase:**
- [ ] Add tests to `internal/server/handlers/auth_handler_test.go`:
  - `TestMe_Authenticated_ReturnsProfile` — authenticated request returns user profile with correct fields
  - `TestMe_LocalTrusted_ReturnsSyntheticIdentity` — returns synthetic local operator `{id: "00000000-...", email: "local@ari.local", displayName: "Local Operator", isAdmin: true}`
  - `TestMe_NoAuth_Returns401` — unauthenticated request returns 401
  - `TestMe_ResponseExcludesPasswordHash` — response JSON does not contain `passwordHash`
  - Expected failure: `Me` method does not exist

**GREEN Phase:**
- [ ] Implement `Me(w http.ResponseWriter, r *http.Request)`:
  1. Extract `Identity` from request context via `UserFromContext`
  2. If `Identity.UserID == uuid.Nil` (local_trusted mode): return hardcoded synthetic user
  3. Otherwise: query `GetUserByID(identity.UserID)` and return user profile
  4. Never include `passwordHash` in response

**REFACTOR Phase:**
- [ ] Extract synthetic user response into a package-level constant/variable
- [ ] Ensure response JSON field names match design (`displayName`, `isAdmin`, `createdAt`)

**Acceptance Criteria:**
- [ ] Authenticated users see their real profile from the database
- [ ] `local_trusted` mode returns synthetic local operator identity
- [ ] Response never contains `passwordHash`
- [ ] Response includes: `id`, `email`, `displayName`, `status`, `isAdmin`, `createdAt`
- [ ] Unauthenticated requests return 401

---

### Component 11: Integration Tests

#### Task 11.1: Full Auth Flow Integration Test

**Linked Requirements:** All REQ-AUTH-* (end-to-end verification)

**RED Phase:**
- [ ] Write `internal/server/handlers/auth_integration_test.go`:
  - `TestFullFlow_RegisterLoginMeLogout` — register user, login, call /me, logout, confirm /me returns 401
  - `TestMultipleSessions_IndependentLogout` — login from two "browsers", logout one, other still works
  - `TestFirstUserAdmin_SecondUserNot` — register two users, verify admin flags
  - `TestAuthenticatedMode_ProtectedEndpoints` — verify non-auth endpoints require auth in `authenticated` mode
  - `TestLocalTrustedMode_NoAuthRequired` — verify all endpoints accessible without auth in `local_trusted` mode
  - Expected failure: Full integration not wired yet

**GREEN Phase:**
- [ ] Set up test helpers:
  - `SetupTestDB` — embedded PostgreSQL with migrations applied
  - `SetupTestServer` — create `AuthHandler` with all dependencies, mount routes on `http.ServeMux`
  - `MintTestToken` — convenience function for creating test JWTs
  - `NewAuthenticatedRequest` — create requests with session cookie
- [ ] Implement all integration test scenarios using `httptest.NewServer`
- [ ] Wire together: config, DB, JWT service, session store, rate limiter, handlers, middleware

**REFACTOR Phase:**
- [ ] Extract common setup/teardown into test helper functions
- [ ] Ensure tests are independent (no shared state between tests)
- [ ] Add table-driven subtests where applicable

**Acceptance Criteria:**
- [ ] Full register -> login -> me -> logout flow works end-to-end
- [ ] Multiple concurrent sessions work independently
- [ ] First-user-is-admin logic works in integration
- [ ] `authenticated` mode blocks unauthenticated access to protected endpoints
- [ ] `local_trusted` mode allows all access without auth
- [ ] All tests pass with embedded PostgreSQL
- [ ] No test pollution (each test cleans up after itself)

---

#### Task 11.2: Server Startup Integration

**Linked Requirements:** REQ-AUTH-010, REQ-AUTH-012, REQ-AUTH-015, REQ-AUTH-063

**RED Phase:**
- [ ] Write test verifying server startup wiring:
  - Test: `authenticated` mode auto-generates JWT secret if not configured, persists to `{data_dir}/secrets/jwt.key`
  - Test: `authenticated` mode loads JWT secret from `ARI_JWT_SECRET` env var when set
  - Test: `local_trusted` mode starts without JWT service initialization
  - Test: Default deployment mode is `local_trusted` when not configured
  - Expected failure: Startup wiring not implemented

**GREEN Phase:**
- [ ] Integrate auth initialization into `cmd/ari/` server startup:
  1. Load `AuthConfig` from environment
  2. If `authenticated` mode: initialize JWT service (auto-generate or load secret), session store, rate limiter
  3. Create `AuthHandler` with all dependencies
  4. Mount auth routes on router
  5. Wrap protected routes with auth middleware
  6. Start session cleanup goroutine (cancel on shutdown)
- [ ] Implement JWT secret auto-generation and persistence logic

**REFACTOR Phase:**
- [ ] Ensure clean shutdown: cancel session cleanup goroutine, close DB connections
- [ ] Validate configuration at startup (reject invalid mode, short JWT secret)

**Acceptance Criteria:**
- [ ] Server starts in `local_trusted` mode by default
- [ ] Server starts in `authenticated` mode when configured
- [ ] JWT secret auto-generated and persisted when not configured
- [ ] `secrets/` directory created with mode `0700` and `jwt.key` written with mode `0600`
- [ ] JWT secret loaded from environment variable when set
- [ ] Auth middleware applied to all `/api/` routes
- [ ] Auth endpoints mounted at `/api/auth/*`
- [ ] Session cleanup goroutine starts and stops cleanly
- [ ] `local_trusted` mode binds to `127.0.0.1` (REQ-AUTH-012)

---

### Final Verification

#### Task 12.1: Pre-Merge Checklist

**Final Checks:**

- [ ] All 28 tasks above completed
- [ ] All tests passing: `make test`
- [ ] No linter errors: `golangci-lint run`
- [ ] No type errors: `go vet ./...`
- [ ] Test coverage >= 80% for `internal/auth/` and auth handler
- [ ] `make sqlc` produces no diff (generated code committed)
- [ ] Database migrations tested (up and down)
- [ ] No debug code or `fmt.Println` statements
- [ ] No commented-out code
- [ ] No passwords, tokens, or secrets in test code (use generated values)
- [ ] All error responses follow scaffold JSON format
- [ ] `passwordHash` never appears in any API response
- [ ] Activity log entries created for auth events (REQ-AUTH-122)
- [ ] Parameterized SQL only — no string concatenation (REQ-AUTH-NFR-003)

**Performance Verification:**
- [ ] Login/register respond within 500ms (REQ-AUTH-NFR-001)
- [ ] Middleware adds < 5ms latency (REQ-AUTH-NFR-002)
- [ ] JWT signing key >= 256 bits (REQ-AUTH-NFR-004)

**Acceptance Criteria:**
- [ ] Feature is production-ready
- [ ] All quality gates passed
- [ ] All REQ-AUTH-* requirements covered by at least one test
- [ ] Ready for PR/merge

---

## Requirement Coverage Matrix

| Requirement | Task(s) | Description |
|-------------|---------|-------------|
| REQ-AUTH-001 | 1.1, 1.3 | User entity schema |
| REQ-AUTH-002 | 1.1, 1.3 | Email uniqueness (case-insensitive) |
| REQ-AUTH-003 | 2.2 | No plaintext password storage |
| REQ-AUTH-004 | 6.2, 7.2, 9.1 | passwordHash never in API responses |
| REQ-AUTH-010 | 5.1, 6.1 | Deployment mode support |
| REQ-AUTH-011 | 5.2 | local_trusted passes all requests |
| REQ-AUTH-012 | 11.2 | local_trusted binds to loopback |
| REQ-AUTH-013 | 5.3 | authenticated requires credentials |
| REQ-AUTH-014 | 5.3 | 401 UNAUTHENTICATED on missing credentials |
| REQ-AUTH-015 | 6.1 | Default to local_trusted mode |
| REQ-AUTH-020 | 6.2 | POST /api/auth/register endpoint |
| REQ-AUTH-021 | 6.2 | First user is admin |
| REQ-AUTH-022 | 6.2 | 409 EMAIL_EXISTS on duplicate |
| REQ-AUTH-023 | 6.2 | Registration disabled gate |
| REQ-AUTH-024 | 6.2 | 201 response on success |
| REQ-AUTH-030 | 2.1 | Password min 8 characters |
| REQ-AUTH-031 | 2.1 | Password uppercase/lowercase/digit |
| REQ-AUTH-032 | 2.1 | Password violation messages |
| REQ-AUTH-033 | 2.2 | bcrypt cost >= 10 |
| REQ-AUTH-040 | 7.2 | POST /api/auth/login endpoint |
| REQ-AUTH-041 | 7.2 | JWT + session cookie on login |
| REQ-AUTH-042 | 7.2 | 401 on unknown email |
| REQ-AUTH-043 | 7.2 | 401 on wrong password |
| REQ-AUTH-044 | 7.1, 7.2 | Anti-enumeration (same error) |
| REQ-AUTH-045 | 7.2 | 403 ACCOUNT_DISABLED |
| REQ-AUTH-050 | 8.1 | POST /api/auth/logout |
| REQ-AUTH-051 | 8.1 | Cookie cleared on logout |
| REQ-AUTH-060 | 3.2 | HS256 signing algorithm |
| REQ-AUTH-061 | 3.2 | JWT claims (sub, email, iat, exp) |
| REQ-AUTH-062 | 3.2 | 24h JWT expiration |
| REQ-AUTH-063 | 3.1, 11.2 | JWT signing key management |
| REQ-AUTH-064 | 3.3 | Reject invalid/malformed JWT |
| REQ-AUTH-065 | 3.3 | Reject expired JWT |
| REQ-AUTH-070 | 7.2 | HTTP-only session cookie |
| REQ-AUTH-071 | 7.2 | Secure flag when not localhost |
| REQ-AUTH-072 | 7.2 | SameSite=Lax |
| REQ-AUTH-073 | 7.2 | Cookie Path=/ |
| REQ-AUTH-074 | 5.3 | Accept cookie or Bearer header |
| REQ-AUTH-080 | 5.3 | Auth middleware on protected endpoints |
| REQ-AUTH-081 | 5.2 | local_trusted passes all requests |
| REQ-AUTH-082 | 5.3 | authenticated validates JWT |
| REQ-AUTH-083 | 5.1, 5.2, 5.3 | Identity injected into context |
| REQ-AUTH-084 | 5.3 | Public endpoint bypass |
| REQ-AUTH-090 | 1.2, 1.4, 4.2 | Session table schema and queries |
| REQ-AUTH-091 | 4.2, 5.3 | JWT + session double validation |
| REQ-AUTH-092 | 4.2, 8.1 | Session deleted on logout |
| REQ-AUTH-093 | 4.2, 11.1 | Multiple concurrent sessions |
| REQ-AUTH-094 | 4.2 | Force logout (delete all sessions) |
| REQ-AUTH-095 | 4.3 | Expired session cleanup |
| REQ-AUTH-100 | 9.1 | GET /api/auth/me endpoint |
| REQ-AUTH-101 | 9.1 | Synthetic identity in local_trusted |
| REQ-AUTH-110 | 6.2, 7.2 | Email format validation |
| REQ-AUTH-111 | 6.2 | displayName length validation |
| REQ-AUTH-112 | 6.2, 7.2 | Required field validation |
| REQ-AUTH-120 | 2.2 | Constant-time password comparison |
| REQ-AUTH-121 | 7.1, 7.2 | No email enumeration on login |
| REQ-AUTH-122 | 7.2, 7.3, 11.2 | Auth event activity logging |
| REQ-AUTH-123 | 10.1, 10.2 | Rate limiting (10/min/IP) |
| REQ-AUTH-124 | 10.1, 10.2 | 429 RATE_LIMITED response |
| REQ-AUTH-NFR-001 | 12.1 | Login/register < 500ms |
| REQ-AUTH-NFR-002 | 12.1 | Middleware < 5ms |
| REQ-AUTH-NFR-003 | 1.3, 1.4, 12.1 | Parameterized SQL |
| REQ-AUTH-NFR-004 | 3.1, 12.1 | JWT key >= 256 bits |

---

## Task Tracking Legend

- `[ ]` — Not started
- `[~]` — In progress
- `[x]` — Completed

## Commit Strategy

After each completed task:
```bash
# After RED phase
git add internal/auth/ internal/database/ internal/server/
git commit -m "test: Add failing tests for [functionality]"

# After GREEN phase
git add internal/auth/ internal/database/ internal/server/
git commit -m "feat: Implement [functionality]"

# After REFACTOR phase
git add internal/auth/ internal/database/ internal/server/
git commit -m "refactor: Clean up [component]"
```

## Notes

### Implementation Notes

- External dependencies to add: `golang.org/x/crypto/bcrypt`, `github.com/golang-jwt/jwt/v5`
- The scaffold's embedded PostgreSQL and goose migrations must be working before starting Task 1.1
- Tests requiring a database should use the scaffold's `SetupTestDB` helper pattern
- All JSON responses must use the scaffold's existing JSON response helpers

### Blockers

- [ ] 01-go-scaffold feature must be complete (HTTP server, router, DB connection, migrations, JSON helpers)

### Future Improvements

- Password reset flow (deferred from Phase 1)
- Configurable session TTL via API
- OAuth2 / social login providers
- MFA (TOTP) support
- Argon2id migration from bcrypt
- Distributed rate limiting (Redis-backed) for multi-instance deployments

### Lessons Learned

[Document insights gained during implementation]
