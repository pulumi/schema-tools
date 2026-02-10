package compare

import (
	"io"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
)

// Compare computes a structured comparison result for two package specs.
func Compare(oldSchema, newSchema schema.PackageSpec, opts CompareOptions) CompareResult {
	report := internalcompare.Analyze(opts.Provider, oldSchema, newSchema)
	sort.Strings(report.NewResources)
	sort.Strings(report.NewFunctions)

	result := CompareResult{
		Summary:         []SummaryItem{},
		BreakingChanges: splitViolations(report, opts.MaxChanges),
		NewResources:    cloneOrEmpty(report.NewResources),
		NewFunctions:    cloneOrEmpty(report.NewFunctions),
		report:          report,
		maxChanges:      opts.MaxChanges,
	}
	return result
}

// RenderText writes the current human-readable compare output.
func RenderText(out io.Writer, result CompareResult) {
	internalcompare.RenderText(out, result.report, result.maxChanges)
}

func splitViolations(report internalcompare.Report, maxChanges int) []string {
	displayed := strings.TrimRight(displayViolations(report, maxChanges), "\n")
	if displayed == "" {
		return []string{}
	}
	return strings.Split(displayed, "\n")
}

func displayViolations(report internalcompare.Report, maxChanges int) string {
	out := &strings.Builder{}
	report.Violations.Display(out, maxChanges)
	return out.String()
}

func cloneOrEmpty(xs []string) []string {
	if len(xs) == 0 {
		return []string{}
	}
	clone := make([]string, len(xs))
	copy(clone, xs)
	return clone
}
