package cmd

import (
	"bytes"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
	comparepkg "github.com/pulumi/schema-tools/pkg/compare"
	"github.com/stretchr/testify/assert"
)

func TestCompareSchemasNoBreakingChanges(t *testing.T) {
	oldSchema := simpleResourceSchema(simpleResource([]string{"value"}, nil))
	newSchema := simpleResourceSchema(simpleResource([]string{"value"}, nil))

	var out bytes.Buffer
	compareSchemas(&out, "my-pkg", oldSchema, newSchema, 10_000)

	assert.Contains(t, out.String(), "### Does the PR have any schema changes?")
	assert.Contains(t, out.String(), "Looking good! No breaking changes found.")
}

func TestRenderCompareOutputModes(t *testing.T) {
	result := comparepkg.CompareResult{
		Summary:         []comparepkg.SummaryItem{{Category: "missing-input", Count: 1, Entries: []string{"e1"}}},
		BreakingChanges: []string{"line-1"},
		NewResources:    []string{"r1"},
		NewFunctions:    []string{"f1"},
	}

	t.Run("json", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), `"breaking_changes": [`)
		assert.Contains(t, out.String(), `"line-1"`)
	})

	t.Run("summary text", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, false, true)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "Summary by category:")
		assert.Contains(t, out.String(), "- missing-input: 1")
		assert.NotContains(t, out.String(), "e1")
	})

	t.Run("json summary", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, true)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), `"summary": [`)
		assert.NotContains(t, out.String(), `"line-1"`)
		assert.NotContains(t, out.String(), `"r1"`)
	})
}

func TestBreakingResourceRequired(t *testing.T) {
	tests := []breakingTestCase{
		{}, // No required => no breaking

		{ // No change => no breaking
			OldRequired: []string{"value"},
			NewRequired: []string{"value"},
		},
		{ // Making an output optional is breaking
			OldRequired: []string{"value"},
			ExpectedOutput: expectedRes(func(n *diagtree.Node) {
				n.Label("required").Value("value").
					SetDescription(diagtree.Info, "property is no longer Required")
			}),
		},
		{ // Making an output required is not breaking
			NewRequired: []string{"value"},
		},
		{ // But making an input required is breaking
			NewRequiredInputs: []string{"list"},
			ExpectedOutput: expectedRes(func(n *diagtree.Node) {
				n.Label("required inputs").Value("list").
					SetDescription(diagtree.Info, "input has changed to Required")
			}),
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
	changes := *breakingChanges(oldSchema, newSchema)
	assert.Equal(t, expectedRes(func(n *diagtree.Node) {
		n.Label("properties").Value("field1").
			SetDescription(diagtree.Warn, `missing output "field1"`)
	}), changes)

}

func TestBreakingFunctionRequired(t *testing.T) {
	tests := []breakingTestCase{
		{}, // No required => no breaking

		{ // No change => no breaking
			OldRequired: []string{"value"},
			NewRequired: []string{"value"},
		},
		{ // Making an output optional is breaking
			OldRequired: []string{"value"},
			ExpectedOutput: expectedFunc(func(n *diagtree.Node) {
				n.Label("outputs").Label("required").Value("value").SetDescription(diagtree.Info,
					"property is no longer Required")
			}),
		},
		{ // Making an output required is not breaking
			NewRequired: []string{"value"},
		},
		{ // But making an input required is breaking
			OldRequiredInputs: []string{},
			NewRequiredInputs: []string{"list"},
			ExpectedOutput: expectedFunc(func(n *diagtree.Node) {
				n.Label("inputs").Label("required").Value("list").SetDescription(diagtree.Info,
					"input has changed to Required")
			}),
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
			ExpectedOutput: expectedTyp(func(n *diagtree.Node) {
				n.Label("required").Value("list").SetDescription(diagtree.Info,
					"property has changed to Required")
			}),
		},
		{ // Removing a requirement is breaking
			OldRequired: []string{"value", "list"},
			NewRequired: []string{"value"},
			ExpectedOutput: expectedTyp(func(n *diagtree.Node) {
				n.Label("required").Value("list").SetDescription(diagtree.Info,
					"property is no longer Required")
			}),
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

func expectedFunc(f func(*diagtree.Node)) diagtree.Node {
	expected := new(diagtree.Node)
	f(expected.Label("Functions").Value("my-pkg:index:MyFunction"))
	return *expected
}

func expectedRes(f func(*diagtree.Node)) diagtree.Node {
	expected := new(diagtree.Node)
	f(expected.Label("Resources").Value("my-pkg:index:MyResource"))
	return *expected
}

func expectedTyp(f func(*diagtree.Node)) diagtree.Node {
	expected := new(diagtree.Node)
	f(expected.Label("Types").Value("my-pkg:index:MyType"))
	return *expected
}

type breakingTestCase struct {
	OldRequired       []string
	OldRequiredInputs []string
	NewRequired       []string
	NewRequiredInputs []string
	ExpectedOutput    diagtree.Node
}

func testBreakingRequired(
	t *testing.T, tests []breakingTestCase,
	newT func(required, requiredInput []string) schema.PackageSpec,
) {
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			oldSchema := newT(tt.OldRequired, tt.OldRequiredInputs)
			newSchema := newT(tt.NewRequired, tt.NewRequiredInputs)

			violations := breakingChanges(oldSchema, newSchema)

			expected, actual := new(bytes.Buffer), new(bytes.Buffer)

			tt.ExpectedOutput.Display(expected, 10_000)
			violations.Display(actual, 10_000)

			assert.Equal(t, expected.String(), actual.String())
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
		p.Name + ":index:MyType": t,
	}
	return p
}
