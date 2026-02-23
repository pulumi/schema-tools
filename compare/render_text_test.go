package compare

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderTextNoBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{}, -1); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("expected no-breaking message, got:\n%s", text)
	}
}

func TestRenderTextPreservesNewResourcesAndFunctionsOrder(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		NewResources: []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions: []string{"zeta.fn", "alpha.fn"},
	}, -1); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

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
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:Res",
				Location: "inputs",
				Path:     `Resources: "pkg:index:Res": inputs: "name"`,
				Kind:     "missing-input",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing",
			},
		},
	}, -1); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Found 1 breaking change:\n") {
		t.Fatalf("expected singular breaking-change header, got:\n%s", text)
	}
	if !strings.Contains(text, "#### Resources") || !strings.Contains(text, `- "pkg:index:Res":`) {
		t.Fatalf("expected grouped resources section, got:\n%s", text)
	}
	if !strings.Contains(text, "`🟡` \"name\" missing") {
		t.Fatalf("expected grouped change line, got:\n%s", text)
	}
}

func TestRenderTextManyBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:A",
				Location: "inputs",
				Path:     `Resources: "pkg:index:A": inputs: "name"`,
				Kind:     "missing-input",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing",
			},
			{
				Scope:    ScopeFunction,
				Token:    "pkg:index:getA",
				Location: "signature",
				Path:     `Functions: "pkg:index:getA"`,
				Kind:     "signature-changed",
				Severity: SeverityError,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "signature change",
			},
		},
	}, -1); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Found 2 breaking changes:\n") {
		t.Fatalf("expected plural breaking-change header, got:\n%s", text)
	}
	if !strings.Contains(text, "#### Resources") || !strings.Contains(text, "#### Functions") {
		t.Fatalf("expected grouped sections, got:\n%s", text)
	}
	if !strings.Contains(text, "`🟡` \"name\" missing") || !strings.Contains(text, "`🔴` signature change") {
		t.Fatalf("expected all grouped change lines, got:\n%s", text)
	}
}

func TestRenderTextGeneralChangesAreSingleLine(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeFunction,
				Token:    "pkg:index:getThing",
				Path:     `Functions: "pkg:index:getThing"`,
				Kind:     "signature-changed",
				Severity: SeverityError,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "signature change",
			},
		},
	}, -1); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, `- `+"`🔴`"+` "pkg:index:getThing" signature change`) {
		t.Fatalf("expected flattened general line, got:\n%s", text)
	}
	if strings.Contains(text, "general:") {
		t.Fatalf("expected general header to be omitted, got:\n%s", text)
	}
}

func TestRenderTextOmitsNonBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:A",
				Location: "inputs",
				Path:     `Resources: "pkg:index:A": inputs: "name"`,
				Kind:     "missing-input",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing",
			},
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:A",
				Location: "general",
				Path:     `Resources: "pkg:index:A"`,
				Kind:     "deprecated-resource-alias",
				Severity: SeverityInfo,
				Breaking: false,
				Source:   SourceNormalize,
				Message:  "retained as deprecated alias",
			},
		},
	}, -1)
	if err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Found 1 breaking change:") {
		t.Fatalf("expected only breaking changes to be counted, got:\n%s", text)
	}
	if strings.Contains(text, "deprecated alias") {
		t.Fatalf("expected non-breaking change to be omitted from text, got:\n%s", text)
	}
}

func TestRenderTextMaxChangesCapsDisplayedLinesOnly(t *testing.T) {
	var out bytes.Buffer
	err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:A",
				Location: "inputs",
				Path:     `Resources: "pkg:index:A": inputs: "name"`,
				Kind:     "missing-input",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing",
			},
			{
				Scope:    ScopeFunction,
				Token:    "pkg:index:getA",
				Location: "outputs",
				Path:     `Functions: "pkg:index:getA": outputs: "value"`,
				Kind:     "missing-output",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing output",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Found 1 breaking change:") {
		t.Fatalf("expected capped breaking count, got:\n%s", text)
	}
	if strings.Contains(text, `Functions: "pkg:index:getA"`) {
		t.Fatalf("expected second change to be capped from text output, got:\n%s", text)
	}
}

func TestRenderTextWriteError(t *testing.T) {
	err := RenderText(failingWriter{}, Result{
		Changes: []Change{{
			Scope:    ScopeResource,
			Token:    "pkg:index:Res",
			Location: "inputs",
			Path:     `Resources: "pkg:index:Res": inputs: "name"`,
			Kind:     "missing-input",
			Severity: SeverityWarn,
			Breaking: true,
			Source:   SourceEngine,
			Message:  "missing",
		}},
	}, -1)
	if err == nil {
		t.Fatal("expected write error")
	}
}
