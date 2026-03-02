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
	"github.com/pulumi/schema-tools/internal/normalize"
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
	if len(result.Changes) == 0 {
		t.Fatal("expected fixture to produce breaking changes")
	}
	for i, change := range result.Changes {
		if change.Scope == "" || change.Token == "" || change.Path == "" || change.Kind == "" || change.Severity == "" {
			t.Fatalf("unexpected incomplete structured change at index %d: %+v", i, change)
		}
	}
}

func TestSchemasFixtureBuildsStructuredGroupedProjection(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	if got, want := len(result.Changes), 8; got != want {
		t.Fatalf("unexpected change count: got %d want %d", got, want)
	}
	if got := countGroupedLeaves(result.Grouped); got != len(result.Changes) {
		t.Fatalf("grouped leaves must match change count: got %d want %d", got, len(result.Changes))
	}
	if _, ok := result.Grouped.Functions["my-pkg:index:MyFunction"]["inputs"]; !ok {
		t.Fatalf("expected grouped functions inputs entry for MyFunction, got %+v", result.Grouped.Functions["my-pkg:index:MyFunction"])
	}
	if _, ok := result.Grouped.Resources["my-pkg:index:RemovedResource"]["general"]; !ok {
		t.Fatalf("expected grouped resources general entry for RemovedResource, got %+v", result.Grouped.Resources["my-pkg:index:RemovedResource"])
	}
}

func TestSchemasBreakingChangesIncludeDirectAndNestedForSameFunction(t *testing.T) {
	oldSchema := schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
		Inputs: &schema.ObjectTypeSpec{},
	})
	newSchema := schemaWithFunction("my-pkg:index:MyFunction", schema.FunctionSpec{
		Inputs: &schema.ObjectTypeSpec{
			Properties: map[string]schema.PropertySpec{
				"arg": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
			Required: []string{"arg"},
		},
	})

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	grouped, ok := result.Grouped.Functions["my-pkg:index:MyFunction"]
	if !ok {
		t.Fatalf("missing grouped entry for function: %+v", result.Grouped.Functions)
	}
	if len(grouped["general"]) == 0 {
		t.Fatalf("expected direct signature change in general bucket, got %+v", grouped)
	}
	if len(grouped["inputs"]) == 0 {
		t.Fatalf("expected nested input required change in inputs bucket, got %+v", grouped)
	}
}

func TestSchemasMaxChangesLimitsStructuredChanges(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: 1})

	if got, want := len(result.Changes), 1; got != want {
		t.Fatalf("unexpected capped change count: got %d want %d", got, want)
	}
	if got := countGroupedLeaves(result.Grouped); got != len(result.Changes) {
		t.Fatalf("grouped leaves must match capped change count: got %d want %d", got, len(result.Changes))
	}
}

func TestSchemasMaxChangesZeroTracksTotalBreakingCount(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: 0})

	if len(result.Changes) != 0 || !isGroupedEmpty(result.Grouped) {
		t.Fatalf("expected no displayed breaking lines with max-changes=0, got changes=%v grouped=%v", result.Changes, result.Grouped)
	}
	if got, want := result.totalBreaking, 8; got != want {
		t.Fatalf("unexpected total breaking count: got %d want %d", got, want)
	}
}

