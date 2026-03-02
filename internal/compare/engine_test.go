package compare

import (
	"reflect"
	"slices"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/normalize"
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

	report := Analyze("my-pkg", oldSchema, newSchema, nil, nil)

	if got, want := report.NewResources, []string{"index.NewResource", "module.AnotherResource"}; !slices.Equal(got, want) {
		t.Fatalf("unexpected new resources: got %v want %v", got, want)
	}

	if got, want := report.NewFunctions, []string{"index.newFunction", "module.otherFunction"}; !slices.Equal(got, want) {
		t.Fatalf("unexpected new functions: got %v want %v", got, want)
	}

	if got, want := report.Changes, []Change{
		{
			Category:    functionsCategory,
			Name:        "my-pkg:index:newFunction",
			Kind:        ChangeKindNewFunction,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: "added",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeNone,
				Lookup:     "ResolveNewToken",
				Candidates: []string{},
			},
		},
		{
			Category:    functionsCategory,
			Name:        "my-pkg:module:otherFunction",
			Kind:        ChangeKindNewFunction,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: "added",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeNone,
				Lookup:     "ResolveNewToken",
				Candidates: []string{},
			},
		},
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index:NewResource",
			Kind:        ChangeKindNewResource,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: "added",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeNone,
				Lookup:     "ResolveNewToken",
				Candidates: []string{},
			},
		},
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:module:AnotherResource",
			Kind:        ChangeKindNewResource,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: "added",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeNone,
				Lookup:     "ResolveNewToken",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected typed additions: got %#v want %#v", got, want)
	}
}

func TestAnalyzeEmitsDeterministicTypedBreakingChanges(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "number"}},
					},
				},
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "number"}},
				},
				RequiredInputs: []string{"value"},
			},
		},
	}

	report := Analyze("my-pkg", oldSchema, newSchema, nil, nil)

	if got, want := report.Changes, []Change{
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index:MyResource",
			Path:        []string{"inputs", "value"},
			Kind:        ChangeKindTypeChanged,
			Severity:    SeverityWarn,
			Breaking:    true,
			Description: `type changed from "string" to "number"`,
		},
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index:MyResource",
			Path:        []string{"properties", "value"},
			Kind:        ChangeKindTypeChanged,
			Severity:    SeverityWarn,
			Breaking:    true,
			Description: `type changed from "string" to "number"`,
		},
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index:MyResource",
			Path:        []string{"required inputs", "value"},
			Kind:        ChangeKindOptionalToRequired,
			Severity:    SeverityInfo,
			Breaking:    true,
			Description: "input has changed to Required",
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected typed changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeSkipsFunctionRequiredToOptionalWhenOutputRemoved(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: &schema.ObjectTypeSpec{
					Required: []string{"value"},
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: &schema.ObjectTypeSpec{
					Required:   []string{},
					Properties: map[string]schema.PropertySpec{},
				},
			},
		},
	}

	report := Analyze("my-pkg", oldSchema, newSchema, nil, nil)
	expectedMissingOutput := Change{
		Category:    functionsCategory,
		Name:        "my-pkg:index:MyFunction",
		Path:        []string{"outputs", "value"},
		Kind:        ChangeKindMissingOutput,
		Severity:    SeverityWarn,
		Breaking:    true,
		Description: "missing output",
	}
	foundMissingOutput := false
	for _, change := range report.Changes {
		if reflect.DeepEqual(change, expectedMissingOutput) {
			foundMissingOutput = true
			break
		}
	}
	if !foundMissingOutput {
		t.Fatalf("expected missing output change, got %#v", report.Changes)
	}
	for _, change := range report.Changes {
		if change.Kind == ChangeKindRequiredToOptional && reflect.DeepEqual(change.Path, []string{"outputs", "required", "value"}) {
			t.Fatalf("unexpected required-to-optional for removed output: %#v", change)
		}
	}
}

func TestAnalyzeSkipsFunctionRequiredToOptionalWhenOutputsNil(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: &schema.ObjectTypeSpec{
					Required: []string{"value"},
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: nil,
			},
		},
	}

	report := Analyze("my-pkg", oldSchema, newSchema, nil, nil)
	expectedMissingOutput := Change{
		Category:    functionsCategory,
		Name:        "my-pkg:index:MyFunction",
		Path:        []string{"outputs", "value"},
		Kind:        ChangeKindMissingOutput,
		Severity:    SeverityWarn,
		Breaking:    true,
		Description: "missing output",
	}
	foundMissingOutput := false
	for _, change := range report.Changes {
		if reflect.DeepEqual(change, expectedMissingOutput) {
			foundMissingOutput = true
			break
		}
	}
	if !foundMissingOutput {
		t.Fatalf("expected missing output change, got %#v", report.Changes)
	}
	for _, change := range report.Changes {
		if change.Kind == ChangeKindRequiredToOptional && reflect.DeepEqual(change.Path, []string{"outputs", "required", "value"}) {
			t.Fatalf("unexpected required-to-optional for removed output: %#v", change)
		}
	}
}

