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
	}

	command.AddCommand(compareCmd())
	command.AddCommand(statsCmd())
	command.AddCommand(versionCmd())
	command.AddCommand(squeezeCmd())

	return command
}

func Execute() {
	if err := rootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
