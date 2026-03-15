# Design: Secrets Management

**Created:** 2026-03-15
**Status:** Ready for Implementation
**Feature:** 19-secrets-management
**Dependencies:** 11-agent-runtime

---

## 1. Architecture Overview

Secrets Management adds an encrypted credential vault layered on top of the existing squad-scoped data model. Secrets are stored in a new `squad_secrets` table with AES-256-GCM encrypted values. A `SecretsService` handles encryption, decryption, CRUD, and master key lifecycle. The `RunService.buildInvokeInput` method is extended to inject decrypted secrets into agent environment variables.

### High-Level Flow

```
User creates secret via API
        |
        v
SecretsService.Create():
  1. Validate name (uppercase pattern)
  2. Generate random 12-byte nonce
  3. Encrypt value with AES-256-GCM(masterKey, nonce, plaintext)
  4. Store (name, encrypted_value, nonce) in squad_secrets
        |
        v
Agent is invoked (RunService.Invoke)
        |
        v
RunService.buildInvokeInput():
  1. Load all secrets for squad
  2. Decrypt each with AES-256-GCM(masterKey, nonce, ciphertext)
  3. Inject as ARI_SECRET_{NAME} into envVars map
        |
        v
Adapter receives InvokeInput with secrets in EnvVars
        |
        v
buildEnv() merges them into subprocess environment
```

### Component Relationships

```
SecretHandler             <- HTTP CRUD for secrets
       |
       v
SecretsService            <- Business logic: encrypt, decrypt, CRUD, rotation
       |
       +---> sqlc Queries (squad_secrets)
       +---> MasterKeyManager (key loading/generation/rotation)
       +---> logActivity() (audit trail)
       |
       v
RunService (modified)     <- Injects decrypted secrets into InvokeInput.EnvVars
       |
       +---> SecretsService.GetDecryptedSecrets(squadID)
```

### Squad Isolation

All secrets are scoped to a single squad via `squad_id` foreign key. Cross-squad access is blocked at the handler level (squad membership check) and the database level (unique constraint on `(squad_id, name)`).

---

## 2. Database Schema

### Migration: `20260316000019_create_squad_secrets.sql`

```sql
-- +goose Up

-- Squad secrets vault (AES-256-GCM encrypted)
CREATE TABLE squad_secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id        UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    encrypted_value BYTEA NOT NULL,
    nonce           BYTEA NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_rotated_at TIMESTAMPTZ,

    CONSTRAINT uq_squad_secrets_squad_name UNIQUE (squad_id, name),
    CONSTRAINT chk_secret_name_format CHECK (name ~ '^[A-Z][A-Z0-9_]{0,127}$')
);

-- Index for listing secrets by squad
CREATE INDEX idx_squad_secrets_squad_id ON squad_secrets(squad_id);

-- Updated_at trigger
CREATE TRIGGER set_squad_secrets_updated_at
    BEFORE UPDATE ON squad_secrets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER IF EXISTS set_squad_secrets_updated_at ON squad_secrets;
DROP TABLE IF EXISTS squad_secrets;
```

### Column Notes

| Column | Type | Notes |
|--------|------|-------|
| `id` | UUID | Primary key, auto-generated |
| `squad_id` | UUID FK | References `squads(id)`, cascade delete |
| `name` | TEXT | Unique per squad, validated pattern `^[A-Z][A-Z0-9_]{0,127}$` |
| `encrypted_value` | BYTEA | AES-256-GCM ciphertext (includes auth tag) |
| `nonce` | BYTEA | 12-byte GCM nonce, unique per encryption |
| `created_at` | TIMESTAMPTZ | Auto-set on insert |
| `updated_at` | TIMESTAMPTZ | Auto-set on update via trigger |
| `last_rotated_at` | TIMESTAMPTZ | Set when value is rotated (update or key rotation) |

---

## 3. SQL Queries (sqlc)

### File: `internal/database/queries/squad_secrets.sql`

