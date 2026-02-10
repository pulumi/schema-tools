package compare

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestCompareSortsNewResourcesAndFunctions(t *testing.T) {
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

	result := Compare(oldSchema, newSchema, CompareOptions{
		Provider:   "my-pkg",
		MaxChanges: -1,
	})

	if len(result.Summary) != 0 {
		t.Fatalf("expected no summary items in foundation scope, got %v", result.Summary)
	}
	if got, want := result.NewResources, []string{"index.ZetaResource", "module.AlphaResource"}; !equalStrings(got, want) {
		t.Fatalf("new resources mismatch: got %v want %v", got, want)
	}
	if got, want := result.NewFunctions, []string{"index.zetaFunction", "module.alphaFunction"}; !equalStrings(got, want) {
		t.Fatalf("new functions mismatch: got %v want %v", got, want)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
