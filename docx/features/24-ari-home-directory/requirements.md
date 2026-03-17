# Requirements: Ari Home Directory

**Created:** 2026-03-17
**Status:** Draft

## Overview

Migrate Ari from project-relative storage (`./data/`) to a user-home-based directory structure (`~/.ari/`). Currently, Ari stores all state -- embedded Postgres data, secrets, and run logs -- in a `./data/` directory relative to the working directory. This is fragile: changing `cwd` loses all state, multiple projects collide, and there is no config file -- everything is environment variables. This feature introduces a `~/.ari/realms/{id}/` structure with a JSON config file, `.env` for secrets, auto-backup, per-agent workspaces, and a clean migration path from the old layout.

## Research Summary

Compared Ari's current config (`internal/config/config.go`) with Paperclip's home directory system (`server/src/home-paths.ts`, `server/src/paths.ts`, `packages/shared/src/config-schema.ts`, `packages/db/src/backup.ts`). Key findings:

- Paperclip uses `~/.paperclip/realms/{id}/` with subdirectories: `db/`, `logs/`, `secrets/`, `data/storage/`, `data/backups/`, `workspaces/`
- Paperclip resolves home via `PAPERCLIP_HOME` env var with `~` expansion, falling back to `os.homedir()/.paperclip`
- Paperclip supports realm IDs via `PAPERCLIP_INSTANCE_ID` (regex `[a-zA-Z0-9_-]+`, default `"default"`)
- Paperclip walks up from cwd looking for `.paperclip/config.json` (like `.git/` discovery)
- Paperclip config file has sections: `$meta`, `database`, `logging`, `server`, `auth`, `storage`, `secrets`
- Paperclip auto-backup runs on a configurable interval with retention-based cleanup
- Ari currently reads all config from env vars (`ARI_*`) with no config file support
- Ari uses `ARI_DATA_DIR` (default `./data`) for all local state

## Functional Requirements

### Directory Structure

- [REQ-100] WHEN Ari resolves its home directory THEN the system SHALL use `~/.ari/` as the default location
- [REQ-101] WHEN the `ARI_HOME` environment variable is set THEN the system SHALL use its value as the home directory instead of `~/.ari/`, supporting `~` prefix expansion
- [REQ-102] WHEN Ari resolves the active realm THEN the system SHALL use the directory `{home}/realms/{realmId}/` where `realmId` defaults to `"default"`
- [REQ-103] WHEN the `ARI_REALM_ID` environment variable is set THEN the system SHALL use its value as the realm identifier, validated against the pattern `^[a-zA-Z0-9_-]+$`
- [REQ-104] WHEN `ARI_REALM_ID` contains characters outside the allowed pattern THEN the system SHALL return an error and refuse to start

### Realm Directory Layout

- [REQ-105] WHEN a realm root is resolved THEN the system SHALL ensure the following subdirectory structure exists:
  - `config.json` -- realm configuration file
  - `.env` -- environment secrets (JWT secret, master key path)
  - `db/` -- embedded PostgreSQL data directory
  - `logs/` -- server and run logs
  - `secrets/` -- master key and JWT key files
  - `data/storage/` -- file storage (attachments, artifacts)
  - `data/backups/` -- database backup files
  - `data/runs/` -- run log output
  - `workspaces/` -- per-agent working directories

### Per-Agent Workspaces

- [REQ-106] WHEN the system resolves a workspace directory for an agent THEN the path SHALL be `{realmRoot}/workspaces/{agentId}/`
- [REQ-107] WHEN resolving an agent workspace path THEN the system SHALL validate the agent ID against `^[a-zA-Z0-9_-]+$` and reject invalid values

### Config File Discovery