```sql
-- name: CreateSquadSecret :one
INSERT INTO squad_secrets (squad_id, name, encrypted_value, nonce)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSquadSecretByName :one
SELECT * FROM squad_secrets
WHERE squad_id = $1 AND name = $2;

-- name: ListSquadSecrets :many
SELECT id, squad_id, name, encrypted_value, nonce, created_at, updated_at, last_rotated_at
FROM squad_secrets
WHERE squad_id = $1
ORDER BY name ASC;

-- name: UpdateSquadSecretValue :one
UPDATE squad_secrets
SET encrypted_value = $1,
    nonce = $2,
    last_rotated_at = now()
WHERE squad_id = $3 AND name = $4
RETURNING *;

-- name: DeleteSquadSecret :exec
DELETE FROM squad_secrets
WHERE squad_id = $1 AND name = $2;

-- name: CountSquadSecrets :one
SELECT count(*) FROM squad_secrets
WHERE squad_id = $1;

-- name: ListAllSecrets :many
SELECT * FROM squad_secrets
ORDER BY squad_id, name;

-- name: UpdateSquadSecretEncryption :exec
UPDATE squad_secrets
SET encrypted_value = $1,
    nonce = $2,
    last_rotated_at = now()
WHERE id = $3;
```

---

## 4. Master Key Manager

### Package: `internal/secrets`

```go
// MasterKeyManager handles loading, generating, and rotating the master key.
type MasterKeyManager struct {
    key     [32]byte
    dataDir string
    mu      sync.RWMutex
}
```

### Key Derivation

- **From env var:** `sha256.Sum256([]byte(os.Getenv("ARI_MASTER_KEY")))` produces a deterministic 32-byte key.
- **Auto-generated:** `crypto/rand.Read(key[:])` produces a random 32-byte key, written to `{dataDir}/master.key` with `os.WriteFile(path, key[:], 0600)`.

### Key File Path

`{ARI_DATA_DIR}/master.key` (default: `./data/master.key`)

### Initialization Order

```
1. Check ARI_MASTER_KEY env var
   → If set: derive key via SHA-256, done
2. Check {dataDir}/master.key file
   → If exists: read 32 bytes, validate length, done
3. Generate new random key
   → Write to {dataDir}/master.key (0600)
   → Log warning: "Auto-generated master key. Set ARI_MASTER_KEY for production."
```

### Encryption / Decryption

```go
func (m *MasterKeyManager) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error)
func (m *MasterKeyManager) Decrypt(ciphertext, nonce []byte) (plaintext []byte, err error)
```

Both use `crypto/aes` + `crypto/cipher` GCM mode. The nonce is 12 bytes from `crypto/rand`.

---

## 5. SecretsService

### Package: `internal/server/handlers`

```go
type SecretsService struct {
    queries  *db.Queries
    dbConn   *sql.DB
    keyMgr   *secrets.MasterKeyManager
}
```

### Methods

| Method | Description |
|--------|-------------|
| `Create(ctx, squadID, name, plainValue)` | Validate name, encrypt, insert, log activity |
| `List(ctx, squadID)` | List secrets with masked values (last 4 chars) |
| `Update(ctx, squadID, name, plainValue)` | Re-encrypt with new nonce, update row, log activity |
| `Delete(ctx, squadID, name)` | Delete row, log activity |
| `GetDecryptedSecrets(ctx, squadID)` | Decrypt all secrets for injection — returns `map[string]string` |
| `RotateMasterKey(ctx)` | Re-encrypt all secrets with a new master key in a transaction |

### Masked Value Logic

```go
func maskValue(plaintext string) string {
    if len(plaintext) <= 4 {
        return "••••"
    }
    return strings.Repeat("•", 8) + plaintext[len(plaintext)-4:]
}
```

The mask is computed at list time by decrypting, masking, then discarding the plaintext. This ensures the mask is always current.

---

## 6. HTTP Handler

### Package: `internal/server/handlers`

### Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/api/squads/{squadId}/secrets` | `CreateSecret` | Create a new secret |
| GET | `/api/squads/{squadId}/secrets` | `ListSecrets` | List secrets (masked) |
| PUT | `/api/squads/{squadId}/secrets/{name}` | `UpdateSecret` | Update secret value |
| DELETE | `/api/squads/{squadId}/secrets/{name}` | `DeleteSecret` | Delete a secret |
| POST | `/api/secrets/rotate-master-key` | `RotateMasterKey` | Re-encrypt all secrets |

### Request/Response DTOs

```go
// CreateSecretRequest is the body for POST /api/squads/{id}/secrets.
type CreateSecretRequest struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}

// SecretResponse is the masked representation of a secret.
type SecretResponse struct {
    ID            uuid.UUID  `json:"id"`
    SquadID       uuid.UUID  `json:"squadId"`
    Name          string     `json:"name"`
    MaskedValue   string     `json:"maskedValue"`
    CreatedAt     time.Time  `json:"createdAt"`
    UpdatedAt     time.Time  `json:"updatedAt"`
    LastRotatedAt *time.Time `json:"lastRotatedAt,omitempty"`
}

// UpdateSecretRequest is the body for PUT /api/squads/{id}/secrets/{name}.
type UpdateSecretRequest struct {
    Value string `json:"value"`
}

// RotateMasterKeyResponse is the response for POST /api/secrets/rotate-master-key.
type RotateMasterKeyResponse struct {
    RotatedCount int `json:"rotatedCount"`
}
```

---

## 7. Secret Injection into Agent Runs

### Modified: `RunService.buildInvokeInput`

After building the base `envVars` map (ARI_API_URL, ARI_API_KEY, etc.), inject secrets:

```go
// Inject squad secrets as ARI_SECRET_* env vars
secrets, err := s.secretsSvc.GetDecryptedSecrets(ctx, wakeup.SquadID)
if err != nil {
    slog.Warn("failed to load secrets for injection", "squad_id", wakeup.SquadID, "error", err)
} else {
    for name, value := range secrets {
        envKey := "ARI_SECRET_" + name
        // Don't override core ARI_* vars
        if _, exists := envVars[envKey]; !exists {
            envVars[envKey] = value
        }
    }
}
```

### Injection Ordering

1. Core env vars: `ARI_API_URL`, `ARI_API_KEY`, `ARI_AGENT_ID`, etc.
2. Task/conversation context vars: `ARI_TASK_ID`, `ARI_CONVERSATION_ID`, etc.
3. **Secrets:** `ARI_SECRET_GITHUB_TOKEN`, `ARI_SECRET_OPENAI_API_KEY`, etc.
4. Prompt assembly: `ARI_PROMPT` (may reference secrets by env var name)

### Security Notes

- Secrets only exist in plaintext in Go memory and the subprocess environment.
- `buildEnv()` in the Claude adapter merges `input.EnvVars` at the highest precedence tier, so secrets cannot be overridden by `adapterConfig.env`.
- Secret values are never logged, never sent via SSE, never included in API responses.

---

## 8. Config Changes

### New config field in `Config` struct:

```go
// MasterKey is the AES-256 master encryption key for secrets.
// Set via ARI_MASTER_KEY env var. If empty, auto-generated.
MasterKey string
```

### Loading in `config.Load()`:

```go
cfg.MasterKey = os.Getenv("ARI_MASTER_KEY")
```

No validation at config level — the `MasterKeyManager` handles derivation/generation.

---

## 9. Domain Model

### Package: `internal/domain`

```go
// Secret represents a squad-scoped encrypted credential.
type Secret struct {
    ID              uuid.UUID
    SquadID         uuid.UUID
    Name            string
    EncryptedValue  []byte
    Nonce           []byte
    CreatedAt       time.Time
    UpdatedAt       time.Time
    LastRotatedAt   *time.Time
}

// CreateSecretRequest is the input for creating a new secret.
type CreateSecretRequest struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}

// UpdateSecretRequest is the input for updating a secret's value.
type UpdateSecretRequest struct {
    Value string `json:"value"`
}

// ValidateSecretName validates that a secret name matches the required pattern.
func ValidateSecretName(name string) error
// ValidateSecretValue validates that a secret value is non-empty.
func ValidateSecretValue(value string) error
```

