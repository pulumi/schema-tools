package cmd

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
)

func TestBreakingResourceRequired(t *testing.T) {
	tests := []breakingTestCase{
		{}, // No required => no breaking

		{ // No change => no breaking
			OldRequired: []string{"value"},
			NewRequired: []string{"value"},
		},
		{ // Making an output optional is breaking
			OldRequired:    []string{"value"},
			ExpectedOutput: []string{`Resource "my-pkg:index:MyResource" property "value" is no longer Required`},
		},
		{ // Making an output required is not breaking
			NewRequired: []string{"value"},
		},
		{ // But making an input required is breaking
			NewRequiredInputs: []string{"list"},
			ExpectedOutput: []string{
				`Resource "my-pkg:index:MyResource" input "list" has changed to Required`,
			},
		},
		{ // Making an input optional is not breaking
			OldRequiredInputs: []string{"list"},
		},
	}

	testBreakingRequired(t, tests, func(required, requiredInputs []string) schema.PackageSpec {
		return simpleResourceSchema(simpleResource(required, requiredInputs))
	})
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

func TestRemovedProperty(t *testing.T) {
	old := simpleResource([]string{"field1"}, nil)
	old.Properties["field1"] = schema.PropertySpec{TypeSpec: schema.TypeSpec{Type: "string"}}
	oldSchema := simpleResourceSchema(old)
	newSchema := simpleResourceSchema(simpleResource(nil, nil))
	changes := breakingChanges(oldSchema, newSchema)
	assert.Equal(t, []string{
		"Resource \"my-pkg:index:MyResource\" missing output \"field1\"",
	}, changes)

}

func TestBreakingFunctionRequired(t *testing.T) {
	tests := []breakingTestCase{
		{}, // No required => no breaking

		{ // No change => no breaking
			OldRequired: []string{"value"},
			NewRequired: []string{"value"},
		},
		{ // Making an output optional is breaking
			OldRequired:    []string{"value"},
			ExpectedOutput: []string{`Function "my-pkg:index:MyFunction" property "value" is no longer Required`},
		},
		{ // Making an output required is not breaking
			NewRequired: []string{"value"},
		},
		{ // But making an input required is breaking
			OldRequiredInputs: []string{},
			NewRequiredInputs: []string{"list"},
			ExpectedOutput: []string{
				`Function "my-pkg:index:MyFunction" input "list" has changed to Required`,
			},
		},
		{ // Making an input optional is not breaking
			OldRequiredInputs: []string{"list"},
		},
	}

	testBreakingRequired(t, tests, func(required, requiredInputs []string) schema.PackageSpec {
		f := schema.FunctionSpec{
			Outputs: &schema.ObjectTypeSpec{
				Required: required,
				Properties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
			Inputs: &schema.ObjectTypeSpec{
				Required: requiredInputs,
				Properties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}

		return simpleFunctionSchema(f)
	})
}

func TestBreakingTypeRequired(t *testing.T) {
	tests := []breakingTestCase{
		{}, // No required => no breaking

		{ // No change => no breaking
			OldRequired: []string{"value"},
			NewRequired: []string{"value"},
		},
		{ // Adding a requirement is breaking
			OldRequired: []string{"value"},
			NewRequired: []string{"value", "list"},
			ExpectedOutput: []string{
				`Type "my-pkg:index:MyResource" property "list" has changed to Required`,
			},
		},
		{ // Removing a requirement is breaking
			OldRequired: []string{"value", "list"},
			NewRequired: []string{"value"},
			ExpectedOutput: []string{
				`Type "my-pkg:index:MyResource" property "list" is no longer Required`,
			},
		},
	}

	testBreakingRequired(t, tests, func(required, _ []string) schema.PackageSpec {
		t := schema.ComplexTypeSpec{
			ObjectTypeSpec: schema.ObjectTypeSpec{
				Properties: map[string]schema.PropertySpec{
					"list": {TypeSpec: schema.TypeSpec{Type: "array"}},
				},
				Required: required,
			},
		}
		return simpleTypeSchema(t)
	})
}

type breakingTestCase struct {
	OldRequired       []string
	OldRequiredInputs []string
	NewRequired       []string
	NewRequiredInputs []string
	ExpectedOutput    []string
}

func testBreakingRequired(
	t *testing.T, tests []breakingTestCase,
	newT func(required, requiredInput []string) schema.PackageSpec,
) {
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			old := newT(tt.OldRequired, tt.OldRequiredInputs)
			new := newT(tt.NewRequired, tt.NewRequiredInputs)

			violations := breakingChanges(old, new)

			assert.Equal(t, tt.ExpectedOutput, violations)
		})
	}
}

func simpleEmptySchema() schema.PackageSpec {
	return schema.PackageSpec{
		Name:    "my-pkg",
		Version: "v1.2.3",
	}
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
		p.Name + ":index:MyResource": t,
	}
	return p
}
