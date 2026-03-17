# Design: Ari Home Directory

**Created:** 2026-03-17
**Status:** Ready for Implementation
**Feature:** 24-ari-home-directory
**Dependencies:** 19-secrets-management, 11-agent-runtime

---

## 1. Architecture Overview

The Ari Home Directory feature replaces the flat `./data/` convention with a structured, XDG-inspired home directory at `~/.ari/`. This provides multi-realm support, config-file-driven settings, automatic backups, and per-agent workspaces. The design mirrors Paperclip's `~/.paperclip/` structure, adapted for Go and Ari's single-binary architecture.

### High-Level Data Flow

```
CLI invocation (ari run --port 3200)
        |
        v
home.Resolve()
  1. ARI_HOME env var -> or ~/.ari
  2. ARI_REALM_ID env var -> or "default"
  3. realmRoot = {homeDir}/realms/{realmId}
        |
        v
home.ResolveDataDir()
  1. ARI_DATA_DIR env var -> use as realmRoot (legacy, log deprecation)
  2. Otherwise -> use realmRoot from above
        |
        v
config.Load(paths, cliFlags)
  1. Read {realmRoot}/config.json  (file layer)
  2. Read ARI_* env vars              (env layer)
  3. Apply CLI flags                  (flag layer)
  4. Merge: flags > env > file > defaults
        |
        v
Resolved *config.Config used by all subsystems
  - database.Open() uses config.Database.*
  - secrets.NewMasterKeyManager() uses config.Secrets.*
  - server.New() uses config.Server.*
  - RunService uses config.Storage.* for run logs
  - Adapter uses config.Workspaces.* for agent dirs
```

### Directory Structure

```
~/.ari/
  realms/
    default/                    # realm root (ARI_REALM_ID=default)
      config.json               # persistent configuration
      db/                       # embedded postgres data
        postgres/               # PG data directory
        pg-runtime/             # PG runtime (sockets, pid)
      logs/                     # application log files
      secrets/
        master.key              # auto-generated AES-256 master key
        jwt.key                 # auto-generated JWT signing key
      data/
        backups/                # automated pg_dump backups
        storage/                # file storage (run logs, artifacts)
          runs/                 # run log JSONL files
      workspaces/               # per-agent working directories
        {agent-id}/             # created on agent creation
    staging/                    # example: second realm
      config.json
      db/
      ...
```

### Component Relationships

```
cmd/ari/root.go
  |
  +---> home.Resolve()             <- NEW: path resolution
  |       |
  |       +---> homeDir()          <- ~/.ari or ARI_HOME
  |       +---> realmRoot()     <- {home}/realms/{id}
  |       +---> configPath()       <- {root}/config.json
  |       +---> dbDir()            <- {root}/db
  |       +---> logsDir()          <- {root}/logs
  |       +---> secretsDir()       <- {root}/secrets
  |       +---> backupDir()        <- {root}/data/backups
  |       +---> storageDir()       <- {root}/data/storage
  |       +---> workspacesDir()    <- {root}/workspaces
  |       +---> agentWorkspaceDir  <- {root}/workspaces/{agentId}
  |
  +---> config.Load(paths, flags)  <- MODIFIED: layered config loading
  |       |
  |       +---> readConfigFile()   <- parse config.json
  |       +---> readEnvVars()      <- read ARI_* env vars
  |       +---> applyFlags()       <- CLI flag overrides
  |       +---> mergeWithDefaults  <- fill gaps with defaults
  |
  +---> database.Open(cfg)         <- MODIFIED: uses cfg.Database paths
  +---> secrets.NewMasterKeyManager(cfg)  <- MODIFIED: uses cfg.Secrets paths
  +---> handlers.NewRunService(cfg)       <- MODIFIED: uses cfg.Storage paths
```

---

## 2. New Package: `internal/home/`

### File: `internal/home/home.go`

