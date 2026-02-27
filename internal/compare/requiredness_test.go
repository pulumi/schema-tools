package compare

import (
	"reflect"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestAnalyzeNoBreakingChanges(t *testing.T) {
	oldSchema := simpleResourceSchema(simpleResource([]string{"value"}, nil))
	newSchema := simpleResourceSchema(simpleResource([]string{"value"}, nil))

	report := Analyze("my-pkg", oldSchema, newSchema)
	if len(report.Changes) != 0 {
		t.Fatalf("expected no changes, got %#v", report.Changes)
	}
	if len(report.NewResources) != 0 {
		t.Fatalf("expected no new resources, got %#v", report.NewResources)
	}
	if len(report.NewFunctions) != 0 {
		t.Fatalf("expected no new functions, got %#v", report.NewFunctions)
	}
}

func TestAnalyzeResourceRequiredness(t *testing.T) {
	tests := []struct {
		name              string
		oldRequired       []string
		oldRequiredInputs []string
		newRequired       []string
		newRequiredInputs []string
		expected          []Change
	}{
		{
			name:        "required output becomes optional",
			oldRequired: []string{"value"},
			expected: []Change{{
				Category:    resourcesCategory,
				Name:        "my-pkg:index:MyResource",
				Path:        []string{"required", "value"},
				Kind:        ChangeKindRequiredToOptional,
				Severity:    SeverityInfo,
				Breaking:    true,
				Description: changedToOptional("property"),
			}},
		},
		{
			name:              "optional input becomes required",
			newRequiredInputs: []string{"list"},
			expected: []Change{{
				Category:    resourcesCategory,
				Name:        "my-pkg:index:MyResource",
				Path:        []string{"required inputs", "list"},
				Kind:        ChangeKindOptionalToRequired,
				Severity:    SeverityInfo,
				Breaking:    true,
				Description: changedToRequired("input"),
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldSchema := simpleResourceSchema(simpleResource(tt.oldRequired, tt.oldRequiredInputs))
			newSchema := simpleResourceSchema(simpleResource(tt.newRequired, tt.newRequiredInputs))

			report := Analyze("my-pkg", oldSchema, newSchema)
			assertExactChanges(t, report.Changes, tt.expected)
		})
	}
}

func TestAnalyzeFunctionRequiredness(t *testing.T) {
	oldSchema := simpleFunctionSchema(schema.FunctionSpec{
		Outputs: &schema.ObjectTypeSpec{
			Required: []string{"value"},
			Properties: map[string]schema.PropertySpec{
				"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
		},
		Inputs: &schema.ObjectTypeSpec{
			Properties: map[string]schema.PropertySpec{
				"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
		},
	})
	newSchema := simpleFunctionSchema(schema.FunctionSpec{
		Outputs: &schema.ObjectTypeSpec{
			Required: []string{},
			Properties: map[string]schema.PropertySpec{
				"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
		},
		Inputs: &schema.ObjectTypeSpec{
			Required: []string{"value"},
			Properties: map[string]schema.PropertySpec{
				"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
		},
	})

	report := Analyze("my-pkg", oldSchema, newSchema)
	assertExactChanges(t, report.Changes, []Change{
		{
			Category:    functionsCategory,
			Name:        "my-pkg:index:MyFunction",
			Path:        []string{"inputs", "required", "value"},
			Kind:        ChangeKindOptionalToRequired,
			Severity:    SeverityInfo,
			Breaking:    true,
			Description: changedToRequired("input"),
		},
		{
			Category:    functionsCategory,
			Name:        "my-pkg:index:MyFunction",
			Path:        []string{"outputs", "required", "value"},
			Kind:        ChangeKindRequiredToOptional,
			Severity:    SeverityInfo,
			Breaking:    true,
			Description: changedToOptional("property"),
		},
	})
}

func TestAnalyzeTypeRequiredness(t *testing.T) {
	oldSchema := simpleTypeSchema(schema.ComplexTypeSpec{
		ObjectTypeSpec: schema.ObjectTypeSpec{
			Properties: map[string]schema.PropertySpec{
				"list":  {TypeSpec: schema.TypeSpec{Type: "array"}},
				"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
			Required: []string{"value", "list"},
		},
	})
	newSchema := simpleTypeSchema(schema.ComplexTypeSpec{
		ObjectTypeSpec: schema.ObjectTypeSpec{
			Properties: map[string]schema.PropertySpec{
				"list":  {TypeSpec: schema.TypeSpec{Type: "array"}},
				"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			},
			Required: []string{"value", "new-required"},
		},
	})

	report := Analyze("my-pkg", oldSchema, newSchema)
	assertExactChanges(t, report.Changes, []Change{
		{
			Category:    typesCategory,
			Name:        "my-pkg:index:MyType",
			Path:        []string{"required", "list"},
			Kind:        ChangeKindRequiredToOptional,
			Severity:    SeverityInfo,
			Breaking:    true,
			Description: changedToOptional("property"),
		},
		{
			Category:    typesCategory,
			Name:        "my-pkg:index:MyType",
			Path:        []string{"required", "new-required"},
			Kind:        ChangeKindOptionalToRequired,
			Severity:    SeverityInfo,
			Breaking:    true,
			Description: changedToRequired("property"),
		},
	})
}

func TestAnalyzeRequirednessSkipsRemovedResourceProperty(t *testing.T) {
	old := simpleResource([]string{"field1"}, nil)
	old.Properties["field1"] = schema.PropertySpec{TypeSpec: schema.TypeSpec{Type: "string"}}
	oldSchema := simpleResourceSchema(old)
	newSchema := simpleResourceSchema(simpleResource(nil, nil))

	report := Analyze("my-pkg", oldSchema, newSchema)
	expectedMissingOutput := Change{
		Category:    resourcesCategory,
		Name:        "my-pkg:index:MyResource",
		Path:        []string{"properties", "field1"},
		Kind:        ChangeKindMissingOutput,
		Severity:    SeverityWarn,
		Breaking:    true,
		Description: `missing output "field1"`,
	}
	assertContainsChange(t, report.Changes, expectedMissingOutput)

	for _, change := range report.Changes {
		if change.Kind == ChangeKindRequiredToOptional && reflect.DeepEqual(change.Path, []string{"required", "field1"}) {
			t.Fatalf("unexpected required-to-optional for removed output property: %#v", change)
		}
	}
}

func assertExactChanges(t *testing.T, got, want []Change) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected changes: got %#v want %#v", got, want)
	}
}

func assertContainsChange(t *testing.T, changes []Change, want Change) {
	t.Helper()
	for _, change := range changes {
		if reflect.DeepEqual(change, want) {
			return
		}
	}
	t.Fatalf("expected change %#v in %#v", want, changes)
}

func simpleEmptySchema() schema.PackageSpec {
	return schema.PackageSpec{
		Name:    "my-pkg",
		Version: "v1.2.3",
	}
}

func simpleResource(required, requiredInputs []string) schema.ResourceSpec {
	props := func() map[string]schema.PropertySpec {
		return map[string]schema.PropertySpec{
			"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
			"list": {TypeSpec: schema.TypeSpec{
				Type:  "array",
				Items: &schema.TypeSpec{Type: "number"},
			}},
		}
	}
	r := schema.ResourceSpec{
		ObjectTypeSpec: schema.ObjectTypeSpec{
			Properties: props(),
			Required:   required,
		},
		InputProperties: props(),
		RequiredInputs:  requiredInputs,
	}
	return r
}

func simpleResourceSchema(r schema.ResourceSpec) schema.PackageSpec {
	p := simpleEmptySchema()
	p.Resources = map[string]schema.ResourceSpec{
		p.Name + ":index:MyResource": r,
	}
	return p
}

func simpleFunctionSchema(f schema.FunctionSpec) schema.PackageSpec {
	p := simpleEmptySchema()
	p.Functions = map[string]schema.FunctionSpec{
		p.Name + ":index:MyFunction": f,
	}
	return p
}

func simpleTypeSchema(t schema.ComplexTypeSpec) schema.PackageSpec {
	p := simpleEmptySchema()
	p.Types = map[string]schema.ComplexTypeSpec{
		p.Name + ":index:MyType": t,
	}
	return p
}
