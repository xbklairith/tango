package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/xb/ari/internal/config"
)

func newBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a database backup",
		Long:  "Create a PostgreSQL dump of the Ari database.",
		RunE:  runBackup,
	}
	cmd.Flags().StringP("output", "o", "", "Output file path (default: ari-backup-{timestamp}.sql)")
	cmd.Flags().String("format", "plain", "Dump format: plain or custom")
	return cmd
}

func runBackup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		output = fmt.Sprintf("ari-backup-%s.sql", time.Now().Format("20060102-150405"))
	}

	format, _ := cmd.Flags().GetString("format")

	pgDump, err := findPgDump(cfg)
	if err != nil {
		return fmt.Errorf("finding pg_dump: %w", err)
	}

	pgArgs := buildPgDumpArgs(cfg, output, format)
	slog.Info("running backup", "binary", pgDump, "output", output)

	execCmd := exec.Command(pgDump, pgArgs...)
	execCmd.Env = pgDumpEnv(cfg)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	fmt.Printf("Backup created: %s\n", output)
	return nil
}

func findPgDump(cfg *config.Config) (string, error) {
	if cfg.UseEmbeddedPostgres() {
		return findEmbeddedPgBinary("pg_dump")
	}
	path, err := exec.LookPath("pg_dump")
	if err != nil {
		return "", fmt.Errorf("pg_dump not found in PATH: %w", err)
	}
	return path, nil
}

func findEmbeddedPgBinary(name string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	// Search in embedded-postgres-go cache directory
	baseDir := filepath.Join(homeDir, ".embedded-postgres-go", "extracted")
	matches, err := filepath.Glob(filepath.Join(baseDir, "*", "bin", name))
	if err != nil {
		return "", fmt.Errorf("globbing for %s: %w", name, err)
	}
	if len(matches) > 0 {
		return matches[0], nil
	}

	// Fallback: try system PATH
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in embedded PG cache or PATH", name)
	}
	return path, nil
}

func buildPgDumpArgs(cfg *config.Config, output, format string) []string {
	args := []string{"--clean", "--if-exists"}

	if format == "custom" {
		args = append(args, "--format=custom")
	} else {
		args = append(args, "--format=plain")
	}

	args = append(args, "--file="+output)

	if cfg.DatabaseURL != "" {
		// Parse the URL to extract components and avoid exposing credentials in process args
		args = append(args, buildPgDumpArgsFromURL(cfg.DatabaseURL)...)
	} else {
		// Embedded PG — construct connection string
		args = append(args,
			"--host=localhost",
			fmt.Sprintf("--port=%d", cfg.EmbeddedPGPort),
			"--username=postgres",
			"--dbname=postgres",
		)
	}

	return args
}

// buildPgDumpArgsFromURL parses a database URL and returns pg_dump flags
// without exposing the password in process arguments.
func buildPgDumpArgsFromURL(dbURL string) []string {
	u, err := url.Parse(dbURL)
	if err != nil {
		// Fallback: pass as dbname (legacy behavior)
		return []string{"--dbname=" + dbURL}
	}

	var args []string
	if u.Hostname() != "" {
		args = append(args, "--host="+u.Hostname())
	}
	if u.Port() != "" {
		args = append(args, "--port="+u.Port())
	}
	if u.User != nil {
		if user := u.User.Username(); user != "" {
			args = append(args, "--username="+user)
		}
	}
	dbName := u.Path
	if len(dbName) > 0 && dbName[0] == '/' {
		dbName = dbName[1:]
	}
	if dbName != "" {
		args = append(args, "--dbname="+dbName)
	}

	return args
}

// pgDumpEnv returns environment variables for pg_dump, passing the password
// via PGPASSWORD instead of command-line arguments.
func pgDumpEnv(cfg *config.Config) []string {
	env := os.Environ()
	if cfg.DatabaseURL != "" {
		u, err := url.Parse(cfg.DatabaseURL)
		if err == nil && u.User != nil {
			if pw, ok := u.User.Password(); ok {
				env = append(env, "PGPASSWORD="+pw)
			}
		}
	}
	return env
}

