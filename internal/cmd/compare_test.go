package cmd

import (
	"bytes"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
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
	changes := *breakingChanges(oldSchema, newSchema, newDiffFilter())
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

			violations := breakingChanges(oldSchema, newSchema, newDiffFilter())

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

func TestLocalTypeName(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{
			name:     "standard local type ref",
			ref:      "#/types/pkg:module:TypeName",
			expected: "pkg:module:TypeName",
		},
		{
			name:     "local type ref with index module",
			ref:      "#/types/aws:index:Tag",
			expected: "aws:index:Tag",
		},
		{
			name:     "empty string",
			ref:      "",
			expected: "",
		},
		{
			name:     "non-type ref",
			ref:      "#/resources/pkg:module:Resource",
			expected: "",
		},
		{
			name:     "external ref",
			ref:      "pulumi.json#/Archive",
			expected: "",
		},
		{
			name:     "partial marker",
			ref:      "#/type/pkg:module:TypeName",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := localTypeName(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIntToNumberCategory(t *testing.T) {
	t.Run("maps input category", func(t *testing.T) {
		assert.Equal(t, diffTypeChangedIntToNumberInput, intToNumberCategory(diffTypeChangedInput))
	})

	t.Run("maps output category", func(t *testing.T) {
		assert.Equal(t, diffTypeChangedIntToNumberOutput, intToNumberCategory(diffTypeChangedOutput))
	})

	t.Run("returns other categories unchanged", func(t *testing.T) {
		assert.Equal(t, diffMissingResource, intToNumberCategory(diffMissingResource))
		assert.Equal(t, diffOptionalToRequiredInput, intToNumberCategory(diffOptionalToRequiredInput))
	})
}

func TestTypeUsageMethods(t *testing.T) {
	t.Run("typeChangeCategory", func(t *testing.T) {
		// Input takes precedence
		assert.Equal(t, diffTypeChangedInput, typeUsage{input: true, output: true}.typeChangeCategory())
		assert.Equal(t, diffTypeChangedInput, typeUsage{input: true, output: false}.typeChangeCategory())
		assert.Equal(t, diffTypeChangedOutput, typeUsage{input: false, output: true}.typeChangeCategory())
		assert.Equal(t, diffTypeChangedOutput, typeUsage{input: false, output: false}.typeChangeCategory())
	})

	t.Run("optionalToRequiredCategory", func(t *testing.T) {
		assert.Equal(t, diffOptionalToRequiredInput, typeUsage{input: true, output: true}.optionalToRequiredCategory())
		assert.Equal(t, diffOptionalToRequiredInput, typeUsage{input: true, output: false}.optionalToRequiredCategory())
		assert.Equal(t, diffOptionalToRequiredOutput, typeUsage{input: false, output: true}.optionalToRequiredCategory())
		assert.Equal(t, diffOptionalToRequiredOutput, typeUsage{input: false, output: false}.optionalToRequiredCategory())
	})

	t.Run("requiredToOptionalCategory", func(t *testing.T) {
		assert.Equal(t, diffRequiredToOptionalInput, typeUsage{input: true, output: true}.requiredToOptionalCategory())
		assert.Equal(t, diffRequiredToOptionalInput, typeUsage{input: true, output: false}.requiredToOptionalCategory())
		assert.Equal(t, diffRequiredToOptionalOutput, typeUsage{input: false, output: true}.requiredToOptionalCategory())
		assert.Equal(t, diffRequiredToOptionalOutput, typeUsage{input: false, output: false}.requiredToOptionalCategory())
	})
}

func TestDiffFilterSummaryLines(t *testing.T) {
	t.Run("empty filter", func(t *testing.T) {
		f := newDiffFilter()
		lines := f.summaryLines()
		assert.Empty(t, lines)
		assert.False(t, f.hasCounts())
	})

	t.Run("single category", func(t *testing.T) {
		f := newDiffFilter()
		f.counts[diffMissingResource] = 3
		lines := f.summaryLines()
		assert.Equal(t, []string{"missing-resource: 3"}, lines)
		assert.True(t, f.hasCounts())
	})

	t.Run("multiple categories in order", func(t *testing.T) {
		f := newDiffFilter()
		f.counts[diffTypeChangedOutput] = 2
		f.counts[diffMissingResource] = 1
		f.counts[diffSignatureChanged] = 5
		lines := f.summaryLines()
		// Should be in categoryOrder order, not insertion order
		assert.Equal(t, []string{
			"missing-resource: 1",
			"type-changed-output: 2",
			"signature-changed: 5",
		}, lines)
	})

	t.Run("zero counts excluded", func(t *testing.T) {
		f := newDiffFilter()
		f.counts[diffMissingResource] = 0
		f.counts[diffMissingFunction] = 2
		lines := f.summaryLines()
		assert.Equal(t, []string{"missing-function: 2"}, lines)
	})
}

func TestBuildTypeUsage(t *testing.T) {
	t.Run("empty schema", func(t *testing.T) {
		spec := simpleEmptySchema()
		usage := buildTypeUsage(spec)
		assert.Empty(t, usage)
	})

	t.Run("type used in resource input", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		spec.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}},
				},
			},
		}
		usage := buildTypeUsage(spec)
		assert.True(t, usage["my-pkg:index:MyType"].input)
		assert.False(t, usage["my-pkg:index:MyType"].output)
	})

	t.Run("type used in resource output", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		spec.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}},
					},
				},
			},
		}
		usage := buildTypeUsage(spec)
		assert.False(t, usage["my-pkg:index:MyType"].input)
		assert.True(t, usage["my-pkg:index:MyType"].output)
	})

	t.Run("type used in both input and output", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		spec.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}},
					},
				},
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}},
				},
			},
		}
		usage := buildTypeUsage(spec)
		assert.True(t, usage["my-pkg:index:MyType"].input)
		assert.True(t, usage["my-pkg:index:MyType"].output)
	})

	t.Run("nested type in array", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:NestedType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		spec.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"items": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/my-pkg:index:NestedType"},
					}},
				},
			},
		}
		usage := buildTypeUsage(spec)
		assert.True(t, usage["my-pkg:index:NestedType"].input)
	})

	t.Run("nested type in map", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:NestedType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		spec.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"mapping": {TypeSpec: schema.TypeSpec{
							Type:                 "object",
							AdditionalProperties: &schema.TypeSpec{Ref: "#/types/my-pkg:index:NestedType"},
						}},
					},
				},
			},
		}
		usage := buildTypeUsage(spec)
		assert.True(t, usage["my-pkg:index:NestedType"].output)
	})

	t.Run("function inputs and outputs", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:InputType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
			"my-pkg:index:OutputType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		spec.Functions = map[string]schema.FunctionSpec{
			"my-pkg:index:myFunction": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"arg": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:InputType"}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:OutputType"}},
					},
				},
			},
		}
		usage := buildTypeUsage(spec)
		assert.True(t, usage["my-pkg:index:InputType"].input)
		assert.False(t, usage["my-pkg:index:InputType"].output)
		assert.False(t, usage["my-pkg:index:OutputType"].input)
		assert.True(t, usage["my-pkg:index:OutputType"].output)
	})

	t.Run("recursive type does not infinite loop", func(t *testing.T) {
		spec := simpleEmptySchema()
		spec.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:RecursiveType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"child": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:RecursiveType"}},
					},
				},
			},
		}
		spec.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tree": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:RecursiveType"}},
				},
			},
		}
		// Should not hang
		usage := buildTypeUsage(spec)
		assert.True(t, usage["my-pkg:index:RecursiveType"].input)
	})
}

