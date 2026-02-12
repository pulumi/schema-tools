package compare

import internalcompare "github.com/pulumi/schema-tools/internal/compare"

// Options configures compare behavior.
type Options struct {
	Provider   string
	MaxChanges int
}

// SummaryItem is one summary category/count entry for compare output.
type SummaryItem struct {
	Category string   `json:"category"`
	Count    int      `json:"count"`
	Entries  []string `json:"entries,omitempty"`
}

// Result is the structured output of schema comparison.
type Result struct {
	Summary         []SummaryItem `json:"summary"`
	BreakingChanges []string      `json:"breaking_changes"`
	NewResources    []string      `json:"new_resources"`
	NewFunctions    []string      `json:"new_functions"`

	report     internalcompare.Report
	maxChanges int
}