```go
package home

import (
    "fmt"
    "os"
    "path/filepath"
    "regexp"
)

const (
    DefaultRealmID = "default"
    ConfigFileName    = "config.json"
)

var realmIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var pathSegmentRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Paths holds all resolved filesystem paths for an Ari realm.
type Paths struct {
    HomeDir       string // ~/.ari
    RealmID       string // "default"
    RealmRoot  string // ~/.ari/realms/default
    ConfigPath    string // ~/.ari/realms/default/config.json
    DBDir         string // ~/.ari/realms/default/db
    LogsDir       string // ~/.ari/realms/default/logs
    SecretsDir    string // ~/.ari/realms/default/secrets
    BackupDir     string // ~/.ari/realms/default/data/backups
    StorageDir    string // ~/.ari/realms/default/data/storage
    WorkspacesDir string // ~/.ari/realms/default/workspaces
}

// Resolve computes all paths from environment variables and defaults.
func Resolve() (*Paths, error) { ... }

// AgentWorkspaceDir returns the workspace directory for a specific agent.
// Validates agentId to prevent path traversal.
func (p *Paths) AgentWorkspaceDir(agentID string) (string, error) { ... }

// InitHomeDir creates all required directories with appropriate permissions.
func (p *Paths) InitHomeDir() error { ... }

// MasterKeyPath returns the path to the master encryption key.
func (p *Paths) MasterKeyPath() string { ... }

// JWTKeyPath returns the path to the JWT signing key.
func (p *Paths) JWTKeyPath() string { ... }

// RunLogPath returns the path for a specific run's JSONL log file.
func (p *Paths) RunLogPath(runID string) string { ... }
```

### Path Resolution Logic

```go
func Resolve() (*Paths, error) {
    homeDir := resolveHomeDir()     // ARI_HOME or ~/.ari
    realmID := resolveRealmID() // ARI_REALM_ID or "default"
    root := filepath.Join(homeDir, "realms", realmID)

    return &Paths{
        HomeDir:       homeDir,
        RealmID:       realmID,
        RealmRoot:  root,
        ConfigPath:    filepath.Join(root, ConfigFileName),
        DBDir:         filepath.Join(root, "db"),
        LogsDir:       filepath.Join(root, "logs"),
        SecretsDir:    filepath.Join(root, "secrets"),
        BackupDir:     filepath.Join(root, "data", "backups"),
        StorageDir:    filepath.Join(root, "data", "storage"),
        WorkspacesDir: filepath.Join(root, "workspaces"),
    }, nil
}

func resolveHomeDir() string {
    if v := os.Getenv("ARI_HOME"); v != "" {
        return expandHomePrefix(v)
    }
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".ari")
}

func resolveRealmID() string {
    if v := os.Getenv("ARI_REALM_ID"); v != "" {
        if !realmIDRe.MatchString(v) {
            // return error — invalid realm ID
        }
        return v
    }
    return DefaultRealmID
}

func expandHomePrefix(value string) string {
    if value == "~" {
        home, _ := os.UserHomeDir()
        return home
    }
    if strings.HasPrefix(value, "~/") {
        home, _ := os.UserHomeDir()
        return filepath.Join(home, value[2:])
    }
    return filepath.Abs(value)
}
```

### Directory Permissions

| Directory | Mode | Rationale |
|-----------|------|-----------|
| `~/.ari/` | 0755 | Top-level, discoverable |
| `realms/{id}/` | 0700 | Realm data is private |
| `db/` | 0700 | Database files |
| `secrets/` | 0700 | Encryption keys |
| `logs/` | 0750 | Logs readable by group |
| `data/` | 0700 | Backups, storage |
| `workspaces/` | 0700 | Agent working dirs |
| `workspaces/{agentId}/` | 0700 | Per-agent isolation |

---

## 3. Config File Schema

### File: `~/.ari/realms/default/config.json`

```json
{
  "$meta": {
    "version": 1,
    "updatedAt": "2026-03-17T10:00:00Z",
    "source": "ari-init"
  },
  "database": {
    "mode": "embedded-postgres",
    "connectionString": "",
    "embeddedPostgresDataDir": "~/.ari/realms/default/db",
    "embeddedPostgresPort": 5433,
    "backup": {
      "enabled": true,
      "intervalMinutes": 60,
      "retentionDays": 30,
      "dir": "~/.ari/realms/default/data/backups"
    }
  },
  "server": {
    "deploymentMode": "local_trusted",
    "host": "127.0.0.1",
    "port": 3100,
    "serveUi": true,
    "shutdownTimeoutSeconds": 30,
    "rateLimitRPS": 100,
    "rateLimitBurst": 200,
    "trustedProxies": ""
  },
  "auth": {
    "baseUrlMode": "auto",
    "disableSignUp": false,
    "sessionTTLMinutes": 1440,
    "jwtSecret": ""
  },
  "logging": {
    "level": "info",
    "mode": "stderr",
    "logDir": "~/.ari/realms/default/logs"
  },
  "secrets": {
    "provider": "local_encrypted",
    "keyFilePath": "~/.ari/realms/default/secrets/master.key"
  },
  "storage": {
    "provider": "local_disk",
    "localDisk": {
      "baseDir": "~/.ari/realms/default/data/storage"
    }
  },
  "runtime": {
    "maxRunsPerSquad": 3,
    "staleCheckoutAge": "2h",
    "agentDrainTimeout": "30s"
  },
  "tls": {
    "certPath": "",
    "keyPath": "",
    "domain": "",
    "redirectPort": 80
  }
}
```

