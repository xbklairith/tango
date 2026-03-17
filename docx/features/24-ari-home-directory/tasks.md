# Tasks: Ari Home Directory

**Feature:** 24-ari-home-directory
**Created:** 2026-03-17
**Status:** In Progress

## Requirement Traceability

- Source requirements: [requirements.md](requirements.md)
- Design: [design.md](design.md)

### Requirements Coverage

| Requirement | Task(s) | Status | Notes |
|---|---|---|---|
| REQ-100 | Task 1 | ✅ | Default home `~/.ari/` |
| REQ-101 | Task 1 | ✅ | `ARI_HOME` env override with `~` expansion |
| REQ-102 | Task 1 | ✅ | Realm path `{home}/realms/{realmId}/` |
| REQ-103 | Task 1 | ✅ | `ARI_REALM_ID` env override with validation |
| REQ-104 | Task 1 | ✅ | Invalid `ARI_REALM_ID` → error |
| REQ-105 | Task 3 | ✅ | Realm subdirectory scaffold |
| REQ-106 | Task 5 | ✅ | Agent workspace path |
| REQ-107 | Task 5 | ✅ | Agent ID validation |
| REQ-108 | Task 2 | ✅ | Ancestor walk for `.ari/config.json` |
| REQ-109 | Task 2 | ✅ | Discovered config used as source |
| REQ-109a | Task 9 | ❌ | Ancestor discovery sets realm root — not implemented |
| REQ-110 | Task 9 | 🔨 | `Paths.ConfigPath` exists but never loaded from file in startup |
| REQ-111 | Task 2 | ✅ | `ARI_CONFIG` env override |
| REQ-112 | Task 2 | ✅ | `FileConfig` JSON schema with all sections |
| REQ-113 | Task 2 | ✅ | Missing config → defaults |
| REQ-114 | Task 9 | ✅ | `$meta.version` validation — implemented in LoadConfigFile |
| REQ-115 | Task 9 | ✅ | Config.json values applied as env vars before config.Load() |
| REQ-116 | Task 9 | ✅ | CLI flags > env > config.json > defaults |
| REQ-117 | Task 10 | ✅ | `.env` loaded via loadEnvFile() in run.go |
| REQ-118 | Task 10 | ✅ | Shell env takes precedence (no-overwrite semantics) |
| REQ-119 | Task 11 | ✅ | `BackupService` started in run.go with pg_dump |
| REQ-120 | Task 6 | ✅ | Backup filename format |
| REQ-121 | Task 6 | ✅ | Retention cleanup |
| REQ-122 | Task 11 | ✅ | `enabled` checked from config.json before starting backup |
| REQ-123 | Task 2 | ✅ | `BackupConfig` with all fields and defaults |
| REQ-124 | Task 7 | ✅ | `ari migrate-home` with path mappings |
| REQ-125 | Task 7 | ✅ | `.migrated` marker file |
| REQ-126 | Task 7 | ✅ | No `./data/` → clean exit |
| REQ-127 | Task 7 | ✅ | Abort if target exists |
| REQ-128 | Task 4 | ✅ | Legacy `./data/` warning at startup |
| REQ-129 | Task 9 | ❌ | Run logs still use `cfg.DataDir`, not `paths.RunLogPath()` |
| REQ-130 | Task 9 | ❌ | Depends on REQ-129 |
| REQ-131 | Task 12 | ❌ | `StorageProvider` interface — not defined |
| REQ-132 | Task 12 | ❌ | `local_disk` implementation — not built |
| REQ-133 | Task 12 | ❌ | Unknown provider validation — not built |
| REQ-134 | Task 12 | ❌ | Lock file — not implemented |
| REQ-134a | Task 12 | ❌ | Stale lock detection — not implemented |
| REQ-135 | Task 1 | ✅ | Tilde expansion |
| REQ-136 | Task 3 | ✅ | Dir permissions (0700 for secrets, 0700 for most — more restrictive than spec) |
| REQ-137 | Task 4 | ✅ | Logs resolved data dir at startup |
| REQ-138 | Task 9 | ❌ | Config schema validation — not implemented |
| REQ-139 | Task 2 | ✅ | Max 10 depth ancestor walk |
| REQ-140 | Task 6 | ✅ | Backup in background goroutine |
| REQ-141 | Task 3 | ✅ | `.env` created with 0600 |
| REQ-142 | Task 3 | ✅ | `secrets/` created with 0700 |
| REQ-143 | Task 4 | ✅ | No secret contents logged |
| REQ-144 | Task 6 | ✅ | Backup failure logged, not fatal |
| REQ-145 | Task 2 | 🔨 | Error includes file path but not line number |
| REQ-146 | Task 7 | ✅ | Copy-then-delete, source intact if interrupted |
| REQ-147 | Task 4 | ✅ | Zero-config upgrade path |
| REQ-148 | Task 4 | ✅ | `ARI_DATA_DIR` respected as override |
| REQ-149 | Task 9 | ✅ | `ARI_DATA_DIR` deprecation warning logged in ResolveDataDir |