- [REQ-108] WHEN Ari starts without an explicit config path THEN the system SHALL walk up from the current working directory looking for `.ari/config.json` in each ancestor directory (analogous to `.git/` discovery)
- [REQ-109] WHEN a `.ari/config.json` file is found via ancestor walk THEN the system SHALL use that file as the configuration source
- [REQ-109a] WHEN a `.ari/config.json` file is found via ancestor walk THEN the parent `.ari/` directory SHALL become the realm root for that session, overriding the default `{home}/realms/{realmId}/` path. All relative paths in the discovered config file resolve relative to that realm root.
- [REQ-110] WHEN no `.ari/config.json` is found via ancestor walk THEN the system SHALL fall back to the default realm config at `{home}/realms/{realmId}/config.json`
- [REQ-111] WHEN the `ARI_CONFIG` environment variable is set THEN the system SHALL use its value as the config file path, skipping ancestor discovery

### Config File Schema

- [REQ-112] WHEN loading a config file THEN the system SHALL parse a JSON object with the following top-level sections:
  - `$meta` -- schema version (integer `1`) and last-updated timestamp
  - `database` -- mode, connection string, embedded PG port, backup settings
  - `server` -- host, port, deployment mode, exposure, TLS settings
  - `auth` -- session TTL, disable signup, OAuth provider credentials
  - `logging` -- log level, log directory
  - `secrets` -- master key file path
  - `storage` -- provider (local_disk), base directory

- [REQ-113] WHEN the config file does not exist THEN the system SHALL start with default values (equivalent to current env-var-only behavior)
- [REQ-114] WHEN the config file contains an unknown `$meta.version` THEN the system SHALL return an error and refuse to start

### Config Precedence

- [REQ-115] WHEN resolving a configuration value THEN the system SHALL apply the following precedence (highest to lowest):
  1. CLI flags (e.g., `--port`)
  2. Environment variables (e.g., `ARI_PORT`)
  3. Config file values (`config.json`)
  4. Built-in defaults

- [REQ-116] WHEN a value is set at a higher-precedence level THEN the system SHALL ignore lower-precedence values for that same field

### Environment Secrets File

- [REQ-117] WHEN an `.env` file exists in the realm root THEN the system SHALL load it and apply its values as environment variables before config resolution
- [REQ-118] WHEN both the `.env` file and the shell environment define the same variable THEN the shell environment SHALL take precedence (`.env` does not override existing env vars)

### Database Auto-Backup

- [REQ-119] WHEN Ari starts with embedded PostgreSQL THEN the system SHALL schedule periodic database backups using `pg_dump`
- [REQ-120] WHEN the backup interval elapses THEN the system SHALL create a compressed backup file in `{realmRoot}/data/backups/` with the naming pattern `ari-backup-{YYYYMMDD-HHMMSS}.sql.gz`
- [REQ-121] WHEN a backup completes THEN the system SHALL delete backup files older than the configured retention period
- [REQ-122] WHEN `database.backup.enabled` is set to `false` in config THEN the system SHALL skip auto-backup entirely
- [REQ-123] WHEN configuring backup THEN the system SHALL support the following settings with defaults:
  - `enabled` -- boolean, default `true`
  - `intervalMinutes` -- integer 1..10080, default `60`
  - `retentionDays` -- integer 1..3650, default `30`
  - `dir` -- string, default `{realmRoot}/data/backups`

### Migration from `./data/`

- [REQ-124] WHEN the user runs `ari migrate-home` THEN the system SHALL move data from the legacy `./data/` directory to `~/.ari/realms/default/` preserving the following mappings:
  - `./data/embedded-postgres/` -> `~/.ari/realms/default/db/`
  - `./data/secrets/` -> `~/.ari/realms/default/secrets/`
  - `./data/runs/` -> `~/.ari/realms/default/data/runs/`
  - Note: Server logs in `./data/` are not migrated as they are ephemeral.

- [REQ-125] WHEN `ari migrate-home` completes successfully THEN the system SHALL create a marker file `./data/.migrated` containing the target path and timestamp to prevent accidental re-use of old data
- [REQ-126] WHEN `ari migrate-home` is run but `./data/` does not exist THEN the system SHALL print a message and exit cleanly (no error)
- [REQ-127] WHEN `ari migrate-home` is run but the target realm directory already contains data THEN the system SHALL abort with an error and NOT overwrite existing data
- [REQ-128] WHEN Ari starts and detects a `./data/` directory without a `.migrated` marker AND the configured data directory is `~/.ari/` THEN the system SHALL print a warning suggesting the user run `ari migrate-home`