### Go Config File Types

```go
// File: internal/home/config.go

// FileConfig represents the JSON config file schema.
type FileConfig struct {
    Meta     *ConfigMeta     `json:"$meta,omitempty"`
    Database *DatabaseConfig `json:"database,omitempty"`
    Server   *ServerConfig   `json:"server,omitempty"`
    Auth     *AuthConfig     `json:"auth,omitempty"`
    Logging  *LoggingConfig  `json:"logging,omitempty"`
    Secrets  *SecretsConfig  `json:"secrets,omitempty"`
    Storage  *StorageConfig  `json:"storage,omitempty"`
    Runtime  *RuntimeConfig  `json:"runtime,omitempty"`
    TLS      *TLSConfig      `json:"tls,omitempty"`
}

type ConfigMeta struct {
    Version   int    `json:"version"`
    UpdatedAt string `json:"updatedAt"`
    Source    string `json:"source"`
}

type DatabaseConfig struct {
    Mode                   string        `json:"mode"`                   // "embedded-postgres" | "postgres"
    ConnectionString       string        `json:"connectionString"`
    EmbeddedPostgresDataDir string       `json:"embeddedPostgresDataDir"`
    EmbeddedPostgresPort   int           `json:"embeddedPostgresPort"`
    Backup                 *BackupConfig `json:"backup,omitempty"`
}

type BackupConfig struct {
    Enabled         bool   `json:"enabled"`
    IntervalMinutes int    `json:"intervalMinutes"`
    RetentionDays   int    `json:"retentionDays"`
    Dir             string `json:"dir"`
}

type ServerConfig struct {
    DeploymentMode        string `json:"deploymentMode"`
    Host                  string `json:"host"`
    Port                  int    `json:"port"`
    ServeUI               bool   `json:"serveUi"`
    ShutdownTimeoutSeconds int   `json:"shutdownTimeoutSeconds"`
    RateLimitRPS          int    `json:"rateLimitRPS"`
    RateLimitBurst        int    `json:"rateLimitBurst"`
    TrustedProxies        string `json:"trustedProxies"`
}

type AuthConfig struct {
    BaseURLMode       string `json:"baseUrlMode"`
    DisableSignUp     bool   `json:"disableSignUp"`
    SessionTTLMinutes int    `json:"sessionTTLMinutes"`
    JWTSecret         string `json:"jwtSecret"`
}

type LoggingConfig struct {
    Level  string `json:"level"`
    Mode   string `json:"mode"`   // "stderr" | "file" | "both"
    LogDir string `json:"logDir"`
}

type SecretsConfig struct {
    Provider    string `json:"provider"`    // "local_encrypted"
    KeyFilePath string `json:"keyFilePath"`
}

type StorageConfig struct {
    Provider  string          `json:"provider"` // "local_disk"
    LocalDisk *LocalDiskConfig `json:"localDisk,omitempty"`
}

type LocalDiskConfig struct {
    BaseDir string `json:"baseDir"`
}

type RuntimeConfig struct {
    MaxRunsPerSquad  int    `json:"maxRunsPerSquad"`
    StaleCheckoutAge string `json:"staleCheckoutAge"`
    AgentDrainTimeout string `json:"agentDrainTimeout"`
}

type TLSConfig struct {
    CertPath     string `json:"certPath"`
    KeyPath      string `json:"keyPath"`
    Domain       string `json:"domain"`
    RedirectPort int    `json:"redirectPort"`
}
```

---

## 4. Config Loading Precedence

Three layers are merged in order: **defaults < config.json < env vars < CLI flags**.

