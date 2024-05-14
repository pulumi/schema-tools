// Copyright 2016-2024, Pulumi Corporation.

package cmd

import (
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

// Navigate through each resource in the schema and find how many types are referenced from that resource,
// recursively. If the count increased for a resource and is above 200, emit a warning.
func concerningTypeStructure(provider string, oldSchema *schema.PackageSpec,
	newSchema *schema.PackageSpec, violations *diagtree.Node) {
	oldTypeCounts := typeCountPerResource(oldSchema)
	newTypeCounts := typeCountPerResource(newSchema)
	section := violations.Label("Max Type Count per Resource")
	for resName, newCount := range newTypeCounts {
		oldCount, _ := oldTypeCounts[resName]
		if newCount > oldCount && newCount > 200 {
			msg := section.Value(formatName(provider, resName))
			msg.SetDescription(diagtree.Warn, "number of types increased from %d to %d", oldCount, newCount)
		}
	}
}

func typeCountPerResource(schema *schema.PackageSpec) map[string]int {
	res := make(map[string]int)
	for name, r := range schema.Resources {
		visitedTypes := make(map[string]bool)
		countTypes(schema, r.InputProperties, visitedTypes)
		countTypes(schema, r.Properties, visitedTypes)
		res[name] = len(visitedTypes)
	}
	return res
}

func countTypes(schema *schema.PackageSpec, props map[string]schema.PropertySpec, visitedTypes map[string]bool) {
	for _, prop := range props {
		countType(schema, &prop.TypeSpec, visitedTypes)
	}
}

func countType(schema *schema.PackageSpec, ts *schema.TypeSpec, visitedTypes map[string]bool) {
	if ts.AdditionalProperties != nil {
		countType(schema, ts.AdditionalProperties, visitedTypes)
	}
	if ts.Items != nil {
		countType(schema, ts.Items, visitedTypes)
	}
	if len(ts.OneOf) > 0 {
		for _, t := range ts.OneOf {
			countType(schema, &t, visitedTypes)
		}
	}
	if strings.HasPrefix(ts.Ref, "#/types/") {
		typeName := strings.TrimPrefix(ts.Ref, "#/types/")
		if _, ok := visitedTypes[typeName]; ok {
			return
		}
		visitedTypes[typeName] = true
		typ := schema.Types[typeName]
		countTypes(schema, typ.Properties, visitedTypes)
	}
}
