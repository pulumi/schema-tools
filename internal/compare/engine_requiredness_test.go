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
