package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/xb/ari/internal/home"
)

func newMigrateHomeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-home",
		Short: "Migrate from ./data/ to ~/.ari/ home directory",
		Long:  "Detects the legacy ./data/ directory and migrates all data to the new ~/.ari/realms/default/ structure.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			sourceDir, _ := cmd.Flags().GetString("source")

			// Resolve target paths
			paths, err := home.Resolve()
			if err != nil {
				return fmt.Errorf("resolving paths: %w", err)
			}

			// Default source is ./data in cwd
			if sourceDir == "" {
				cwd, _ := os.Getwd()
				sourceDir = fmt.Sprintf("%s/data", cwd)
			}

			// Check source exists
			if !home.DetectLegacyDir(sourceDir) {
				cmd.Println("No legacy ./data/ directory detected. Nothing to migrate.")
				return nil
			}

			// Plan migration
			plan, err := home.PlanMigration(sourceDir, paths.RealmRoot)
			if err != nil {
				return fmt.Errorf("planning migration: %w", err)
			}

			if len(plan.Items) == 0 {
				cmd.Println("No items to migrate.")
				return nil
			}

			// Print plan
			cmd.Printf("Migration plan:\n")
			cmd.Printf("  Source: %s\n", sourceDir)
			cmd.Printf("  Target: %s\n", paths.RealmRoot)
			cmd.Println()
			for _, item := range plan.Items {
				cmd.Printf("  %s -> %s  (%s)\n", item.Source, item.Dest, item.Description)
			}
			cmd.Println()

			if dryRun {
				cmd.Println("Dry run — no changes made.")
				return nil
			}

			// Initialize target directory first
			if err := home.InitHomeDir(paths.RealmRoot); err != nil {
				return fmt.Errorf("initializing target: %w", err)
			}

			// Execute migration
			result, err := home.ExecuteMigration(plan, false)
			if err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			slog.Info("migration complete", "files_moved", result.FilesMoved)
			cmd.Printf("Migration complete: %d items moved.\n", result.FilesMoved)
			cmd.Printf("Your data is now at: %s\n", paths.RealmRoot)

			// Write marker file
			marker := fmt.Sprintf("Migrated to %s\n", paths.RealmRoot)
			os.WriteFile(fmt.Sprintf("%s/.migrated", sourceDir), []byte(marker), 0644)

			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Show what would be moved without making changes")
	cmd.Flags().String("source", "", "Override legacy directory (default: ./data)")

	return cmd
}