### Name Validation Rules

- Must match `^[A-Z][A-Z0-9_]{0,127}$`
- Starts with uppercase letter
- Contains only uppercase letters, digits, underscores
- Length 1-128 characters
- Examples: `GITHUB_TOKEN`, `OPENAI_API_KEY`, `DB_PASSWORD`, `A`

---

## 10. Activity Logging

All secret operations emit activity log entries using the existing `logActivity()` pattern:

| Action | Entity Type | Metadata |
|--------|-------------|----------|
| `secret.created` | `secret` | `{"name": "GITHUB_TOKEN", "squad_id": "..."}` |
| `secret.updated` | `secret` | `{"name": "GITHUB_TOKEN", "squad_id": "..."}` |
| `secret.deleted` | `secret` | `{"name": "GITHUB_TOKEN", "squad_id": "..."}` |
| `secrets.master_key_rotated` | `system` | `{"rotated_count": 42}` |

**Security:** Metadata never contains secret values.

---

## 11. React UI Components

### Pages

- `web/src/pages/SecretsPage.tsx` — List secrets for the current squad with create/update/delete actions.

### Components

- `web/src/components/secrets/SecretsList.tsx` — Table of secrets showing name, masked value, timestamps, and action buttons (edit, delete).
- `web/src/components/secrets/CreateSecretDialog.tsx` — Modal form for creating a new secret (name + value inputs).
- `web/src/components/secrets/UpdateSecretDialog.tsx` — Modal form for updating a secret's value (value input only, name shown as read-only).

### Hooks

- `web/src/hooks/useSecrets.ts` — API client hook: `listSecrets`, `createSecret`, `updateSecret`, `deleteSecret`.

### UI Behavior

- Secret values are shown as masked (e.g., `••••••••3abc`).
- The "Create Secret" form validates the name pattern client-side before submission.
- The "Update Secret" form shows only a value input — the name cannot be changed.
- Delete requires confirmation dialog.
- After create/update/delete, the list refreshes automatically.

---

## 12. Server Wiring

### Initialization Order (in `cmd/ari/run.go`):

```
1. Load config (includes MasterKey from env)
2. Initialize MasterKeyManager(config.MasterKey, config.DataDir)
3. Create SecretsService(queries, dbConn, keyMgr)
4. Create SecretHandler(queries, secretsSvc)
5. Register secret routes on mux
6. Pass SecretsService to RunService (for injection)
```

### RunService Changes

Add `secretsSvc *SecretsService` field to `RunService`. Inject via constructor or setter:

```go
func NewRunService(
    dbConn *sql.DB,
    queries *db.Queries,
    registry *adapter.Registry,
    tokenSvc *auth.RunTokenService,
    sseHub *sse.Hub,
    apiURL string,
    secretsSvc *SecretsService, // NEW
) *RunService
```

---

## 13. End-to-End Scenario

```
1. Operator starts Ari with ARI_MASTER_KEY=my-production-key-here
   → MasterKeyManager derives key via SHA-256

2. User creates squad "DevTeam" and agent "Coder" (claude_local adapter)

3. User creates secrets:
   POST /api/squads/{squadId}/secrets
   {"name": "GITHUB_TOKEN", "value": "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
   → Encrypted and stored in squad_secrets
   → Activity log: secret.created

4. User creates issue assigned to Coder, Ari wakes agent

5. RunService.buildInvokeInput:
   - Loads secrets for DevTeam squad
   - Decrypts GITHUB_TOKEN
   - Sets envVars["ARI_SECRET_GITHUB_TOKEN"] = "ghp_xxxx..."

6. Claude adapter receives InvokeInput with secret in EnvVars
   - buildEnv() includes ARI_SECRET_GITHUB_TOKEN in subprocess env

7. Agent uses GITHUB_TOKEN to push code to GitHub

8. User rotates the token:
   PUT /api/squads/{squadId}/secrets/GITHUB_TOKEN
   {"value": "ghp_yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"}
   → Re-encrypted with new nonce, last_rotated_at updated

9. Next agent run picks up the new token automatically
```
