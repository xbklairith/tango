# Tasks: Secrets Management

**Feature:** 19-secrets-management
**Created:** 2026-03-15
**Status:** Pending

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-SEC-001 through REQ-SEC-051 (including 028a, 028b, 045, 046), REQ-SEC-NF-001 through REQ-SEC-NF-005

## Implementation Approach

Work bottom-up through the dependency graph: domain model and validation first, then master key manager with encryption primitives, then database migration and sqlc queries, then SecretsService for CRUD and injection, then HTTP handler, then RunService integration for secret injection, and finally React UI. Each task follows the Red-Green-Refactor TDD cycle.

## Progress Summary

- Total Tasks: 9
- Completed: 0/9
- In Progress: None

---

## Tasks (TDD: Red-Green-Refactor)

---

### [ ] Task 01 — Domain Model: Secret Types and Validation

**Requirements:** REQ-SEC-001, REQ-SEC-002, REQ-SEC-004, REQ-SEC-028
**Estimated time:** 20 min

#### Context

Define the secret domain types, request/response DTOs, and validation functions. Secret names must match the pattern `^[A-Z][A-Z0-9_]{0,127}$` and values must be non-empty. This is the foundation for all secrets logic.

#### RED — Write Failing Tests

Write `internal/domain/secret_test.go`:

1. `TestValidateSecretName_Valid` — verify valid names accepted: `GITHUB_TOKEN`, `A`, `OPENAI_API_KEY`, `DB_PASSWORD_2`.
2. `TestValidateSecretName_Invalid` — verify rejected: empty string, lowercase `github_token`, starts with digit `1TOKEN`, starts with underscore `_TOKEN`, contains spaces `MY TOKEN`, contains hyphen `MY-TOKEN`, 129 characters (too long).
3. `TestValidateSecretValue_Valid` — verify non-empty string (>= 8 chars, <= 64KB) accepted.
4. `TestValidateSecretValue_Invalid` — verify empty string rejected, string shorter than 8 characters rejected, string exceeding 64KB rejected.
5. `TestValidateCreateSecretInput` — verify combined validation (name + value, including min/max size).

#### GREEN — Implement

Create `internal/domain/secret.go`:

- `Secret` struct with all fields (ID, SquadID, Name, EncryptedValue, Nonce, CreatedAt, UpdatedAt, LastRotatedAt)
- `CreateSecretRequest` and `UpdateSecretRequest` DTOs
- `ValidateSecretName(name string) error` — regex match against `^[A-Z][A-Z0-9_]{0,127}$`
- `ValidateSecretValue(value string) error` — non-empty check, minimum 8 characters, maximum 64KB (65,536 bytes)
- `ValidateCreateSecretInput(req CreateSecretRequest) error` — calls both validators

#### REFACTOR

Ensure JSON tags use camelCase and pointer types for optional fields.

#### Files

- Create: `internal/domain/secret.go`
- Create: `internal/domain/secret_test.go`

---

### [ ] Task 02 — Master Key Manager: Key Loading, Generation, and Encryption

**Requirements:** REQ-SEC-006, REQ-SEC-007, REQ-SEC-010, REQ-SEC-011, REQ-SEC-012, REQ-SEC-013, REQ-SEC-014
**Estimated time:** 45 min

#### Context

The `MasterKeyManager` in `internal/secrets/` handles the full master key lifecycle: derivation from `ARI_MASTER_KEY` env var, auto-generation with file persistence, and AES-256-GCM encrypt/decrypt operations. This is a standalone package with no dependencies on the database or HTTP layer.

#### RED — Write Failing Tests

Write `internal/secrets/master_key_test.go`:

