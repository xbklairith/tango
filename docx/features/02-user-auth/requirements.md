# Requirements: User Authentication

**Feature:** 02-user-auth
**Created:** 2026-03-14
**Status:** Draft
**Dependencies:** 01-go-scaffold

## Overview

User authentication provides identity management, credential verification, session handling, and deployment mode configuration for the Ari control plane. It supports two deployment modes: `local_trusted` (no auth required, single operator) and `authenticated` (full auth required). The first registered user becomes the system admin.

**PRD References:** User entity (4.2), Deployment modes (3.3), Auth tiers (12.1), Security measures (12.2), Auth config (11), Phase 1 scope (14)

---

## EARS Requirements

### User Entity

**REQ-AUTH-001:** The system **shall** store User records with the following fields: `id` (UUID, primary key), `email` (string, unique), `displayName` (string), `passwordHash` (string, bcrypt), `status` (enum: active, disabled), `createdAt` (timestamp), and `updatedAt` (timestamp).

**REQ-AUTH-002:** The system **shall** enforce email uniqueness across all User records (case-insensitive comparison).

**REQ-AUTH-003:** The system **shall** never store plaintext passwords; only bcrypt hashes **shall** be persisted.

**REQ-AUTH-004:** The system **shall** never return `passwordHash` in any API response or log output.

### Deployment Modes

**REQ-AUTH-010:** The system **shall** support two deployment modes: `local_trusted` and `authenticated`, configured via the server configuration (`server.deploymentMode`).

**REQ-AUTH-011:** When the deployment mode is `local_trusted`, the system **shall** allow all API requests without authentication, granting full access to the single operator.

**REQ-AUTH-012:** When the deployment mode is `local_trusted`, the system **shall** bind only to the loopback interface (127.0.0.1) by default.

**REQ-AUTH-013:** When the deployment mode is `authenticated`, the system **shall** require valid authentication credentials on all API requests except registration and login endpoints.

**REQ-AUTH-014:** When the deployment mode is `authenticated` and a request lacks valid credentials, the system **shall** respond with HTTP 401 and error code `UNAUTHENTICATED`.

**REQ-AUTH-015:** The system **shall** default to `local_trusted` mode when no deployment mode is explicitly configured.

### User Registration

**REQ-AUTH-020:** When the deployment mode is `authenticated`, the system **shall** provide a `POST /api/auth/register` endpoint that accepts `email`, `displayName`, and `password`.

**REQ-AUTH-021:** When the first user registers in the system, the system **shall** automatically grant that user system admin privileges.

**REQ-AUTH-022:** When a registration request contains an email that already exists, the system **shall** reject the request with HTTP 409 and error code `EMAIL_EXISTS`.

**REQ-AUTH-023:** When registration is disabled via configuration (`auth.disableSignUp: true`), the system **shall** reject registration requests with HTTP 403 and error code `REGISTRATION_DISABLED`, unless no users exist yet (first user setup).

**REQ-AUTH-024:** Upon successful registration, the system **shall** return the created User object (excluding `passwordHash`) and an HTTP 201 status.

### Password Validation

**REQ-AUTH-030:** The system **shall** require passwords to be at least 8 characters in length.

**REQ-AUTH-031:** The system **shall** require passwords to contain at least one uppercase letter, one lowercase letter, and one digit.

**REQ-AUTH-032:** When a password fails validation, the system **shall** respond with HTTP 400, error code `INVALID_PASSWORD`, and a message describing which rules were not met.

**REQ-AUTH-033:** The system **shall** hash passwords using bcrypt with a cost factor of at least 10.

### Login

**REQ-AUTH-040:** When the deployment mode is `authenticated`, the system **shall** provide a `POST /api/auth/login` endpoint that accepts `email` and `password`.

**REQ-AUTH-041:** When login credentials are valid, the system **shall** return a JWT access token and set an HTTP-only session cookie.

**REQ-AUTH-042:** When the email does not match any user record, the system **shall** respond with HTTP 401 and error code `INVALID_CREDENTIALS`.

**REQ-AUTH-043:** When the password does not match the stored bcrypt hash, the system **shall** respond with HTTP 401 and error code `INVALID_CREDENTIALS`.

**REQ-AUTH-044:** The system **shall** use the same error response for invalid email and invalid password to prevent user enumeration.

**REQ-AUTH-045:** When a disabled user attempts to log in, the system **shall** respond with HTTP 403 and error code `ACCOUNT_DISABLED`.

### Logout

**REQ-AUTH-050:** The system **shall** provide a `POST /api/auth/logout` endpoint that invalidates the current session.

**REQ-AUTH-051:** Upon logout, the system **shall** clear the session cookie by setting it to an expired value.

### JWT Token Management

**REQ-AUTH-060:** The system **shall** generate JWT tokens using the HS256 (HMAC-SHA256) signing algorithm.

**REQ-AUTH-061:** The system **shall** include the following claims in User session JWTs: `sub` (user ID), `email`, `iat` (issued at), and `exp` (expiration).

**REQ-AUTH-062:** The system **shall** set User session JWT expiration to 24 hours from issuance.

**REQ-AUTH-063:** The system **shall** sign JWTs with a server-generated secret key that is persisted across restarts.

**REQ-AUTH-064:** When a JWT signature is invalid or the token is malformed, the system **shall** reject the request with HTTP 401 and error code `INVALID_TOKEN`.

**REQ-AUTH-065:** When a JWT has expired, the system **shall** reject the request with HTTP 401 and error code `TOKEN_EXPIRED`.

### Cookie-Based Sessions

**REQ-AUTH-070:** The system **shall** set session tokens as HTTP-only cookies with the name `ari_session`.

