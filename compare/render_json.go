package compare

import (
	"encoding/json"
	"slices"
	"sort"
)

// SummaryJSONOutput is the compare --json --summary payload shape.
type SummaryJSONOutput struct {
	Summary []SummaryItem `json:"summary"`
}

// FullJSONOutput is the compare --json payload shape.
type FullJSONOutput struct {
	Summary         []SummaryItem `json:"summary"`
	BreakingChanges []string      `json:"breaking_changes"`
	NewResources    []string      `json:"new_resources"`
	NewFunctions    []string      `json:"new_functions"`
}

// MarshalJSON produces deterministic output ordering and non-nil slices.
// It preserves SummaryItem.Entries as the full structured API representation.
func (result Result) MarshalJSON() ([]byte, error) {
	normalized := normalizeForJSON(result)
	type alias Result
	return json.Marshal(alias(normalized))
}

// NewSummaryJSONOutput normalizes a compare result to the summary-only CLI JSON
// shape, preserving summary entries.
func NewSummaryJSONOutput(result Result) SummaryJSONOutput {
	normalized := normalizeForJSON(result)
	return SummaryJSONOutput{Summary: normalized.Summary}
}

// NewFullJSONOutput normalizes a compare result to the full CLI JSON shape.
// Summary entries are omitted to keep payloads compact.
func NewFullJSONOutput(result Result) FullJSONOutput {
	normalized := normalizeForJSON(result)
	return FullJSONOutput{
		Summary:         summaryWithoutEntries(normalized.Summary),
		BreakingChanges: normalized.BreakingChanges,
		NewResources:    normalized.NewResources,
		NewFunctions:    normalized.NewFunctions,
	}
}

func normalizeForJSON(result Result) Result {
	normalized := Result{
		Summary:         normalizeSummary(result.Summary),
		BreakingChanges: ensureSlice(slices.Clone(result.BreakingChanges)),
		NewResources:    ensureSlice(slices.Clone(result.NewResources)),
		NewFunctions:    ensureSlice(slices.Clone(result.NewFunctions)),
	}
	return normalized
}

func normalizeSummary(items []SummaryItem) []SummaryItem {
	if len(items) == 0 {
		return []SummaryItem{}
	}
	normalized := make([]SummaryItem, len(items))
	for i, item := range items {
		entryCopy := ensureSlice(slices.Clone(item.Entries))
		sort.Strings(entryCopy)
		normalized[i] = SummaryItem{
			Category: item.Category,
			Count:    item.Count,
			Entries:  entryCopy,
		}
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Category != normalized[j].Category {
			return normalized[i].Category < normalized[j].Category
		}
		return normalized[i].Count < normalized[j].Count
	})
	return normalized
}

func summaryWithoutEntries(items []SummaryItem) []SummaryItem {
	if len(items) == 0 {
		return []SummaryItem{}
	}
	stripped := make([]SummaryItem, len(items))
	for i, item := range items {
		stripped[i] = SummaryItem{Category: item.Category, Count: item.Count}
	}
	return stripped
}
