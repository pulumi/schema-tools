package compare

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

func TestRenderTextViolationSummary(t *testing.T) {
	tests := []struct {
		name     string
		report   Report
		expected string
	}{
		{
			name: "zero violations",
			report: Report{
				Violations: &diagtree.Node{},
			},
			expected: "Looking good! No breaking changes found.",
		},
		{
			name: "one violation",
			report: Report{
				Violations: violations("first"),
			},
			expected: "Found 1 breaking change:",
		},
		{
			name: "many violations",
			report: Report{
				Violations: violations("first", "second"),
			},
			expected: "Found 2 breaking changes:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			RenderText(&out, tt.report, 500)
			if !strings.Contains(out.String(), tt.expected) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.expected, out.String())
			}
		})
	}
}

func TestRenderTextSortsNewResourcesAndFunctions(t *testing.T) {
	report := Report{
		Violations:   &diagtree.Node{},
		NewResources: []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions: []string{"zeta.fn", "alpha.fn"},
	}

	var out bytes.Buffer
	RenderText(&out, report, 500)

	text := out.String()
	if strings.Contains(text, "No new resources/functions.") {
		t.Fatalf("expected new resources/functions section, got:\n%s", text)
	}

	if first, second := strings.Index(text, "- `alpha.Resource`"), strings.Index(text, "- `zeta.Resource`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected resources to be sorted, got:\n%s", text)
	}
	if first, second := strings.Index(text, "- `alpha.fn`"), strings.Index(text, "- `zeta.fn`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected functions to be sorted, got:\n%s", text)
	}
}

func TestRenderTextFixtureReport(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	report := Analyze("my-pkg", oldSchema, newSchema)

	var out bytes.Buffer
	RenderText(&out, report, -1)
	text := out.String()

	if !strings.Contains(text, "Found 8 breaking changes:") {
		t.Fatalf("expected breaking change count in output, got:\n%s", text)
	}

	expectedFragments := []string{
		"`🔴` \"my-pkg:index:RemovedResource\" missing",
		"`🔴` \"my-pkg:index:removedFunction\" missing",
		`type changed from "string" to "integer"`,
		`input has changed to Required`,
		`property is no longer Required`,
		`No new resources/functions.`,
	}
	for _, fragment := range expectedFragments {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected output to contain %q, got:\n%s", fragment, text)
		}
	}
}

func violations(names ...string) *diagtree.Node {
	root := &diagtree.Node{}
	for _, name := range names {
		root.Label("Resources").Value(name).SetDescription(diagtree.Danger, "missing")
	}
	return root
}

func mustLoadFixtureSchemas(t testing.TB) (schema.PackageSpec, schema.PackageSpec) {
	t.Helper()
	oldSchema := mustReadFixtureSchema(t, "schema-old.json")
	newSchema := mustReadFixtureSchema(t, "schema-new.json")
	return oldSchema, newSchema
}

func mustReadFixtureSchema(t testing.TB, name string) schema.PackageSpec {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "compare", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	var spec schema.PackageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal fixture %q: %v", name, err)
	}
	return spec
}
