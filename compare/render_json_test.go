package compare

import (
	"bytes"
	"testing"
)

func TestRenderJSONDeterministicOrdering(t *testing.T) {
	result := Result{
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
	if err := RenderJSON(&out, result); err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}

	expected := `{
  "summary": [
    {
      "category": "alpha-category",
      "count": 2,
      "entries": [
        "a",
        "b"
      ]
    },
    {
      "category": "zeta-category",
      "count": 1,
      "entries": [
        "a",
        "c"
      ]
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
