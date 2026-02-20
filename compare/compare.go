package compare

import (
	"cmp"
	"fmt"
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
	changes := sortChanges(changesFromDiagnostics(report.Diagnostics))
	changes = attachTypeImpactMetadata(changes, oldSchema, newSchema)

	result := Result{
		Summary:      summarize(changes),
		Changes:      ensureChangeSlice(changes),
		Grouped:      groupChanges(changes),
		NewResources: ensureSlice(slices.Clone(report.NewResources)),
		NewFunctions: ensureSlice(slices.Clone(report.NewFunctions)),
	}
	return result
}

// MergeChanges appends additional change records and recomputes derived views.
func MergeChanges(result Result, additional []Change) Result {
	if len(additional) == 0 {
		return result
	}
	merged := make([]Change, 0, len(result.Changes)+len(additional))
	merged = append(merged, result.Changes...)
	merged = append(merged, additional...)
	merged = sortChanges(merged)
	result.Changes = ensureChangeSlice(merged)
	result.Grouped = groupChanges(merged)
	result.Summary = summarize(merged)
	return result
}

func attachTypeImpactMetadata(changes []Change, oldSchema, newSchema schema.PackageSpec) []Change {
	if len(changes) == 0 {
		return []Change{}
	}

	impactIndex := buildTypeImpactIndex(oldSchema, newSchema)
	if len(impactIndex) == 0 {
		return changes
	}

	out := slices.Clone(changes)
	for i := range out {
		if out[i].Scope != ScopeType || out[i].Token == "" {
			continue
		}
		impacts := filterTypeImpactsForChange(out[i].Token, impactIndex[out[i].Token])
		if len(impacts) == 0 {
			continue
		}
		out[i].ImpactedBy = impacts
		out[i].ImpactCount = len(impacts)
	}
	return out
}

func filterTypeImpactsForChange(typeToken string, impacts []ImpactRef) []ImpactRef {
	if len(impacts) == 0 {
		return nil
	}
	out := make([]ImpactRef, 0, len(impacts))
	for _, impact := range impacts {
		if impact.Scope == ScopeType && impact.Token == typeToken {
			// Self-references are not useful in impact metadata.
			continue
		}
		out = append(out, impact)
	}
	slices.SortFunc(out, func(a, b ImpactRef) int {
		if d := cmp.Compare(scopeSortOrder(a.Scope), scopeSortOrder(b.Scope)); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Token, b.Token); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Location, b.Location); d != 0 {
			return d
		}
		return cmp.Compare(a.Path, b.Path)
	})
	return out
}

func buildTypeImpactIndex(oldSchema, newSchema schema.PackageSpec) map[string][]ImpactRef {
	index := map[string][]ImpactRef{}
	seen := map[string]map[string]struct{}{}

	collectTypeImpactRefs(oldSchema, index, seen)
	collectTypeImpactRefs(newSchema, index, seen)

	return index
}

func collectTypeImpactRefs(
	spec schema.PackageSpec,
	index map[string][]ImpactRef,
	seen map[string]map[string]struct{},
) {
	for _, token := range sortedMapKeys(spec.Resources) {
		resource := spec.Resources[token]
		collectTypeImpactRefsInPropertyMap(ScopeResource, token, "inputs", resource.InputProperties, index, seen)
		collectTypeImpactRefsInPropertyMap(ScopeResource, token, "properties", resource.Properties, index, seen)
	}

	for _, token := range sortedMapKeys(spec.Functions) {
		function := spec.Functions[token]
		if function.Inputs != nil {
			collectTypeImpactRefsInPropertyMap(ScopeFunction, token, "inputs", function.Inputs.Properties, index, seen)
		}
		if function.Outputs != nil {
			collectTypeImpactRefsInPropertyMap(ScopeFunction, token, "outputs", function.Outputs.Properties, index, seen)
		}
	}

	for _, token := range sortedMapKeys(spec.Types) {
		typ := spec.Types[token]
		collectTypeImpactRefsInPropertyMap(ScopeType, token, "properties", typ.Properties, index, seen)
	}
}

func collectTypeImpactRefsInPropertyMap(
	scope ChangeScope,
	token string,
	location string,
	properties map[string]schema.PropertySpec,
	index map[string][]ImpactRef,
	seen map[string]map[string]struct{},
) {
	for _, propertyName := range sortedMapKeys(properties) {
		property := properties[propertyName]
		collectTypeImpactRefsInTypeSpec(
			scope,
			token,
			location,
			propertyName,
			property.TypeSpec,
			index,
			seen,
		)
	}
}

func collectTypeImpactRefsInTypeSpec(
	scope ChangeScope,
	token string,
	location string,
	path string,
	typeSpec schema.TypeSpec,
	index map[string][]ImpactRef,
	seen map[string]map[string]struct{},
) {
	if referencedTypeToken, ok := extractLocalTypeToken(typeSpec.Ref); ok {
		addTypeImpactRef(index, seen, referencedTypeToken, ImpactRef{
			Scope:    scope,
			Token:    token,
			Location: location,
			Path:     path,
		})
	}

	if typeSpec.Items != nil {
		collectTypeImpactRefsInTypeSpec(
			scope,
			token,
			location,
			path+"[*]",
			*typeSpec.Items,
			index,
			seen,
		)
	}
	if typeSpec.AdditionalProperties != nil {
		collectTypeImpactRefsInTypeSpec(
			scope,
			token,
			location,
			path+"{}",
			*typeSpec.AdditionalProperties,
			index,
			seen,
		)
	}
	for i, oneOfType := range typeSpec.OneOf {
		collectTypeImpactRefsInTypeSpec(
			scope,
			token,
			location,
			fmt.Sprintf("%s|oneOf[%d]", path, i),
			oneOfType,
			index,
			seen,
		)
	}
}

