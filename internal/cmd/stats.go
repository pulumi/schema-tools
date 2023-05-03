package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/internal/pkg"
)

func statsCmd() *cobra.Command {
	var provider string
	var details bool

	command := &cobra.Command{
		Use:   "stats",
		Short: "Get the stats of a current schema",
		RunE: func(command *cobra.Command, args []string) error {
			return stats(provider, details)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "",
		"the provider whose schema we should analyze")
	_ = command.MarkFlagRequired("provider")

	command.Flags().BoolVarP(&details, "details", "d", false,
		"show the details with a list of all resources and functions")

	return command
}

func stats(provider string, details bool) error {
	schemaUrl := fmt.Sprintf("https://raw.githubusercontent.com/pulumi/pulumi-%s/master/provider/cmd/pulumi-resource-%[1]s/schema.json", provider)
	sch, err := pkg.DownloadSchema(schemaUrl)
	if err != nil {
		return err
	}

	schemaStats := pkg.CountStats(sch)

	fmt.Printf("Provider: %s\n", provider)
	statsBytes, _ := json.MarshalIndent(schemaStats, "", "  ")
	statsString := string(statsBytes)
	fmt.Printf(statsString)

	if details {
		fmt.Printf("\n\n### All Resources:\n\n")
		for _, n := range codegen.SortedKeys(sch.Resources) {
			fmt.Println(n)
		}
		fmt.Printf("\n### All Functions:\n\n")
		for _, n := range codegen.SortedKeys(sch.Functions) {
			fmt.Println(n)
		}
	}

	return nil
}
