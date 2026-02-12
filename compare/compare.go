package compare

import (
	"slices"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
)

// Schemas computes a structured comparison result for two package specs.
func Schemas(oldSchema, newSchema schema.PackageSpec, opts Options) Result {
	report := internalcompare.Analyze(opts.Provider, oldSchema, newSchema)
	sort.Strings(report.NewResources)
	sort.Strings(report.NewFunctions)

	result := Result{
		Summary:         []SummaryItem{},
		BreakingChanges: splitViolations(report, opts.MaxChanges),
		NewResources:    ensureSlice(slices.Clone(report.NewResources)),
		NewFunctions:    ensureSlice(slices.Clone(report.NewFunctions)),
	}
	return result
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

func ensureSlice(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