1. `TestNewMasterKeyManager_FromEnvVar_HKDF` — set env var (non-hex), verify key derived via HKDF(SHA-256, salt="ari-master-key-v1", info="ari-secrets-encryption").
2. `TestNewMasterKeyManager_FromEnvVar_RawHex` — set env var to exactly 64 hex characters, verify key decoded directly (not HKDF).
3. `TestNewMasterKeyManager_FromEnvVar_EmptyRejected` — set env var to empty string, verify error.
4. `TestNewMasterKeyManager_AutoGenerate` — no env var, no file, verify key generated and written to `{tempDir}/master.key` with 0600 permissions.
5. `TestNewMasterKeyManager_LoadFromFile` — write a 32-byte key file, no env var, verify loaded correctly.
6. `TestNewMasterKeyManager_LoadFromFile_PermissionsWarning` — write key file with 0644 permissions, verify warning logged about permissive permissions.
7. `TestNewMasterKeyManager_InvalidFile` — write a file with wrong length (16 bytes), verify error.
8. `TestEncryptDecrypt_Roundtrip` — encrypt a plaintext, decrypt it, verify match.
9. `TestEncryptDecrypt_DifferentNonces` — encrypt same plaintext twice, verify nonces differ and ciphertexts differ.
10. `TestDecrypt_WrongKey` — encrypt with one key, attempt decrypt with different key, verify error.
11. `TestDecrypt_TamperedCiphertext` — modify ciphertext, verify decrypt fails with authentication error.
12. `TestEncrypt_EmptyPlaintext` — verify empty plaintext encrypts/decrypts correctly.
13. `TestKeyDerivation_Deterministic` — same env var value produces same HKDF-derived key.

#### GREEN — Implement

Create `internal/secrets/master_key.go`:

- `MasterKeyManager` struct with `key [32]byte`, `dataDir string`, `mu sync.RWMutex`
- `NewMasterKeyManager(masterKeyEnv string, dataDir string) (*MasterKeyManager, error)` — implements the initialization order from design.md section 4. Uses HKDF (golang.org/x/crypto/hkdf) for key derivation from env var, or hex.DecodeString for raw 64-char hex keys. Checks file permissions on master.key and warns if more permissive than 0600.
- `Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error)` — AES-256-GCM encrypt with random 12-byte nonce
- `Decrypt(ciphertext, nonce []byte) (plaintext []byte, err error)` — AES-256-GCM decrypt
- `RotateKey() ([32]byte, error)` — generate new random key, swap atomically, return old key for re-encryption
- `PersistKey() error` — write current key to `{dataDir}/master.key`
- `Key() [32]byte` — return current key (for testing)

#### Files

- Create: `internal/secrets/master_key.go`
- Create: `internal/secrets/master_key_test.go`

---

### [ ] Task 03 — Database Migration: squad_secrets Table

**Requirements:** REQ-SEC-001, REQ-SEC-002, REQ-SEC-003, REQ-SEC-004, REQ-SEC-NF-004, REQ-SEC-NF-005
**Estimated time:** 20 min

#### Context

Create the `squad_secrets` table with all columns, constraints, indexes, and the `updated_at` trigger. Follow the pattern from existing migrations. The next migration number is `000019`.

#### RED — Write Failing Tests

Add assertions to migration smoke tests:

1. After `RunMigrations()`, the table `squad_secrets` exists with expected columns (including `masked_hint VARCHAR(12)`).
2. Unique constraint `uq_squad_secrets_squad_name` on `(squad_id, name)` exists.
3. Check constraint `chk_secret_name_format` exists.
4. Index `idx_squad_secrets_squad_id` exists.
5. Foreign key on `squad_id` references `squads(id)` with ON DELETE RESTRICT (not CASCADE). Verify that deleting a squad with existing secrets fails.
6. The `masked_hint` column exists and has a default value of empty string.

#### GREEN — Implement

Create `internal/database/migrations/20260316000019_create_squad_secrets.sql` with the schema from design.md section 2.

#### Files

- Create: `internal/database/migrations/20260316000019_create_squad_secrets.sql`
- Modify: `internal/database/database_test.go` (add migration assertions)

---

### [ ] Task 04 — SQL Queries and sqlc Generation: Secret CRUD

**Requirements:** REQ-SEC-020, REQ-SEC-021, REQ-SEC-022, REQ-SEC-023, REQ-SEC-026, REQ-SEC-030
**Estimated time:** 30 min

#### Context

Write sqlc query definitions for secret CRUD operations: create, list by squad, get by name, update value, delete, count by squad, list all (for rotation), and update encryption (for rotation). Run `make sqlc` to generate Go code.

#### RED — Write Failing Tests

Write `internal/database/db/squad_secrets_test.go`:

