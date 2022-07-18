package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/pulumi/schema-tools/pkg"
	"github.com/spf13/cobra"
)

func statsCmd() *cobra.Command {
	var provider string

	command := &cobra.Command{
		Use:   "stats",
		Short: "Get the stats of a current schema",
		RunE: func(command *cobra.Command, args []string) error {
			return stats(provider)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "",
		"the provider whose schema we should analyze")
	_ = command.MarkFlagRequired("provider")

	return command
}

func stats(provider string) error {
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

	return nil
}