func TestMergeTypeUsage(t *testing.T) {
	t.Run("merge into nil", func(t *testing.T) {
		src := map[string]typeUsage{
			"type1": {input: true, output: false},
		}
		result := mergeTypeUsage(nil, src)
		assert.Equal(t, src, result)
	})

	t.Run("merge empty into empty", func(t *testing.T) {
		result := mergeTypeUsage(map[string]typeUsage{}, map[string]typeUsage{})
		assert.Empty(t, result)
	})

	t.Run("merge combines flags", func(t *testing.T) {
		dst := map[string]typeUsage{
			"type1": {input: true, output: false},
		}
		src := map[string]typeUsage{
			"type1": {input: false, output: true},
		}
		result := mergeTypeUsage(dst, src)
		assert.True(t, result["type1"].input)
		assert.True(t, result["type1"].output)
	})

	t.Run("merge adds new types", func(t *testing.T) {
		dst := map[string]typeUsage{
			"type1": {input: true, output: false},
		}
		src := map[string]typeUsage{
			"type2": {input: false, output: true},
		}
		result := mergeTypeUsage(dst, src)
		assert.True(t, result["type1"].input)
		assert.True(t, result["type2"].output)
	})
}

func TestCategoryCounting(t *testing.T) {
	t.Run("counts missing resource", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {},
		}
		newSchema := simpleEmptySchema()

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMissingResource])
	})

	t.Run("counts missing function", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Functions = map[string]schema.FunctionSpec{
			"my-pkg:index:myFunc": {},
		}
		newSchema := simpleEmptySchema()

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMissingFunction])
	})

	t.Run("counts missing type", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {},
		}
		newSchema := simpleEmptySchema()

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMissingType])
	})

	t.Run("counts missing input", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"prop1": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMissingInput])
	})

	t.Run("counts type changed input", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"prop1": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"prop1": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedInput])
	})

	t.Run("counts optional to required input", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"prop1": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				RequiredInputs: []string{},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"prop1": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				RequiredInputs: []string{"prop1"},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffOptionalToRequiredInput])
	})

	t.Run("multiple categories counted correctly", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:Resource1": {},
			"my-pkg:index:Resource2": {},
		}
		oldSchema.Functions = map[string]schema.FunctionSpec{
			"my-pkg:index:func1": {},
		}
		newSchema := simpleEmptySchema()

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 2, filter.counts[diffMissingResource])
		assert.Equal(t, 1, filter.counts[diffMissingFunction])
	})

	t.Run("counts integer to number input change separately", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"count": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"count": {TypeSpec: schema.TypeSpec{Type: "number"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedIntToNumberInput])
		assert.Equal(t, 0, filter.counts[diffTypeChangedInput])
	})

	t.Run("counts integer to number output change separately", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"total": {TypeSpec: schema.TypeSpec{Type: "integer"}},
					},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"total": {TypeSpec: schema.TypeSpec{Type: "number"}},
					},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedIntToNumberOutput])
		assert.Equal(t, 0, filter.counts[diffTypeChangedOutput])
	})

	t.Run("other type changes still use generic category", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedInput])
		assert.Equal(t, 0, filter.counts[diffTypeChangedIntToNumberInput])
	})

	t.Run("number to integer uses generic category", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "number"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedInput])
		assert.Equal(t, 0, filter.counts[diffTypeChangedIntToNumberInput])
	})
}