### Merge Algorithm

```
config.Load(paths *home.Paths, flagOverrides map[string]any) (*Config, error)

  1. Start with hardcoded defaults (current behavior)
  2. If config.json exists at paths.ConfigPath:
       - Parse JSON into FileConfig struct
       - Overlay non-zero values onto defaults
       - Expand ~ prefixes in all path fields via home.ExpandHomePrefix()
  3. Read ARI_* environment variables (current behavior)
       - Overlay non-empty env values onto result
  4. Apply CLI flag overrides (--port, --host, etc.)
       - Overlay explicitly-set flags onto result
  5. Run cross-field validation (current behavior)
  6. Return final *Config
```

### Modified `config.Load()` Signature

```go
// Before (current):
func Load() (*Config, error)

// After:
func Load(paths *home.Paths, flags *FlagOverrides) (*Config, error)

// FlagOverrides carries explicitly-set CLI flags.
// nil/zero values mean "not set" and are skipped during merge.
type FlagOverrides struct {
    Port *int
    Host *string
    // ... additional flags as needed
}
```

### Integration Point: `cmd/ari/root.go`

The root command resolves paths once and passes them down via a shared context or struct:

```go
func newRootCmd(version string) *cobra.Command {
    root := &cobra.Command{
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            paths, err := home.Resolve()
            if err != nil {
                return fmt.Errorf("resolving home directory: %w", err)
            }
            // Check for legacy ./data/ directory
            if shouldUseLegacyDataDir(paths) {
                slog.Warn("using legacy ./data/ directory — run 'ari migrate-home' to upgrade")
                paths = legacyPaths()
            }
            cmd.SetContext(home.WithPaths(cmd.Context(), paths))
            return nil
        },
    }
    // ...
}
```

---

## 5. Config Discovery

Config discovery walks up from cwd looking for a project-local `.ari/config.json`, falling back to the realm default. This allows project-specific Ari configurations (e.g., a monorepo with its own Ari realm).

### Discovery Order

```
1. --config flag (explicit path)           -> use directly
2. ARI_CONFIG env var                      -> use directly
3. Walk from cwd upward looking for:
     .ari/config.json                      -> use if found
4. Fall back to:
     ~/.ari/realms/{id}/config.json     -> default location
```

### Implementation

```go
// File: internal/home/discovery.go

// DiscoverConfigPath finds the config file using the search order above.
func DiscoverConfigPath(overridePath string) string {
    if overridePath != "" {
        return filepath.Clean(overridePath)
    }
    if v := os.Getenv("ARI_CONFIG"); v != "" {
        return expandHomePrefix(v)
    }
    if found := findConfigFromAncestors(getCwd()); found != "" {
        return found
    }
    return resolveDefaultConfigPath()
}

func findConfigFromAncestors(startDir string) string {
    dir := filepath.Clean(startDir)
    maxDepth := 10 // guard against excessively deep traversal
    for depth := 0; depth < maxDepth; depth++ {
        candidate := filepath.Join(dir, ".ari", ConfigFileName)
        if fileExists(candidate) {
            return candidate
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            break
        }
        dir = parent
    }
    return ""
}
```

When a project-local config is discovered, the realm root is derived from the config's parent directory (the `.ari/` folder), overriding the default `~/.ari/realms/default/` root. All relative paths in that config file resolve relative to that realm root.

---

## 6. Backward Compatibility

### Legacy `./data/` Detection

If `./data/` exists in cwd and no `~/.ari/realms/default/` directory exists, Ari uses the legacy layout with a deprecation warning:

```go
func shouldUseLegacyDataDir(paths *home.Paths) bool {
    _, errLegacy := os.Stat("./data")
    _, errNew := os.Stat(paths.RealmRoot)
    return errLegacy == nil && os.IsNotExist(errNew)
}

func legacyPaths() *home.Paths {
    abs, _ := filepath.Abs("./data")
    return &home.Paths{
        HomeDir:       filepath.Dir(abs),
        RealmID:       "legacy",
        RealmRoot:  abs,
        ConfigPath:    "", // no config file in legacy mode
        DBDir:         abs,
        LogsDir:       abs,
        SecretsDir:    abs,
        BackupDir:     filepath.Join(abs, "backups"),
        StorageDir:    abs,
        WorkspacesDir: filepath.Join(abs, "workspaces"),
    }
}
```

