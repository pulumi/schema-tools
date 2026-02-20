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
	internalcompare "github.com/pulumi/schema-tools/internal/compare"
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

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg"})

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
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg"})

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
	if len(result.Changes) == 0 {
		t.Fatal("expected fixture to produce canonical changes")
	}
	for i, change := range result.Changes {
		if change.Kind == "" {
			t.Fatalf("change[%d] missing kind: %+v", i, change)
		}
		if change.Path == "" {
			t.Fatalf("change[%d] missing path: %+v", i, change)
		}
		if change.Source != SourceEngine {
			t.Fatalf("change[%d] unexpected source: %+v", i, change)
		}
		if change.Severity == "" {
			t.Fatalf("change[%d] missing severity: %+v", i, change)
		}
	}
}

func TestClassifySeverityByKind(t *testing.T) {
	tests := []struct {
		kind     string
		severity ChangeSeverity
		breaking bool
	}{
		{kind: "missing-resource", severity: SeverityError, breaking: true},
		{kind: "missing-function", severity: SeverityError, breaking: true},
		{kind: "missing-type", severity: SeverityError, breaking: true},
		{kind: "signature-changed", severity: SeverityError, breaking: true},
		{kind: "optional-to-required", severity: SeverityError, breaking: true},
		{kind: "type-changed", severity: SeverityWarn, breaking: true},
		{kind: "missing-input", severity: SeverityWarn, breaking: true},
		{kind: "missing-output", severity: SeverityWarn, breaking: true},
		{kind: "missing-property", severity: SeverityWarn, breaking: true},
		{kind: "max-items-one-changed", severity: SeverityError, breaking: true},
		{kind: "renamed-resource", severity: SeverityError, breaking: true},
		{kind: "renamed-function", severity: SeverityError, breaking: true},
		{kind: "required-to-optional", severity: SeverityInfo, breaking: false},
		{kind: "deprecated-resource-alias", severity: SeverityInfo, breaking: false},
		{kind: "deprecated-function-alias", severity: SeverityInfo, breaking: false},
		{kind: "unknown-kind", severity: SeverityWarn, breaking: true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			gotSeverity, gotBreaking := classifySeverity(tt.kind)
			if gotSeverity != tt.severity || gotBreaking != tt.breaking {
				t.Fatalf("classifySeverity(%q) = (%q,%v), want (%q,%v)",
					tt.kind, gotSeverity, gotBreaking, tt.severity, tt.breaking)
			}
		})
	}
}

func TestSortChangesDeterministic(t *testing.T) {
	changes := []Change{
		{
			Scope:    ScopeType,
			Token:    "pkg:index:Type",
			Location: "properties",
			Path:     `Types: "pkg:index:Type": properties: "a"`,
			Kind:     "missing-property",
			Message:  "missing",
			Source:   SourceEngine,
		},
		{
			Scope:    ScopeResource,
			Token:    "pkg:index:Res",
			Location: "inputs",
			Path:     `Resources: "pkg:index:Res": inputs: "name"`,
			Kind:     "missing-input",
			Message:  "missing",
			Source:   SourceEngine,
		},
		{
			Scope:    ScopeFunction,
			Token:    "pkg:index:getThing",
			Location: "signature",
			Path:     `Functions: "pkg:index:getThing"`,
			Kind:     "signature-changed",
			Message:  "signature change",
			Source:   SourceEngine,
		},
	}

	got := sortChanges(changes)
	wantScopes := []ChangeScope{ScopeResource, ScopeFunction, ScopeType}
	if len(got) != len(wantScopes) {
		t.Fatalf("unexpected sorted size: got %d want %d", len(got), len(wantScopes))
	}
	for i, want := range wantScopes {
		if got[i].Scope != want {
			t.Fatalf("sort order mismatch at %d: got %q want %q (changes=%+v)", i, got[i].Scope, want, got)
		}
	}
}

func TestChangesFromDiagnostics(t *testing.T) {
	diagnostics := []internalcompare.Diagnostic{
		{
			Scope:       "Resources",
			Token:       "pkg:index:Res",
			Location:    "inputs",
			Path:        `Resources: "pkg:index:Res": inputs: "name"`,
			Description: "missing",
		},
		{
			Scope:       "Functions",
			Token:       "pkg:index:getThing",
			Location:    "signature",
			Path:        `Functions: "pkg:index:getThing"`,
			Description: "signature change",
		},
	}

	changes := changesFromDiagnostics(diagnostics)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].Scope != ScopeResource || changes[0].Kind != "missing-input" {
		t.Fatalf("unexpected resource conversion: %+v", changes[0])
	}
	if !changes[0].Breaking || changes[0].Severity != SeverityWarn {
		t.Fatalf("unexpected breaking/severity conversion: %+v", changes[0])
	}
	if changes[1].Scope != ScopeFunction || changes[1].Kind != "signature-changed" {
		t.Fatalf("unexpected function conversion: %+v", changes[1])
	}
	if !changes[1].Breaking || changes[1].Severity != SeverityError {
		t.Fatalf("unexpected signature severity: %+v", changes[1])
	}
}

