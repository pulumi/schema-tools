package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of schema-tools",
		Run: func(command *cobra.Command, args []string) {
			fmt.Println("TODO")
		},
	}
}