**REQ-AUTH-071:** The system **shall** set the `Secure` flag on session cookies when the server is not running on localhost.

**REQ-AUTH-072:** The system **shall** set the `SameSite` attribute to `Lax` on session cookies.

**REQ-AUTH-073:** The system **shall** set the cookie `Path` to `/`.

**REQ-AUTH-074:** The system **shall** accept authentication via either the `ari_session` cookie or an `Authorization: Bearer <token>` header.

### Authentication Middleware

**REQ-AUTH-080:** The system **shall** provide HTTP middleware that validates authentication on protected endpoints.

**REQ-AUTH-081:** When the deployment mode is `local_trusted`, the middleware **shall** pass all requests through without authentication checks.

**REQ-AUTH-082:** When the deployment mode is `authenticated`, the middleware **shall** extract and validate the JWT from the cookie or Authorization header.

**REQ-AUTH-083:** Upon successful authentication, the middleware **shall** inject the authenticated user's identity (user ID, email) into the request context for downstream handlers.

**REQ-AUTH-084:** The middleware **shall** skip authentication for the following public endpoints: `POST /api/auth/register`, `POST /api/auth/login`, and `GET /api/health`.

### Session Management

**REQ-AUTH-090:** The system **shall** track active sessions in the database with fields: `id` (UUID), `userId` (FK), `tokenHash` (string), `expiresAt` (timestamp), `createdAt` (timestamp).

**REQ-AUTH-091:** When validating a session token, the system **shall** verify both the JWT signature and the existence of a matching active session record.

**REQ-AUTH-092:** When a user logs out, the system **shall** delete the corresponding session record from the database.

**REQ-AUTH-093:** The system **shall** support multiple concurrent sessions per user (e.g., different browsers/devices).

**REQ-AUTH-094:** The system **shall** provide a mechanism to invalidate all sessions for a given user (force logout). *Note: In Phase 1, `DeleteByUserID` is an internal capability for future features (user disable, password change) and is not exposed as a direct API endpoint.*

**REQ-AUTH-095:** The system **shall** periodically clean up expired session records from the database.

### Current User Endpoint

**REQ-AUTH-100:** The system **shall** provide a `GET /api/auth/me` endpoint that returns the currently authenticated user's profile (excluding `passwordHash`).

**REQ-AUTH-101:** When the deployment mode is `local_trusted`, the `/api/auth/me` endpoint **shall** return a synthetic local operator identity.

### Input Validation

**REQ-AUTH-110:** The system **shall** validate that the `email` field is a well-formed email address on registration and login.

**REQ-AUTH-111:** The system **shall** validate that the `displayName` field is between 1 and 255 characters on registration.

**REQ-AUTH-112:** When any required field is missing or empty, the system **shall** respond with HTTP 400 and error code `VALIDATION_ERROR`.

### Security

**REQ-AUTH-120:** The system **shall** enforce constant-time comparison for password verification (provided by bcrypt).

**REQ-AUTH-121:** The system **shall** not disclose whether an email exists in the system through error messages on login (REQ-AUTH-044).

**REQ-AUTH-122:** The system **shall** log authentication events (login success, login failure, registration, logout) to the activity log without including sensitive data (passwords, tokens).

**REQ-AUTH-123:** The system **shall** rate-limit failed login attempts to no more than 10 per minute per IP address.

**REQ-AUTH-124:** When the rate limit is exceeded, the system **shall** respond with HTTP 429 and error code `RATE_LIMITED`.

---

## Non-Functional Requirements

**REQ-AUTH-NFR-001:** Login and registration endpoints **shall** respond within 500ms under normal load (excluding bcrypt hashing time which is intentionally slow).

**REQ-AUTH-NFR-002:** The authentication middleware **shall** add no more than 5ms of latency to request processing (JWT validation path).

**REQ-AUTH-NFR-003:** All auth-related database queries **shall** use parameterized statements to prevent SQL injection.

**REQ-AUTH-NFR-004:** The JWT signing secret **shall** be at least 256 bits (32 bytes) in length.

---

## Requirement Traceability

| Requirement ID | PRD Section | Description |
|---------------|-------------|-------------|
| REQ-AUTH-001 | 4.2 (User entity) | User entity schema |
| REQ-AUTH-010..015 | 3.3 (Deployment modes) | Deployment mode configuration |
| REQ-AUTH-020..024 | 4.2, 14 (Phase 1) | User registration |
| REQ-AUTH-030..033 | 12.2 (Security) | Password policy |
| REQ-AUTH-040..045 | 12.1 (Auth tiers) | Login flow |
| REQ-AUTH-050..051 | 12.1 (User Session) | Logout flow |
| REQ-AUTH-060..065 | 12.1 (Auth tiers), 3.2 | JWT management |
| REQ-AUTH-070..074 | 12.1 (User Session) | Cookie sessions |
| REQ-AUTH-080..084 | 12.1, 12.2 | Auth middleware |
| REQ-AUTH-090..095 | 12.1 (User Session) | Session management |
| REQ-AUTH-100..101 | 12.1 | Current user endpoint |
| REQ-AUTH-110..112 | 12.2 (Security) | Input validation |
| REQ-AUTH-120..124 | 12.2 (Security) | Security hardening |

---

## Open Questions

- [ ] Should session JWT TTL be configurable via server config, or fixed at 24h?
- [ ] Should the system support password reset in Phase 1, or defer to a later phase?
- [ ] Should `local_trusted` mode create a default User record in the database, or operate entirely without user records?
- [ ] What should the JWT signing key storage strategy be for production (config file, environment variable, auto-generated)?
