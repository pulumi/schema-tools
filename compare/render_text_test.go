package compare

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderTextNoBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	RenderText(&out, Result{})

	text := out.String()
	if !strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("expected no-breaking message, got:\n%s", text)
	}
}

func TestRenderTextSortsNewResourcesAndFunctions(t *testing.T) {
	var out bytes.Buffer
	RenderText(&out, Result{
		NewResources: []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions: []string{"zeta.fn", "alpha.fn"},
	})

	text := out.String()
	if first, second := strings.Index(text, "- `alpha.Resource`"), strings.Index(text, "- `zeta.Resource`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected resources to be sorted, got:\n%s", text)
	}
	if first, second := strings.Index(text, "- `alpha.fn`"), strings.Index(text, "- `zeta.fn`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected functions to be sorted, got:\n%s", text)
	}
}
