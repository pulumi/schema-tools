package compare

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
)

// Schemas computes a structured comparison result for two package specs.
func Schemas(oldSchema, newSchema schema.PackageSpec, opts Options) Result {
	report := internalcompare.Analyze(opts.Provider, oldSchema, newSchema)
	sort.Strings(report.NewResources)
	sort.Strings(report.NewFunctions)
	breakingChanges := sortAndFilterBreaking(report.Changes)
	displayed := selectDisplayedChanges(breakingChanges, opts.MaxChanges)
	structuredChanges := sortStructuredChanges(buildStructuredChanges(displayed))

	result := Result{
		Summary:       summarize(breakingChanges),
		Changes:       ensureChangeSlice(structuredChanges),
		Grouped:       groupStructuredChanges(structuredChanges),
		NewResources:  ensureSlice(slices.Clone(report.NewResources)),
		NewFunctions:  ensureSlice(slices.Clone(report.NewFunctions)),
		totalBreaking: len(breakingChanges),
	}
	return result
}

func selectDisplayedChanges(changes []internalcompare.Change, maxChanges int) []internalcompare.Change {
	if len(changes) == 0 {
		return []internalcompare.Change{}
	}
	if maxChanges == 0 {
		return []internalcompare.Change{}
	}
	displayed := make([]internalcompare.Change, 0, len(changes))
	emittedEntries := 0
	withinLimit := func() bool {
		return maxChanges < 0 || emittedEntries < maxChanges
	}
	for _, category := range collectCategories(changes) {
		if !withinLimit() {
			break
		}
		for _, name := range collectNames(changes, category) {
			if !withinLimit() {
				break
			}
			nameChanges := changesForName(changes, category, name)
			for _, change := range nameChanges {
				if !withinLimit() {
					break
				}
				emittedEntries++
				displayed = append(displayed, change)
			}
		}
	}
	return displayed
}