The legacy layout maps as follows:

| Legacy Path | New Path | Notes |
|-------------|----------|-------|
| `./data/postgres/` | `~/.ari/realms/default/db/postgres/` | PG data dir |
| `./data/pg-runtime/` | `~/.ari/realms/default/db/pg-runtime/` | PG runtime |
| `./data/master.key` | `~/.ari/realms/default/secrets/master.key` | Master key |
| `./data/secrets/jwt.key` | `~/.ari/realms/default/secrets/jwt.key` | JWT key |
| `./data/runs/` | `~/.ari/realms/default/data/storage/runs/` | Run logs |
| (none) | `~/.ari/realms/default/data/backups/` | New |
| (none) | `~/.ari/realms/default/workspaces/` | New |

---

## 7. Migration Command: `ari migrate-home`

### CLI Definition

```go
func newMigrateHomeCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "migrate-home",
        Short: "Migrate from ./data/ to ~/.ari/ home directory",
        Long:  "Detects the legacy ./data/ directory and migrates all data " +
               "to the new ~/.ari/realms/default/ structure.",
        RunE:  runMigrateHome,
    }
    cmd.Flags().Bool("dry-run", false, "Show what would be moved without making changes")
    cmd.Flags().Bool("confirm", false, "Confirm the migration (required)")
    return cmd
}
```

### Migration Steps

```
ari migrate-home --confirm

Step 1: Validate preconditions
  - ./data/ must exist
  - ~/.ari/realms/default/ must NOT exist (or be empty)
  - Ari server must NOT be running (check PG pid file)

Step 2: Create target directory structure
  mkdir -p ~/.ari/realms/default/{db,logs,secrets,data/backups,data/storage,workspaces}

Step 3: Move database files
  mv ./data/postgres/     -> ~/.ari/realms/default/db/postgres/
  mv ./data/pg-runtime/   -> ~/.ari/realms/default/db/pg-runtime/

Step 4: Move secrets
  mv ./data/master.key    -> ~/.ari/realms/default/secrets/master.key
  mv ./data/secrets/jwt.key -> ~/.ari/realms/default/secrets/jwt.key

Step 5: Move run logs
  mv ./data/runs/          -> ~/.ari/realms/default/data/storage/runs/

Step 6: Generate config.json with current settings
  - Read ARI_* env vars to capture current configuration
  - Write ~/.ari/realms/default/config.json with resolved values
  - Set $meta.source = "migrate-home"

Step 7: Verify integrity
  - Check all moved files exist in new locations
  - Verify master.key is readable and correct size (32 bytes)
  - Print summary of moved files

Step 8: Clean up
  - Leave ./data/ in place with a MIGRATED.txt marker file
  - MIGRATED.txt contains: timestamp, new location, instructions
```

### Dry-Run Mode

```
$ ari migrate-home --dry-run

Migration plan:
  Source: ./data/
  Target: ~/.ari/realms/default/

  Files to move:
    ./data/postgres/        -> ~/.ari/realms/default/db/postgres/       (245 MB)
    ./data/pg-runtime/      -> ~/.ari/realms/default/db/pg-runtime/     (12 KB)
    ./data/master.key       -> ~/.ari/realms/default/secrets/master.key (32 B)
    ./data/secrets/jwt.key  -> ~/.ari/realms/default/secrets/jwt.key    (32 B)
    ./data/runs/            -> ~/.ari/realms/default/data/storage/runs/ (1.2 MB)

  Will generate: ~/.ari/realms/default/config.json

  No changes made. Run with --confirm to proceed.
```

### Implementation: `internal/home/migrate.go`

```go
// MigrateResult holds the outcome of a migration operation.
type MigrateResult struct {
    FilesMoved   []FileMoveRecord
    ConfigPath   string
    TotalBytes   int64
    DryRun       bool
}

type FileMoveRecord struct {
    Source string
    Dest   string
    Size   int64
}

// MigrateLegacy moves files from the legacy ./data/ layout to the new home structure.
func MigrateLegacy(legacyDir string, target *Paths, dryRun bool) (*MigrateResult, error) { ... }
```

---

## 8. Auto-Backup

### Background Backup Goroutine

When `database.backup.enabled` is true, a background goroutine runs `pg_dump` on an interval with retention cleanup.