func TestCategoryForKind(t *testing.T) {
	tests := []struct {
		name string
		kind internalcompare.ChangeKind
		want string
	}{
		{
			name: "missing resource",
			kind: internalcompare.ChangeKindMissingResource,
			want: "missing-resource",
		},
		{
			name: "missing function",
			kind: internalcompare.ChangeKindMissingFunction,
			want: "missing-function",
		},
		{
			name: "missing type",
			kind: internalcompare.ChangeKindMissingType,
			want: "missing-type",
		},
		{
			name: "missing input",
			kind: internalcompare.ChangeKindMissingInput,
			want: "missing-input",
		},
		{
			name: "missing output",
			kind: internalcompare.ChangeKindMissingOutput,
			want: "missing-output",
		},
		{
			name: "missing property",
			kind: internalcompare.ChangeKindMissingProperty,
			want: "missing-property",
		},
		{
			name: "type changed",
			kind: internalcompare.ChangeKindTypeChanged,
			want: "type-changed",
		},
		{
			name: "optional to required",
			kind: internalcompare.ChangeKindOptionalToRequired,
			want: "optional-to-required",
		},
		{
			name: "required to optional",
			kind: internalcompare.ChangeKindRequiredToOptional,
			want: "required-to-optional",
		},
		{
			name: "signature changed",
			kind: internalcompare.ChangeKindSignatureChanged,
			want: "signature-changed",
		},
		{
			name: "new resource not summarized",
			kind: internalcompare.ChangeKindNewResource,
			want: "other",
		},
		{
			name: "new function not summarized",
			kind: internalcompare.ChangeKindNewFunction,
			want: "other",
		},
		{
			name: "token remap summarized",
			kind: internalcompare.ChangeKindTokenRemapped,
			want: "token-remapped",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := categoryForKind(tc.kind); got != tc.want {
				t.Fatalf("categoryForKind(%q) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

func TestSchemasSuppressesAddRemoveForResolvedResourceTokenRemap(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v2:Widget": {},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget"}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v2:Widget","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}]}}}}`)

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1, OldMetadata: oldMetadata, NewMetadata: newMetadata})
	if got := result.totalBreaking; got != 0 {
		t.Fatalf("expected no breaking changes, got total=%d details=%v", got, result.Changes)
	}
	if got, want := result.Summary, []SummaryItem{{Category: "token-remapped", Count: 1, Entries: []string{`Resources: "my-pkg:index/v1:Widget" token remapped: migrate from "my-pkg:index/v1:Widget" to "my-pkg:index/v2:Widget"`}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected summary: got %#v want %#v", got, want)
	}
	if got, want := result.Changes, []Change{
		{
			Scope:    ScopeResource,
			Token:    "my-pkg:index/v1:Widget",
			Path:     `Resources: "my-pkg:index/v1:Widget"`,
			Kind:     "token-remapped",
			Severity: SeverityInfo,
			Breaking: false,
			Message:  `token remapped: migrate from "my-pkg:index/v1:Widget" to "my-pkg:index/v2:Widget"`,
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected remap changes: got %#v want %#v", got, want)
	}
	if got, want := result.NewResources, []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected new resources: got %#v want %#v", got, want)
	}
}

func TestSchemasRetainedInCodegenAliasStillListsCanonicalNewResource(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"aws:s3/bucketAclV2:BucketAclV2": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"aws:s3/bucketAclV2:BucketAclV2": {},
			"aws:s3/bucketAcl:BucketAcl":     {},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"aws_s3_bucket_acl":{"current":"aws:s3/bucketAclV2:BucketAclV2"}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"aws_s3_bucket_acl":{"current":"aws:s3/bucketAcl:BucketAcl","past":[{"name":"aws:s3/bucketAclV2:BucketAclV2","inCodegen":true,"majorVersion":7}]}}}}`)

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "aws",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	if got, want := result.NewResources, []string{"s3/bucketAcl.BucketAcl"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected NewResources to contain s3/bucketAcl.BucketAcl, got %#v", got)
	}
	if got, want := len(result.NewResources), 1; got != want {
		t.Fatalf("expected exactly one canonical new resource, got %d (%#v)", got, result.NewResources)
	}
	if got, want := result.Summary, []SummaryItem{{Category: "token-remapped", Count: 1, Entries: []string{`Resources: "aws:s3/bucketAclV2:BucketAclV2" token deprecated: prefer "aws:s3/bucketAcl:BucketAcl" instead of "aws:s3/bucketAclV2:BucketAclV2"`}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected non-breaking remap summary signal, got %#v", got)
	}
	if got, want := result.Changes, []Change{
		{
			Scope:    ScopeResource,
			Token:    "aws:s3/bucketAclV2:BucketAclV2",
			Path:     `Resources: "aws:s3/bucketAclV2:BucketAclV2"`,
			Kind:     "token-remapped",
			Severity: SeverityInfo,
			Breaking: false,
			Message:  `token deprecated: prefer "aws:s3/bucketAcl:BucketAcl" instead of "aws:s3/bucketAclV2:BucketAclV2"`,
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected remap/deprecation change lines, got %#v", got)
	}
}

func TestSchemasRetainedInCodegenAliasStillListsCanonicalNewFunction(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"aws:s3/getBucketAclV2:getBucketAclV2": {},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"aws:s3/getBucketAclV2:getBucketAclV2": {},
			"aws:s3/getBucketAcl:getBucketAcl":     {},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"datasources":{"aws_s3_bucket_acl":{"current":"aws:s3/getBucketAclV2:getBucketAclV2"}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"datasources":{"aws_s3_bucket_acl":{"current":"aws:s3/getBucketAcl:getBucketAcl","past":[{"name":"aws:s3/getBucketAclV2:getBucketAclV2","inCodegen":true,"majorVersion":7}]}}}}`)

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "aws",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	if got, want := result.NewFunctions, []string{"s3/getBucketAcl.getBucketAcl"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected NewFunctions to contain s3/getBucketAcl.getBucketAcl, got %#v", got)
	}
	if got, want := result.Summary, []SummaryItem{{Category: "token-remapped", Count: 1, Entries: []string{`Functions: "aws:s3/getBucketAclV2:getBucketAclV2" token deprecated: prefer "aws:s3/getBucketAcl:getBucketAcl" instead of "aws:s3/getBucketAclV2:getBucketAclV2"`}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected non-breaking remap summary signal, got %#v", got)
	}
	if got, want := result.Changes, []Change{
		{
			Scope:    ScopeFunction,
			Token:    "aws:s3/getBucketAclV2:getBucketAclV2",
			Path:     `Functions: "aws:s3/getBucketAclV2:getBucketAclV2"`,
			Kind:     "token-remapped",
			Severity: SeverityInfo,
			Breaking: false,
			Message:  `token deprecated: prefer "aws:s3/getBucketAcl:getBucketAcl" instead of "aws:s3/getBucketAclV2:getBucketAclV2"`,
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected remap/deprecation change lines, got %#v", got)
	}
}

func TestSchemasSuppressesAddRemoveForResolvedFunctionTokenRemap(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index/v1:getWidget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index/v2:getWidget": {},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_widget":{"current":"my-pkg:index/v1:getWidget"}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_widget":{"current":"my-pkg:index/v2:getWidget","past":[{"name":"my-pkg:index/v1:getWidget","inCodegen":false,"majorVersion":1}]}}}}`)

	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1, OldMetadata: oldMetadata, NewMetadata: newMetadata})
	if got := result.totalBreaking; got != 0 {
		t.Fatalf("expected no breaking changes, got total=%d details=%v", got, result.Changes)
	}
	if got, want := result.Summary, []SummaryItem{{Category: "token-remapped", Count: 1, Entries: []string{`Functions: "my-pkg:index/v1:getWidget" token remapped: migrate from "my-pkg:index/v1:getWidget" to "my-pkg:index/v2:getWidget"`}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected summary: got %#v want %#v", got, want)
	}
	if got, want := result.Changes, []Change{
		{
			Scope:    ScopeFunction,
			Token:    "my-pkg:index/v1:getWidget",
			Path:     `Functions: "my-pkg:index/v1:getWidget"`,
			Kind:     "token-remapped",
			Severity: SeverityInfo,
			Breaking: false,
			Message:  `token remapped: migrate from "my-pkg:index/v1:getWidget" to "my-pkg:index/v2:getWidget"`,
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected remap changes: got %#v want %#v", got, want)
	}
	if got, want := result.NewFunctions, []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected new functions: got %#v want %#v", got, want)
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
			result := Schemas(tc.oldSchema, tc.newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

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

func mustParseMetadataCompare(t testing.TB, metadata string) *normalize.MetadataEnvelope {
	t.Helper()
	parsed, err := normalize.ParseMetadata([]byte(metadata))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	return parsed
}
