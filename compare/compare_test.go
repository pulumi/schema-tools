package compare

import (
	"slices"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestSchemasSortsNewResourcesAndFunctions(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:ExistingResource": {},
		},
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:existingFunction": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:ExistingResource": {},
			"my-pkg:index:ZetaResource":     {},
			"my-pkg:module:AlphaResource":   {},
		},
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:existingFunction": {},
			"my-pkg:index:zetaFunction":     {},
			"my-pkg:module:alphaFunction":   {},
		},
	}

	result := Schemas(oldSchema, newSchema, Options{
		Provider:   "my-pkg",
		MaxChanges: -1,
	})

	if len(result.Summary) != 0 {
		t.Fatalf("expected no summary items in foundation scope, got %v", result.Summary)
	}
	if got, want := result.NewResources, []string{"index.ZetaResource", "module.AlphaResource"}; !slices.Equal(got, want) {
		t.Fatalf("new resources mismatch: got %v want %v", got, want)
	}
	if got, want := result.NewFunctions, []string{"index.zetaFunction", "module.alphaFunction"}; !slices.Equal(got, want) {
		t.Fatalf("new functions mismatch: got %v want %v", got, want)
	}
}