```go
// File: internal/backup/backup.go

type BackupService struct {
    cfg        BackupConfig
    connStr    string
    pgDumpPath string
}

type BackupConfig struct {
    Enabled         bool
    IntervalMinutes int
    RetentionDays   int
    Dir             string
}

// Start launches the backup loop. Blocks until ctx is cancelled.
func (s *BackupService) Start(ctx context.Context) {
    if !s.cfg.Enabled {
        return
    }
    ticker := time.NewTicker(time.Duration(s.cfg.IntervalMinutes) * time.Minute)
    defer ticker.Stop()

    // Run initial backup on startup after a short delay
    time.AfterFunc(30*time.Second, func() { s.runBackup() })

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.runBackup()
        }
    }
}

func (s *BackupService) runBackup() {
    // 1. Create timestamped dump: ari-backup-20260317-100000.sql
    // 2. Run pg_dump to backup dir
    // 3. Delete backups older than retentionDays
    // 4. Log result
}

// RunOnce performs a single backup (used by `ari backup` command).
func (s *BackupService) RunOnce(outputPath string, format string) error { ... }

// Cleanup removes backups older than the retention window.
func (s *BackupService) Cleanup() (removed int, err error) { ... }
```

### Retention Cleanup

```go
func (s *BackupService) Cleanup() (int, error) {
    cutoff := time.Now().Add(-time.Duration(s.cfg.RetentionDays) * 24 * time.Hour)
    entries, _ := os.ReadDir(s.cfg.Dir)

    removed := 0
    for _, entry := range entries {
        if !strings.HasPrefix(entry.Name(), "ari-backup-") {
            continue
        }
        info, _ := entry.Info()
        if info.ModTime().Before(cutoff) {
            os.Remove(filepath.Join(s.cfg.Dir, entry.Name()))
            removed++
        }
    }
    return removed, nil
}
```

### Integration in `runServer()`

```go
// In cmd/ari/run.go, after database.Open():
if cfg.Database.Backup.Enabled && cfg.UseEmbeddedPostgres() {
    backupSvc := backup.New(backup.BackupConfig{
        Enabled:         cfg.Database.Backup.Enabled,
        IntervalMinutes: cfg.Database.Backup.IntervalMinutes,
        RetentionDays:   cfg.Database.Backup.RetentionDays,
        Dir:             paths.BackupDir,
    }, connStr, pgDumpPath)
    go backupSvc.Start(ctx)
}
```

---

## 9. Per-Agent Workspaces

### Workspace Lifecycle

Workspaces are created when an agent is created and cleaned up when an agent is deleted.

```
Agent Created (POST /api/squads/{id}/agents)
        |
        v
AgentHandler.Create():
  1. Insert agent record in DB
  2. Call paths.EnsureAgentWorkspace(agentID)
     -> mkdir -p ~/.ari/realms/default/workspaces/{agentID}/
  3. Return agent response with workspace_path field
        |
        v
Agent Invoked (RunService.Invoke)
        |
        v
buildInvokeInput():
  1. Set WorkingDir = paths.AgentWorkspaceDir(agentID)
  2. Adapter launches process in that directory
```

### Agent Workspace Directory

```go
// In internal/home/home.go

func (p *Paths) AgentWorkspaceDir(agentID string) (string, error) {
    id := strings.TrimSpace(agentID)
    if !pathSegmentRe.MatchString(id) {
        return "", fmt.Errorf("invalid agent ID for workspace path: %q", agentID)
    }
    return filepath.Join(p.WorkspacesDir, id), nil
}

func (p *Paths) EnsureAgentWorkspace(agentID string) (string, error) {
    dir, err := p.AgentWorkspaceDir(agentID)
    if err != nil {
        return "", err
    }
    if err := os.MkdirAll(dir, 0700); err != nil {
        return "", fmt.Errorf("creating agent workspace: %w", err)
    }
    return dir, nil
}

func (p *Paths) RemoveAgentWorkspace(agentID string) error {
    dir, err := p.AgentWorkspaceDir(agentID)
    if err != nil {
        return err
    }
    return os.RemoveAll(dir)
}
```

### Workspace Content

Each agent workspace contains:
```
workspaces/{agentId}/
  .ari-agent.json          # metadata: agent name, squad, created timestamp
  repo/                    # cloned repository (if configured)
  scratch/                 # temporary working files
```

