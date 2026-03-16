package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "ari",
		Short:         "Ari — The Control Plane for AI Agents",
		Long:          "Ari — The Control Plane for AI Agents.\nDeploy, govern, and share AI agent workforces.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newRunCmd(version))
	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newBackupCmd())
	root.AddCommand(newRestoreCmd())
	root.AddCommand(newInitCmd())

	return root
}
