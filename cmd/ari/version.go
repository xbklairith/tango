package main

import (
	"github.com/spf13/cobra"
)

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Ari version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("ari version %s\n", version)
		},
	}
}