1. `TestCreateSquadSecret` — insert a secret, verify all fields returned including generated UUID and timestamps.
2. `TestCreateSquadSecret_DuplicateName` — verify unique constraint violation on duplicate `(squad_id, name)`.
3. `TestCreateSquadSecret_InvalidName` — verify check constraint rejects lowercase name.
4. `TestGetSquadSecretByName` — insert and retrieve by name, verify match.
5. `TestListSquadSecrets` — insert multiple secrets, verify ordered by name ASC.
6. `TestUpdateSquadSecretValue` — update encrypted_value and nonce, verify `last_rotated_at` set.
7. `TestDeleteSquadSecret` — insert and delete, verify gone.
8. `TestCountSquadSecrets` — verify count returns correct number.
9. `TestListAllSecrets` — insert secrets across multiple squads, verify all returned ordered by squad_id, name.
10. `TestUpdateSquadSecretEncryption` — update by ID, verify fields updated.

#### GREEN — Implement

Create `internal/database/queries/squad_secrets.sql` with queries from design.md section 3 (note: CreateSquadSecret includes `masked_hint` param, ListSquadSecrets returns `masked_hint` instead of encrypted_value/nonce, UpdateSquadSecretValue includes `masked_hint` param). Run `make sqlc`.

#### Files

- Create: `internal/database/queries/squad_secrets.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/squad_secrets_test.go`

---

### [ ] Task 05 — SecretsService: CRUD, Encryption, and Injection

**Requirements:** REQ-SEC-006, REQ-SEC-007, REQ-SEC-020, REQ-SEC-021, REQ-SEC-022, REQ-SEC-023, REQ-SEC-026, REQ-SEC-027, REQ-SEC-028, REQ-SEC-040, REQ-SEC-042, REQ-SEC-043, REQ-SEC-050
**Estimated time:** 60 min

#### Context

The `SecretsService` encapsulates all secret business logic: creating secrets (validate + encrypt + insert), listing with masked values, updating (re-encrypt + update), deleting, and decrypting all secrets for agent injection. Activity logging follows the existing `logActivity()` pattern used by InboxService and PipelineService.

#### RED — Write Failing Tests

Write `internal/server/handlers/secrets_service_test.go`:

1. `TestSecretsService_Create` — verify secret encrypted and persisted, activity log `secret.created` entry created.
2. `TestSecretsService_Create_DuplicateName` — verify 409 error with `SECRET_NAME_CONFLICT`.
3. `TestSecretsService_Create_InvalidName` — verify 400 error with `VALIDATION_ERROR` for lowercase name.
4. `TestSecretsService_Create_EmptyValue` — verify 400 error with `VALIDATION_ERROR`.
4a. `TestSecretsService_Create_ShortValue` — verify 400 error with `VALIDATION_ERROR` for value shorter than 8 characters.
4b. `TestSecretsService_Create_OversizedValue` — verify 400 error with `VALIDATION_ERROR` for value exceeding 64KB.
5. `TestSecretsService_List` — create multiple secrets, verify list returns pre-computed `masked_hint` values from DB (no decryption performed).
6. `TestSecretsService_List_MaskedValue` — verify masking logic: `"ghp_abcdefghij1234"` → `"••••••••1234"` (computed at create time, stored in `masked_hint` column).
7. `TestSecretsService_Update` — verify re-encrypted with new nonce, `last_rotated_at` set, activity log `secret.updated`.
8. `TestSecretsService_Update_NotFound` — verify 404 for non-existent secret name.
9. `TestSecretsService_Delete` — verify deleted, activity log `secret.deleted`.
10. `TestSecretsService_Delete_NotFound` — verify 404 for non-existent secret name.
11. `TestSecretsService_GetDecryptedSecrets` — create multiple secrets, verify all decrypted correctly as `map[string]string`.
12. `TestSecretsService_GetDecryptedSecrets_CorruptSecret` — simulate corrupt ciphertext, verify warning logged and secret skipped (not fatal).
13. `TestSecretsService_GetDecryptedSecrets_EmptySquad` — verify empty map returned for squad with no secrets.

#### GREEN — Implement

Create `internal/server/handlers/secrets_service.go`:

- `SecretsService` struct with `queries`, `dbConn`, `keyMgr` fields
- `NewSecretsService(q, dbConn, keyMgr)` constructor
- `Create(ctx, squadID, req)` — validate name/value (including min 8 chars, max 64KB), compute masked_hint, encrypt, insert with masked_hint, activity log
- `List(ctx, squadID)` — list all from DB, return response DTOs using pre-computed `masked_hint` (NO decryption)
- `Update(ctx, squadID, name, req)` — validate value, encrypt with new nonce, update, activity log
- `Delete(ctx, squadID, name)` — delete, activity log
- `GetDecryptedSecrets(ctx, squadID)` — list all, decrypt each, return `map[string]string`, log+skip failures