func ensureSlice(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func summarize(changes []internalcompare.Change) []SummaryItem {
	counts := map[string]int{}
	entries := map[string][]string{}

	for _, change := range changes {
		category := categoryForKind(change.Kind)
		if category == "other" {
			continue
		}
		path := changePath(change)
		entry := nodeEntry(path, change.Description)
		counts[category]++
		if entry != "" {
			entries[category] = append(entries[category], entry)
		}
	}

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

func categoryForKind(kind internalcompare.ChangeKind) string {
	switch kind {
	case internalcompare.ChangeKindMissingResource:
		return "missing-resource"
	case internalcompare.ChangeKindMissingFunction:
		return "missing-function"
	case internalcompare.ChangeKindMissingType:
		return "missing-type"
	case internalcompare.ChangeKindMissingInput:
		return "missing-input"
	case internalcompare.ChangeKindMissingOutput:
		return "missing-output"
	case internalcompare.ChangeKindMissingProperty:
		return "missing-property"
	case internalcompare.ChangeKindTypeChanged:
		return "type-changed"
	case internalcompare.ChangeKindOptionalToRequired:
		return "optional-to-required"
	case internalcompare.ChangeKindRequiredToOptional:
		return "required-to-optional"
	case internalcompare.ChangeKindSignatureChanged:
		return "signature-changed"
	default:
		return "other"
	}
}

func sortAndFilterBreaking(changes []internalcompare.Change) []internalcompare.Change {
	out := make([]internalcompare.Change, 0, len(changes))
	for _, change := range changes {
		if change.Breaking {
			out = append(out, change)
		}
	}
	return out
}

func collectCategories(changes []internalcompare.Change) []string {
	seen := map[string]struct{}{}
	categories := make([]string, 0, len(changes))
	for _, change := range changes {
		if _, ok := seen[change.Category]; ok {
			continue
		}
		seen[change.Category] = struct{}{}
		categories = append(categories, change.Category)
	}
	return categories
}

func collectNames(changes []internalcompare.Change, category string) []string {
	seen := map[string]struct{}{}
	names := []string{}
	for _, change := range changes {
		if change.Category != category {
			continue
		}
		if _, ok := seen[change.Name]; ok {
			continue
		}
		seen[change.Name] = struct{}{}
		names = append(names, change.Name)
	}
	return names
}

func changesForName(changes []internalcompare.Change, category, name string) []internalcompare.Change {
	var out []internalcompare.Change
	for _, change := range changes {
		if change.Category == category && change.Name == name {
			out = append(out, change)
		}
	}
	return out
}

func changePath(change internalcompare.Change) string {
	if change.Category == "" && change.Name == "" && len(change.Path) == 0 {
		return ""
	}
	var b strings.Builder
	if change.Category != "" {
		b.WriteString(change.Category)
	}
	if change.Name != "" {
		if b.Len() > 0 {
			b.WriteString(": ")
		}
		b.WriteString(strconv.Quote(change.Name))
	}
	for _, segment := range change.Path {
		if segment == "" {
			continue
		}
		b.WriteString(": ")
		if isPathLabel(segment) {
			b.WriteString(segment)
		} else {
			b.WriteString(strconv.Quote(segment))
		}
	}
	return b.String()
}

func isPathLabel(segment string) bool {
	switch segment {
	case "inputs", "outputs", "properties", "required", "required inputs", "items", "additional properties":
		return true
	}
	return false
}

func nodeEntry(path string, description string) string {
	if description == "" {
		return path
	}
	if path == "" {
		return description
	}
	return path + " " + description
}

func ensureChangeSlice(changes []Change) []Change {
	if changes == nil {
		return []Change{}
	}
	return changes
}

func buildStructuredChanges(changes []internalcompare.Change) []Change {
	if len(changes) == 0 {
		return []Change{}
	}
	out := make([]Change, 0, len(changes))
	for _, change := range changes {
		out = append(out, Change{
			Scope:    scopeFromCategory(change.Category),
			Token:    change.Name,
			Location: locationFromPath(change.Path),
			Path:     changePath(change),
			Kind:     string(change.Kind),
			Severity: severityFromInternal(change.Severity),
			Breaking: change.Breaking,
			Message:  change.Description,
		})
	}
	return out
}

func scopeFromCategory(category string) ChangeScope {
	switch category {
	case "Resources":
		return ScopeResource
	case "Functions":
		return ScopeFunction
	case "Types":
		return ScopeType
	}
	panic(fmt.Sprintf("unsupported internal compare category %q", category))
}

func locationFromPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[0]
}

func severityFromInternal(severity internalcompare.Severity) ChangeSeverity {
	switch severity {
	case internalcompare.SeverityDanger:
		return SeverityError
	case internalcompare.SeverityWarn:
		return SeverityWarn
	case internalcompare.SeverityInfo:
		return SeverityInfo
	}
	panic(fmt.Sprintf("unsupported internal compare severity %q", severity))
}

func sortStructuredChanges(changes []Change) []Change {
	if len(changes) == 0 {
		return []Change{}
	}
	out := slices.Clone(changes)
	slices.SortFunc(out, func(a, b Change) int {
		if d := cmpInt(scopeSortOrder(a.Scope), scopeSortOrder(b.Scope)); d != 0 {
			return d
		}
		if d := strings.Compare(a.Token, b.Token); d != 0 {
			return d
		}
		if d := strings.Compare(a.Location, b.Location); d != 0 {
			return d
		}
		if d := strings.Compare(a.Path, b.Path); d != 0 {
			return d
		}
		if d := strings.Compare(a.Kind, b.Kind); d != 0 {
			return d
		}
		if d := strings.Compare(a.Message, b.Message); d != 0 {
			return d
		}
		return strings.Compare(string(a.Severity), string(b.Severity))
	})
	return out
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func scopeSortOrder(scope ChangeScope) int {
	switch scope {
	case ScopeResource:
		return 0
	case ScopeFunction:
		return 1
	case ScopeType:
		return 2
	default:
		return 3
	}
}

func groupStructuredChanges(changes []Change) GroupedChanges {
	grouped := GroupedChanges{
		Resources: map[string]map[string][]Change{},
		Functions: map[string]map[string][]Change{},
		Types:     map[string]map[string][]Change{},
	}
	for _, change := range changes {
		location := change.Location
		if location == "" {
			location = "general"
		}
		switch change.Scope {
		case ScopeResource:
			appendGrouped(grouped.Resources, change.Token, location, change)
		case ScopeFunction:
			appendGrouped(grouped.Functions, change.Token, location, change)
		case ScopeType:
			appendGrouped(grouped.Types, change.Token, location, change)
		}
	}
	return grouped
}

func appendGrouped(group map[string]map[string][]Change, token, location string, change Change) {
	if token == "" {
		return
	}
	if group[token] == nil {
		group[token] = map[string][]Change{}
	}
	group[token][location] = append(group[token][location], change)
}

func sortAndUnique(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := slices.Clone(values)
	// slices.Compact only removes adjacent duplicates, so sorting is required.
	sort.Strings(out)
	return slices.Compact(out)
}