func TestAnalyzeSkipsTypeRequiredToOptionalWhenPropertyRemoved(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Required: []string{"value"},
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
					Required:   []string{},
					Properties: map[string]schema.PropertySpec{},
				},
			},
		},
	}

	report := Analyze("my-pkg", oldSchema, newSchema, nil, nil)
	expectedMissingProperty := Change{
		Category:    typesCategory,
		Name:        "my-pkg:index:MyType",
		Path:        []string{"properties", "value"},
		Kind:        ChangeKindMissingProperty,
		Severity:    SeverityWarn,
		Breaking:    true,
		Description: "missing",
	}
	foundMissingProperty := false
	for _, change := range report.Changes {
		if reflect.DeepEqual(change, expectedMissingProperty) {
			foundMissingProperty = true
			break
		}
	}
	if !foundMissingProperty {
		t.Fatalf("expected missing property change, got %#v", report.Changes)
	}
	for _, change := range report.Changes {
		if change.Kind == ChangeKindRequiredToOptional && reflect.DeepEqual(change.Path, []string{"required", "value"}) {
			t.Fatalf("unexpected required-to-optional for removed type property: %#v", change)
		}
	}
}

func TestAnalyzeResolvesResourceTokenRemapFromMetadata(t *testing.T) {
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
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v2:Widget","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}]}}}}`)

	report := Analyze("my-pkg", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.NewResources, []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected new resources: got %#v want %#v", got, want)
	}
	if got, want := report.Changes, []Change{
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index/v1:Widget",
			Kind:        ChangeKindTokenRemapped,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: `token remapped: migrate from "my-pkg:index/v1:Widget" to "my-pkg:index/v2:Widget"`,
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeResolved,
				Lookup:     "ResolveToken",
				Token:      "my-pkg:index/v2:Widget",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected remap changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeRetainedInCodegenAliasKeepsCanonicalNewResourceAndRemapSignal(t *testing.T) {
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
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"aws_s3_bucket_acl":{"current":"aws:s3/bucketAclV2:BucketAclV2"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"aws_s3_bucket_acl":{"current":"aws:s3/bucketAcl:BucketAcl","past":[{"name":"aws:s3/bucketAclV2:BucketAclV2","inCodegen":true,"majorVersion":7}]}}}}`)

	report := Analyze("aws", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.NewResources, []string{"s3/bucketAcl.BucketAcl"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected new resources: got %#v want %#v", got, want)
	}
	if got, want := report.Changes, []Change{
		{
			Category:    resourcesCategory,
			Name:        "aws:s3/bucketAcl:BucketAcl",
			Kind:        ChangeKindNewResource,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: "added",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeResolved,
				Lookup:     "ResolveNewToken",
				Token:      "aws:s3/bucketAclV2:BucketAclV2",
				Candidates: []string{},
			},
		},
		{
			Category:    resourcesCategory,
			Name:        "aws:s3/bucketAclV2:BucketAclV2",
			Kind:        ChangeKindTokenRemapped,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: `token deprecated: prefer "aws:s3/bucketAcl:BucketAcl" instead of "aws:s3/bucketAclV2:BucketAclV2"`,
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeResolved,
				Lookup:     "ResolveNewToken",
				Token:      "aws:s3/bucketAclV2:BucketAclV2",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected retained alias resource changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeRetainedInCodegenAliasKeepsCanonicalNewFunctionAndRemapSignal(t *testing.T) {
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
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"aws_s3_bucket_acl":{"current":"aws:s3/getBucketAclV2:getBucketAclV2"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"aws_s3_bucket_acl":{"current":"aws:s3/getBucketAcl:getBucketAcl","past":[{"name":"aws:s3/getBucketAclV2:getBucketAclV2","inCodegen":true,"majorVersion":7}]}}}}`)

	report := Analyze("aws", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.NewFunctions, []string{"s3/getBucketAcl.getBucketAcl"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected new functions: got %#v want %#v", got, want)
	}
	if got, want := report.Changes, []Change{
		{
			Category:    functionsCategory,
			Name:        "aws:s3/getBucketAcl:getBucketAcl",
			Kind:        ChangeKindNewFunction,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: "added",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeResolved,
				Lookup:     "ResolveNewToken",
				Token:      "aws:s3/getBucketAclV2:getBucketAclV2",
				Candidates: []string{},
			},
		},
		{
			Category:    functionsCategory,
			Name:        "aws:s3/getBucketAclV2:getBucketAclV2",
			Kind:        ChangeKindTokenRemapped,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: `token deprecated: prefer "aws:s3/getBucketAcl:getBucketAcl" instead of "aws:s3/getBucketAclV2:getBucketAclV2"`,
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeResolved,
				Lookup:     "ResolveNewToken",
				Token:      "aws:s3/getBucketAclV2:getBucketAclV2",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected retained alias function changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeMarksAmbiguousTokenLookupOnMissingResource(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{},
	}
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{
		"tf_widget_a":{"current":"my-pkg:index/v2:WidgetA","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}]},
		"tf_widget_b":{"current":"my-pkg:index/v2:WidgetB","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}]}
	}}}`)

	report := Analyze("my-pkg", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.Changes, []Change{
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index/v1:Widget",
			Kind:        ChangeKindMissingResource,
			Severity:    SeverityDanger,
			Breaking:    true,
			Description: `missing (lookup ambiguous: my-pkg:index/v2:WidgetA, my-pkg:index/v2:WidgetB)`,
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeAmbiguous,
				Lookup:     "ResolveToken",
				Candidates: []string{"my-pkg:index/v2:WidgetA", "my-pkg:index/v2:WidgetB"},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ambiguous lookup changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeMarksNoneTokenLookupOnMissingResource(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{},
	}
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"resources":{"tf_other":{"current":"my-pkg:index/v2:Other"}}}}`)

	report := Analyze("my-pkg", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.Changes, []Change{
		{
			Category:    resourcesCategory,
			Name:        "my-pkg:index/v1:Widget",
			Kind:        ChangeKindMissingResource,
			Severity:    SeverityDanger,
			Breaking:    true,
			Description: "missing",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeNone,
				Lookup:     "ResolveToken",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected no-evidence lookup changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeResolvesFunctionTokenRemapFromMetadata(t *testing.T) {
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
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_widget":{"current":"my-pkg:index/v1:getWidget"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_widget":{"current":"my-pkg:index/v2:getWidget","past":[{"name":"my-pkg:index/v1:getWidget","inCodegen":false,"majorVersion":1}]}}}}`)

	report := Analyze("my-pkg", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.NewFunctions, []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected new functions: got %#v want %#v", got, want)
	}
	for _, change := range report.Changes {
		if change.Kind == ChangeKindMissingFunction || change.Kind == ChangeKindNewFunction {
			t.Fatalf("unexpected add/remove function diagnostic for resolved remap: %#v", change)
		}
	}
	if got, want := report.Changes, []Change{
		{
			Category:    functionsCategory,
			Name:        "my-pkg:index/v1:getWidget",
			Kind:        ChangeKindTokenRemapped,
			Severity:    SeverityInfo,
			Breaking:    false,
			Description: `token remapped: migrate from "my-pkg:index/v1:getWidget" to "my-pkg:index/v2:getWidget"`,
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeResolved,
				Lookup:     "ResolveToken",
				Token:      "my-pkg:index/v2:getWidget",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected function remap changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeMarksAmbiguousTokenLookupOnMissingFunction(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index/v1:getWidget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{},
	}
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_widget":{"current":"my-pkg:index/v1:getWidget"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{
		"tf_widget_a":{"current":"my-pkg:index/v2:getWidgetA","past":[{"name":"my-pkg:index/v1:getWidget","inCodegen":false,"majorVersion":1}]},
		"tf_widget_b":{"current":"my-pkg:index/v2:getWidgetB","past":[{"name":"my-pkg:index/v1:getWidget","inCodegen":false,"majorVersion":1}]}
	}}}`)

	report := Analyze("my-pkg", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.Changes, []Change{
		{
			Category:    functionsCategory,
			Name:        "my-pkg:index/v1:getWidget",
			Kind:        ChangeKindMissingFunction,
			Severity:    SeverityDanger,
			Breaking:    true,
			Description: `missing (lookup ambiguous: my-pkg:index/v2:getWidgetA, my-pkg:index/v2:getWidgetB)`,
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeAmbiguous,
				Lookup:     "ResolveToken",
				Candidates: []string{"my-pkg:index/v2:getWidgetA", "my-pkg:index/v2:getWidgetB"},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ambiguous function lookup changes: got %#v want %#v", got, want)
	}
}

func TestAnalyzeMarksNoneTokenLookupOnMissingFunction(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index/v1:getWidget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{},
	}
	oldMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_widget":{"current":"my-pkg:index/v1:getWidget"}}}}`)
	newMetadata := mustParseMetadata(t, `{"auto-aliasing":{"version":1,"datasources":{"tf_other":{"current":"my-pkg:index/v2:getOther"}}}}`)

	report := Analyze("my-pkg", oldSchema, newSchema, oldMetadata, newMetadata)
	if got, want := report.Changes, []Change{
		{
			Category:    functionsCategory,
			Name:        "my-pkg:index/v1:getWidget",
			Kind:        ChangeKindMissingFunction,
			Severity:    SeverityDanger,
			Breaking:    true,
			Description: "missing",
			Reason: &NormalizationReason{
				Outcome:    NormalizationOutcomeNone,
				Lookup:     "ResolveToken",
				Candidates: []string{},
			},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected no-evidence function lookup changes: got %#v want %#v", got, want)
	}
}

func mustParseMetadata(t testing.TB, metadata string) *normalize.MetadataEnvelope {
	t.Helper()
	parsed, err := normalize.ParseMetadata([]byte(metadata))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	return parsed
}