#### Files

- Create: `internal/server/handlers/secrets_service.go`
- Create: `internal/server/handlers/secrets_service_test.go`

---

### [ ] Task 06 — SecretsService: Master Key Rotation

**Requirements:** REQ-SEC-030, REQ-SEC-031, REQ-SEC-032, REQ-SEC-033, REQ-SEC-051
**Estimated time:** 30 min

#### Context

Master key rotation re-encrypts all secrets across all squads atomically. It generates a new master key, decrypts each secret with the old key, re-encrypts with the new key and a fresh nonce, and updates all rows in a single database transaction. If any step fails, the transaction is rolled back and the old key is preserved.

#### RED — Write Failing Tests

Extend `internal/server/handlers/secrets_service_test.go`:

1. `TestSecretsService_RotateMasterKey` — create secrets in multiple squads, rotate, verify all secrets still decrypt correctly with the new key, old key no longer works.
2. `TestSecretsService_RotateMasterKey_EmptyDB` — verify rotation succeeds with 0 secrets (no-op).
3. `TestSecretsService_RotateMasterKey_ActivityLog` — verify activity log entry `secrets.master_key_rotated` with correct rotated count.
4. `TestSecretsService_RotateMasterKey_Atomic` — simulate failure mid-rotation (e.g., corrupt one secret), verify transaction rolled back and all secrets still decrypt with old key.

#### GREEN — Implement

Add to `internal/server/handlers/secrets_service.go`:

- `RotateMasterKey(ctx) (int, error)` — (1) generate new key via `keyMgr.RotateKey()`, (2) write new key to `{dataDir}/master.key.tmp` temp file, (3) begin transaction, list all secrets, decrypt with old key, re-encrypt each with new key + fresh nonce, update all rows, (4) commit transaction, (5) rename `master.key.tmp` to `master.key`. On transaction failure: delete temp file, restore old key in memory, rollback. This ordering eliminates the crash window where the new key is persisted but secrets are not yet re-encrypted. Log activity on success.

#### Files

- Modify: `internal/server/handlers/secrets_service.go`
- Modify: `internal/server/handlers/secrets_service_test.go`

---

### [ ] Task 07 — SecretHandler: HTTP Endpoints

**Requirements:** REQ-SEC-020, REQ-SEC-021, REQ-SEC-022, REQ-SEC-023, REQ-SEC-024, REQ-SEC-025, REQ-SEC-026, REQ-SEC-027, REQ-SEC-028, REQ-SEC-030, REQ-SEC-033
**Estimated time:** 45 min

#### Context

The HTTP handler exposes the secret REST API. It handles auth (JWT), input validation, squad membership checks, and delegates to `SecretsService` for business logic. Follow the pattern from existing handlers like `pipeline_handler.go`.

#### RED — Write Failing Tests

Write `internal/server/handlers/secret_handler_test.go`:

1. `TestCreateSecret` — POST with valid body, verify 201 and response shape (id, name, maskedValue, no plaintext).
2. `TestCreateSecret_InvalidName` — lowercase name, verify 400 with `VALIDATION_ERROR`.
3. `TestCreateSecret_EmptyValue` — empty value, verify 400 with `VALIDATION_ERROR`.
3a. `TestCreateSecret_ShortValue` — value shorter than 8 chars, verify 400 with `VALIDATION_ERROR`.
3b. `TestCreateSecret_OversizedValue` — value exceeding 64KB, verify 400 with `VALIDATION_ERROR`.
4. `TestCreateSecret_DuplicateName` — verify 409 with `SECRET_NAME_CONFLICT`.
5. `TestListSecrets` — GET, verify response with masked values, ordered by name.
6. `TestListSecrets_SquadIsolation` — verify 403 for non-squad-member.
7. `TestUpdateSecret` — PUT with new value, verify 200 and response shape.
8. `TestUpdateSecret_NotFound` — verify 404.
9. `TestUpdateSecret_EmptyValue` — verify 400 with `VALIDATION_ERROR`.
10. `TestDeleteSecret` — DELETE, verify 204.
11. `TestDeleteSecret_NotFound` — verify 404.
12. `TestRotateMasterKey` — POST, verify 200 with rotated count.
12a. `TestRotateMasterKey_NonAdmin` — POST as non-admin user (`is_admin=false`), verify 403.
12b. `TestRotateMasterKey_Admin` — POST as admin user (`is_admin=true`), verify 200.
13. `TestAllEndpoints_RequireAuth` — verify 401 for unauthenticated requests.
13a. `TestSecretEndpoints_ViewerDenied` — verify all CRUD endpoints return 403 for users with viewer role.