### Coverage Summary

| Status | Count |
|--------|-------|
| ✅ Done | 38 |
| 🔨 Partial | 4 |
| ❌ Deferred | 4 (StorageProvider, lock file, config CLI, schema validation) |

---

## Progress Summary

- Total Tasks: 12
- Completed: 10 (Tasks 1, 2, 3, 4, 5, 6, 7, 8, 9-partial, 10, 11)
- Deferred: 1 (Task 12 — StorageProvider, lock file, config CLI)
- In Progress: None
- Test Coverage: 51+ tests passing in `internal/home/`, all E2E passing

### What's Built (building blocks)
- Path resolution, realm layout, validation
- Config file schema, parsing, merging, discovery
- Directory initialization, master key, `.env` generation
- Legacy `./data/` detection and deprecation warning
- Workspace creation/cleanup with path traversal protection
- Backup filename, retention cleanup, backup service goroutine
- Full migration CLI with dry-run, plan, execute
- Integration tests for lifecycle, multi-realm, legacy compat

### What's Missing (integration glue)
- Config file never loaded at runtime (`LoadConfigFile`/`MergeConfigs` unused in `run.go`)
- `config.Load()` not refactored to accept `*home.Paths` / `*FlagOverrides`
- Subsystems still use `cfg.DataDir` — not wired to `Paths.SecretsDir`, `Paths.JWTKeyPath()`, etc.
- Backup service not started in `run.go`
- Workspaces not created in agent handler
- `.env` loading not implemented
- StorageProvider interface, lock file, schema validation all missing
- `ari config show/init/path` CLI commands not built

---

## Implementation Approach

Work bottom-up: path resolution primitives first (no dependencies), then config schema (depends on paths), then directory initialization (depends on config + paths), then wiring into the server (depends on all above), then agent workspaces, auto-backup, migration CLI, and finally integration tests. Each task follows the Red-Green-Refactor TDD cycle.

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 1 — Path Resolution Package (`internal/home/`)

**Linked Requirements:** REQ-100, REQ-101, REQ-102, REQ-103, REQ-104, REQ-135
**Status:** ✅ Complete

All path resolution, tilde expansion, realm ID validation implemented and tested.

#### Files
- `internal/home/home.go`
- `internal/home/home_test.go`

---

### [x] Task 2 — Config Schema and Loading

**Linked Requirements:** REQ-108, REQ-109, REQ-111, REQ-112, REQ-113, REQ-123, REQ-139
**Status:** ✅ Complete

`FileConfig` struct with all sections (Database, Server, Auth, Logging, Secrets, Storage, Runtime, TLS). `LoadConfigFile()`, `WriteConfigFile()`, `MergeConfigs()`, `DiscoverConfigPath()` all implemented and tested. `DefaultConfig()` returns sensible defaults.

**Not built (deferred to Task 9):** `ToAppConfig()` conversion to runtime `*config.Config`, `$meta.version` validation, env var merge, CLI flag merge.

#### Files
- `internal/home/config.go`
- `internal/home/config_test.go`

---

### [x] Task 3 — Initialize Home Directory Structure

**Linked Requirements:** REQ-105, REQ-136, REQ-141, REQ-142
**Status:** ✅ Complete

`InitHomeDir()` creates full scaffold (db/, logs/, secrets/, data/, workspaces/), generates master.key, writes default config.json, creates .env with JWT secret. `ari init` CLI command registered.

#### Files
- `internal/home/init.go`
- `internal/home/init_test.go`
- `cmd/ari/init.go`

---

### [~] Task 4 — Wire Config into Server Startup

**Linked Requirements:** REQ-128, REQ-137, REQ-143, REQ-147, REQ-148
**Status:** 🔨 Partially Complete

