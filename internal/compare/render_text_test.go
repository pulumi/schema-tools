package compare

import (
	"bytes"
	"strings"
	"testing"

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

func violations(names ...string) *diagtree.Node {
	root := &diagtree.Node{}
	for _, name := range names {
		root.Label("Resources").Value(name).SetDescription(diagtree.Danger, "missing")
	}
	return root
}
