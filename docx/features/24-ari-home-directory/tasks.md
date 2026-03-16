# Tasks: Ari Home Directory

**Feature:** 24-ari-home-directory
**Created:** 2026-03-17
**Status:** In Progress

## Requirement Traceability

- Source requirements: [requirements.md](requirements.md)
- Design: [design.md](design.md)

### Requirements Coverage

| Requirement | Task(s) | Notes |
|---|---|---|
| REQ-100 | Task 1 | Home dir at ~/.ari/ |
| REQ-101 | Task 1 | Multi-instance layout ~/.ari/realms/{id}/ |
| REQ-102 | Task 1 | ARI_HOME env var override |
| REQ-103 | Task 1 | ARI_INSTANCE_ID env var |
| REQ-104 | Task 2 | JSON config schema |
| REQ-105 | Task 2 | Config merge: file < env < CLI flags |
| REQ-106 | Task 2 | Config discovery (walk up cwd) |
| REQ-107 | Task 3 | ari init / auto-init on first run |
| REQ-108 | Task 3 | Directory scaffold: db/, logs/, secrets/, data/, workspaces/ |
| REQ-109 | Task 3 | Master key auto-generation |
| REQ-110 | Task 3 | Default config.json generation |
| REQ-111 | Task 3 | .env generation with JWT secret |
| REQ-112 | Task 4 | Wire config into server startup |
| REQ-113 | Task 4 | Backward compat with ./data/ |
| REQ-114 | Task 5 | Per-agent workspace directories |
| REQ-115 | Task 5 | Wire workspace into adapter WorkingDir |
| REQ-116 | Task 6 | Auto-backup via pg_dump |
| REQ-117 | Task 6 | Backup retention cleanup |
| REQ-118 | Task 7 | Migration CLI: ari migrate-home |
| REQ-119 | Task 8 | Integration and verification |

## Progress Summary

- Total Tasks: 8
- Completed: 2/8
- In Progress: Task 3
- Test Coverage: Existing

## Implementation Approach

Work bottom-up: path resolution primitives first (no dependencies), then config schema (depends on paths), then directory initialization (depends on config + paths), then wiring into the server (depends on all above), then agent workspaces, auto-backup, migration CLI, and finally integration tests. Each task follows the Red-Green-Refactor TDD cycle.

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 1 — Path Resolution Package (`internal/home/`)

**Linked Requirements:** REQ-100, REQ-101, REQ-102, REQ-103
**Estimated time:** 45 min

#### Context

Create the `internal/home/` package that resolves all Ari directory paths. This is the foundation: every other task depends on it. The package must support multi-instance layouts under `~/.ari/realms/{id}/` and allow overrides via `ARI_HOME` and `ARI_INSTANCE_ID` environment variables.

#### RED Phase — Write Failing Tests

Write `internal/home/home_test.go`:

1. `TestHomeDir_Default` — verify default returns `~/.ari` (using `os.UserHomeDir()`).
2. `TestHomeDir_ARI_HOME_Override` — set `ARI_HOME=/tmp/custom-ari`, verify `HomeDir()` returns `/tmp/custom-ari`.
3. `TestInstanceRoot_Default` — verify `InstanceRoot()` returns `~/.ari/realms/default` when `ARI_INSTANCE_ID` is unset.
4. `TestInstanceRoot_CustomID` — set `ARI_INSTANCE_ID=staging`, verify `InstanceRoot()` returns `~/.ari/realms/staging`.
5. `TestInstanceRoot_ARI_HOME_And_InstanceID` — set both env vars, verify combined path.
6. `TestSubdirectories` — verify `ConfigPath()`, `DBDir()`, `LogsDir()`, `SecretsDir()`, `WorkspacesDir()`, `DataDir()` all return correct paths under instance root.
7. `TestAgentWorkspaceDir` — verify `AgentWorkspaceDir(agentID)` returns `{instanceRoot}/workspaces/{agentID}`.
8. `TestValidateInstanceID_Valid` — verify accepted: `default`, `staging`, `my-project-1`, `prod_v2`.
9. `TestValidateInstanceID_Invalid` — verify rejected: empty string, contains `/`, contains `..`, starts with `-`, longer than 64 characters, contains spaces, contains special characters.
10. `TestValidateInstanceID_PathTraversal` — verify `../escape` and similar patterns are rejected.

