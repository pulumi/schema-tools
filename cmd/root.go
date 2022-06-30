package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

func rootCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "schema-tools",
		Short: "schema-tools is a CLI utility to analyze Pulumi schemas",
		Run: func(cmd *cobra.Command, args []string) {
			// Do Stuff Here
		},
	}

	command.AddCommand(compareCmd())
	command.AddCommand(statsCmd())
	command.AddCommand(versionCmd())

	return command
}

func Execute() {
	if err := rootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