#### GREEN — Implement

Create `internal/server/handlers/secret_handler.go`:

- `SecretHandler` struct with `queries`, `secretsSvc`
- `NewSecretHandler(queries, secretsSvc)` constructor
- `RegisterRoutes(mux)` — register all 5 routes
- `CreateSecret(w, r)` — parse squadId from URL, parse body, delegate to service
- `ListSecrets(w, r)` — parse squadId, squad membership check, delegate
- `UpdateSecret(w, r)` — parse squadId and name from URL, parse body, delegate
- `DeleteSecret(w, r)` — parse squadId and name, delegate
- `RotateMasterKey(w, r)` — auth check (admin only), delegate

#### Files

- Create: `internal/server/handlers/secret_handler.go`
- Create: `internal/server/handlers/secret_handler_test.go`

---

### [ ] Task 08 — RunService Integration: Secret Injection into Agent Runs

**Requirements:** REQ-SEC-040, REQ-SEC-041, REQ-SEC-042, REQ-SEC-043
**Estimated time:** 30 min

#### Context

Modify `RunService` to accept `SecretsService` and inject decrypted secrets into `InvokeInput.EnvVars` during `buildInvokeInput`. Secrets are prefixed with `ARI_SECRET_` and injected after core env vars but before prompt assembly. Failed decryption is logged and skipped.

#### RED — Write Failing Tests

Extend `internal/server/handlers/run_handler_test.go` (or create a new test file):

1. `TestBuildInvokeInput_SecretsInjected` — create secrets for a squad, build invoke input, verify `ARI_SECRET_{NAME}` keys present in envVars with correct decrypted values. Verify info log line listing injected secret names (not values).
2. `TestBuildInvokeInput_SecretsNoOverrideCoreVars` — create a secret named `API_URL` (which would become `ARI_SECRET_API_URL`), verify it does NOT override `ARI_API_URL`.
3. `TestBuildInvokeInput_SecretsDecryptionFailure` — simulate a corrupt secret, verify warning logged, other secrets still injected, run not aborted.
4. `TestBuildInvokeInput_NoSecrets` — squad with no secrets, verify envVars has no `ARI_SECRET_*` keys and no errors.
5. `TestBuildInvokeInput_SecretsServiceNil` — verify graceful handling when secretsSvc is nil (backward compatibility during migration).

#### GREEN — Implement

Modify `internal/server/handlers/run_handler.go`:

- Add `secretsSvc *SecretsService` field to `RunService`
- Update `NewRunService` to accept `*SecretsService` parameter (or add a setter `SetSecretsService`)
- In `buildInvokeInput`, after building base envVars and before prompt assembly, call `s.secretsSvc.GetDecryptedSecrets(ctx, wakeup.SquadID)` and inject results with `ARI_SECRET_` prefix
- Guard against nil `secretsSvc` for backward compatibility

#### Files

- Modify: `internal/server/handlers/run_handler.go`
- Modify or create: `internal/server/handlers/run_handler_test.go`

---

### [ ] Task 09 — Server Wiring and React UI

**Requirements:** All (integration), REQ-SEC-NF-001, REQ-SEC-NF-002
**Estimated time:** 60 min

#### Context

Wire `MasterKeyManager`, `SecretsService`, and `SecretHandler` into server initialization. Pass `SecretsService` to `RunService` for injection. Register routes. Build the React UI for secret management.

#### RED — Write Failing Tests

Write integration tests:

1. `TestFullSecretLifecycle` — end-to-end: create squad, create secret, list (verify masked), update, delete, verify activity log entries.
2. `TestSecretInjectionE2E` — create secret, trigger agent invocation, verify secret appears in InvokeInput envVars.
3. `TestMasterKeyRotationE2E` — create secrets, rotate master key, verify secrets still accessible.
4. `TestSquadDeletion_BlockedBySecrets` — attempt to delete squad with existing secrets, verify deletion fails (ON DELETE RESTRICT). Then delete all secrets, verify squad deletion succeeds.