#### GREEN Phase — Implement

Create `internal/home/home.go`:

- `HomeDir() string` — returns `ARI_HOME` or `~/.ari`
- `InstanceRoot() string` — returns `{homeDir}/realms/{instanceID}`
- `ConfigPath() string` — returns `{instanceRoot}/config.json`
- `DBDir() string` — returns `{instanceRoot}/db`
- `LogsDir() string` — returns `{instanceRoot}/logs`
- `SecretsDir() string` — returns `{instanceRoot}/secrets`
- `DataDir() string` — returns `{instanceRoot}/data`
- `WorkspacesDir() string` — returns `{instanceRoot}/workspaces`
- `AgentWorkspaceDir(agentID string) string` — returns `{instanceRoot}/workspaces/{agentID}`
- `ValidateInstanceID(id string) error` — regex `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`, reject path traversal

Use `os.UserHomeDir()` for home detection. Cache nothing — always resolve from env vars so tests can swap them.

#### REFACTOR Phase

- Extract a `resolveEnvOrDefault(key, fallback)` helper if repeated
- Ensure all path functions are pure (no side effects, no directory creation)

#### Acceptance Criteria

- [ ] `HomeDir()` respects `ARI_HOME` env var
- [ ] `InstanceRoot()` respects `ARI_INSTANCE_ID` env var (default: `default`)
- [ ] All subdirectory functions return correct paths under instance root
- [ ] `AgentWorkspaceDir()` returns workspace path for given agent ID
- [ ] `ValidateInstanceID()` rejects invalid/dangerous instance IDs
- [ ] No directory creation in this package (paths only)

#### Files

- Create: `internal/home/home.go`
- Create: `internal/home/home_test.go`

---

### [x] Task 2 — Config Schema and Loading

**Linked Requirements:** REQ-104, REQ-105, REQ-106
**Estimated time:** 60 min

#### Context

Define a JSON config file schema that covers all Ari settings (database, server, auth, logging, secrets, storage). The config loader reads from file, merges with env vars, then CLI flags (highest priority). Config discovery walks up from cwd looking for `.ari/config.json`.

#### RED Phase — Write Failing Tests

Write `internal/home/config_test.go`:

1. `TestDefaultConfig` — verify `DefaultConfig()` returns a struct with all fields set to sensible defaults (port 3100, log level "info", etc.).
2. `TestLoadConfigFromFile` — write a JSON config file to temp dir, verify `LoadConfig(path)` parses all sections (database, server, auth, logging, secrets, storage).
3. `TestLoadConfigFromFile_PartialOverride` — write JSON with only `server.port = 8080`, verify other fields retain defaults.
4. `TestLoadConfigFromFile_InvalidJSON` — write malformed JSON, verify error returned.
5. `TestLoadConfigFromFile_NotFound` — pass nonexistent path, verify defaults returned (no error).
6. `TestMergeEnvVars` — set `ARI_PORT=9000`, `ARI_LOG_LEVEL=debug`, verify they override config file values.
7. `TestMergeCLIFlags` — verify CLI flag map overrides both file and env values.
8. `TestMergePrecedence` — set file port=3100, env port=9000, CLI port=8080, verify final value is 8080.
9. `TestDiscoverConfig_InCwd` — create `.ari/config.json` in temp dir, verify `DiscoverConfig(tempDir)` finds it.
10. `TestDiscoverConfig_InParent` — create `.ari/config.json` in parent dir, run discovery from child, verify found.
11. `TestDiscoverConfig_NotFound` — run discovery from temp dir with no config, verify returns empty path.
12. `TestDiscoverConfig_MaxDepth` — verify discovery stops after 10 levels (no infinite traversal).
13. `TestConfigSections` — verify all config sections serialize/deserialize correctly:
    - `database`: `url`, `embeddedPort`, `dataDir`
    - `server`: `host`, `port`, `shutdownTimeout`
    - `auth`: `deploymentMode`, `jwtSecret`, `sessionTTL`, `disableSignUp`
    - `logging`: `level`, `dir`
    - `secrets`: `masterKey`, `keyFilePath`
    - `storage`: `dataDir`, `backupInterval`, `backupRetention`

#### GREEN Phase — Implement

