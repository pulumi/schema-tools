package compare

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
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
		Changes:      []Change{{Scope: ScopeResource, Token: "pkg:index:Res", Kind: "missing-input"}},
		Grouped:      GroupedChanges{Resources: map[string]map[string][]Change{"pkg:index:Res": {"general": {{Scope: ScopeResource, Token: "pkg:index:Res", Kind: "missing-input"}}}}, Functions: map[string]map[string][]Change{}, Types: map[string]map[string][]Change{}},
		NewResources: []string{"r1"},
		NewFunctions: []string{"f1"},
	}

	data, err := json.Marshal(NewSummaryJSONOutput(result))
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if bytes.Contains(data, []byte("line-1")) {
		t.Fatalf("expected summary-only JSON to omit change lines, got %s", string(data))
	}
	if bytes.Contains(data, []byte("r1")) || bytes.Contains(data, []byte("f1")) {
		t.Fatalf("expected summary-only JSON to omit new resources/functions, got %s", string(data))
	}
	if !bytes.Contains(data, []byte("missing-input")) {
		t.Fatalf("expected summary category in summary-only output, got %s", string(data))
	}
	if bytes.Contains(data, []byte(`"entries"`)) || bytes.Contains(data, []byte("e1")) {
		t.Fatalf("expected summary-only JSON to omit entry strings, got %s", string(data))
	}
	if bytes.Contains(data, []byte(`"changes"`)) || bytes.Contains(data, []byte(`"grouped"`)) {
		t.Fatalf("expected summary-only JSON to omit changes/grouped keys, got %s", string(data))
	}
	if bytes.Contains(data, []byte(`"new_resources"`)) || bytes.Contains(data, []byte(`"new_functions"`)) {
		t.Fatalf("expected summary-only JSON to omit new_resources/new_functions keys, got %s", string(data))
	}
}

func TestMarshalJSONAndFullJSONOutputUseDifferentSummaryShapes(t *testing.T) {
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
	if bytes.Contains(fullData, []byte(`"entries"`)) {
		t.Fatalf("expected full JSON output to omit entries, got %s", string(fullData))
	}
	if !bytes.Contains(fullData, []byte(`"scope"`)) || !bytes.Contains(fullData, []byte(`"grouped"`)) {
		t.Fatalf("expected full JSON output to include structured changes/grouped, got %s", string(fullData))
	}
}

func TestJSONOutputFixtureContent(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	t.Run("full", func(t *testing.T) {
		payload := NewFullJSONOutput(result)

		if got, want := payload.Changes, result.Changes; !reflect.DeepEqual(got, want) {
			t.Fatalf("changes mismatch: got %v want %v", got, want)
		}
		if got, want := payload.Grouped, result.Grouped; !reflect.DeepEqual(got, want) {
			t.Fatalf("grouped mismatch: got %v want %v", got, want)
		}
		if len(payload.NewResources) != 0 || len(payload.NewFunctions) != 0 {
			t.Fatalf("expected no new resources/functions, got resources=%v functions=%v", payload.NewResources, payload.NewFunctions)
		}
		if len(payload.Changes) == 0 {
			t.Fatalf("expected structured changes to be populated, got %+v", payload)
		}
		for i, change := range payload.Changes {
			if change.Scope == "" {
				t.Fatalf("unexpected unknown/empty scope at index %d: %+v", i, change)
			}
			if change.Token == "" || change.Kind == "" || change.Path == "" || change.Severity == "" {
				t.Fatalf("expected structured fields to be populated at index %d: %+v", i, change)
			}
		}
		if got, want := countGroupedLeaves(payload.Grouped), len(payload.Changes); got != want {
			t.Fatalf("grouped leaf count mismatch: got %d want %d", got, want)
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
		payload := NewSummaryJSONOutput(result)

		gotCounts := map[string]int{}
		for _, item := range payload.Summary {
			if len(item.Entries) != 0 {
				t.Fatalf("summary JSON must omit entries, found %v in %q", item.Entries, item.Category)
			}
			gotCounts[item.Category] = item.Count
		}
		if !reflect.DeepEqual(gotCounts, expectedFixtureSummaryCounts()) {
			t.Fatalf("summary counts mismatch: got %v want %v", gotCounts, expectedFixtureSummaryCounts())
		}
	})
}

func TestJSONOutputFixtureGoldens(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	assertGoldenJSON(t, NewFullJSONOutput(result), "compare-full.golden.json")
	assertGoldenJSON(t, NewSummaryJSONOutput(result), "compare-summary.golden.json")
}

func TestNewFullJSONOutputStructuredContractKeys(t *testing.T) {
	oldSchema, newSchema := mustLoadFixtureSchemas(t)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1})

	data, err := json.Marshal(NewFullJSONOutput(result))
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	mustHave := []string{"summary", "changes", "grouped", "new_resources", "new_functions"}
	for _, key := range mustHave {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q to be present, got keys=%v", key, sortedKeys(payload))
		}
	}

	if _, ok := payload["breaking_changes"]; ok {
		t.Fatalf("expected key %q to be absent, got keys=%v", "breaking_changes", sortedKeys(payload))
	}
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func countGroupedLeaves(grouped GroupedChanges) int {
	count := 0
	for _, byLocation := range grouped.Resources {
		for _, changes := range byLocation {
			count += len(changes)
		}
	}
	for _, byLocation := range grouped.Functions {
		for _, changes := range byLocation {
			count += len(changes)
		}
	}
	for _, byLocation := range grouped.Types {
		for _, changes := range byLocation {
			count += len(changes)
		}
	}
	return count
}

func assertGoldenJSON(t testing.TB, payload any, goldenName string) {
	t.Helper()
	gotBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal payload for %s: %v", goldenName, err)
	}
	wantBytes := mustReadTestdataFile(t, goldenName)
	got := strings.TrimSpace(string(gotBytes))
	want := strings.TrimSpace(string(wantBytes))
	if got != want {
		t.Fatalf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", goldenName, got, want)
	}
}