func TestTypeIdentifier(t *testing.T) {
	t.Run("nil TypeSpec returns empty string", func(t *testing.T) {
		assert.Equal(t, "", typeIdentifier(nil))
	})

	t.Run("returns Ref when present", func(t *testing.T) {
		ts := &schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType", Type: "object"}
		assert.Equal(t, "#/types/my-pkg:index:MyType", typeIdentifier(ts))
	})

	t.Run("returns Type when Ref is empty", func(t *testing.T) {
		ts := &schema.TypeSpec{Type: "string"}
		assert.Equal(t, "string", typeIdentifier(ts))
	})

	t.Run("returns Type for primitive types", func(t *testing.T) {
		for _, typ := range []string{"string", "integer", "number", "boolean", "array", "object"} {
			ts := &schema.TypeSpec{Type: typ}
			assert.Equal(t, typ, typeIdentifier(ts))
		}
	})
}

func TestIsArrayType(t *testing.T) {
	t.Run("nil TypeSpec returns false", func(t *testing.T) {
		assert.False(t, isArrayType(nil))
	})

	t.Run("array type returns true", func(t *testing.T) {
		ts := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}}
		assert.True(t, isArrayType(ts))
	})

	t.Run("non-array types return false", func(t *testing.T) {
		for _, typ := range []string{"string", "integer", "number", "boolean", "object"} {
			ts := &schema.TypeSpec{Type: typ}
			assert.False(t, isArrayType(ts))
		}
	})
}

