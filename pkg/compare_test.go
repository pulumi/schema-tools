package pkg

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
)

func TestCompareSame(t *testing.T) {
	actual := Compare("my-provider", schema.PackageSpec{}, schema.PackageSpec{})
	expected := `Looking good! No breaking changes found.
No new resources/functions.
`
	assert.Equal(t, expected, actual)
}

func TestCompareTypeMissing(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			"my-type": {},
		},
	}
	newSchema := schema.PackageSpec{}
	actual := Compare("my-provider", oldSchema, newSchema)
	expected := `Found 1 breaking change:
Type "my-type" missing
No new resources/functions.
`
	assert.Equal(t, expected, actual)
}

func TestCompareResourceMissing(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-resource": {},
		},
	}
	newSchema := schema.PackageSpec{}
	actual := Compare("my-provider", oldSchema, newSchema)
	expected := `Found 1 breaking change:
Resource "my-resource" missing
No new resources/functions.
`
	assert.Equal(t, expected, actual)
}