Create `internal/home/config.go`:

- `FileConfig` struct with nested sections: `Database`, `Server`, `Auth`, `Logging`, `Secrets`, `Storage`
- `DefaultConfig() *FileConfig` — all defaults
- `LoadConfig(path string) (*FileConfig, error)` — read JSON, unmarshal, merge with defaults
- `MergeEnvVars(cfg *FileConfig)` — overlay `ARI_*` env vars
- `MergeCLIFlags(cfg *FileConfig, flags map[string]interface{})` — overlay CLI flag values
- `DiscoverConfig(startDir string) string` — walk up looking for `.ari/config.json`, max 10 levels
- `ToAppConfig(fc *FileConfig) *config.Config` — convert FileConfig to existing `config.Config` for backward compat

#### REFACTOR Phase

- Use `json.Decoder` with `DisallowUnknownFields` for strict parsing (optional, warn instead of error)
- Extract section-level merge helpers to reduce repetition
- Ensure `ToAppConfig()` maps every field correctly to existing `config.Config`

#### Acceptance Criteria

- [ ] JSON config with all sections parses correctly
- [ ] Missing file returns defaults (not an error)
- [ ] Env vars override file values
- [ ] CLI flags override env vars
- [ ] Config discovery walks up from cwd, max 10 levels
- [ ] `ToAppConfig()` produces a valid `config.Config`

#### Files

- Create: `internal/home/config.go`
- Create: `internal/home/config_test.go`

---

### [ ] Task 3 — Initialize Home Directory Structure

**Linked Requirements:** REQ-107, REQ-108, REQ-109, REQ-110, REQ-111
**Estimated time:** 45 min

#### Context

Implement the `ari init` command and auto-init logic that creates the full home directory scaffold on first run. This creates `db/`, `logs/`, `secrets/`, `data/`, `workspaces/`, generates `master.key` if missing, writes a default `config.json`, and creates `.env` with a JWT secret.

#### RED Phase — Write Failing Tests

Write `internal/home/init_test.go`:

1. `TestInitHomeDir_CreatesAllDirectories` — call `InitHomeDir(tempRoot)`, verify `db/`, `logs/`, `secrets/`, `data/`, `workspaces/` all exist with 0700 permissions.
2. `TestInitHomeDir_CreatesMasterKey` — verify `secrets/master.key` created with 32 bytes and 0600 permissions.
3. `TestInitHomeDir_SkipsExistingMasterKey` — pre-create `secrets/master.key` with known content, call init, verify content unchanged.
4. `TestInitHomeDir_CreatesDefaultConfig` — verify `config.json` created with valid JSON and all default sections.
5. `TestInitHomeDir_SkipsExistingConfig` — pre-create `config.json` with custom port, call init, verify custom port preserved.
6. `TestInitHomeDir_CreatesEnvFile` — verify `.env` created containing `ARI_JWT_SECRET=<hex>` with 64+ hex chars.
7. `TestInitHomeDir_SkipsExistingEnvFile` — pre-create `.env` with known content, call init, verify unchanged.
8. `TestInitHomeDir_Idempotent` — call `InitHomeDir()` twice, verify no errors and no data loss.

Write `cmd/ari/init_test.go`:

9. `TestInitCommand_CreatesStructure` — run `ari init` command, verify directory structure created.
10. `TestInitCommand_CustomPath` — run `ari init --home /tmp/custom`, verify structure at custom path.

#### GREEN Phase — Implement

Create `internal/home/init.go`:

- `InitHomeDir(root string) error` — creates full directory scaffold
  - `os.MkdirAll` for each subdirectory with 0700
  - Generate `secrets/master.key` (32 random bytes, 0600) if not exists
  - Write `config.json` from `DefaultConfig()` if not exists
  - Write `.env` with generated `ARI_JWT_SECRET` if not exists

Create `cmd/ari/init.go`:

- `newInitCmd()` — Cobra command `ari init`
  - `--home` flag to override home directory
  - `--instance` flag to set instance ID
  - Calls `home.InitHomeDir(home.InstanceRoot())`
  - Prints summary of created directories/files

#### REFACTOR Phase

- Extract key generation to a shared helper (reuse in secrets package)
- Add a `--force` flag to overwrite existing config/env (with confirmation prompt)

