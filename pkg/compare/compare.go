package compare

import (
	"io"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

// Compare computes a structured comparison result for two package specs.
func Compare(oldSchema, newSchema schema.PackageSpec, opts CompareOptions) CompareResult {
	report := analyzeAndSort(opts.Provider, oldSchema, newSchema)
	result := CompareResult{
		Summary:         summarize(report),
		BreakingChanges: splitViolations(report, opts.MaxChanges),
		NewResources:    cloneOrEmpty(report.NewResources),
		NewFunctions:    cloneOrEmpty(report.NewFunctions),
		report:          report,
		maxChanges:      opts.MaxChanges,
	}
	return result
}

// CompareForText computes just the data needed for RenderText output.
func CompareForText(oldSchema, newSchema schema.PackageSpec, opts CompareOptions) CompareResult {
	report := analyzeAndSort(opts.Provider, oldSchema, newSchema)
	return CompareResult{
		NewResources: cloneOrEmpty(report.NewResources),
		NewFunctions: cloneOrEmpty(report.NewFunctions),
		report:       report,
		maxChanges:   opts.MaxChanges,
	}
}

// RenderText writes the current human-readable compare output.
func RenderText(out io.Writer, result CompareResult) {
	internalcompare.RenderText(out, result.report, result.maxChanges)
}

func analyzeAndSort(provider string, oldSchema, newSchema schema.PackageSpec) internalcompare.Report {
	report := internalcompare.Analyze(provider, oldSchema, newSchema)
	sort.Strings(report.NewResources)
	sort.Strings(report.NewFunctions)
	return report
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

func summarize(report internalcompare.Report) []SummaryItem {
	counts := map[string]int{}
	entries := map[string][]string{}

	report.Violations.WalkDisplayed(func(node *diagtree.Node) {
		if node.Description == "" {
			return
		}
		path := internalcompare.NodePath(node)
		entry := internalcompare.NodeEntry(node)
		category := classify(path, node.Description)
		counts[category]++
		if entry != "" {
			entries[category] = append(entries[category], entry)
		}
	})

	if len(counts) == 0 {
		return []SummaryItem{}
	}

	categories := make([]string, 0, len(counts))
	for category := range counts {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	summary := make([]SummaryItem, 0, len(categories))
	for _, category := range categories {
		summary = append(summary, SummaryItem{
			Category: category,
			Count:    counts[category],
			Entries:  sortAndUnique(entries[category]),
		})
	}

	return summary
}

func classify(path string, description string) string {
	switch {
	case strings.HasPrefix(path, "Resources:") && strings.Contains(path, ": inputs:") && description == "missing":
		return categoryMissingInput
	case strings.HasPrefix(path, "Types:") && strings.Contains(path, ": properties:") && description == "missing":
		return categoryMissingProperty
	case strings.HasPrefix(description, "missing input"):
		return categoryMissingInput
	case strings.HasPrefix(description, "missing output"):
		return categoryMissingOutput
	case description == "missing" && strings.HasPrefix(path, "Resources:"):
		return categoryMissingResource
	case description == "missing" && strings.HasPrefix(path, "Functions:"):
		return categoryMissingFunction
	case description == "missing" && strings.HasPrefix(path, "Types:"):
		return categoryMissingType
	case strings.Contains(description, "max-items-one"):
		return categoryMaxItemsOneChanged
	case strings.Contains(description, "type changed") || strings.Contains(description, "had no type") || strings.Contains(description, "now has no type"):
		return categoryTypeChanged
	case strings.Contains(description, "has changed to Required"):
		return categoryOptionalToRequired
	case strings.Contains(description, "is no longer Required"):
		return categoryRequiredToOptional
	case strings.Contains(description, "signature change"):
		return categorySignatureChanged
	default:
		return categoryOther
	}
}

func sortAndUnique(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
