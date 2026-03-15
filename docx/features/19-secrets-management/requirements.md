# Requirements: Secrets Management

**Created:** 2026-03-15
**Status:** Draft
**Feature:** 19-secrets-management
**Dependencies:** 11-agent-runtime

## Overview

Secrets Management provides a secure, squad-scoped vault for storing API keys, tokens, and credentials that agents need to interact with external services. Secrets are encrypted at rest using AES-256-GCM with a master key, and are injected as environment variables when agents are spawned. The system supports CRUD operations, value rotation, and a React UI with masked display. Secret values are never exposed via the API -- only names, metadata, and a masked hint (last 4 characters) are returned.

## Scope

**In Scope:**
- Squad-scoped secret storage with AES-256-GCM encryption at rest
- Master key configuration via `ARI_MASTER_KEY` environment variable or auto-generation
- CRUD API for secrets (create, list, delete) with masked GET responses
- Individual secret value rotation (update encrypted value)
- Automatic injection of secrets as `ARI_SECRET_*` environment variables during agent invocation
- Activity log entries for secret lifecycle events
- React UI: secret management page with masked values and CRUD actions

**Out of Scope (future):**
- Secret versioning / rollback to previous values
- Cross-squad secret sharing
- Secret access policies (restrict which agents can access which secrets)
- External vault integration (HashiCorp Vault, AWS Secrets Manager)
- Secret expiration / TTL with auto-rotation
- Audit log of secret access per agent run
- Bulk import/export of secrets

## Definitions

| Term | Definition |
|------|------------|
| Secret | A squad-scoped, named key-value pair stored with AES-256-GCM encryption. The value is a credential (API key, token, password). |
| Master Key | A 256-bit AES key used to encrypt and decrypt all secret values. Derived from `ARI_MASTER_KEY` env var or auto-generated. |
| Nonce | A unique 12-byte value generated per encryption operation. Stored alongside the ciphertext to enable decryption. |
| Masked Value | A redacted representation of a secret value showing only the last 4 characters (e.g., `••••••••3abc`). |
| Secret Injection | The process of decrypting secrets and passing them as `ARI_SECRET_{NAME}` environment variables to agent subprocesses. |
| Key Rotation | Re-encrypting all squad secrets with a new master key. |

## Requirements (EARS Format)

### Secret Entity

**REQ-SEC-001:** WHEN a secret is created, the system SHALL assign a UUID as the primary key (`id`).

**REQ-SEC-002:** The system SHALL store the following fields on each secret: `id` (UUID), `squad_id` (FK), `name` (string, required), `encrypted_value` (bytea, required), `nonce` (bytea, required), `created_at` (timestamp), `updated_at` (timestamp), `last_rotated_at` (timestamp, nullable).

**REQ-SEC-003:** The system SHALL enforce that secret `name` is unique within a squad.

**REQ-SEC-004:** The system SHALL enforce that secret `name` matches the pattern `^[A-Z][A-Z0-9_]{0,127}$` (uppercase alphanumeric with underscores, 1-128 characters, starting with a letter).

**REQ-SEC-005:** The system SHALL always scope secrets to a single squad; cross-squad secret access SHALL be rejected with HTTP 403.

**REQ-SEC-006:** The system SHALL store secret values encrypted using AES-256-GCM. Plaintext values SHALL never be persisted to disk or database.

**REQ-SEC-007:** The system SHALL generate a unique 12-byte nonce for each encryption operation and store it in the `nonce` column alongside the ciphertext.

### Master Key Management

**REQ-SEC-010:** IF the environment variable `ARI_MASTER_KEY` is set, THEN the system SHALL derive the AES-256 master key from it using HKDF (RFC 5869) with SHA-256, a fixed salt `[]byte("ari-master-key-v1")`, and info string `[]byte("ari-secrets-encryption")`. The env var value is treated as raw keying material. Alternatively, operators MAY provide exactly 64 hex characters (representing a raw 256-bit key), in which case the system SHALL decode the hex directly and skip HKDF. The system SHALL reject values that are neither valid hex(64) nor usable as HKDF input (empty string). HKDF derivation is the preferred and default path.

**REQ-SEC-011:** IF `ARI_MASTER_KEY` is not set, THEN the system SHALL auto-generate a cryptographically random 256-bit master key at first startup and persist it to `{ARI_DATA_DIR}/master.key` (file permissions 0600). The `master.key` file MUST be excluded from any backup procedures. The system SHALL log a warning at startup if the file permissions are more permissive than 0600.

**REQ-SEC-012:** WHEN the system starts, IF a persisted master key file exists AND `ARI_MASTER_KEY` is not set, THEN the system SHALL load the master key from the file.