#### Acceptance Criteria

- [ ] `ari init` creates full directory scaffold
- [ ] `master.key` generated with crypto/rand, 32 bytes, 0600 permissions
- [ ] `config.json` written with defaults
- [ ] `.env` written with generated JWT secret
- [ ] Idempotent: re-running does not overwrite existing files
- [ ] Auto-init on `ari run` when home dir does not exist

#### Files

- Create: `internal/home/init.go`
- Create: `internal/home/init_test.go`
- Create: `cmd/ari/init.go`
- Create: `cmd/ari/init_test.go`
- Modify: `cmd/ari/root.go` (register init command)

---

### [ ] Task 4 — Wire Config into Server Startup

**Linked Requirements:** REQ-112, REQ-113
**Estimated time:** 60 min

#### Context

Replace all hardcoded `./data/` references with config-resolved paths from `internal/home/`. Update embedded postgres, log output, and secrets service to use the new paths. Maintain backward compatibility: if `./data/` exists and no `~/.ari/` structure is found, use the legacy path with a deprecation warning.

#### RED Phase — Write Failing Tests

Write `internal/home/compat_test.go`:

1. `TestResolveLegacyDataDir_NoLegacy_NoHome` — neither `./data/` nor `~/.ari/` exist, verify returns home dir path (triggers auto-init).
2. `TestResolveLegacyDataDir_LegacyExists_NoHome` — `./data/` exists but no `~/.ari/`, verify returns `./data/` path.
3. `TestResolveLegacyDataDir_HomeExists` — `~/.ari/` exists, verify returns home dir path (ignores `./data/`).
4. `TestResolveLegacyDataDir_BothExist` — both exist, verify returns home dir path with info log about legacy dir.
5. `TestResolveLegacyWarning` — verify a deprecation warning is returned/logged when legacy path is used.

Write `internal/config/config_test.go` (add to existing):

6. `TestLoad_UsesHomeDir` — verify `config.Load()` resolves `DataDir` from home package when `ARI_DATA_DIR` is unset and home dir exists.
7. `TestLoad_ARI_DATA_DIR_Override` — verify explicit `ARI_DATA_DIR` still takes priority over home dir.

Update `cmd/ari/run_test.go` (or integration test):

8. `TestRunServer_AutoInit` — verify `runServer()` calls auto-init when home dir does not exist.
9. `TestRunServer_LegacyCompat` — verify `runServer()` uses `./data/` with warning when no home dir and `./data/` exists.

#### GREEN Phase — Implement

Create `internal/home/compat.go`:

- `ResolveDataDir() (string, bool)` — returns resolved data directory and whether legacy mode is active
  - Check `ARI_DATA_DIR` env var first (explicit override)
  - Check home dir exists → use `home.DataDir()`
  - Check `./data/` exists → use it with deprecation warning, return `legacy=true`
  - Fall back to home dir (will be created by auto-init)

Modify `cmd/ari/run.go`:

- Before `config.Load()`, call `home.ResolveDataDir()` to determine paths
- If home dir does not exist, call `home.InitHomeDir()` (auto-init)
- If legacy mode, log deprecation warning: "Using ./data/ — run `ari migrate-home` to migrate"
- Pass resolved paths to `config.Load()` or set env vars before load

Modify `internal/config/config.go`:

- Update `Load()` to accept an optional `DataDir` override parameter (or use the env var set by run.go)

Modify paths in `cmd/ari/run.go`:

- `database.Open()` — `cfg.DataDir` already used, but verify it flows through
- `secrets.NewMasterKeyManager()` — currently uses `cfg.DataDir`, verify
- `resolveJWTKey()` — currently uses `cfg.DataDir`, verify
- `handlers.NewRunService()` — passes `cfg.DataDir`, verify
- `handlers.NewRuntimeHandler()` — passes `cfg.DataDir`, verify

#### REFACTOR Phase

- Remove any remaining hardcoded `"./data"` strings outside of compat logic
- Add structured logging for which paths are being used on startup
- Print a startup banner showing instance root and data dir

#### Acceptance Criteria

