package compare

import internalcompare "github.com/pulumi/schema-tools/internal/compare"

// CompareOptions configures compare behavior.
type CompareOptions struct {
	Provider   string
	MaxChanges int
}

// SummaryItem is one summary category/count entry for compare output.
type SummaryItem struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	// Entries are concrete diagnostics in "path + message" form.
	Entries []string `json:"entries,omitempty"`
}

// CompareResult is the structured output of schema comparison.
type CompareResult struct {
	Summary         []SummaryItem `json:"summary"`
	BreakingChanges []string      `json:"breaking_changes"`
	NewResources    []string      `json:"new_resources"`
	NewFunctions    []string      `json:"new_functions"`

	report     internalcompare.Report
	maxChanges int
}