The `.ari-agent.json` is written on workspace creation:
```json
{
  "agentId": "abc-123",
  "agentName": "code-reviewer",
  "squadId": "squad-456",
  "createdAt": "2026-03-17T10:00:00Z"
}
```

---

## 10. Modified Subsystems

### 10.1 `config.Load()` Refactor

The existing `config.Load()` in `internal/config/config.go` must be refactored to accept `*home.Paths` and support the layered merge:

```go
func Load(paths *home.Paths, flags *FlagOverrides) (*Config, error) {
    // 1. Start with defaults (same as current)
    cfg := defaults(paths)

    // 2. Overlay config.json if it exists
    if paths.ConfigPath != "" {
        if fileConf, err := readConfigFile(paths.ConfigPath); err == nil {
            mergeFileConfig(cfg, fileConf, paths)
        }
        // Missing config file is not an error — use defaults
    }

    // 3. Overlay env vars (same as current, but env > file)
    mergeEnvVars(cfg)

    // 4. Overlay CLI flags
    if flags != nil {
        mergeFlags(cfg, flags)
    }

    // 5. Validate (same as current)
    return cfg, validate(cfg)
}
```

The existing `Config` struct remains the canonical runtime config type. The new `FileConfig` struct is only for JSON serialization/deserialization.

### 10.2 `database.Open()` Changes

Replace `cfg.DataDir` references with specific paths from the `Config`:

```go
// Before:
pgDataDir := filepath.Join(cfg.DataDir, "postgres")
pgRuntimeDir := filepath.Join(cfg.DataDir, "pg-runtime")

// After:
pgDataDir := filepath.Join(cfg.EmbeddedPostgresDataDir, "postgres")
pgRuntimeDir := filepath.Join(cfg.EmbeddedPostgresDataDir, "pg-runtime")
```

The `Config` struct gains `EmbeddedPostgresDataDir` populated from the resolved paths.

### 10.3 `secrets.NewMasterKeyManager()` Changes

Instead of accepting a bare `dataDir` string, accept the specific key file path:

```go
// Before:
func NewMasterKeyManager(masterKeyEnv string, dataDir string) (*MasterKeyManager, error)

// After:
func NewMasterKeyManager(masterKeyEnv string, keyFilePath string) (*MasterKeyManager, error)
```

The caller passes `paths.MasterKeyPath()` instead of `cfg.DataDir`.

### 10.4 `RunService` Changes

Run log storage uses the resolved storage directory:

```go
// Before:
logDir := filepath.Join(s.dataDir, "runs")

// After:
logDir := filepath.Join(s.storageDir, "runs")
```

### 10.5 JWT Key Resolution

The `resolveJWTKey()` function in `run.go` uses `paths.JWTKeyPath()` instead of constructing the path manually:

```go
// Before:
secretsDir := filepath.Join(cfg.DataDir, "secrets")
keyPath := filepath.Join(secretsDir, "jwt.key")

// After:
keyPath := paths.JWTKeyPath()
```

---

## 11. CLI Changes

### New/Modified Commands

| Command | Change | Description |
|---------|--------|-------------|
| `ari run` | Modified | Resolves home paths before config load |
| `ari backup` | Modified | Uses resolved backup dir as default output |
| `ari restore` | Modified | Uses resolved paths for PG connection |
| `ari migrate-home` | New | Migrates `./data/` to `~/.ari/` |
| `ari config show` | New | Prints resolved config with source annotations |
| `ari config init` | New | Creates `config.json` with defaults |
| `ari config path` | New | Prints resolved config file path |

### `ari config show` Output

```
$ ari config show

Config source: ~/.ari/realms/default/config.json
Realm: default

database.mode = embedded-postgres         [config.json]
database.embeddedPostgresPort = 5433      [default]
database.backup.enabled = true            [config.json]
database.backup.intervalMinutes = 60      [config.json]
server.port = 3200                        [flag: --port]
server.host = 127.0.0.1                   [env: ARI_HOST]
server.deploymentMode = local_trusted     [default]
auth.disableSignUp = false                [default]
logging.level = info                      [default]
secrets.keyFilePath = ~/.ari/.../master.key [config.json]
...
```

---

## 12. Implementation Order

