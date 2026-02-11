package compare

import (
	"slices"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

const (
	categoryMissingInput       = "missing-input"
	categoryMissingProperty    = "missing-property"
	categoryMissingOutput      = "missing-output"
	categoryMissingResource    = "missing-resource"
	categoryMissingFunction    = "missing-function"
	categoryMissingType        = "missing-type"
	categoryTypeChanged        = "type-changed"
	categoryMaxItemsOneChanged = "max-items-one-changed"
	categoryOptionalToRequired = "optional-to-required"
	categoryRequiredToOptional = "required-to-optional"
	categorySignatureChanged   = "signature-changed"
	categoryOther              = "other"
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
	// Keep specific "missing" path patterns first so they do not get swallowed
	// by the broader "description == missing" cases below.
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