**REQ-SEC-013:** WHEN the system starts, the system SHALL validate that the master key is exactly 32 bytes (256 bits). IF validation fails, THEN startup SHALL abort with a clear error message.

**REQ-SEC-014:** The system SHALL log a warning at startup IF using an auto-generated master key, recommending the operator set `ARI_MASTER_KEY` for production deployments.

### Secret CRUD API

**REQ-SEC-020:** The system SHALL expose `POST /api/squads/{squadId}/secrets` to create a new secret. The request body SHALL contain `name` (string) and `value` (string). The response SHALL return the secret metadata (id, name, created_at) but SHALL NOT return the plaintext value.

**REQ-SEC-021:** The system SHALL expose `GET /api/squads/{squadId}/secrets` to list all secrets within a squad. The response SHALL include each secret's `id`, `name`, `maskedValue` (last 4 chars), `createdAt`, `updatedAt`, and `lastRotatedAt`. The response SHALL NOT include plaintext values.

**REQ-SEC-022:** The system SHALL expose `DELETE /api/squads/{squadId}/secrets/{name}` to delete a secret by name.

**REQ-SEC-023:** The system SHALL expose `PUT /api/squads/{squadId}/secrets/{name}` to update a secret's value. The request body SHALL contain `value` (string). The system SHALL re-encrypt with a new nonce and update `updated_at` and `last_rotated_at`.

**REQ-SEC-024:** All secret endpoints SHALL require authentication (valid JWT).

**REQ-SEC-025:** All secret endpoints SHALL enforce squad-scoped data isolation: a user can only access secrets belonging to squads they are a member of.

**REQ-SEC-026:** WHEN a secret is created with a name that already exists in the squad, THEN the system SHALL return HTTP 409 with code `SECRET_NAME_CONFLICT`.

**REQ-SEC-027:** WHEN a secret name does not match the required pattern, THEN the system SHALL return HTTP 400 with code `VALIDATION_ERROR` and a message explaining the naming rules.

**REQ-SEC-028:** WHEN a secret value is empty, THEN the system SHALL return HTTP 400 with code `VALIDATION_ERROR`.

**REQ-SEC-028a:** WHEN a secret value exceeds 64KB (65,536 bytes), THEN the system SHALL return HTTP 400 with code `VALIDATION_ERROR` and message "secret value exceeds maximum size of 64KB".

**REQ-SEC-028b:** WHEN a secret plaintext value is shorter than 8 characters, THEN the system SHALL return HTTP 400 with code `VALIDATION_ERROR` and message "secret value must be at least 8 characters for security". This minimum length ensures meaningful masked display.

### Key Rotation

**REQ-SEC-030:** The system SHALL expose `POST /api/secrets/rotate-master-key` to re-encrypt all secrets across all squads with a new master key.

**REQ-SEC-031:** WHEN master key rotation is triggered, the system SHALL: (1) generate a new master key, (2) write the new key to a temporary file `{dataDir}/master.key.tmp`, (3) decrypt each secret with the old key and re-encrypt with the new key and a fresh nonce, (4) update all secrets in a single database transaction and commit, (5) rename the temporary key file to `{dataDir}/master.key`. IF the transaction commit fails, the system SHALL delete the temporary key file and restore the old key in memory. This ordering ensures no crash window where the new key is persisted but secrets are not yet re-encrypted.

**REQ-SEC-032:** IF master key rotation fails mid-way, the system SHALL rollback the transaction and preserve the old master key. No secrets SHALL be left in an inconsistent state.

**REQ-SEC-033:** The master key rotation endpoint SHALL require authentication and SHALL be restricted to users with admin privileges (checked via `users.is_admin` column). Master key rotation is a system-level operation separate from squad RBAC; squad roles do not grant rotation permission.

### Secret Injection into Agent Runs

**REQ-SEC-040:** WHEN an agent is invoked (via `RunService.Invoke`), the system SHALL decrypt all secrets for the agent's squad and inject them into `InvokeInput.EnvVars` with the prefix `ARI_SECRET_` (e.g., secret named `GITHUB_TOKEN` becomes env var `ARI_SECRET_GITHUB_TOKEN`). The system SHALL log an info-level message listing the injected secret NAMES (not values) for auditability (e.g., `"injecting secrets: GITHUB_TOKEN, OPENAI_API_KEY"`). **Known limitation:** All squad secrets are injected into every agent run. Per-agent secret filtering is a future enhancement.

**REQ-SEC-041:** Secret injection SHALL occur after the base env vars (ARI_API_URL, ARI_API_KEY, etc.) are set but before the adapter receives the input, ensuring secrets do not override core Ari env vars.

**REQ-SEC-042:** IF secret decryption fails for any secret during injection, the system SHALL log a warning and skip the failed secret rather than aborting the entire run.