### Run Log Storage

- [REQ-129] WHEN a run produces log output THEN the system SHALL write it to `{realmRoot}/data/runs/{runId}/` instead of the legacy `{dataDir}/runs/` path
- [REQ-130] WHEN the run log directory does not exist THEN the system SHALL create it on first write

### Storage Abstraction

- [REQ-131] WHEN the system stores or retrieves files THEN it SHALL use a `StorageProvider` interface with methods: `Put(ctx, key, reader) error`, `Get(ctx, key) (ReadCloser, error)`, `Delete(ctx, key) error`, `Exists(ctx, key) (bool, error)`
- [REQ-132] WHEN `storage.provider` is `"local_disk"` THEN the system SHALL implement the `StorageProvider` using the local filesystem rooted at `storage.localDisk.baseDir`
- [REQ-133] WHEN `storage.provider` is an unrecognized value THEN the system SHALL return an error at startup

### State-Driven Requirements

- [REQ-134] WHILE the Ari server is running with embedded PostgreSQL THEN the system SHALL maintain a lock file at `{realmRoot}/db/.lock` containing the process ID (PID) and start timestamp in JSON format
- [REQ-134a] WHEN Ari starts and a lock file exists THEN the system SHALL check if the PID in the lock file is still alive. If the process is dead, the lock file SHALL be treated as stale and removed. If the process is alive, the system SHALL refuse to start with an error.
- [REQ-135] WHILE resolving paths that begin with `~` THEN the system SHALL expand the prefix to the current user's home directory

### Ubiquitous Requirements

- [REQ-136] The system SHALL create missing directories in the realm tree on first access with permissions `0700` for `secrets/` and `0755` for all other directories
- [REQ-137] The system SHALL log the resolved realm root at startup (info level)
- [REQ-138] The system SHALL validate the config file against the schema on load and return descriptive errors for invalid fields

## Non-Functional Requirements

### Performance

- [REQ-139] The config file discovery (ancestor walk) SHALL complete in under 10ms on a filesystem with a depth of 20 directories
- [REQ-140] Database backup SHALL run in a background goroutine and not block request processing

### Security

- [REQ-141] The `.env` file SHALL be created with file permissions `0600` (owner read/write only)
- [REQ-142] The `secrets/` directory SHALL be created with permissions `0700` (owner only)
- [REQ-143] The system SHALL NOT log the contents of `.env` or secret key files

### Reliability

- [REQ-144] WHEN a backup fails THEN the system SHALL log the error and continue operating (backup failure is not fatal)
- [REQ-145] WHEN the config file is malformed JSON THEN the system SHALL return a descriptive parse error including the file path and line number if available
- [REQ-146] WHEN `ari migrate-home` is interrupted mid-copy THEN the system SHALL leave the source `./data/` intact (copy-then-delete, not move)

### Backward Compatibility

- [REQ-147] WHEN no config file exists and no `ARI_HOME` is set THEN the system SHALL behave identically to the current env-var-only configuration (zero-config upgrade path)
- [REQ-148] WHEN `ARI_DATA_DIR` is explicitly set THEN the system SHALL respect it as an override for the realm data directory, maintaining backward compatibility with existing deployments
- [REQ-149] WHEN `ARI_DATA_DIR` is explicitly set THEN the system SHALL use its value as the realm root directory, bypassing `ARI_HOME` and `ARI_REALM_ID` resolution. Config discovery and `.env` loading still operate relative to the `ARI_DATA_DIR` path. `ARI_DATA_DIR` is a legacy escape hatch and SHALL log a deprecation warning recommending migration to `ARI_HOME`.

## Constraints