func TestGroupChangesByScopeTokenLocation(t *testing.T) {
	changes := sortChanges([]Change{
		{
			Scope:    ScopeFunction,
			Token:    "pkg:index:getThing",
			Location: "inputs",
			Path:     `Functions: "pkg:index:getThing": inputs: "arg"`,
			Kind:     "missing-input",
			Severity: SeverityWarn,
			Breaking: true,
			Source:   SourceEngine,
			Message:  "missing input",
		},
		{
			Scope:    ScopeResource,
			Token:    "pkg:index:Widget",
			Location: "properties",
			Path:     `Resources: "pkg:index:Widget": properties: "id"`,
			Kind:     "missing-output",
			Severity: SeverityWarn,
			Breaking: true,
			Source:   SourceEngine,
			Message:  "missing output",
		},
		{
			Scope:    ScopeType,
			Token:    "pkg:index:SharedType",
			Location: "properties",
			Path:     `Types: "pkg:index:SharedType": properties: "value"`,
			Kind:     "type-changed",
			Severity: SeverityWarn,
			Breaking: true,
			Source:   SourceEngine,
			Message:  "type changed",
		},
	})

	grouped := groupChanges(changes)
	if got := len(grouped.Resources["pkg:index:Widget"]["properties"]); got != 1 {
		t.Fatalf("expected one grouped resource change, got %d", got)
	}
	if got := len(grouped.Functions["pkg:index:getThing"]["inputs"]); got != 1 {
		t.Fatalf("expected one grouped function change, got %d", got)
	}
	if got := len(grouped.Types["pkg:index:SharedType"]["properties"]); got != 1 {
		t.Fatalf("expected one grouped type change, got %d", got)
	}
}

func TestSchemasAttachesTypeImpactMetadata(t *testing.T) {
	typeToken := "my-pkg:index:SharedConfig"
	oldSchema := schema.PackageSpec{
		Name: "my-pkg",
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
			"my-pkg:index:Wrapper": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:getWidget": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Name: "my-pkg",
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "integer"}},
					},
				},
			},
			"my-pkg:index:Wrapper": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:getWidget": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
	}

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg"})

	var typeChange *Change
	for i := range result.Changes {
		change := &result.Changes[i]
		if change.Scope == ScopeType && change.Token == typeToken && change.Kind == "type-changed" {
			typeChange = change
			break
		}
	}
	if typeChange == nil {
		t.Fatalf("expected type change for %q, got %+v", typeToken, result.Changes)
	}

	if typeChange.ImpactCount != 3 {
		t.Fatalf("expected 3 direct impacts, got %d (%+v)", typeChange.ImpactCount, typeChange.ImpactedBy)
	}
	wantImpacts := []ImpactRef{
		{Scope: ScopeResource, Token: "my-pkg:index:Widget", Location: "inputs", Path: "config"},
		{Scope: ScopeFunction, Token: "my-pkg:index:getWidget", Location: "inputs", Path: "config"},
		{Scope: ScopeType, Token: "my-pkg:index:Wrapper", Location: "properties", Path: "config"},
	}
	if !reflect.DeepEqual(typeChange.ImpactedBy, wantImpacts) {
		t.Fatalf("impact metadata mismatch:\n got: %+v\nwant: %+v", typeChange.ImpactedBy, wantImpacts)
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

func TestSchemasClassificationContractWithInternalDiagnostics(t *testing.T) {
	tests := []struct {
		name            string
		oldSchema       schema.PackageSpec
		newSchema       schema.PackageSpec
		wantCategory    string
		wantCategoryCnt int
	}{
		{
			name: "missing function input maps to missing-input",
			oldSchema: schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			}),
			newSchema: schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
				Inputs: &schema.ObjectTypeSpec{Properties: map[string]schema.PropertySpec{}},
			}),
			wantCategory:    "missing-input",
			wantCategoryCnt: 1,
		},
		{
			name: "missing function output maps to missing-output",
			oldSchema: schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			}),
			newSchema: schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
				Outputs: &schema.ObjectTypeSpec{Properties: map[string]schema.PropertySpec{}},
			}),
			wantCategory:    "missing-output",
			wantCategoryCnt: 1,
		},
		{
			name: "missing type property maps to missing-property",
			oldSchema: schemaWithType("my-pkg:index:MyType", schema.ComplexTypeSpec{
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			}),
			newSchema: schemaWithType("my-pkg:index:MyType", schema.ComplexTypeSpec{
				ObjectTypeSpec: schema.ObjectTypeSpec{Properties: map[string]schema.PropertySpec{}},
			}),
			wantCategory:    "missing-property",
			wantCategoryCnt: 1,
		},
		{
			name: "invoke signature change maps to signature-changed",
			oldSchema: schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
				Inputs: nil,
			}),
			newSchema: schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"arg": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			}),
			wantCategory:    "signature-changed",
			wantCategoryCnt: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Schemas(tc.oldSchema, tc.newSchema, Options{Provider: "my-pkg"})

			gotCounts := map[string]int{}
			for _, item := range result.Summary {
				gotCounts[item.Category] = item.Count
			}
			if gotCounts["other"] != 0 {
				t.Fatalf("unexpected fallback category in summary: %v", result.Summary)
			}
			if gotCounts[tc.wantCategory] != tc.wantCategoryCnt {
				t.Fatalf("expected %q count %d, got summary=%v", tc.wantCategory, tc.wantCategoryCnt, result.Summary)
			}
		})
	}
}

func mustLoadFixtureSchemas(t testing.TB) (schema.PackageSpec, schema.PackageSpec) {
	t.Helper()
	// Keep in sync with internal/cmd/compare_test.go helpers by design:
	// tests in these two packages cannot share *_test.go helpers directly.
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

func schemaWithFunction(token string, fn schema.FunctionSpec) schema.PackageSpec {
	return schema.PackageSpec{
		Name: "my-pkg",
		Functions: map[string]schema.FunctionSpec{
			token: fn,
		},
	}
}

func schemaWithType(token string, typ schema.ComplexTypeSpec) schema.PackageSpec {
	return schema.PackageSpec{
		Name: "my-pkg",
		Types: map[string]schema.ComplexTypeSpec{
			token: typ,
		},
	}
}