**REQ-SEC-043:** The system SHALL NOT include secret values in any log output, SSE events, or API responses. Secret values SHALL only exist in memory during injection and in the agent subprocess environment.

### RBAC and Permissions

**REQ-SEC-045:** The system SHALL add `ResourceSecret` to the RBAC permission matrix. Owner and admin roles SHALL have full CRUD access to secrets. Viewer role SHALL be denied all secret operations (HTTP 403). **Note:** Feature 17 (RBAC) must add `ResourceSecret` as a recognized resource type.

**REQ-SEC-046:** Master key rotation SHALL require `users.is_admin = true`, independent of squad-level RBAC roles.

### Activity Logging

**REQ-SEC-050:** WHEN a secret is created, updated, or deleted, the system SHALL append an activity log entry with `entity_type=secret` and action `secret.created`, `secret.updated`, or `secret.deleted`. The metadata SHALL include the secret name but SHALL NOT include the secret value.

**REQ-SEC-051:** WHEN master key rotation completes, the system SHALL append an activity log entry with action `secrets.master_key_rotated` including the count of re-encrypted secrets.

---

## Error Handling

| Scenario | HTTP Status | Error Code |
|----------|-------------|------------|
| Secret not found | 404 | `NOT_FOUND` |
| Secret name already exists in squad | 409 | `SECRET_NAME_CONFLICT` |
| Invalid secret name (pattern mismatch) | 400 | `VALIDATION_ERROR` |
| Empty secret value | 400 | `VALIDATION_ERROR` |
| Secret value too short (< 8 chars) | 400 | `VALIDATION_ERROR` |
| Secret value too large (> 64KB) | 400 | `VALIDATION_ERROR` |
| Viewer role attempting secret access | 403 | `FORBIDDEN` |
| Squad not found | 404 | `NOT_FOUND` |
| Unauthorized access | 403 | `FORBIDDEN` |
| Master key not initialized | 500 | `MASTER_KEY_ERROR` |
| Decryption failure (corrupt data or wrong key) | 500 | `DECRYPTION_ERROR` |

---

## Non-Functional Requirements

**REQ-SEC-NF-001:** Secret CRUD operations SHALL respond within 100ms for squads with up to 500 secrets.

**REQ-SEC-NF-002:** Secret decryption during agent injection SHALL complete within 50ms for squads with up to 100 secrets.

**REQ-SEC-NF-003:** Master key rotation SHALL complete within 30 seconds for up to 10,000 total secrets across all squads.

**REQ-SEC-NF-004:** The `encrypted_value` column SHALL use `bytea` type with no size limit to support values up to 64KB.

**REQ-SEC-NF-005:** The database schema SHALL include an index on `squad_id` and a unique constraint on `(squad_id, name)` for efficient lookups.

---

## Acceptance Criteria

1. Secrets can be created within a squad with a unique uppercase name
2. Secret values are encrypted with AES-256-GCM before storage; plaintext never touches the database
3. GET endpoint returns masked values (last 4 chars) but never plaintext
4. Secrets can be updated (re-encrypted with new nonce) and deleted
5. Master key is derived from `ARI_MASTER_KEY` env var via HKDF (or decoded from 64-char hex), or auto-generated and persisted to file
6. Master key rotation re-encrypts all secrets atomically: new key written to temp file, transaction committed, then temp file renamed to final path
7. Secrets are injected as `ARI_SECRET_{NAME}` env vars when agents are spawned
8. Secret injection does not override core Ari env vars (ARI_API_URL, etc.)
9. Failed decryption during injection is logged and skipped, not fatal
10. Activity log captures create, update, delete, and rotation events
11. All endpoints enforce JWT auth and squad-scoped isolation
12. React UI shows secret names with masked values and supports create, update, delete
13. Secret names are validated against `^[A-Z][A-Z0-9_]{0,127}$`
14. Secret values must be at least 8 characters and no more than 64KB
15. RBAC enforced: owner+admin=full CRUD, viewer=denied all secret operations
16. Master key rotation requires `users.is_admin` (system-level, not squad RBAC)
17. Auto-generated master.key file has 0600 permissions; startup warns if more permissive
18. Injected secret names (not values) are logged at info level for auditability
19. Squad deletion requires explicit secret cleanup (ON DELETE RESTRICT)

---

## References

- Agent Runtime: `docx/features/11-agent-runtime/`
- Adapter interface: `internal/adapter/adapter.go` (InvokeInput.EnvVars)
- Claude adapter: `internal/adapter/claude/claude.go` (buildEnv function)
- Run handler: `internal/server/handlers/run_handler.go` (buildInvokeInput)
- Config: `internal/config/config.go`
- Activity log migration: `internal/database/migrations/20260315000011_create_activity_log.sql`
