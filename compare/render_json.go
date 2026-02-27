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
// shape with category counts only.
func NewSummaryJSONOutput(result Result) SummaryJSONOutput {
	normalized := normalizeForJSON(result)
	return SummaryJSONOutput{Summary: summaryWithoutEntries(normalized.Summary)}
}

// NewFullJSONOutput normalizes a compare result to the full CLI JSON shape.
// Summary entries are omitted to keep payloads compact.
func NewFullJSONOutput(result Result) FullJSONOutput {
	normalized := normalizeForJSON(result)
	return FullJSONOutput{
		Summary:      summaryWithoutEntries(normalized.Summary),
		Changes:      normalized.Changes,
		Grouped:      normalized.Grouped,
		NewResources: normalized.NewResources,
		NewFunctions: normalized.NewFunctions,
	}
}

func normalizeForJSON(result Result) Result {
	normalizedChanges := sortStructuredChanges(ensureChangeSlice(slices.Clone(result.Changes)))
	normalizedGrouped := normalizeGrouped(result.Grouped)
	if isGroupedEmpty(normalizedGrouped) {
		normalizedGrouped = groupStructuredChanges(normalizedChanges)
	}
	normalized := Result{
		Summary:       normalizeSummary(result.Summary),
		Changes:       normalizedChanges,
		Grouped:       normalizedGrouped,
		NewResources:  ensureSlice(slices.Clone(result.NewResources)),
		NewFunctions:  ensureSlice(slices.Clone(result.NewFunctions)),
		totalBreaking: result.totalBreaking,
	}
	return normalized
}

func isGroupedEmpty(grouped GroupedChanges) bool {
	return len(grouped.Resources) == 0 && len(grouped.Functions) == 0 && len(grouped.Types) == 0
}

func normalizeGrouped(grouped GroupedChanges) GroupedChanges {
	return GroupedChanges{
		Resources: normalizeGroupedScope(grouped.Resources),
		Functions: normalizeGroupedScope(grouped.Functions),
		Types:     normalizeGroupedScope(grouped.Types),
	}
}

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
			locationCopy[location] = sortStructuredChanges(changes)
		}
		out[token] = locationCopy
	}
	return out
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
