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
		t.Fatalf("expected no summary items, got %v", result.Summary)
	}
	if got, want := result.NewResources, []string{"index.ZetaResource", "module.AlphaResource"}; !equalStrings(got, want) {
		t.Fatalf("new resources mismatch: got %v want %v", got, want)
	}
	if got, want := result.NewFunctions, []string{"index.zetaFunction", "module.alphaFunction"}; !equalStrings(got, want) {
		t.Fatalf("new functions mismatch: got %v want %v", got, want)
	}
}

func TestCompareBuildsSummaryWithEntriesAndPaths(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "number"}},
					},
				},
			},
		},
	}

	result := Compare(oldSchema, newSchema, CompareOptions{Provider: "my-pkg", MaxChanges: -1})

	if len(result.Summary) == 0 {
		t.Fatalf("expected summary entries")
	}

	seenMissingInput := false
	seenTypeChanged := false
	for _, item := range result.Summary {
		if item.Category == "missing-input" {
			seenMissingInput = true
			if item.Count != 1 {
				t.Fatalf("expected missing-input count 1, got %d", item.Count)
			}
			if len(item.Entries) == 0 {
				t.Fatalf("expected missing-input entries, got %+v", item)
			}
		}
		if item.Category == "type-changed" {
			seenTypeChanged = true
			if item.Count != 1 {
				t.Fatalf("expected type-changed count 1, got %d", item.Count)
			}
			if len(item.Entries) == 0 {
				t.Fatalf("expected type-changed entries, got %+v", item)
			}
		}
	}

	if !seenMissingInput {
		t.Fatalf("expected missing-input category in summary")
	}
	if !seenTypeChanged {
		t.Fatalf("expected type-changed category in summary")
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
