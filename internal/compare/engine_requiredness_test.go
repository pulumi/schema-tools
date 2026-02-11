package compare

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestBreakingChangesFunctionOutputsNilDoesNotPanic(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					Required: []string{"value"},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {},
		},
	}

	node := BreakingChanges(oldSchema, newSchema)
	var out bytes.Buffer
	node.Display(&out, -1)
	text := out.String()
	if !strings.Contains(text, "missing output") {
		t.Fatalf("expected missing output diagnostic, got:\n%s", text)
	}
}

func TestBreakingChangesFunctionOutputsMissingDoesNotEmitRequiredToOptional(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					Required: []string{"value"},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Functions: map[string]schema.FunctionSpec{
			"my-pkg:index:MyFunction": {
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{},
					Required:   []string{},
				},
			},
		},
	}

	node := BreakingChanges(oldSchema, newSchema)
	var out bytes.Buffer
	node.Display(&out, -1)
	text := out.String()
	if strings.Contains(text, "property is no longer Required") {
		t.Fatalf("expected no required-to-optional diagnostic when output is removed, got:\n%s", text)
	}
}

func TestBreakingChangesTypeMissingDoesNotEmitRequiredToOptional(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
					Required: []string{"field"},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			"my-pkg:index:MyType": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{},
					Required:   []string{},
				},
			},
		},
	}

	node := BreakingChanges(oldSchema, newSchema)
	var out bytes.Buffer
	node.Display(&out, -1)
	text := out.String()
	if strings.Contains(text, "property is no longer Required") {
		t.Fatalf("expected no required-to-optional diagnostic when type property is removed, got:\n%s", text)
	}
}