- [ ] No hardcoded `./data/` paths remain (except compat detection)
- [ ] Embedded postgres uses `cfg.DataDir` from resolved home paths
- [ ] Secrets service uses resolved `SecretsDir()`
- [ ] JWT key resolution uses resolved `SecretsDir()`
- [ ] Log output directed to resolved `LogsDir()` (if file logging enabled)
- [ ] Backward compat: `./data/` works with deprecation warning
- [ ] Auto-init on first `ari run` when no home dir exists

#### Files

- Create: `internal/home/compat.go`
- Create: `internal/home/compat_test.go`
- Modify: `cmd/ari/run.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

---

### [ ] Task 5 — Per-Agent Workspace Directories

**Linked Requirements:** REQ-114, REQ-115
**Estimated time:** 30 min

#### Context

Create a workspace directory for each agent under `{instanceRoot}/workspaces/{agentID}/` at agent creation time. Wire the workspace path into the adapter's `InvokeInput.WorkingDir` resolution so agents run in their own isolated directories. Optionally clean up workspace on agent deletion.

#### RED Phase — Write Failing Tests

Write `internal/home/workspace_test.go`:

1. `TestEnsureAgentWorkspace_CreatesDir` — call `EnsureAgentWorkspace(root, agentID)`, verify directory exists with 0700 permissions.
2. `TestEnsureAgentWorkspace_Idempotent` — call twice, verify no error.
3. `TestEnsureAgentWorkspace_InvalidAgentID` — pass agent ID with path traversal (`../escape`), verify error.
4. `TestCleanupAgentWorkspace_RemovesDir` — create workspace, call cleanup, verify removed.
5. `TestCleanupAgentWorkspace_NonExistent` — call cleanup for nonexistent workspace, verify no error.

Update agent handler tests (`internal/server/handlers/agent_handler_test.go` or integration test):

6. `TestCreateAgent_CreatesWorkspace` — create agent via handler, verify workspace directory created.
7. `TestDeleteAgent_CleansWorkspace` — delete agent, verify workspace directory removed (when flag enabled).

Update run handler tests:

8. `TestRunService_UsesAgentWorkspace` — verify `InvokeInput.WorkingDir` is set to agent workspace path when no explicit `workingDir` in adapter config.
9. `TestRunService_ExplicitWorkingDir_OverridesWorkspace` — verify explicit `workingDir` in adapter config takes priority over workspace.

#### GREEN Phase — Implement

Create `internal/home/workspace.go`:

- `EnsureAgentWorkspace(root, agentID string) (string, error)` — creates `{root}/workspaces/{agentID}/` with 0700, returns path
- `CleanupAgentWorkspace(root, agentID string) error` — removes workspace directory
- Validate agent ID (UUID format, no path traversal)

Modify `internal/server/handlers/agent_handler.go`:

- In `CreateAgent()`, after DB insert, call `home.EnsureAgentWorkspace()`
- In `DeleteAgent()` (if exists), call `home.CleanupAgentWorkspace()` (behind a flag/config)

Modify `internal/server/handlers/run_handler.go` (or `run_service.go`):

- In `buildInvokeInput()` or equivalent, resolve `WorkingDir`:
  1. Use explicit `workingDir` from adapter config if set
  2. Fall back to `home.AgentWorkspaceDir(instanceRoot, agentID)`

#### REFACTOR Phase

- Add a config flag `storage.cleanupWorkspacesOnDelete` (default: false) to control deletion behavior
- Log workspace path on agent run start for debugging

#### Acceptance Criteria

- [ ] Workspace directory created at `{instanceRoot}/workspaces/{agentID}/` on agent creation
- [ ] Agent runs use workspace as working directory when no explicit override
- [ ] Explicit `workingDir` in adapter config takes priority
- [ ] Path traversal in agent ID rejected
- [ ] Workspace cleanup on deletion is optional (behind flag)

#### Files

- Create: `internal/home/workspace.go`
- Create: `internal/home/workspace_test.go`
- Modify: `internal/server/handlers/agent_handler.go`
- Modify: `internal/server/handlers/run_handler.go` (or `run_service.go`)

---

### [ ] Task 6 — Auto-Backup

**Linked Requirements:** REQ-116, REQ-117
**Estimated time:** 45 min

#### Context

Implement a background goroutine that runs `pg_dump` on a configurable interval (default: 60 minutes) and stores compressed backups in `{instanceRoot}/data/backups/`. Old backups beyond retention period (default: 30 days) are cleaned up automatically. The goroutine must shut down gracefully when the server stops.

#### RED Phase — Write Failing Tests

Write `internal/home/backup_test.go`:

1. `TestBackupFileName` — verify format: `ari-backup-{YYYYMMDD-HHMMSS}.sql.gz`.
2. `TestBackupRetention_RemovesOldFiles` — create backup files with timestamps beyond retention, call cleanup, verify old files removed.
3. `TestBackupRetention_KeepsRecentFiles` — create recent backup files, call cleanup, verify kept.
4. `TestBackupRetention_EmptyDir` — call cleanup on empty dir, verify no error.
5. `TestParseBackupTimestamp` — verify timestamp extraction from backup filename.

Write `internal/home/backup_service_test.go`:

6. `TestBackupService_RunsOnInterval` — create service with 100ms interval, verify backup function called at least twice within 500ms.
7. `TestBackupService_GracefulShutdown` — start service, cancel context, verify goroutine exits without panic.
8. `TestBackupService_SkipsWhenExternalDB` — verify no backups taken when using external PostgreSQL (not embedded).

Write `cmd/ari/backup_test.go` (extend existing):

9. `TestBackupCommand_Integration` — verify `ari backup` manual trigger creates a backup file.

#### GREEN Phase — Implement

Create `internal/home/backup.go`:

- `BackupFileName() string` — generates timestamped filename
- `CleanupOldBackups(dir string, retention time.Duration) error` — removes backups older than retention
- `ParseBackupTimestamp(filename string) (time.Time, error)` — extracts timestamp from filename

Create `internal/home/backup_service.go`:

- `BackupService` struct with `interval`, `retention`, `backupDir`, `databaseURL`, `cancel`
- `NewBackupService(cfg BackupConfig) *BackupService`
- `Start(ctx context.Context)` — background goroutine with `time.Ticker`
  - On each tick: run `pg_dump` → compress → write to backup dir → cleanup old backups
  - Skip if `databaseURL` is empty (external DB managed separately)
- `runBackup(ctx context.Context) error` — executes pg_dump, compresses output
- `Stop()` — cancels context, waits for goroutine to finish

Modify `cmd/ari/run.go`:

- After database is up, start `BackupService` if config `storage.backupInterval > 0`
- Add to graceful shutdown sequence

#### REFACTOR Phase

- Use `exec.CommandContext` for pg_dump to respect cancellation
- Add backup size logging for monitoring
- Consider adding a `ari backup` manual trigger command (reuse same logic)

#### Acceptance Criteria

- [ ] pg_dump runs on configurable interval (default: 60 min)
- [ ] Backups stored in `{instanceRoot}/data/backups/` with timestamp filenames
- [ ] Old backups beyond retention period (default: 30 days) auto-cleaned
- [ ] Graceful shutdown: in-progress backup completes before exit
- [ ] No backups for external PostgreSQL (only embedded)
- [ ] Backup files are gzip-compressed

#### Files

- Create: `internal/home/backup.go`
- Create: `internal/home/backup_test.go`
- Create: `internal/home/backup_service.go`
- Create: `internal/home/backup_service_test.go`
- Modify: `cmd/ari/run.go`

---

### [ ] Task 7 — Migration CLI (`ari migrate-home`)

**Linked Requirements:** REQ-118
**Estimated time:** 45 min

#### Context

Implement the `ari migrate-home` command that detects the legacy `./data/` directory, creates the new `~/.ari/realms/default/` structure, and moves all data. This is a one-time migration path for existing users.

#### RED Phase — Write Failing Tests

Write `internal/home/migrate_test.go`:

1. `TestDetectLegacyDir_Exists` — create `./data/` with DB files, verify detection returns true.
2. `TestDetectLegacyDir_NotExists` — verify detection returns false for empty dir.
3. `TestPlanMigration_AllItems` — create legacy structure with `data/db/`, `data/master.key`, `data/runs/`, verify plan lists all items to move.
4. `TestPlanMigration_PartialItems` — create legacy structure with only `data/db/`, verify plan only includes existing items.
5. `TestExecuteMigration_MovesDB` — execute migration, verify `db/` moved from `./data/db/` to `{instanceRoot}/db/`.
6. `TestExecuteMigration_MovesMasterKey` — verify `master.key` moved to `{instanceRoot}/secrets/master.key`.
7. `TestExecuteMigration_MovesRuns` — verify `runs/` moved to `{instanceRoot}/data/runs/`.
8. `TestExecuteMigration_GeneratesConfig` — verify `config.json` generated at `{instanceRoot}/config.json`.
9. `TestExecuteMigration_AlreadyMigrated` — `~/.ari/` exists with data, verify migration aborted with message.
10. `TestExecuteMigration_DryRun` — with `--dry-run` flag, verify no files moved but plan printed.

Write `cmd/ari/migrate_home_test.go`:

11. `TestMigrateHomeCommand_E2E` — set up legacy dir, run command, verify structure.
12. `TestMigrateHomeCommand_NoLegacyDir` — run with no `./data/`, verify helpful error message.

#### GREEN Phase — Implement

Create `internal/home/migrate.go`:

- `DetectLegacyDir(dir string) bool` — checks for `./data/` with DB artifacts
- `MigrationPlan` struct with `Items []MigrationItem` (source, destination, type)
- `PlanMigration(legacyDir, targetRoot string) (*MigrationPlan, error)` — builds migration plan
  - Map: `data/db/` → `{instanceRoot}/db/`
  - Map: `data/master.key` → `{instanceRoot}/secrets/master.key`
  - Map: `data/runs/` → `{instanceRoot}/data/runs/`
  - Map: `data/secrets/` → `{instanceRoot}/secrets/`
- `ExecuteMigration(plan *MigrationPlan, dryRun bool) (*MigrationReport, error)` — execute the plan
  - Create target directories
  - Move files/directories (prefer `os.Rename`, fall back to copy+delete for cross-device)
  - Generate `config.json` from defaults
  - Return report of what was moved
- `MigrationReport` struct with summary (items moved, errors, warnings)

Create `cmd/ari/migrate_home.go`:

- `newMigrateHomeCmd()` — Cobra command `ari migrate-home`
  - `--dry-run` flag: print plan without executing
  - `--source` flag: override legacy dir (default: `./data`)
  - `--instance` flag: target instance ID (default: `default`)
  - Print summary table of what was moved

#### REFACTOR Phase

- Add rollback support: if migration fails midway, restore moved files
- Create a `.migration-complete` marker file to prevent re-running

#### Acceptance Criteria

- [ ] Detects `./data/` directory with DB artifacts
- [ ] Creates `~/.ari/realms/default/` structure
- [ ] Moves `db/`, `master.key`, `runs/`, `secrets/` to correct locations
- [ ] Generates `config.json` from defaults
- [ ] `--dry-run` shows plan without executing
- [ ] Aborts if target already exists (prevents data loss)
- [ ] Prints clear summary of what was moved

#### Files

- Create: `internal/home/migrate.go`
- Create: `internal/home/migrate_test.go`
- Create: `cmd/ari/migrate_home.go`
- Create: `cmd/ari/migrate_home_test.go`
- Modify: `cmd/ari/root.go` (register migrate-home command)

---

### [ ] Task 8 — Integration and Verification

**Linked Requirements:** REQ-119
**Estimated time:** 45 min

#### Context

End-to-end tests verifying the full home directory lifecycle: fresh startup creating the structure, legacy compatibility with deprecation warning, migration from `./data/`, and agent workspace creation. These tests exercise the integration between all previous tasks.

#### RED Phase — Write Failing Tests

Write `cmd/ari/e2e_home_test.go`:

1. `TestE2E_FreshRun_CreatesHomeStructure` — start server with `ARI_HOME` set to temp dir, verify:
   - `realms/default/db/` exists
   - `realms/default/logs/` exists
   - `realms/default/secrets/` exists
   - `realms/default/secrets/master.key` exists
   - `realms/default/data/` exists
   - `realms/default/workspaces/` exists
   - `realms/default/config.json` exists and is valid JSON
   - `.env` exists with JWT secret

2. `TestE2E_LegacyDataDir_DeprecationWarning` — create `./data/` directory with DB artifacts, start server without `~/.ari/`, verify:
   - Server starts successfully using `./data/`
   - Log output contains deprecation warning mentioning `ari migrate-home`
   - No `~/.ari/` structure created (legacy mode)

3. `TestE2E_MigrateHome_MovesData` — set up legacy `./data/` with DB files and master.key:
   - Run `ari migrate-home` command
   - Verify `~/.ari/realms/default/db/` contains DB files
   - Verify `~/.ari/realms/default/secrets/master.key` contains key
   - Verify `config.json` generated
   - Verify original `./data/` is empty or contains only a marker file

4. `TestE2E_AgentWorkspace_CreatedOnAgentCreation` — start server, create agent via API:
   - POST to `/api/agents` to create a new agent
   - Verify `{instanceRoot}/workspaces/{agentID}/` directory exists
   - Verify workspace has 0700 permissions

5. `TestE2E_MultiInstance` — start two servers with different `ARI_INSTANCE_ID` values:
   - Instance `alpha` creates `~/.ari/realms/alpha/`
   - Instance `beta` creates `~/.ari/realms/beta/`
   - Verify data isolation (different DB dirs, different secrets)

6. `TestE2E_CustomARI_HOME` — set `ARI_HOME=/tmp/test-ari-home`, start server:
   - Verify structure created under `/tmp/test-ari-home/` instead of `~/.ari/`
   - Verify all paths resolve correctly

#### GREEN Phase — Implement

- All tests should pass once Tasks 1-7 are complete
- If any test fails, fix the underlying implementation in the relevant task's code
- Add test helpers for:
  - `setupLegacyDataDir(t, dir)` — creates a realistic `./data/` structure
  - `verifyHomeStructure(t, root)` — asserts all expected directories and files exist
  - `startTestServer(t, envVars)` — starts Ari with custom env vars for testing

#### REFACTOR Phase

- Extract test helpers into a shared `testutil` package if not already present
- Ensure all temp directories are cleaned up in test teardown
- Add timeout to E2E tests (30 second max per test)

#### Acceptance Criteria

- [ ] Fresh `ari run` creates full `~/.ari/` structure
- [ ] Existing `./data/` works with deprecation warning
- [ ] `ari migrate-home` moves data correctly
- [ ] Agent workspace created on agent creation
- [ ] Multi-instance isolation verified
- [ ] Custom `ARI_HOME` path works
- [ ] All tests clean up after themselves

#### Files

- Create: `cmd/ari/e2e_home_test.go`

---

## Commit Strategy

After each completed task:
```bash
git add internal/home/ cmd/ari/ internal/config/
git commit -m "feat(home): <task description>"
```

Suggested commit messages:
- Task 1: `feat(home): add path resolution package with multi-instance support`
- Task 2: `feat(home): add JSON config schema with file/env/CLI merge`
- Task 3: `feat(home): add ari init command and auto-init on first run`
- Task 4: `feat(home): wire config-resolved paths into server startup`
- Task 5: `feat(home): add per-agent workspace directories`
- Task 6: `feat(home): add auto-backup with pg_dump and retention cleanup`
- Task 7: `feat(home): add ari migrate-home CLI for legacy migration`
- Task 8: `feat(home): add E2E integration tests for home directory lifecycle`

## Notes

### Implementation Order

Tasks must be implemented in order 1 through 8. Each task builds on the previous:
- Task 1 is **prerequisite** for all others (path resolution)
- Task 2 depends on Task 1 (config references paths)
- Task 3 depends on Tasks 1-2 (init uses paths and config schema)
- Task 4 depends on Tasks 1-3 (wiring requires paths, config, and init)
- Task 5 depends on Task 1 (workspace paths) and Task 4 (wired into handlers)
- Task 6 depends on Task 1 (backup paths) and Task 4 (wired into startup)
- Task 7 depends on Tasks 1-3 (migration creates home structure)
- Task 8 depends on all prior tasks (integration verification)

### Backward Compatibility

- Existing `./data/` installations must continue working with a deprecation warning
- No data loss during migration — `ari migrate-home` must be explicitly run
- `ARI_DATA_DIR` env var override still works (highest priority)
- Config file values override defaults but not env vars or CLI flags

### Security Considerations

- `master.key` must be 0600 permissions (owner read/write only)
- Secrets directory must be 0700 (owner read/write/execute only)
- Instance IDs validated against path traversal attacks
- Agent IDs validated before workspace directory creation
- `.env` file must be 0600 permissions
