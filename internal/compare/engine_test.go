package compare

import (
	"io"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestAnalyzeListsOnlyNewResourcesAndFunctions(t *testing.T) {
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
			"my-pkg:index:NewResource":      {},
			"my-pkg:module:AnotherResource": {},
		},
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:existingFunction": {},
			"my-pkg:index:newFunction":      {},
			"my-pkg:module:otherFunction":   {},
		},
	}

	report := Analyze("my-pkg", oldSchema, newSchema)

	if got := report.Violations.Display(io.Discard, -1); got != 0 {
		t.Fatalf("expected no violations, got %d", got)
	}

	expectedResources := map[string]bool{
		"index.NewResource":      true,
		"module.AnotherResource": true,
	}
	if len(report.NewResources) != len(expectedResources) {
		t.Fatalf("expected %d new resources, got %d (%v)", len(expectedResources), len(report.NewResources), report.NewResources)
	}
	for _, resource := range report.NewResources {
		if !expectedResources[resource] {
			t.Fatalf("unexpected new resource %q (all: %v)", resource, report.NewResources)
		}
	}

	expectedFunctions := map[string]bool{
		"index.newFunction":    true,
		"module.otherFunction": true,
	}
	if len(report.NewFunctions) != len(expectedFunctions) {
		t.Fatalf("expected %d new functions, got %d (%v)", len(expectedFunctions), len(report.NewFunctions), report.NewFunctions)
	}
	for _, function := range report.NewFunctions {
		if !expectedFunctions[function] {
			t.Fatalf("unexpected new function %q (all: %v)", function, report.NewFunctions)
		}
	}
}
