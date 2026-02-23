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
	Summary      []SummaryItem  `json:"summary"`
	Changes      []Change       `json:"changes"`
	Grouped      GroupedChanges `json:"grouped"`
	NewResources []string       `json:"new_resources"`
	NewFunctions []string       `json:"new_functions"`
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
func NewFullJSONOutput(result Result) FullJSONOutput {
	normalized := normalizeForJSON(result)
	return FullJSONOutput{
		Summary:      normalized.Summary,
		Changes:      normalized.Changes,
		Grouped:      normalized.Grouped,
		NewResources: normalized.NewResources,
		NewFunctions: normalized.NewFunctions,
	}
}

// normalizeForJSON enforces deterministic ordering and non-nil slices/maps for
// all structured JSON payload fields.
func normalizeForJSON(result Result) Result {
	normalizedChanges := sortChanges(result.Changes)
	grouped := result.Grouped
	if isGroupedEmpty(grouped) {
		grouped = groupChanges(normalizedChanges)
	}
	normalized := Result{
		Summary:      normalizeSummary(result.Summary),
		Changes:      ensureChangeSlice(slices.Clone(normalizedChanges)),
		Grouped:      normalizeGrouped(grouped),
		NewResources: ensureSlice(slices.Clone(result.NewResources)),
		NewFunctions: ensureSlice(slices.Clone(result.NewFunctions)),
	}
	return normalized
}

// isGroupedEmpty reports whether grouped sections were not precomputed.
func isGroupedEmpty(grouped GroupedChanges) bool {
	return len(grouped.Resources) == 0 && len(grouped.Functions) == 0 && len(grouped.Types) == 0
}

// normalizeGrouped normalizes all grouped scopes for stable JSON output.
func normalizeGrouped(grouped GroupedChanges) GroupedChanges {
	return GroupedChanges{
		Resources: normalizeGroupedScope(grouped.Resources),
		Functions: normalizeGroupedScope(grouped.Functions),
		Types:     normalizeGroupedScope(grouped.Types),
	}
}

// normalizeGroupedScope copies and sorts one grouped scope map.
func normalizeGroupedScope(grouped map[string]map[string][]Change) map[string]map[string][]Change {
	if len(grouped) == 0 {
		return map[string]map[string][]Change{}
	}
	out := make(map[string]map[string][]Change, len(grouped))
	for token, byLocation := range grouped {
		if len(byLocation) == 0 {
			out[token] = map[string][]Change{}
			continue
		}
		locationCopy := make(map[string][]Change, len(byLocation))
		for location, changes := range byLocation {
			locationCopy[location] = sortChanges(changes)
		}
		out[token] = locationCopy
	}
	return out
}

// normalizeSummary sorts summary items and their entry lists deterministically.
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
