package compare

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
)

// RenderJSON writes a deterministic JSON payload for compare results.
func RenderJSON(out io.Writer, result Result) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compare JSON: %w", err)
	}
	if _, err := out.Write(data); err != nil {
		return fmt.Errorf("write compare JSON: %w", err)
	}
	return nil
}

// MarshalJSON produces deterministic output ordering and non-nil slices.
func (result Result) MarshalJSON() ([]byte, error) {
	normalized := normalizeForJSON(result)
	type alias Result
	return json.Marshal(alias(normalized))
}

func normalizeForJSON(result Result) Result {
	normalized := Result{
		Summary:         normalizeSummary(result.Summary),
		BreakingChanges: ensureSlice(slices.Clone(result.BreakingChanges)),
		NewResources:    ensureSlice(slices.Clone(result.NewResources)),
		NewFunctions:    ensureSlice(slices.Clone(result.NewFunctions)),
	}
	sort.Strings(normalized.NewResources)
	sort.Strings(normalized.NewFunctions)
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
