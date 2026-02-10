package compare

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type summaryOnlyJSON struct {
	Summary []SummaryItem `json:"summary"`
}

type fullJSON struct {
	Summary         []SummaryItem `json:"summary"`
	BreakingChanges []string      `json:"breaking_changes"`
	NewResources    []string      `json:"new_resources"`
	NewFunctions    []string      `json:"new_functions"`
}

// RenderJSON writes a deterministic JSON payload for compare results.
func RenderJSON(out io.Writer, result CompareResult, summaryOnly bool) error {
	normalized := normalizeForJSON(result)

	var payload any
	if summaryOnly {
		payload = summaryOnlyJSON{Summary: normalized.Summary}
	} else {
		payload = fullJSON{
			Summary:         summaryWithoutEntries(normalized.Summary),
			BreakingChanges: normalized.BreakingChanges,
			NewResources:    normalized.NewResources,
			NewFunctions:    normalized.NewFunctions,
		}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compare JSON: %w", err)
	}
	if _, err := out.Write(data); err != nil {
		return fmt.Errorf("write compare JSON: %w", err)
	}
	return nil
}

func normalizeForJSON(result CompareResult) CompareResult {
	normalized := CompareResult{
		Summary:         normalizeSummary(result.Summary),
		BreakingChanges: cloneOrEmpty(result.BreakingChanges),
		NewResources:    cloneOrEmpty(result.NewResources),
		NewFunctions:    cloneOrEmpty(result.NewFunctions),
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
		entryCopy := cloneOrEmpty(item.Entries)
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
