package compare

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestMarshalJSONDeterministicOrdering(t *testing.T) {
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
		Changes: []Change{
			{
				Scope:    ScopeType,
				Token:    "pkg:index:Type",
				Location: "properties",
				Path:     `Types: "pkg:index:Type": properties: "z"`,
				Kind:     "type-changed",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "type changed",
			},
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:Res",
				Location: "inputs",
				Path:     `Resources: "pkg:index:Res": inputs: "a"`,
				Kind:     "missing-input",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing input",
			},
		},
		NewResources: []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions: []string{"zeta.fn", "alpha.fn"},
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}

	var payload struct {
		Summary []SummaryItem  `json:"summary"`
		Changes []Change       `json:"changes"`
		Grouped GroupedChanges `json:"grouped"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(payload.Summary) != 2 {
		t.Fatalf("expected 2 summary rows, got %d", len(payload.Summary))
	}
	if payload.Summary[0].Category != "alpha-category" || payload.Summary[1].Category != "zeta-category" {
		t.Fatalf("expected sorted summary categories, got %+v", payload.Summary)
	}
	if len(payload.Changes) != 2 || payload.Changes[0].Scope != ScopeResource || payload.Changes[1].Scope != ScopeType {
		t.Fatalf("expected sorted changes by scope, got %+v", payload.Changes)
	}
	if _, ok := payload.Grouped.Resources["pkg:index:Res"]["inputs"]; !ok {
		t.Fatalf("expected grouped resources.inputs to be populated, got %+v", payload.Grouped)
	}
}

func TestNewSummaryJSONOutput(t *testing.T) {
	result := Result{
		Summary:      []SummaryItem{{Category: "missing-input", Count: 1, Entries: []string{"e1"}}},
		NewResources: []string{"r1"},
		NewFunctions: []string{"f1"},
	}

	data, err := json.Marshal(NewSummaryJSONOutput(result))
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if bytes.Contains(data, []byte("r1")) || bytes.Contains(data, []byte("f1")) {
		t.Fatalf("expected summary-only JSON to omit new resources/functions, got %s", string(data))
	}
	if !bytes.Contains(data, []byte("missing-input")) || !bytes.Contains(data, []byte("e1")) {
		t.Fatalf("expected summary entries in summary-only output, got %s", string(data))
	}
	if bytes.Contains(data, []byte(`"new_resources"`)) || bytes.Contains(data, []byte(`"new_functions"`)) {
		t.Fatalf("expected summary-only JSON to omit new_resources/new_functions keys, got %s", string(data))
	}
}

func TestMarshalJSONAndFullJSONOutputUseStructuredContract(t *testing.T) {
	result := Result{
		Summary: []SummaryItem{
			{Category: "missing-input", Count: 1, Entries: []string{"entry-1"}},
		},
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:Res",
				Location: "inputs",
				Path:     `Resources: "pkg:index:Res": inputs: "arg"`,
				Kind:     "missing-input",
				Severity: SeverityWarn,
				Breaking: true,
				Source:   SourceEngine,
				Message:  "missing input",
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !bytes.Contains(data, []byte(`"entries"`)) {
		t.Fatalf("expected MarshalJSON output to include entries, got %s", string(data))
	}

	fullData, err := json.Marshal(NewFullJSONOutput(result))
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !bytes.Contains(fullData, []byte(`"entries"`)) {
		t.Fatalf("expected full JSON output to include entries, got %s", string(fullData))
	}
	if !bytes.Contains(fullData, []byte(`"changes"`)) || !bytes.Contains(fullData, []byte(`"grouped"`)) {
		t.Fatalf("expected full JSON output to include changes/grouped, got %s", string(fullData))
	}
}

func TestJSONOutputFixtureContent(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg"})

	t.Run("full", func(t *testing.T) {
		payload := NewFullJSONOutput(result)

		if len(payload.NewResources) != 0 || len(payload.NewFunctions) != 0 {
			t.Fatalf("expected no new resources/functions, got resources=%v functions=%v", payload.NewResources, payload.NewFunctions)
		}
		if len(payload.Changes) == 0 {
			t.Fatalf("expected full output to include changes, got %+v", payload)
		}
		if len(payload.Grouped.Resources) == 0 && len(payload.Grouped.Functions) == 0 && len(payload.Grouped.Types) == 0 {
			t.Fatalf("expected full output to include grouped projection, got %+v", payload.Grouped)
		}

		gotSummaryCounts := map[string]int{}
		for _, item := range payload.Summary {
			gotSummaryCounts[item.Category] = item.Count
		}
		if !reflect.DeepEqual(gotSummaryCounts, expectedFixtureSummaryCounts()) {
			t.Fatalf("summary count mismatch: got %v want %v", gotSummaryCounts, expectedFixtureSummaryCounts())
		}
		gotEntries := map[string][]string{}
		for _, item := range payload.Summary {
			gotEntries[item.Category] = item.Entries
		}
		if !reflect.DeepEqual(gotEntries, expectedFixtureSummaryEntries()) {
			t.Fatalf("summary entries mismatch:\n got: %v\nwant: %v", gotEntries, expectedFixtureSummaryEntries())
		}
	})

	t.Run("summary", func(t *testing.T) {
		payload := NewSummaryJSONOutput(result)

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
