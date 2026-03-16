package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/xb/ari/internal/home"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Ari home directory structure",
		Long:  "Create the ~/.ari/ directory structure with default config, secrets, and workspace directories.",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeOverride, _ := cmd.Flags().GetString("home")
			realmOverride, _ := cmd.Flags().GetString("realm")

			if homeOverride != "" {
				os.Setenv("ARI_HOME", homeOverride)
			}
			if realmOverride != "" {
				if err := home.ValidateRealmID(realmOverride); err != nil {
					return fmt.Errorf("invalid realm ID: %w", err)
				}
				os.Setenv("ARI_REALM_ID", realmOverride)
			}

			paths, err := home.Resolve()
			if err != nil {
				return fmt.Errorf("resolving paths: %w", err)
			}

			slog.Info("initializing ari home", "root", paths.RealmRoot)

			if err := home.InitHomeDir(paths.RealmRoot); err != nil {
				return fmt.Errorf("initializing home directory: %w", err)
			}

			cmd.Printf("Ari home initialized at: %s\n", paths.RealmRoot)
			cmd.Printf("  config:     %s\n", paths.ConfigPath)
			cmd.Printf("  database:   %s\n", paths.DBDir)
			cmd.Printf("  secrets:    %s\n", paths.SecretsDir)
			cmd.Printf("  logs:       %s\n", paths.LogsDir)
			cmd.Printf("  workspaces: %s\n", paths.WorkspacesDir)

			return nil
		},
	}

	cmd.Flags().String("home", "", "Override home directory (default: ~/.ari)")
	cmd.Flags().String("realm", "", "Realm ID (default: default)")

	return cmd
}