func TestIsObjectOrRef(t *testing.T) {
	t.Run("nil TypeSpec returns false", func(t *testing.T) {
		assert.False(t, isObjectOrRef(nil))
	})

	t.Run("object type returns true", func(t *testing.T) {
		ts := &schema.TypeSpec{Type: "object"}
		assert.True(t, isObjectOrRef(ts))
	})

	t.Run("ref type returns true", func(t *testing.T) {
		ts := &schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}
		assert.True(t, isObjectOrRef(ts))
	})

	t.Run("primitive types return false", func(t *testing.T) {
		for _, typ := range []string{"string", "integer", "number", "boolean", "array"} {
			ts := &schema.TypeSpec{Type: typ}
			assert.False(t, isObjectOrRef(ts))
		}
	})
}

func TestIsMaxItemsOneChange(t *testing.T) {
	t.Run("nil old returns false", func(t *testing.T) {
		new := &schema.TypeSpec{Type: "object"}
		assert.False(t, isMaxItemsOneChange(nil, new))
	})

	t.Run("nil new returns false", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "object"}
		assert.False(t, isMaxItemsOneChange(old, nil))
	})

	t.Run("object to array of objects returns true", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "object"}
		new := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "object"}}
		assert.True(t, isMaxItemsOneChange(old, new))
	})

	t.Run("ref to array of refs returns true", func(t *testing.T) {
		old := &schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}
		new := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}}
		assert.True(t, isMaxItemsOneChange(old, new))
	})

	t.Run("array of objects to object returns true", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "object"}}
		new := &schema.TypeSpec{Type: "object"}
		assert.True(t, isMaxItemsOneChange(old, new))
	})

	t.Run("array of refs to ref returns true", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}}
		new := &schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}
		assert.True(t, isMaxItemsOneChange(old, new))
	})

	t.Run("string to array of strings returns false", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "string"}
		new := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}}
		assert.False(t, isMaxItemsOneChange(old, new))
	})

	t.Run("array of strings to string returns false", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}}
		new := &schema.TypeSpec{Type: "string"}
		assert.False(t, isMaxItemsOneChange(old, new))
	})

	t.Run("object to object returns false", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "object"}
		new := &schema.TypeSpec{Type: "object"}
		assert.False(t, isMaxItemsOneChange(old, new))
	})

	t.Run("string to integer returns false", func(t *testing.T) {
		old := &schema.TypeSpec{Type: "string"}
		new := &schema.TypeSpec{Type: "integer"}
		assert.False(t, isMaxItemsOneChange(old, new))
	})
}

func TestPluralizationCandidates(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		assert.Nil(t, pluralizationCandidates(""))
	})

	t.Run("singular returns plural", func(t *testing.T) {
		candidates := pluralizationCandidates("tag")
		assert.Contains(t, candidates, "tags")
	})

	t.Run("plural returns singular", func(t *testing.T) {
		candidates := pluralizationCandidates("tags")
		assert.Contains(t, candidates, "tag")
	})

	t.Run("excludes original name", func(t *testing.T) {
		candidates := pluralizationCandidates("tag")
		assert.NotContains(t, candidates, "tag")
	})

	t.Run("handles irregular plurals", func(t *testing.T) {
		candidates := pluralizationCandidates("person")
		assert.Contains(t, candidates, "people")
	})

	t.Run("handles words that are same singular and plural", func(t *testing.T) {
		// "sheep" pluralizes to "sheep", so candidates should be empty
		candidates := pluralizationCandidates("sheep")
		assert.NotContains(t, candidates, "sheep")
	})
}

func TestPluralizationRenameCategory(t *testing.T) {
	t.Run("maps input category", func(t *testing.T) {
		assert.Equal(t, diffPluralizationRenameInput, pluralizationRenameCategory(diffTypeChangedInput))
	})

	t.Run("maps output category", func(t *testing.T) {
		assert.Equal(t, diffPluralizationRenameOutput, pluralizationRenameCategory(diffTypeChangedOutput))
	})

	t.Run("returns other categories unchanged", func(t *testing.T) {
		assert.Equal(t, diffMissingResource, pluralizationRenameCategory(diffMissingResource))
		assert.Equal(t, diffOptionalToRequiredInput, pluralizationRenameCategory(diffOptionalToRequiredInput))
	})
}

