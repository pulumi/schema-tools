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
			},
			"test:index/getBar:getBar": {
				Description: "01234",
			},
		},
		Resources: map[string]schema.ResourceSpec{
			"test:index/foo:Foo": {
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

	assert.Equal(t, 3, stats.TotalResourceInputs)
	assert.Equal(t, 2, stats.ResourceInputsMissingDesc)
	assert.Equal(t, 1, stats.TotalResources)
	assert.Equal(t, 2, stats.TotalFunctions)
}
