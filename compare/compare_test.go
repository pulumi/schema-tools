package compare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
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
		t.Fatalf("expected no summary items, got %v", result.Summary)
	}
	if got, want := result.NewResources, []string{"index.ZetaResource", "module.AlphaResource"}; !slices.Equal(got, want) {
		t.Fatalf("new resources mismatch: got %v want %v", got, want)
	}
	if got, want := result.NewFunctions, []string{"index.zetaFunction", "module.alphaFunction"}; !slices.Equal(got, want) {
		t.Fatalf("new functions mismatch: got %v want %v", got, want)
	}
}

func TestSchemasBuildsSummaryWithEntriesAndPaths(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	wantCategories := []string{
		"missing-function",
		"missing-resource",
		"optional-to-required",
		"required-to-optional",
		"type-changed",
	}
	gotCategories := make([]string, 0, len(result.Summary))
	gotCounts := map[string]int{}
	gotEntries := map[string][]string{}
	for _, item := range result.Summary {
		gotCategories = append(gotCategories, item.Category)
		gotCounts[item.Category] = item.Count
		gotEntries[item.Category] = item.Entries
		if !sort.StringsAreSorted(item.Entries) {
			t.Fatalf("entries for %q are not sorted: %v", item.Category, item.Entries)
		}
	}

	if !reflect.DeepEqual(gotCategories, wantCategories) {
		t.Fatalf("summary categories mismatch: got %v want %v", gotCategories, wantCategories)
	}
	if !reflect.DeepEqual(gotCounts, expectedFixtureSummaryCounts()) {
		t.Fatalf("summary counts mismatch: got %v want %v", gotCounts, expectedFixtureSummaryCounts())
	}
	if !reflect.DeepEqual(gotEntries, expectedFixtureSummaryEntries()) {
		t.Fatalf("summary entries mismatch:\n got: %v\nwant: %v", gotEntries, expectedFixtureSummaryEntries())
	}
	if len(result.BreakingChanges) == 0 {
		t.Fatal("expected fixture to produce breaking changes")
	}
	for i, line := range result.BreakingChanges {
		if line == "" {
			t.Fatalf("unexpected blank breaking change line at index %d", i)
		}
	}
}

func TestClassifyDiagnosticDescriptions(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		description string
		want        string
	}{
		{
			name:        "resource missing input by path and missing description",
			path:        `Resources: "pkg:index:Res": inputs: "name"`,
			description: "missing",
			want:        "missing-input",
		},
		{
			name:        "type missing property by path and missing description",
			path:        `Types: "pkg:index:Type": properties: "name"`,
			description: "missing",
			want:        "missing-property",
		},
		{
			name:        "function missing input by message",
			path:        `Functions: "pkg:index:getThing": inputs: "name"`,
			description: `missing input "name"`,
			want:        "missing-input",
		},
		{
			name:        "missing output",
			path:        `Functions: "pkg:index:getThing": outputs: "name"`,
			description: "missing output",
			want:        "missing-output",
		},
		{
			name:        "missing resource",
			path:        `Resources: "pkg:index:Res"`,
			description: "missing",
			want:        "missing-resource",
		},
		{
			name:        "missing function",
			path:        `Functions: "pkg:index:getThing"`,
			description: "missing",
			want:        "missing-function",
		},
		{
			name:        "missing type",
			path:        `Types: "pkg:index:Type"`,
			description: "missing",
			want:        "missing-type",
		},
		{
			name:        "type changed",
			path:        "any",
			description: `type changed from "string" to "integer"`,
			want:        "type-changed",
		},
		{
			name:        "had no type",
			path:        "any",
			description: "had no type but now has %+v",
			want:        "type-changed",
		},
		{
			name:        "now has no type",
			path:        "any",
			description: "had %+v but now has no type",
			want:        "type-changed",
		},
		{
			name:        "optional to required",
			path:        "any",
			description: "input has changed to Required",
			want:        "optional-to-required",
		},
		{
			name:        "required to optional",
			path:        "any",
			description: "property is no longer Required",
			want:        "required-to-optional",
		},
		{
			name:        "signature changed",
			path:        `Functions: "pkg:index:getThing"`,
			description: "signature change (pulumi.InvokeOptions)->T => (Args, pulumi.InvokeOptions)->T",
			want:        "signature-changed",
		},
		{
			name:        "fallback other",
			path:        "any",
			description: "something unexpected",
			want:        "other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.path, tc.description); got != tc.want {
				t.Fatalf("classify(%q, %q) = %q, want %q", tc.path, tc.description, got, tc.want)
			}
		})
	}
}

func mustLoadFixtureSchemas(t testing.TB) (schema.PackageSpec, schema.PackageSpec) {
	t.Helper()
	oldSchema := mustReadFixtureSchema(t, "schema-old.json")
	newSchema := mustReadFixtureSchema(t, "schema-new.json")
	return oldSchema, newSchema
}

func mustReadFixtureSchema(t testing.TB, name string) schema.PackageSpec {
	t.Helper()
	data := mustReadTestdataFile(t, name)
	var spec schema.PackageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal fixture %q: %v", name, err)
	}
	return spec
}

func mustReadTestdataFile(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "testdata", "compare", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return data
}

func expectedFixtureSummaryCounts() map[string]int {
	return map[string]int{
		"missing-function":     1,
		"missing-resource":     1,
		"optional-to-required": 3,
		"required-to-optional": 2,
		"type-changed":         1,
	}
}

func expectedFixtureSummaryEntries() map[string][]string {
	return map[string][]string{
		"missing-function": {
			`Functions: "my-pkg:index:removedFunction" missing`,
		},
		"missing-resource": {
			`Resources: "my-pkg:index:RemovedResource" missing`,
		},
		"optional-to-required": {
			`Functions: "my-pkg:index:MyFunction": inputs: required: "arg" input has changed to Required`,
			`Resources: "my-pkg:index:MyResource": required inputs: "count" input has changed to Required`,
			`Types: "my-pkg:index:MyType": required: "count" property has changed to Required`,
		},
		"required-to-optional": {
			`Resources: "my-pkg:index:MyResource": required: "value" property is no longer Required`,
			`Types: "my-pkg:index:MyType": required: "field" property is no longer Required`,
		},
		"type-changed": {
			`Types: "my-pkg:index:MyType": properties: "field" type changed from "string" to "integer"`,
		},
	}
}