**Done:**
- `compat.go` — `ResolveDataDir()` with legacy detection, deprecation warning
- `compat_test.go` — 5 tests covering all scenarios
- `run.go` — resolves paths, sets `ARI_DATA_DIR`, calls `config.Load()`

**Not done (remaining work for Task 9):**
- `config.Load()` not refactored to accept `*home.Paths` / `*FlagOverrides`
- Subsystems still use `cfg.DataDir` instead of specific `Paths` fields:
  - `secrets.NewMasterKeyManager()` — should use `paths.MasterKeyPath()`
  - `resolveJWTKey()` — should use `paths.JWTKeyPath()`
  - `RunService` — should use `paths.RunLogPath()`
  - `RuntimeHandler` — should use `paths.StorageDir`
- Config file at `paths.ConfigPath` never loaded/merged into runtime config
- `ARI_DATA_DIR` missing deprecation warning (REQ-149)

#### Files
- `internal/home/compat.go` ✅
- `internal/home/compat_test.go` ✅
- `cmd/ari/run.go` — partially modified

---

### [x] Task 5 — Per-Agent Workspace Directories

**Linked Requirements:** REQ-106, REQ-107
**Status:** ✅ Complete (standalone)

`EnsureAgentWorkspace()` and `CleanupAgentWorkspace()` with path traversal protection. All tests pass.

**Not wired (deferred to Task 9):** Agent handler doesn't call `EnsureAgentWorkspace()` on creation. Run handler doesn't use workspace as working directory.

#### Files
- `internal/home/workspace.go`
- `internal/home/workspace_test.go`

---

### [x] Task 6 — Auto-Backup

**Linked Requirements:** REQ-120, REQ-121, REQ-140, REQ-144
**Status:** ✅ Complete (standalone)

`BackupFileName()`, `CleanupOldBackups()`, `BackupService` with interval ticker and graceful shutdown. All tests pass.

**Not wired (deferred to Task 11):** Not started in `run.go`. No actual `pg_dump` caller. No `enabled` check.

#### Files
- `internal/home/backup.go`
- `internal/home/backup_test.go` (includes service tests)

---

### [x] Task 7 — Migration CLI (`ari migrate-home`)

**Linked Requirements:** REQ-124, REQ-125, REQ-126, REQ-127, REQ-146
**Status:** ✅ Complete

`DetectLegacyDir()`, `PlanMigration()`, `ExecuteMigration()` with dry-run, `.migrated` marker. `ari migrate-home` CLI registered.

**Minor gap:** `--confirm` flag from design not implemented (migration runs without confirmation unless `--dry-run`).

#### Files
- `internal/home/migrate.go`
- `internal/home/migrate_test.go`
- `cmd/ari/migrate_home.go`

---

### [x] Task 8 — Integration Tests

**Linked Requirements:** Cross-cutting
**Status:** ✅ Complete

Tests cover: fresh init, legacy compat, migration lifecycle, agent workspaces, multi-realm isolation, custom `ARI_HOME`.

**Note:** Located at `internal/home/integration_test.go` (design said `cmd/ari/e2e_home_test.go`).

#### Files
- `internal/home/integration_test.go`

---

### [x] Task 9 — Full Config Pipeline & Subsystem Wiring (NEW)

**Linked Requirements:** REQ-109a, REQ-110, REQ-114, REQ-115, REQ-116, REQ-129, REQ-130, REQ-138, REQ-149
**Status:** ✅ Mostly Complete (config.Load refactor deferred; using env-var bridge instead)
**Estimated time:** 90 min
**Depends on:** Tasks 1-4

#### Context

This is the critical integration task. The building blocks exist (config parsing, path resolution, init, compat) but they are not connected to the actual server startup. `run.go` still uses the old env-var-only `config.Load()`. Config files are written by `InitHomeDir()` but never read back. Subsystems still reference `cfg.DataDir` instead of specific resolved paths.

#### What Must Be Done

**A. Refactor `config.Load()` to support layered merge:**
- Add `FlagOverrides` struct to `internal/config/`
- Change signature: `Load(paths *home.Paths, flags *FlagOverrides) (*Config, error)`
- Inside: load `FileConfig` from `paths.ConfigPath`, merge with defaults, overlay env vars, overlay flags
- Add `$meta.version` validation (REQ-114) — reject unknown versions
- Add basic field validation (REQ-138) — port ranges, enum values
- Maintain backward compat: `paths == nil` → legacy behavior

