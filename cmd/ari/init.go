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
			instanceOverride, _ := cmd.Flags().GetString("instance")

			if homeOverride != "" {
				os.Setenv("ARI_HOME", homeOverride)
			}
			if instanceOverride != "" {
				if err := home.ValidateInstanceID(instanceOverride); err != nil {
					return fmt.Errorf("invalid instance ID: %w", err)
				}
				os.Setenv("ARI_INSTANCE_ID", instanceOverride)
			}

			paths, err := home.Resolve()
			if err != nil {
				return fmt.Errorf("resolving paths: %w", err)
			}

			slog.Info("initializing ari home", "root", paths.InstanceRoot)

			if err := home.InitHomeDir(paths.InstanceRoot); err != nil {
				return fmt.Errorf("initializing home directory: %w", err)
			}

			cmd.Printf("Ari home initialized at: %s\n", paths.InstanceRoot)
			cmd.Printf("  config:     %s\n", paths.ConfigPath)
			cmd.Printf("  database:   %s\n", paths.DBDir)
			cmd.Printf("  secrets:    %s\n", paths.SecretsDir)
			cmd.Printf("  logs:       %s\n", paths.LogsDir)
			cmd.Printf("  workspaces: %s\n", paths.WorkspacesDir)

			return nil
		},
	}

	cmd.Flags().String("home", "", "Override home directory (default: ~/.ari)")
	cmd.Flags().String("instance", "", "Instance ID (default: default)")

	return cmd
}
