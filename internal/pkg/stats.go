package pkg

import (
	"fmt"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

type PulumiSchemaStatsV1 struct {
	Functions FunctionStatsV1 `json:"Functions"`
	Resources ResourceStatsV1 `json:"Resources"`
}

// ResourceStatsV1 contains statistics relating to the resources section of a Pulumi schema.
type ResourceStatsV1 struct {
	// TotalResources is the total number of Pulumi resources in the schema.
	TotalResources int

	// TotalDescriptionBytes is the sum total of all bytes in the descriptions of the resources themselves, not
	// including any inputs and outputs.
	TotalDescriptionBytes int

	// TotalInputProperties is the total number of inputs across all resources, including nested types.
	// Given a complex input type Foo with one property, Foo.Bar, both Foo and Foo.Bar are counted as inputs.
	TotalInputProperties int

	// InputPropertiesMissingDescriptions is the total number of all resource input properties missing descriptions,
	// including nested types.
	InputPropertiesMissingDescriptions int

	// TotalOutputProperties is the total number of outputs across all resources, including nested types.
	// Given a complex output type Foo with one property, Foo.Bar, both Foo and Foo.Bar are counted as outputs.
	TotalOutputProperties int

	// OutputPropertiesMissingDescriptions is the total number of all resource output properties missing descriptions.
	OutputPropertiesMissingDescriptions int
}

// FunctionStatsV1 contain statistics relating to the functions section of a Pulumi schema.
type FunctionStatsV1 struct {
	// TotalFunctions is the total number of Pulumi Functions in the schema.
	TotalFunctions int

	// TotalDescriptionBytes is the sum total of all bytes in the descriptions of the functions themselves,
	// not including inputs and outputs.
	TotalDescriptionBytes int

	// TotalInputPropertyDescriptionBytes is the sum total of all bytes in descriptions of function input properties,
	// not including the input type description.
	TotalInputPropertyDescriptionBytes int

	// InputPropertiesMissingDescriptions is the total number of all function input properties missing descriptions.
	InputPropertiesMissingDescriptions int

	// TotalOutputPropertyDescriptionBytes is the sum total of all bytes in description of function output properties,
	// not include the output type description.
	TotalOutputPropertyDescriptionBytes int

	// OutputPropertiesMissingDescriptions is the total number of all function output properties missing descriptions.
	OutputPropertiesMissingDescriptions int
}

func CountStats(sch schema.PackageSpec) PulumiSchemaStatsV1 {
	stats := PulumiSchemaStatsV1{
		Resources: ResourceStatsV1{},
		Functions: FunctionStatsV1{},
	}

	uniques := mapset.NewSet[string]()
	visitedTypes := mapset.NewSet[string]()

	type propCountResult struct {
		totalInputs        int
		inputsMissingDesc  int
		totalOutputs       int
		outputsMissingDesc int
	}

	var propCount func(string) propCountResult
	propCount = func(typeName string) propCountResult {
		if visitedTypes.Contains(typeName) {
			return propCountResult{}
		}

		res := propCountResult{}

		visitedTypes.Add(typeName)

		t := sch.Types[typeName]

		res.totalInputs = len(t.Properties)

		for _, input := range t.Properties {
			if input.Description == "" {
				res.inputsMissingDesc++
			}

			if input.Ref != "" {
				tn := strings.TrimPrefix(input.Ref, "#/types/")
				nestedRes := propCount(tn)

				res.totalInputs += nestedRes.totalInputs
				res.totalOutputs += nestedRes.totalOutputs
				res.inputsMissingDesc += nestedRes.inputsMissingDesc
				res.outputsMissingDesc += nestedRes.outputsMissingDesc
			}
		}

		res.totalOutputs = len(t.ObjectTypeSpec.Properties)

		for _, output := range t.ObjectTypeSpec.Properties {
			if output.Description == "" {
				res.outputsMissingDesc++
			}

			if output.Ref != "" {
				tn := strings.TrimPrefix(output.Ref, "#/types/")
				nestedRes := propCount(tn)

				res.totalInputs += nestedRes.totalInputs
				res.totalOutputs += nestedRes.totalOutputs
				res.inputsMissingDesc += nestedRes.inputsMissingDesc
				res.outputsMissingDesc += nestedRes.outputsMissingDesc
			}
		}

		return res
	}

	for n, r := range sch.Resources {
		baseName := VersionlessName(n)
		if uniques.Contains(baseName) {
			continue
		}
		uniques.Add(baseName)

		stats.Resources.TotalInputProperties += len(r.InputProperties)
		stats.Resources.TotalDescriptionBytes += len(r.Description)

		for _, input := range r.InputProperties {
			if input.Description == "" {
				stats.Resources.InputPropertiesMissingDescriptions++
			}

			if input.Ref != "" {
				typeName := strings.TrimPrefix(input.Ref, "#/types/")
				res := propCount(typeName)
				stats.Resources.TotalInputProperties += res.totalInputs
				stats.Resources.InputPropertiesMissingDescriptions += res.inputsMissingDesc
				stats.Resources.TotalOutputProperties += res.totalOutputs
				stats.Resources.OutputPropertiesMissingDescriptions += res.outputsMissingDesc
			}
		}

		stats.Resources.TotalOutputProperties += len(r.ObjectTypeSpec.Properties)

		for _, output := range r.ObjectTypeSpec.Properties {
			if output.Description == "" {
				stats.Resources.OutputPropertiesMissingDescriptions++
			}

			if output.Ref != "" {
				typeName := strings.TrimPrefix(output.Ref, "#/types/")
				res := propCount(typeName)
				stats.Resources.TotalInputProperties += res.totalInputs
				stats.Resources.InputPropertiesMissingDescriptions += res.inputsMissingDesc
				stats.Resources.TotalOutputProperties += res.totalOutputs
				stats.Resources.OutputPropertiesMissingDescriptions += res.outputsMissingDesc
			}
		}
	}

	stats.Resources.TotalResources = uniques.Cardinality()

	stats.Functions.TotalFunctions = len(sch.Functions)
	for _, v := range sch.Functions {
		stats.Functions.TotalDescriptionBytes += len(v.Description)

		if v.Inputs != nil && v.Inputs.Properties != nil {
			for _, vv := range v.Inputs.Properties {
				stats.Functions.TotalInputPropertyDescriptionBytes += len(vv.Description)
				if vv.Description == "" {
					stats.Functions.InputPropertiesMissingDescriptions++
				}
			}
		}

		if v.Outputs != nil && v.Outputs.Properties != nil {
			for _, vv := range v.Outputs.Properties {
				stats.Functions.TotalOutputPropertyDescriptionBytes += len(vv.Description)
				if vv.Description == "" {
					stats.Functions.OutputPropertiesMissingDescriptions++
				}
			}
		}
	}

	return stats
}

// "azure-native:appplatform/v20230101preview" -> "appplatform"
func VersionlessName(name string) string {
	parts := strings.Split(name, ":")
	mod := parts[1]
	modParts := strings.Split(mod, "/")
	if len(modParts) == 2 {
		mod = modParts[0]
	}
	return fmt.Sprintf("%s:%s", mod, parts[2])
}

// Is it of the form "azure-native:appplatform/v20230101preview" or just "azure-native:appplatform"?
func IsVersionedName(name string) bool {
	return strings.Contains(name, "/v")
}
