package pkg

import (
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCountStats(t *testing.T) {
	testSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"test:index/getFoo:getFoo": {
				Description: "0123456789",
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"input1": {
							Description: "0123456789",
						},
						"input2": {
							Description: "0",
						},
						"inputMissingDesc1": {},
						"inputMissingDesc2": {},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"output1": {
							Description: "0123456789",
						},
						"output2": {
							Description: "01",
						},
						"outputMissingDesc1": {},
						"outputMissingDesc2": {},
						"outputMissingDesc3": {},
					},
				},
			},
			"test:index/getBar:getBar": {
				Description: "0",
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"input1": {
							Description: "0",
						},
						"inputMissingDesc3": {},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"output1": {
							Description: "0",
						},
						"outputMissingDesc4": {},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			"test:index/foo:Foo": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Description: "0123456789",
					Properties: map[string]schema.PropertySpec{
						"output1": {
							Description: "0123456789",
						},
						"output2": {
							Description: "01234",
						},
						"noDesc1": {
							Description: "",
						},
						"noDesc2": {
							Description: "",
						},
						"noDesc3": {
							Description: "",
						},
						"noDesc4": {
							Description: "",
						},
						"noDesc5": {
							Description: "",
						},
					},
				},
				InputProperties: map[string]schema.PropertySpec{
					"noDesc1": {
						Description: "",
					},
					"noDesc2": {
						Description: "",
					},
					"hasDesc": {
						Description: "What it does",
					},
				},
			},
		},
	}

	stats := CountStats(testSchema)

	// The example schema above is designed to give a unique number for each property as much as possible to provide
	// the highest-confidence test results.
	assert.Equal(t, 1, stats.Resources.TotalResources)
	assert.Equal(t, 10, stats.Resources.TotalDescriptionBytes)

	assert.Equal(t, 3, stats.Resources.TotalInputProperties)
	assert.Equal(t, 2, stats.Resources.InputPropertiesMissingDescriptions)

	assert.Equal(t, 7, stats.Resources.TotalOutputProperties)
	assert.Equal(t, 5, stats.Resources.OutputPropertiesMissingDescriptions)

	assert.Equal(t, 2, stats.Functions.TotalFunctions)
	assert.Equal(t, 11, stats.Functions.TotalDescriptionBytes)

	assert.Equal(t, 3, stats.Functions.InputPropertiesMissingDescriptions)
	assert.Equal(t, 12, stats.Functions.TotalInputPropertyDescriptionBytes)

	assert.Equal(t, 4, stats.Functions.OutputPropertiesMissingDescriptions)
	assert.Equal(t, 13, stats.Functions.TotalOutputPropertyDescriptionBytes)
}

// TODO: Add a test case that throoughly tests all possible type references.