func extractLocalTypeToken(ref string) (string, bool) {
	const marker = "#/types/"
	idx := strings.Index(ref, marker)
	if idx == -1 {
		return "", false
	}
	token := strings.TrimSpace(ref[idx+len(marker):])
	if token == "" {
		return "", false
	}
	return token, true
}

func addTypeImpactRef(
	index map[string][]ImpactRef,
	seen map[string]map[string]struct{},
	typeToken string,
	impact ImpactRef,
) {
	if typeToken == "" || impact.Token == "" {
		return
	}
	if seen[typeToken] == nil {
		seen[typeToken] = map[string]struct{}{}
	}
	key := impactRefKey(impact)
	if _, ok := seen[typeToken][key]; ok {
		return
	}
	seen[typeToken][key] = struct{}{}
	index[typeToken] = append(index[typeToken], impact)
}

func impactRefKey(impact ImpactRef) string {
	return string(impact.Scope) + "|" + impact.Token + "|" + impact.Location + "|" + impact.Path
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type classifier struct {
	severity ChangeSeverity
	breaking bool
}

var classifiers = map[string]classifier{
	"missing-resource":          {severity: SeverityError, breaking: true},
	"missing-function":          {severity: SeverityError, breaking: true},
	"missing-type":              {severity: SeverityError, breaking: true},
	"signature-changed":         {severity: SeverityError, breaking: true},
	"optional-to-required":      {severity: SeverityError, breaking: true},
	"type-changed":              {severity: SeverityWarn, breaking: true},
	"missing-input":             {severity: SeverityWarn, breaking: true},
	"missing-output":            {severity: SeverityWarn, breaking: true},
	"missing-property":          {severity: SeverityWarn, breaking: true},
	"max-items-one-changed":     {severity: SeverityError, breaking: true},
	"renamed-resource":          {severity: SeverityError, breaking: true},
	"renamed-function":          {severity: SeverityError, breaking: true},
	"required-to-optional":      {severity: SeverityInfo, breaking: false},
	"deprecated-resource-alias": {severity: SeverityInfo, breaking: false},
	"deprecated-function-alias": {severity: SeverityInfo, breaking: false},
}

func classifySeverity(kind string) (ChangeSeverity, bool) {
	c, ok := classifiers[kind]
	if ok {
		return c.severity, c.breaking
	}
	return SeverityWarn, true
}

func ensureSlice(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func ensureChangeSlice(changes []Change) []Change {
	if changes == nil {
		return []Change{}
	}
	return changes
}

func summarize(changes []Change) []SummaryItem {
	counts := map[string]int{}
	entries := map[string][]string{}

	for _, change := range changes {
		counts[change.Kind]++
		entry := nodeEntry(change.Path, change.Message)
		if entry != "" {
			entries[change.Kind] = append(entries[change.Kind], entry)
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

func changesFromDiagnostics(diagnostics []internalcompare.Diagnostic) []Change {
	if len(diagnostics) == 0 {
		return []Change{}
	}
	changes := make([]Change, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		kind := classify(diagnostic.Path, diagnostic.Description)
		severity, breaking := classifySeverity(kind)
		changes = append(changes, Change{
			Scope:    scopeFromDiagnostic(diagnostic.Scope),
			Token:    diagnostic.Token,
			Location: diagnostic.Location,
			Path:     diagnostic.Path,
			Kind:     kind,
			Severity: severity,
			Breaking: breaking,
			Source:   SourceEngine,
			Message:  diagnostic.Description,
		})
	}
	return changes
}

func scopeFromDiagnostic(scope string) ChangeScope {
	switch scope {
	case "Resources":
		return ScopeResource
	case "Functions":
		return ScopeFunction
	case "Types":
		return ScopeType
	default:
		return ScopeUnknown
	}
}

func sortChanges(changes []Change) []Change {
	if len(changes) == 0 {
		return []Change{}
	}
	out := slices.Clone(changes)
	slices.SortFunc(out, func(a, b Change) int {
		if d := cmp.Compare(scopeSortOrder(a.Scope), scopeSortOrder(b.Scope)); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Token, b.Token); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Location, b.Location); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Path, b.Path); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Kind, b.Kind); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Message, b.Message); d != 0 {
			return d
		}
		return cmp.Compare(a.Source, b.Source)
	})
	return out
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

func groupChanges(changes []Change) GroupedChanges {
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

func appendGrouped(
	group map[string]map[string][]Change,
	token string,
	location string,
	change Change,
) {
	if group[token] == nil {
		group[token] = map[string][]Change{}
	}
	group[token][location] = append(group[token][location], change)
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

func nodeEntry(path string, description string) string {
	if description == "" {
		return path
	}
	if path == "" {
		return description
	}
	return path + " " + description
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
