package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
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

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
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
    "zeta.Resource",
    "alpha.Resource"
  ],
  "new_functions": [
    "zeta.fn",
    "alpha.fn"
  ]
}`
	if string(data) != expected {
		t.Fatalf("unexpected JSON output:\n%s", string(data))
	}
}

func TestRenderJSONSummaryOnly(t *testing.T) {
	result := Result{
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
	if bytes.Contains(out.Bytes(), []byte(`"breaking_changes"`)) {
		t.Fatalf("expected summary-only JSON to omit breaking_changes key, got %s", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte(`"new_resources"`)) || bytes.Contains(out.Bytes(), []byte(`"new_functions"`)) {
		t.Fatalf("expected summary-only JSON to omit new_resources/new_functions keys, got %s", out.String())
	}
}

func TestRenderJSONWriteError(t *testing.T) {
	result := Result{
		Summary: []SummaryItem{{Category: "missing-input", Count: 1}},
	}
	err := RenderJSON(jsonFailingWriter{}, result, false)
	if err == nil {
		t.Fatal("expected write error")
	}
}

type jsonFailingWriter struct{}

func (jsonFailingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestRenderJSONFixtureContent(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	t.Run("full", func(t *testing.T) {
		var out bytes.Buffer
		if err := RenderJSON(&out, result, false); err != nil {
			t.Fatalf("RenderJSON failed: %v", err)
		}

		var payload fullJSON
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("failed to unmarshal full payload: %v", err)
		}

		if got, want := payload.BreakingChanges, result.BreakingChanges; !reflect.DeepEqual(got, want) {
			t.Fatalf("breaking changes mismatch: got %v want %v", got, want)
		}
		if len(payload.NewResources) != 0 || len(payload.NewFunctions) != 0 {
			t.Fatalf("expected no new resources/functions, got resources=%v functions=%v", payload.NewResources, payload.NewFunctions)
		}

		gotSummaryCounts := map[string]int{}
		for _, item := range payload.Summary {
			if len(item.Entries) != 0 {
				t.Fatalf("full JSON summary must omit entries, found %v in %q", item.Entries, item.Category)
			}
			gotSummaryCounts[item.Category] = item.Count
		}
		if !reflect.DeepEqual(gotSummaryCounts, expectedFixtureSummaryCounts()) {
			t.Fatalf("summary count mismatch: got %v want %v", gotSummaryCounts, expectedFixtureSummaryCounts())
		}
	})

	t.Run("summary", func(t *testing.T) {
		var out bytes.Buffer
		if err := RenderJSON(&out, result, true); err != nil {
			t.Fatalf("RenderJSON failed: %v", err)
		}

		var payload summaryOnlyJSON
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("failed to unmarshal summary payload: %v", err)
		}

		gotEntries := map[string][]string{}
		gotCounts := map[string]int{}
		for _, item := range payload.Summary {
			gotEntries[item.Category] = item.Entries
			gotCounts[item.Category] = item.Count
		}
		if !reflect.DeepEqual(gotCounts, expectedFixtureSummaryCounts()) {
			t.Fatalf("summary counts mismatch: got %v want %v", gotCounts, expectedFixtureSummaryCounts())
		}
		if !reflect.DeepEqual(gotEntries, expectedFixtureSummaryEntries()) {
			t.Fatalf("summary entries mismatch:\n got: %v\nwant: %v", gotEntries, expectedFixtureSummaryEntries())
		}
	})
}
