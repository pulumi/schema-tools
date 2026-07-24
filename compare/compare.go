package compare

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
	schematoolsversion "github.com/pulumi/schema-tools/version"
)

// Schemas computes a structured comparison result for two package specs.
func Schemas(oldSchema, newSchema schema.PackageSpec, opts Options) Result {
	report := internalcompare.Analyze(opts.Provider, oldSchema, newSchema, opts.OldMetadata, opts.NewMetadata)
	sort.Strings(report.NewResources)
	sort.Strings(report.NewFunctions)
	breakingChanges := sortAndFilterBreaking(report.Changes)
	remapChanges := sortAndFilterTokenRemaps(report.Changes)
	displayed := selectDisplayedChanges(breakingChanges, opts.MaxChanges)
	publicChanges := sortAndMergeChanges(breakingChanges, remapChanges)
	structuredChanges := sortStructuredChanges(buildStructuredChanges(publicChanges))
	displayedStructured := sortStructuredChanges(buildStructuredChanges(displayed))
	summaryChanges := sortAndMergeChanges(breakingChanges, remapChanges)

	result := Result{
		SchemaToolsVersion: schematoolsversion.Version,
		Summary:            summarize(summaryChanges),
		Changes:            ensureChangeSlice(structuredChanges),
		Grouped:            groupStructuredChanges(structuredChanges),
		NewResources:       ensureSlice(slices.Clone(report.NewResources)),
		NewFunctions:       ensureSlice(slices.Clone(report.NewFunctions)),
		NewTypeCount:       report.NewTypeCount,
		totalBreaking:      len(breakingChanges),
		displayed:          ensureChangeSlice(displayedStructured),
	}
	return result
}

func selectDisplayedChanges(changes []internalcompare.Change, maxChanges int) []internalcompare.Change {
	if len(changes) == 0 || maxChanges == 0 {
		return []internalcompare.Change{}
	}
	if maxChanges < 0 || maxChanges >= len(changes) {
		return slices.Clone(changes)
	}
	return slices.Clone(changes[:maxChanges])
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
	case internalcompare.ChangeKindTokenRemapped:
		return "token-remapped"
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
	slices.SortFunc(out, compareInternalChanges)
	return out
}

func sortAndFilterTokenRemaps(changes []internalcompare.Change) []internalcompare.Change {
	out := make([]internalcompare.Change, 0, len(changes))
	for _, change := range changes {
		if change.Kind == internalcompare.ChangeKindTokenRemapped && !change.Breaking {
			out = append(out, change)
		}
	}
	slices.SortFunc(out, compareInternalChanges)
	return out
}

func sortAndMergeChanges(first, second []internalcompare.Change) []internalcompare.Change {
	if len(first) == 0 && len(second) == 0 {
		return []internalcompare.Change{}
	}
	merged := make([]internalcompare.Change, 0, len(first)+len(second))
	merged = append(merged, first...)
	merged = append(merged, second...)
	slices.SortFunc(merged, compareInternalChanges)
	return merged
}

func compareInternalChanges(a, b internalcompare.Change) int {
	if d := cmpInt(internalSeveritySortOrder(a.Severity), internalSeveritySortOrder(b.Severity)); d != 0 {
		return d
	}
	if d := strings.Compare(a.Category, b.Category); d != 0 {
		return d
	}
	if d := strings.Compare(a.Name, b.Name); d != 0 {
		return d
	}
	if d := strings.Compare(strings.Join(a.Path, "\x00"), strings.Join(b.Path, "\x00")); d != 0 {
		return d
	}
	if d := strings.Compare(string(a.Kind), string(b.Kind)); d != 0 {
		return d
	}
	if d := strings.Compare(string(a.Severity), string(b.Severity)); d != 0 {
		return d
	}
	if d := strings.Compare(a.Description, b.Description); d != 0 {
		return d
	}
	return strings.Compare(reasonSortKey(a.Reason), reasonSortKey(b.Reason))
}

func reasonSortKey(reason *internalcompare.NormalizationReason) string {
	if reason == nil {
		return ""
	}
	return string(reason.Outcome) + "\x00" + reason.Lookup + "\x00" + reason.Token + "\x00" + strings.Join(reason.Candidates, "\x00")
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
		if d := cmpInt(severitySortOrder(a.Severity), severitySortOrder(b.Severity)); d != 0 {
			return d
		}
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

func internalSeveritySortOrder(severity internalcompare.Severity) int {
	switch severity {
	case internalcompare.SeverityDanger:
		return 0
	case internalcompare.SeverityWarn:
		return 1
	case internalcompare.SeverityInfo:
		return 2
	default:
		return 3
	}
}

func severitySortOrder(severity ChangeSeverity) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarn:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
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
