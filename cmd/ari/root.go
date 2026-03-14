package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "ari",
		Short:         "Ari — The Control Plane for AI Agents",
		Long:          "Deploy, govern, and share AI agent workforces.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	return root
}
