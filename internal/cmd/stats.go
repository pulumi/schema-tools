package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/internal/pkg"
)

func statsCmd() *cobra.Command {
	var provider string
	var tag string
	var details bool

	command := &cobra.Command{
		Use:   "stats",
		Short: "Get the stats of a current schema",
		RunE: func(command *cobra.Command, args []string) error {
			return stats(provider, tag, details)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "",
		"the provider whose schema we should analyze")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&tag, "tag", "t", "",
	"the tag of the provider whose schema we should analyze")
_ = command.MarkFlagRequired("tag")

	command.Flags().BoolVarP(&details, "details", "d", false,
		"show the details with a list of all resources and functions")

	return command
}

func stats(provider string, tag string, details bool) error {
	schemaUrl := fmt.Sprintf("https://raw.githubusercontent.com/pulumi/pulumi-%s/%s/provider/cmd/pulumi-resource-%[1]s/schema.json", provider, tag)
	sch, err := pkg.DownloadSchema(schemaUrl)
	if err != nil {
		return err
	}

	schemaStats := pkg.CountStats(sch)

	fmt.Printf("Provider: %s\n", provider)
	statsBytes, _ := json.MarshalIndent(schemaStats, "", "  ")
	_, err = os.Stdout.Write(statsBytes)
	if err != nil {
		return fmt.Errorf("main stats: %w", err)
	}

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
