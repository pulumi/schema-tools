package pkg

import (
	"fmt"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"strings"
)

type PulumiSchemaStats struct {
	TotalResources            int
	TotalResourceInputs       int
	ResourceInputsMissingDesc int
	TotalFunctions            int
}

func CountStats(sch schema.PackageSpec) PulumiSchemaStats {
	stats := PulumiSchemaStats{}

	uniques := codegen.NewStringSet()
	visitedTypes := codegen.NewStringSet()

	var propCount func(string) (int, int)
	propCount = func(typeName string) (totalProperties int, propertiesMissingDesc int) {
		if visitedTypes.Has(typeName) {
			return 0, 0
		}

		visitedTypes.Add(typeName)

		t := sch.Types[typeName]

		totalProperties = len(t.Properties)
		propertiesMissingDesc = 0

		for _, p := range t.Properties {
			if p.Description == "" {
				propertiesMissingDesc++
			}

			if p.Ref != "" {
				tn := strings.TrimPrefix(p.Ref, "#/types/")
				nestedTotalProps, nestedPropsMissingDesc := propCount(tn)
				totalProperties += nestedTotalProps
				propertiesMissingDesc += nestedPropsMissingDesc
			}
		}
		return totalProperties, propertiesMissingDesc
	}

	for n, r := range sch.Resources {
		baseName := versionlessName(n)
		if uniques.Has(baseName) {
			continue
		}
		uniques.Add(baseName)
		stats.TotalResourceInputs += len(r.InputProperties)
		for _, p := range r.InputProperties {
			if p.Description == "" {
				stats.ResourceInputsMissingDesc++
			}

			if p.Ref != "" {
				typeName := strings.TrimPrefix(p.Ref, "#/types/")
				nestedTotalProps, nestedPropsMissingDesc := propCount(typeName)
				stats.TotalResourceInputs += nestedTotalProps
				stats.ResourceInputsMissingDesc += nestedPropsMissingDesc
			}
		}
	}

	stats.TotalResources = len(uniques)
	stats.TotalFunctions = len(sch.Functions)

	return stats
}

func versionlessName(name string) string {
	parts := strings.Split(name, ":")
	mod := parts[1]
	modParts := strings.Split(mod, "/")
	if len(modParts) == 2 {
		mod = modParts[0]
	}
	return fmt.Sprintf("%s:%s", mod, parts[2])
}
