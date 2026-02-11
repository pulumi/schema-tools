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

func TestCompareClassifiesDirectMaxItemsOneTypeChange(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
		},
	}

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})
	gotCounts := map[string]int{}
	for _, item := range result.Summary {
		gotCounts[item.Category] = item.Count
	}

	wantCounts := map[string]int{
		categoryMaxItemsOneChanged: 1,
	}
	if !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatalf("summary counts mismatch: got %v want %v", gotCounts, wantCounts)
	}
}

func TestComparePluralizedMaxItemsOneRenameIsConservative(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				RequiredInputs: []string{"filter"},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"filters": {
						TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						},
					},
				},
				RequiredInputs: []string{"filters"},
			},
		},
	}

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})
	gotCounts := map[string]int{}
	for _, item := range result.Summary {
		gotCounts[item.Category] = item.Count
	}
	wantCounts := map[string]int{
		categoryMaxItemsOneChanged: 1,
	}
	if !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatalf("summary counts mismatch: got %v want %v", gotCounts, wantCounts)
	}

	newSchema.Resources["my-pkg:index:MyResource"] = schema.ResourceSpec{
		InputProperties: map[string]schema.PropertySpec{
			"filters": {
				TypeSpec: schema.TypeSpec{
					Type:  "array",
					Items: &schema.TypeSpec{Type: "integer"},
				},
			},
		},
	}

	result = Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})
	gotCounts = map[string]int{}
	for _, item := range result.Summary {
		gotCounts[item.Category] = item.Count
	}
	wantCounts = map[string]int{
		categoryMissingInput: 1,
	}
	if !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatalf("expected non-matching pluralization to stay missing-input, got %v", gotCounts)
	}
}

func TestComparePluralizedMaxItemsOneRenameDoesNotSuppressWhenKeysCoexist(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				RequiredInputs: []string{"filter"},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					Required: []string{"value"},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
					"filters": {
						TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						},
					},
				},
				RequiredInputs: []string{"filters"},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
						"values": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
					},
					Required: []string{"values"},
				},
			},
		},
	}

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})
	gotCounts := map[string]int{}
	for _, item := range result.Summary {
		gotCounts[item.Category] = item.Count
	}

	wantCounts := map[string]int{
		categoryOptionalToRequired: 2,
		categoryRequiredToOptional: 1,
	}
	if !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatalf("coexisting singular/plural keys should keep requiredness deltas: got %v want %v", gotCounts, wantCounts)
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
