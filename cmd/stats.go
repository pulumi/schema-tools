package cmd

import (
	"fmt"
	"github.com/pulumi/schema-tools/pkg"
	"github.com/spf13/cobra"
)

func statsCmd() *cobra.Command {
	var provider string

	command := &cobra.Command{
		Use:   "stats",
		Short: "Get the stats of a current schema",
		Run: func(command *cobra.Command, args []string) {
			stats(provider)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "",
		"the provider whose schema we should analyze")
	_ = command.MarkFlagRequired("provider")

	return command
}

func stats(provider string) {
	schemaUrl := fmt.Sprintf("https://raw.githubusercontent.com/pulumi/pulumi-%s/master/provider/cmd/pulumi-resource-%[1]s/schema.json", provider)
	sch := downloadSchema(schemaUrl)

	schemaStats := pkg.CountStats(sch)

	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Total resource types: %d\n", schemaStats.TotalResources)
	fmt.Printf("Total input properties: %d\n", schemaStats.TotalResourceInputs)
}