func TestPluralizationRenameDetection(t *testing.T) {
	t.Run("detects simple rename from singular to plural", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tag": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tags": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffPluralizationRenameInput])
		assert.Equal(t, 0, filter.counts[diffMissingInput])
	})

	t.Run("detects rename with max-items-one change (object to array of objects)", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:ConfigType"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"configs": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/my-pkg:index:ConfigType"},
					}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMaxItemsOneChanged])
		assert.Equal(t, 0, filter.counts[diffMissingInput])
		assert.Equal(t, 0, filter.counts[diffPluralizationRenameInput])
	})

	t.Run("detects rename with primitive array change as type-changed (not max-items-one)", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tag": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tags": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Type: "string"},
					}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedInput])
		assert.Equal(t, 0, filter.counts[diffMaxItemsOneChanged])
		assert.Equal(t, 0, filter.counts[diffMissingInput])
	})

	t.Run("detects rename with int to number change", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"count": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"counts": {TypeSpec: schema.TypeSpec{Type: "number"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedIntToNumberInput])
		assert.Equal(t, 0, filter.counts[diffMissingInput])
	})

	t.Run("detects rename with other type change", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"item": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"items": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffTypeChangedInput])
		assert.Equal(t, 0, filter.counts[diffMissingInput])
	})

	t.Run("detects output pluralization rename", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"results": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffPluralizationRenameOutput])
		assert.Equal(t, 0, filter.counts[diffMissingOutput])
	})

	t.Run("detects function input pluralization rename", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Functions = map[string]schema.FunctionSpec{
			"my-pkg:index:myFunc": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Functions = map[string]schema.FunctionSpec{
			"my-pkg:index:myFunc": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"filters": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffPluralizationRenameInput])
		assert.Equal(t, 0, filter.counts[diffMissingInput])
	})

	t.Run("detects type property pluralization rename", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		// Mark as input type by using it in a resource input
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Types = map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"values": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		}
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:MyType"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffPluralizationRenameInput])
		assert.Equal(t, 0, filter.counts[diffMissingProperty])
	})

	t.Run("falls back to missing-input when no pluralization match", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"foo": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"bar": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMissingInput])
		assert.Equal(t, 0, filter.counts[diffPluralizationRenameInput])
	})
}

func TestMaxItemsOneChanged(t *testing.T) {
	t.Run("detects array of objects to single object", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"configs": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/my-pkg:index:ConfigType"},
					}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"configs": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:ConfigType"}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMaxItemsOneChanged])
		assert.Equal(t, 0, filter.counts[diffTypeChangedInput])
	})

	t.Run("detects single object to array of objects", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/my-pkg:index:ConfigType"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/my-pkg:index:ConfigType"},
					}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMaxItemsOneChanged])
		assert.Equal(t, 0, filter.counts[diffTypeChangedInput])
	})

	t.Run("primitive array change is type-changed not max-items-one", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tags": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"tags": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Type: "string"},
					}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 0, filter.counts[diffMaxItemsOneChanged])
		// Count is 2: one for string→array, one for nil→string on Items
		assert.Equal(t, 2, filter.counts[diffTypeChangedInput])
	})

	t.Run("object type to array of objects", func(t *testing.T) {
		oldSchema := simpleEmptySchema()
		oldSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"settings": {TypeSpec: schema.TypeSpec{Type: "object"}},
				},
			},
		}
		newSchema := simpleEmptySchema()
		newSchema.Resources = map[string]schema.ResourceSpec{
			"my-pkg:index:MyResource": {
				InputProperties: map[string]schema.PropertySpec{
					"settings": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Type: "object"},
					}},
				},
			},
		}

		filter := newDiffFilter()
		breakingChanges(oldSchema, newSchema, filter)

		assert.Equal(t, 1, filter.counts[diffMaxItemsOneChanged])
		assert.Equal(t, 0, filter.counts[diffTypeChangedInput])
	})
}