### Task 1: `internal/home/` Package (foundation)
- [ ] `home.go` — `Paths` struct, `Resolve()`, `InitHomeDir()`
- [ ] Path helper methods (`MasterKeyPath`, `JWTKeyPath`, `RunLogPath`, `AgentWorkspaceDir`)
- [ ] `discovery.go` — `DiscoverConfigPath()`, ancestor walk
- [ ] Unit tests for path resolution, discovery, tilde expansion, validation

### Task 2: Config File Types
- [ ] `internal/home/config.go` — `FileConfig` struct and all section types
- [ ] `readConfigFile()` — JSON parsing with `~` expansion on path fields
- [ ] `writeConfigFile()` — JSON serialization with `$meta` update
- [ ] Unit tests for parse/write round-trip

### Task 3: Layered Config Loading
- [ ] Add `FlagOverrides` struct to `internal/config/`
- [ ] Refactor `config.Load()` to accept `*home.Paths` and `*FlagOverrides`
- [ ] Implement merge functions: `mergeFileConfig`, `mergeEnvVars`, `mergeFlags`
- [ ] Update all callers of `config.Load()` (run.go, backup.go, restore.go)
- [ ] Maintain backward compatibility: if no `home.Paths` provided, use legacy behavior
- [ ] Unit tests for precedence: flag > env > file > default

### Task 4: Backward Compatibility and Legacy Detection
- [ ] `shouldUseLegacyDataDir()` in root command
- [ ] `legacyPaths()` mapping
- [ ] Deprecation warning log on legacy detection
- [ ] Integration test: legacy `./data/` still works

### Task 5: Migration Command
- [ ] `cmd/ari/migrate_home.go` — command definition
- [ ] `internal/home/migrate.go` — migration logic
- [ ] Dry-run mode
- [ ] File move operations with rollback on failure
- [ ] Config.json generation from current env vars
- [ ] MIGRATED.txt marker file
- [ ] Integration tests for migration

### Task 6: Subsystem Refactoring
- [ ] Update `database.Open()` to use resolved paths
- [ ] Update `secrets.NewMasterKeyManager()` signature
- [ ] Update `RunService` to use resolved storage dir
- [ ] Update `resolveJWTKey()` to use resolved paths
- [ ] Update `RuntimeHandler` log path resolution
- [ ] Verify all tests pass with new path resolution

### Task 7: Auto-Backup Service
- [ ] `internal/backup/backup.go` — `BackupService` struct
- [ ] `Start()` — background loop with interval ticker
- [ ] `RunOnce()` — single backup execution
- [ ] `Cleanup()` — retention-based deletion
- [ ] Integration in `runServer()` startup
- [ ] Refactor existing `ari backup` command to delegate to `BackupService.RunOnce()`
- [ ] Unit tests for retention cleanup logic

### Task 8: Per-Agent Workspaces
- [ ] `EnsureAgentWorkspace()` and `RemoveAgentWorkspace()` in home package
- [ ] Hook workspace creation into `AgentHandler.Create()`
- [ ] Hook workspace cleanup into agent deletion
- [ ] Write `.ari-agent.json` metadata on workspace creation
- [ ] Integration tests for workspace lifecycle

### Task 9: Config CLI Commands
- [ ] `ari config show` — print resolved config with source annotations
- [ ] `ari config init` — generate default config.json
- [ ] `ari config path` — print resolved config file path
- [ ] Register commands in root.go

### Task 10: Documentation and Cleanup
- [ ] Remove `ARI_DATA_DIR` env var support (keep as deprecated alias for one release)
- [ ] Update root command help text
- [ ] Remove `DataDir` field from `Config` struct (replaced by resolved paths)

---

## 13. Testing Strategy

### Unit Tests
- Path resolution with various `ARI_HOME` / `ARI_REALM_ID` values
- Tilde expansion edge cases
- Config file parsing and merge precedence
- Agent ID validation for workspace paths (path traversal prevention)
- Backup retention cleanup logic

### Integration Tests
- Full startup with `~/.ari/` home directory in temp dir
- Legacy `./data/` backward compatibility
- Migration from `./data/` to `~/.ari/`
- Config discovery ancestor walk
- Auto-backup creates and cleans up dump files

### Security Tests
- Agent ID with `../` is rejected
- Realm ID with special characters is rejected
- Directory permissions are enforced (0700 for secrets)
- Master key file permissions are checked
