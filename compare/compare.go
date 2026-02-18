package compare

import (
	"slices"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

// Schemas computes a structured comparison result for two package specs.
func Schemas(oldSchema, newSchema schema.PackageSpec, opts Options) Result {
	report := internalcompare.Analyze(opts.Provider, oldSchema, newSchema)
	sort.Strings(report.NewResources)
	sort.Strings(report.NewFunctions)

	result := Result{
		Summary:         summarize(report),
		BreakingChanges: splitViolations(report, opts.MaxChanges),
		NewResources:    ensureSlice(slices.Clone(report.NewResources)),
		NewFunctions:    ensureSlice(slices.Clone(report.NewFunctions)),
	}
	return result
}

func splitViolations(report internalcompare.Report, maxChanges int) []string {
	// BreakingChanges currently stores rendered output lines from the internal
	// diagnostic tree. This assumes each displayed violation item is single-line.
	displayed := strings.TrimRight(displayViolations(report, maxChanges), "\n")
	if displayed == "" {
		return []string{}
	}
	lines := strings.Split(displayed, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
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

func summarize(report internalcompare.Report) []SummaryItem {
	counts := map[string]int{}
	entries := map[string][]string{}

	report.Violations.WalkDisplayed(func(node *diagtree.Node) {
		// WalkDisplayed includes displayed branch nodes. Summary items only count
		// concrete diagnostics, which always carry a description.
		if node.Description == "" {
			return
		}
		path := nodePath(node)
		entry := nodeEntry(node)
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
	// NOTE: category matching is intentionally coupled to internal compare
	// diagnostics text (for example "missing input", "has changed to Required").
	// If those strings change in internal/compare, update this mapping.
	// Keep specific "missing" path patterns first so they do not get swallowed
	// by the broader "description == missing" cases below.
	switch {
	case strings.HasPrefix(path, "Resources:") && strings.Contains(path, ": inputs:") && description == "missing":
		return "missing-input"
	case strings.HasPrefix(path, "Types:") && strings.Contains(path, ": properties:") && description == "missing":
		return "missing-property"
	case strings.HasPrefix(description, "missing input"):
		return "missing-input"
	case strings.HasPrefix(description, "missing output"):
		return "missing-output"
	case description == "missing" && strings.HasPrefix(path, "Resources:"):
		return "missing-resource"
	case description == "missing" && strings.HasPrefix(path, "Functions:"):
		return "missing-function"
	case description == "missing" && strings.HasPrefix(path, "Types:"):
		return "missing-type"
	case strings.Contains(description, "type changed") || strings.Contains(description, "had no type") || strings.Contains(description, "now has no type"):
		return "type-changed"
	case strings.Contains(description, "has changed to Required"):
		return "optional-to-required"
	case strings.Contains(description, "is no longer Required"):
		return "required-to-optional"
	case strings.Contains(description, "signature change"):
		return "signature-changed"
	default:
		return "other"
	}
}

func nodePath(node *diagtree.Node) string {
	if node == nil {
		return ""
	}
	return strings.Join(node.PathTitles(), ": ")
}

func nodeEntry(node *diagtree.Node) string {
	if node == nil {
		return ""
	}
	path := nodePath(node)
	if node.Description == "" {
		return path
	}
	if path == "" {
		return node.Description
	}
	return path + " " + node.Description
}

func sortAndUnique(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := slices.Clone(values)
	sort.Strings(out)
	return slices.Compact(out)
}
