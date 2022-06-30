package cmd

import (
	"fmt"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/spf13/cobra"
	"strings"
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

	uniques := codegen.NewStringSet()
	visitedTypes := codegen.NewStringSet()
	var propCount func(string) int
	propCount = func(typeName string) int {
		if visitedTypes.Has(typeName) {
			return 0
		}
		visitedTypes.Add(typeName)
		t := sch.Types[typeName]
		result := len(t.Properties)
		for _, p := range t.Properties {
			if p.Ref != "" {
				tn := strings.TrimPrefix(p.Ref, "#/types/")
				result += propCount(tn)
			}
		}
		return result
	}
	properties := 0
	for n, r := range sch.Resources {
		baseName := versionlessName(n)
		if uniques.Has(baseName) {
			continue
		}
		uniques.Add(baseName)
		properties += len(r.InputProperties)
		for _, p := range r.InputProperties {
			if p.Ref != "" {
				typeName := strings.TrimPrefix(p.Ref, "#/types/")
				properties += propCount(typeName)
			}
		}
	}

	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Total resource types: %d\n", len(uniques))
	fmt.Printf("Total input properties: %d\n", properties)
}
