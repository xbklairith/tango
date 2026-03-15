package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/xb/ari/internal/config"
)

func newRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore database from backup",
		Long:  "Restore the Ari database from a pg_dump backup file.",
		RunE:  runRestore,
	}
	cmd.Flags().StringP("input", "i", "", "Input backup file path (required)")
	cmd.Flags().Bool("confirm", false, "Confirm destructive restore operation")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func runRestore(cmd *cobra.Command, args []string) error {
	confirm, _ := cmd.Flags().GetBool("confirm")
	if !confirm {
		fmt.Fprintln(os.Stderr, "WARNING: This will overwrite the current database.")
		fmt.Fprintln(os.Stderr, "Use --confirm to proceed with the restore operation.")
		return fmt.Errorf("restore aborted: --confirm flag required")
	}

	input, _ := cmd.Flags().GetString("input")
	if _, err := os.Stat(input); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", input)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	psqlBin, err := findPsql(cfg)
	if err != nil {
		return fmt.Errorf("finding psql: %w", err)
	}

	psqlArgs := buildPsqlArgs(cfg, input)
	slog.Info("running restore", "binary", psqlBin, "input", input)

	execCmd := exec.Command(psqlBin, psqlArgs...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %w", err)
	}

	fmt.Printf("Database restored from: %s\n", input)
	fmt.Println("Note: Run 'ari run' to apply any pending migrations.")
	return nil
}

func findPsql(cfg *config.Config) (string, error) {
	if cfg.UseEmbeddedPostgres() {
		return findEmbeddedPgBinary("psql")
	}
	path, err := exec.LookPath("psql")
	if err != nil {
		return "", fmt.Errorf("psql not found in PATH: %w", err)
	}
	return path, nil
}

func buildPsqlArgs(cfg *config.Config, input string) []string {
	args := []string{"--file=" + input}

	if cfg.DatabaseURL != "" {
		args = append(args, cfg.DatabaseURL)
	} else {
		// Embedded PG — construct connection args
		args = append(args,
			"--host=localhost",
			fmt.Sprintf("--port=%d", cfg.EmbeddedPGPort),
			"--username=postgres",
			"--dbname=postgres",
		)
	}

	return args
}
