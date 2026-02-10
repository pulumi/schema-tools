package compare

import (
	"bytes"
	"testing"
)

func TestRenderJSONDeterministicOrdering(t *testing.T) {
	result := CompareResult{
		Summary: []SummaryItem{
			{
				Category: "zeta-category",
				Count:    1,
				Entries:  []string{"c", "a"},
			},
			{
				Category: "alpha-category",
				Count:    2,
				Entries:  []string{"b", "a"},
			},
		},
		BreakingChanges: []string{"line-2", "line-1"},
		NewResources:    []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions:    []string{"zeta.fn", "alpha.fn"},
	}

	var out bytes.Buffer
	if err := RenderJSON(&out, result, false); err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}

	expected := `{
  "summary": [
    {
      "category": "alpha-category",
      "count": 2
    },
    {
      "category": "zeta-category",
      "count": 1
    }
  ],
  "breaking_changes": [
    "line-2",
    "line-1"
  ],
  "new_resources": [
    "alpha.Resource",
    "zeta.Resource"
  ],
  "new_functions": [
    "alpha.fn",
    "zeta.fn"
  ]
}`
	if out.String() != expected {
		t.Fatalf("unexpected JSON output:\n%s", out.String())
	}
}

func TestRenderJSONSummaryOnly(t *testing.T) {
	result := CompareResult{
		Summary:         []SummaryItem{{Category: "missing-input", Count: 1, Entries: []string{"e1"}}},
		BreakingChanges: []string{"line-1"},
		NewResources:    []string{"r1"},
		NewFunctions:    []string{"f1"},
	}

	var out bytes.Buffer
	if err := RenderJSON(&out, result, true); err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}

	if bytes.Contains(out.Bytes(), []byte("line-1")) {
		t.Fatalf("expected summary-only JSON to omit breaking_changes, got %s", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("r1")) || bytes.Contains(out.Bytes(), []byte("f1")) {
		t.Fatalf("expected summary-only JSON to omit new resources/functions, got %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("missing-input")) || !bytes.Contains(out.Bytes(), []byte("e1")) {
		t.Fatalf("expected summary entries in summary-only output, got %s", out.String())
	}
}