Frontend tests (verify component rendering):

5. `SecretsPage` renders secret list from API with masked values.
6. `CreateSecretDialog` validates name pattern client-side.
7. `UpdateSecretDialog` shows name as read-only with value input.

#### GREEN — Implement

**Server wiring** — Modify `cmd/ari/run.go` or `internal/server/server.go`:

- Initialize `MasterKeyManager` with `os.Getenv("ARI_MASTER_KEY")` and `config.DataDir` (do NOT store master key in Config struct)
- Create `SecretsService` with dependencies
- Create `SecretHandler` and call `RegisterRoutes(mux)`
- Pass `SecretsService` to `RunService` via constructor or setter

**Config** — `internal/config/config.go` is NOT modified. The master key is read directly from `os.Getenv("ARI_MASTER_KEY")` during `MasterKeyManager` initialization, not stored in any config struct.

**React UI** — Create components:

- `web/src/pages/SecretsPage.tsx` — list view with create button
- `web/src/components/secrets/SecretsList.tsx` — table with masked values and actions
- `web/src/components/secrets/CreateSecretDialog.tsx` — modal form for creating secrets
- `web/src/components/secrets/UpdateSecretDialog.tsx` — modal form for updating secret values
- `web/src/hooks/useSecrets.ts` — API client hook
- Add route to `web/src/App.tsx` and sidebar navigation link

#### Files

- Modify: `cmd/ari/run.go` or `internal/server/server.go` (server initialization, read ARI_MASTER_KEY directly from env)
- Create: `web/src/pages/SecretsPage.tsx`
- Create: `web/src/components/secrets/SecretsList.tsx`
- Create: `web/src/components/secrets/CreateSecretDialog.tsx`
- Create: `web/src/components/secrets/UpdateSecretDialog.tsx`
- Create: `web/src/hooks/useSecrets.ts`
- Modify: `web/src/App.tsx` (add route)
- Modify: sidebar/nav component (add secrets link)

---

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-SEC-001 | Task 01, Task 03 |
| REQ-SEC-002 | Task 01, Task 03 |
| REQ-SEC-003 | Task 03 |
| REQ-SEC-004 | Task 01, Task 03 |
| REQ-SEC-005 | Task 03, Task 07 |
| REQ-SEC-006 | Task 02, Task 05 |
| REQ-SEC-007 | Task 02, Task 05 |
| REQ-SEC-010 | Task 02 |
| REQ-SEC-011 | Task 02 |
| REQ-SEC-012 | Task 02 |
| REQ-SEC-013 | Task 02 |
| REQ-SEC-014 | Task 02 |
| REQ-SEC-020 | Task 04, Task 05, Task 07 |
| REQ-SEC-021 | Task 04, Task 05, Task 07 |
| REQ-SEC-022 | Task 04, Task 05, Task 07 |
| REQ-SEC-023 | Task 04, Task 05, Task 07 |
| REQ-SEC-024 | Task 07 |
| REQ-SEC-025 | Task 07 |
| REQ-SEC-026 | Task 04, Task 05, Task 07 |
| REQ-SEC-027 | Task 05, Task 07 |
| REQ-SEC-028 | Task 01, Task 05, Task 07 |
| REQ-SEC-030 | Task 04, Task 06, Task 07 |
| REQ-SEC-031 | Task 06 |
| REQ-SEC-032 | Task 06 |
| REQ-SEC-033 | Task 07 |
| REQ-SEC-040 | Task 08 |
| REQ-SEC-041 | Task 08 |
| REQ-SEC-042 | Task 05, Task 08 |
| REQ-SEC-043 | Task 05, Task 08 |
| REQ-SEC-050 | Task 05, Task 06 |
| REQ-SEC-051 | Task 06 |
| REQ-SEC-028a | Task 01, Task 05, Task 07 |
| REQ-SEC-028b | Task 01, Task 05, Task 07 |
| REQ-SEC-045 | Task 07 |
| REQ-SEC-046 | Task 07 |
| REQ-SEC-NF-001 | Task 03 (indexes), Task 09 |
| REQ-SEC-NF-002 | Task 08, Task 09 |
| REQ-SEC-NF-003 | Task 06 |
| REQ-SEC-NF-004 | Task 03 |
| REQ-SEC-NF-005 | Task 03 |
