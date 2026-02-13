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

func TestRenderTextPreservesNewResourcesAndFunctionsOrder(t *testing.T) {
	var out bytes.Buffer
	RenderText(&out, Result{
		NewResources: []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions: []string{"zeta.fn", "alpha.fn"},
	})

	text := out.String()
	if first, second := strings.Index(text, "- `zeta.Resource`"), strings.Index(text, "- `alpha.Resource`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected resources to preserve input order, got:\n%s", text)
	}
	if first, second := strings.Index(text, "- `zeta.fn`"), strings.Index(text, "- `alpha.fn`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected functions to preserve input order, got:\n%s", text)
	}
}

func TestRenderTextOneBreakingChange(t *testing.T) {
	var out bytes.Buffer
	RenderText(&out, Result{BreakingChanges: []string{"`🔴` test violation"}})

	text := out.String()
	if !strings.Contains(text, "Found 1 breaking change:\n") {
		t.Fatalf("expected singular breaking-change header, got:\n%s", text)
	}
	if !strings.Contains(text, "`🔴` test violation") {
		t.Fatalf("expected single breaking change line, got:\n%s", text)
	}
}

func TestRenderTextManyBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	RenderText(&out, Result{BreakingChanges: []string{"`🔴` first", "`🟡` second"}})

	text := out.String()
	if !strings.Contains(text, "Found 2 breaking changes:\n") {
		t.Fatalf("expected plural breaking-change header, got:\n%s", text)
	}
	if !strings.Contains(text, "`🔴` first\n`🟡` second") {
		t.Fatalf("expected all breaking change lines, got:\n%s", text)
	}
}