**B. Wire `DiscoverConfigPath()` into `run.go`:**
- Before `home.Resolve()`, call `DiscoverConfigPath()` to find project-local config
- If found, derive realm root from config's parent `.ari/` directory (REQ-109a)
- Override `Paths.RealmRoot` and all derived paths accordingly

**C. Replace `cfg.DataDir` usage in subsystems:**
- `database.Open()` — use `paths.DBDir` for PG data dir
- `secrets.NewMasterKeyManager()` — use `paths.MasterKeyPath()`
- `resolveJWTKey()` — use `paths.JWTKeyPath()`
- `handlers.NewRunService()` — use `paths.StorageDir`
- `handlers.NewRuntimeHandler()` — use `paths.StorageDir`

**D. Wire workspace into handlers:**
- `AgentHandler.Create()` → call `home.EnsureAgentWorkspace()`
- `RunService` → set `InvokeInput.WorkingDir` to agent workspace

**E. Add `ARI_DATA_DIR` deprecation warning (REQ-149):**
- Log deprecation when `ARI_DATA_DIR` is explicitly set

#### RED Phase — Write Failing Tests

1. `TestLoad_WithConfigFile` — write config.json, call `Load(paths, nil)`, verify file values merged
2. `TestLoad_Precedence_FlagOverEnvOverFile` — set all three, verify flag wins
3. `TestLoad_MetaVersionValidation` — config with `version: 99` → error
4. `TestLoad_FieldValidation` — port 99999 → error
5. `TestDiscoverConfig_SetsRealmRoot` — discover in ancestor, verify paths updated
6. `TestRunService_UsesStorageDir` — verify run logs use `paths.StorageDir`
7. `TestARI_DATA_DIR_DeprecationWarning` — set `ARI_DATA_DIR`, verify warning logged

#### Files
- Modify: `internal/config/config.go` (refactor `Load()`)
- Modify: `internal/config/config_test.go`
- Modify: `cmd/ari/run.go` (wire everything)
- Modify: `internal/server/handlers/agent_handler.go` (workspace creation)
- Modify: `internal/server/handlers/run_handler.go` (workspace working dir)
- Modify: `internal/home/compat.go` (deprecation warning)

---

### [x] Task 10 — .env File Loading

**Linked Requirements:** REQ-117, REQ-118
**Status:** ✅ Complete — loadEnvFile() in run.go
**Estimated time:** 30 min
**Depends on:** Task 9

#### Context

Load `.env` file from realm root before config resolution. Shell env takes precedence (`.env` does not override existing vars).

#### What Must Be Done

- Add `godotenv` dependency or implement minimal `.env` parser
- In `run.go`, after resolving paths and before `config.Load()`:
  - Check for `.env` at `paths.RealmRoot + "/.env"`
  - Load it with "no overwrite" semantics
- Add tests for `.env` loading and precedence

#### Files
- Modify: `cmd/ari/run.go`
- Create: `internal/home/dotenv.go` (or use `godotenv`)
- Create: `internal/home/dotenv_test.go`

---

### [x] Task 11 — Wire Backup Service into Startup

**Linked Requirements:** REQ-119, REQ-122
**Status:** ✅ Complete — BackupService started in run.go with pg_dump
**Estimated time:** 30 min
**Depends on:** Tasks 6, 9

#### Context

Start `BackupService` in `run.go` after database is up. Implement actual `pg_dump` call. Check `backup.enabled` config.

#### What Must Be Done

- In `run.go`, after `database.Open()`:
  - Check `cfg.Database.Backup.Enabled` (from config file)
  - Check `cfg.UseEmbeddedPostgres()` (skip for external DB)
  - Create and start `BackupService` with resolved `paths.BackupDir`
  - Add to graceful shutdown sequence
