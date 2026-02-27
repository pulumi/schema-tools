package compare

import (
	"reflect"
	"slices"
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

	if got, want := report.NewResources, []string{"index.NewResource", "module.AnotherResource"}; !slices.Equal(got, want) {
		t.Fatalf("unexpected new resources: got %v want %v", got, want)
	}

	if got, want := report.NewFunctions, []string{"index.newFunction", "module.otherFunction"}; !slices.Equal(got, want) {
		t.Fatalf("unexpected new functions: got %v want %v", got, want)
	}

	if got, want := report.Changes, []Change{
		{Category: functionsCategory, Name: "my-pkg:index:newFunction", Kind: ChangeKindNewFunction, Severity: SeverityInfo, Breaking: false, Description: "added"},
		{Category: functionsCategory, Name: "my-pkg:module:otherFunction", Kind: ChangeKindNewFunction, Severity: SeverityInfo, Breaking: false, Description: "added"},
		{Category: resourcesCategory, Name: "my-pkg:index:NewResource", Kind: ChangeKindNewResource, Severity: SeverityInfo, Breaking: false, Description: "added"},
		{Category: resourcesCategory, Name: "my-pkg:module:AnotherResource", Kind: ChangeKindNewResource, Severity: SeverityInfo, Breaking: false, Description: "added"},
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

	report := Analyze("my-pkg", oldSchema, newSchema)

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

	report := Analyze("my-pkg", oldSchema, newSchema)
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

	report := Analyze("my-pkg", oldSchema, newSchema)
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

	report := Analyze("my-pkg", oldSchema, newSchema)
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