- Must not break existing `config.Load()` callers -- new config resolution wraps the existing logic
- Must not require an external `pg_dump` binary -- use the embedded PostgreSQL's bundled tools or Go-native backup
- Config file format is JSON (not YAML, TOML, or HCL) to match Paperclip's convention and avoid new dependencies
- Realm IDs are restricted to `[a-zA-Z0-9_-]` to ensure safe filesystem paths across platforms
- The `StorageProvider` interface must be defined but only the `local_disk` implementation is built in this feature
- Path handling assumes POSIX semantics (tilde expansion, octal permissions, flock). Windows is not supported.

## Acceptance Criteria

- [ ] `ari run` starts with `~/.ari/realms/default/` as the data root when no env vars are set
- [ ] `ARI_HOME=/tmp/myari ari run` uses `/tmp/myari/realms/default/`
- [ ] `ARI_REALM_ID=staging ari run` uses `~/.ari/realms/staging/`
- [ ] Config file at `~/.ari/realms/default/config.json` is loaded and merged with env vars
- [ ] CLI flag `--port 4000` overrides both env var and config file
- [ ] Walking up from cwd discovers `.ari/config.json` and uses it
- [ ] `.env` file in realm root is loaded for secrets
- [ ] Auto-backup creates compressed snapshots on interval
- [ ] Old backups beyond retention period are deleted
- [ ] `ari migrate-home` moves `./data/` contents to `~/.ari/realms/default/`
- [ ] `ari migrate-home` refuses to overwrite existing realm data
- [ ] Warning printed at startup when legacy `./data/` exists without `.migrated`
- [ ] Run logs written to `{realmRoot}/data/runs/{runId}/`
- [ ] `StorageProvider` interface defined with `local_disk` implementation
- [ ] Lock file prevents two Ari processes from using the same realm's embedded PG
- [ ] `secrets/` directory created with `0700` permissions
- [ ] `.env` file created with `0600` permissions
- [ ] All existing tests pass without modification
- [ ] Config precedence: CLI flags > env vars > config.json > defaults

## Out of Scope

- S3 storage implementation (interface only, `local_disk` built)
- Remote/cloud realm management
- Web UI for config editing
- Config file migration tooling (auto-upgrading `$meta.version` across schema changes)
- Multi-realm orchestration (running multiple realms simultaneously)

## Dependencies

- Feature 01: Go Scaffold (CLI framework, Cobra commands)
- Feature 11: Agent Runtime (run log paths, `DataDir` usage)
- Feature 19: Secrets Management (master key path resolution)

## Risks & Assumptions

**Assumptions:**
- Users run Ari as a single realm per machine (multi-realm is an advanced use case via `ARI_REALM_ID`)
- The embedded PostgreSQL library (`embedded-postgres-go`) supports custom data directories (already proven with `ARI_DATA_DIR`)
- `pg_dump` equivalent functionality is available either via the embedded PG binary or a Go library

**Risks:**
- Existing deployments using `ARI_DATA_DIR=./data` must not break after upgrade -- mitigated by REQ-147 and REQ-148 (backward compatibility)
- Config file discovery walk could be slow on network-mounted filesystems -- mitigated by REQ-139 (10ms limit) and `ARI_CONFIG` escape hatch
- Backup of large databases may consume significant disk space -- mitigated by configurable retention (REQ-123) and ability to disable (REQ-122)
- Concurrent `ari migrate-home` invocations could corrupt data -- mitigated by REQ-127 (abort if target exists) and REQ-146 (copy-then-delete)

## References

- Paperclip home paths: `/Users/xb/builder/paperclip/server/src/home-paths.ts`
- Paperclip config discovery: `/Users/xb/builder/paperclip/server/src/paths.ts`
- Paperclip config schema: `/Users/xb/builder/paperclip/packages/shared/src/config-schema.ts`
- Paperclip auto-backup: `/Users/xb/builder/paperclip/packages/db/src/backup.ts`
- Paperclip instance config example: `/Users/xb/.paperclip/realms/default/config.json`
- Ari current startup: `/Users/xb/builder/ari/cmd/ari/run.go`
- Ari current config: `/Users/xb/builder/ari/internal/config/config.go`