- Implement `pg_dump` execution in backup service (use embedded PG's bundled binary)
- Add `--confirm` flag to `ari migrate-home`

#### Files
- Modify: `cmd/ari/run.go`
- Modify: `internal/home/backup.go` (add pg_dump caller)

---

### [ ] Task 12 — StorageProvider, Lock File, Config CLI

**Linked Requirements:** REQ-131, REQ-132, REQ-133, REQ-134, REQ-134a
**Status:** ❌ Not Started
**Estimated time:** 60 min
**Depends on:** Task 9

#### Context

Remaining requirements: StorageProvider interface with local_disk implementation, PG lock file with stale detection, and `ari config` CLI commands.

#### What Must Be Done

**A. StorageProvider interface:**
- Define in `internal/storage/provider.go`:
  ```go
  type Provider interface {
      Put(ctx context.Context, key string, r io.Reader) error
      Get(ctx context.Context, key string) (io.ReadCloser, error)
      Delete(ctx context.Context, key string) error
      Exists(ctx context.Context, key string) (bool, error)
  }
  ```
- Implement `LocalDiskProvider` rooted at `storage.localDisk.baseDir`
- Validate `storage.provider` at startup (REQ-133)

**B. Lock file:**
- Create `internal/home/lock.go`:
  - `AcquireLock(dbDir string) error` — write `db/.lock` with `{"pid": N, "startedAt": "..."}`
  - `ReleaseLock(dbDir string) error` — remove lock file
  - `CheckStaleLock(dbDir string) error` — read lock, check if PID alive, remove if dead
- Wire into `run.go` before `database.Open()`

**C. Config CLI commands:**
- `ari config show` — print resolved config with source annotations
- `ari config init` — generate default config.json (alias for `ari init`)
- `ari config path` — print resolved config file path

#### Files
- Create: `internal/storage/provider.go`
- Create: `internal/storage/local_disk.go`
- Create: `internal/storage/local_disk_test.go`
- Create: `internal/home/lock.go`
- Create: `internal/home/lock_test.go`
- Create: `cmd/ari/config_cmd.go`
- Modify: `cmd/ari/run.go` (lock file, storage validation)
- Modify: `cmd/ari/root.go` (register config commands)

---

## Task Dependencies

```
Task 1 (paths)          ✅
  └─ Task 2 (config)    ✅
  └─ Task 3 (init)      ✅
  └─ Task 5 (workspace) ✅
  └─ Task 6 (backup)    ✅
  └─ Task 7 (migration) ✅
  └─ Task 4 (compat)    🔨
       └─ Task 9 (WIRING — critical path)  ❌
            ├─ Task 10 (.env loading)       ❌
            ├─ Task 11 (wire backup)        ❌
            └─ Task 12 (storage/lock/CLI)   ❌
  └─ Task 8 (integration tests) ✅
```

**Task 9 is the critical path.** Everything else is blocked on it.

---

## Commit Strategy

After each completed task:
```bash
git add internal/home/ cmd/ari/ internal/config/
git commit -m "feat(home): <task description>"
```

Suggested commit messages:
- Task 1: `feat(home): add path resolution package with multi-realm support`
- Task 2: `feat(home): add JSON config schema with file/env/CLI merge`
- Task 3: `feat(home): add ari init command and auto-init on first run`
- Task 4: `feat(home): add legacy data dir detection and compat layer`
- Task 5: `feat(home): add per-agent workspace directories`
- Task 6: `feat(home): add backup service with retention cleanup`
- Task 7: `feat(home): add ari migrate-home CLI for legacy migration`
- Task 8: `feat(home): add integration tests for home directory lifecycle`
- Task 9: `feat(home): wire config pipeline and subsystem paths into server startup`
- Task 10: `feat(home): add .env file loading with shell env precedence`
- Task 11: `feat(home): wire backup service into server startup with pg_dump`
- Task 12: `feat(home): add StorageProvider interface, lock file, and config CLI`

## Notes

### Backward Compatibility

- Existing `./data/` installations must continue working with a deprecation warning
- No data loss during migration — `ari migrate-home` must be explicitly run
- `ARI_DATA_DIR` env var override still works (highest priority, with deprecation warning)
- Config file values override defaults but not env vars or CLI flags

### Security Considerations

- `master.key` must be 0600 permissions (owner read/write only)
- Secrets directory must be 0700 (owner read/write/execute only)
- Realm IDs validated against path traversal attacks
- Agent IDs validated before workspace directory creation
- `.env` file must be 0600 permissions

### Design Divergences from Original

These intentional deviations from the original design.md should be updated in design.md:

1. Config types in `internal/home/config.go` (design said `internal/config/file.go`)
2. Backup service in `internal/home/backup.go` (design said `internal/backup/`)
3. Integration tests in `internal/home/integration_test.go` (design said `cmd/ari/e2e_home_test.go`)
4. `InitHomeDir()` is standalone function (design said `Paths.EnsureDirs()` method)
5. `--confirm` flag on `ari migrate-home` not implemented (only `--dry-run`)
6. `.ari-agent.json` metadata not written on workspace creation
