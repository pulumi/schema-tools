package normalize

import (
	"sort"

	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

type fieldLookupDirection string

const (
	fieldLookupDirectionOldToNew fieldLookupDirection = "old-to-new"
	fieldLookupDirectionNewToOld fieldLookupDirection = "new-to-old"
)

// MaxItemsOneTransition describes old/new maxItemsOne relationship at one field path.
type MaxItemsOneTransition string

const (
	// MaxItemsOneTransitionChanged means old/new are both known and differ.
	MaxItemsOneTransitionChanged MaxItemsOneTransition = "changed"
	// MaxItemsOneTransitionUnchanged means old/new are both known and equal.
	MaxItemsOneTransitionUnchanged MaxItemsOneTransition = "unchanged"
	// MaxItemsOneTransitionUnknown means one side is absent/unspecified.
	MaxItemsOneTransitionUnknown MaxItemsOneTransition = "unknown"
)

// FlattenFieldHistory flattens nested field/elem history into dot paths.
// Example: fields.a.elem.b becomes "a[*].b".
func FlattenFieldHistory(fields map[string]*FieldHistory) map[string]*bool {
	if len(fields) == 0 {
		return map[string]*bool{}
	}

	out := map[string]*bool{}
	for _, fieldName := range sortedMapKeys(fields) {
		flattenFieldHistoryNode(resource.PropertyPath{fieldName}, fields[fieldName], out)
	}
	return out
}

// ClassifyMaxItemsOneTransition compares old/new maxItemsOne values.
// Example: old=true and new=false -> MaxItemsOneTransitionChanged.
func ClassifyMaxItemsOneTransition(oldValue, newValue *bool) MaxItemsOneTransition {
	if oldValue == nil || newValue == nil {
		return MaxItemsOneTransitionUnknown
	}
	if *oldValue == *newValue {
		return MaxItemsOneTransitionUnchanged
	}
	return MaxItemsOneTransitionChanged
}

// ResolveField resolves an old-snapshot field path to new-snapshot evidence.
// Resolution is source-gated: token and field path must exist in source metadata.
//
// Example:
//
//	result := ResolveField(oldMeta, newMeta, "resources", "pkg:index/widget:Widget", "spec")
func ResolveField(oldMetadata, newMetadata *MetadataEnvelope, scope, oldToken, oldField string) FieldLookupResult {
	return resolveField(oldMetadata, newMetadata, fieldLookupDirectionOldToNew, scope, oldToken, oldField)
}

// ResolveNewField resolves a new-snapshot field path to old-snapshot evidence.
// Resolution is source-gated: token and field path must exist in source metadata.
//
// Example:
//
//	result := ResolveNewField(oldMeta, newMeta, "resources", "pkg:index/widgetV2:Widget", "spec")
func ResolveNewField(oldMetadata, newMetadata *MetadataEnvelope, scope, newToken, newField string) FieldLookupResult {
	return resolveField(oldMetadata, newMetadata, fieldLookupDirectionNewToOld, scope, newToken, newField)
}

// resolveField applies direction mapping, source evidence gating, and exact-path
// transition matching to return none/resolved/ambiguous outcomes.
func resolveField(
	oldMetadata, newMetadata *MetadataEnvelope,
	direction fieldLookupDirection,
	scope, token, field string,
) FieldLookupResult {
	if token == "" || field == "" {
		return fieldLookupNoneResult()
	}

	fromMap, toMap := resolveFieldDirectionMaps(oldMetadata, newMetadata, direction, scope)
	if len(fromMap) == 0 {
		return fieldLookupNoneResult()
	}

	sourceMatched := false
	exactMatch := false
	weakTargetMiss := false
	exactTransitions := map[MaxItemsOneTransition]struct{}{}
	fieldCandidates := lookupFieldPathCandidates(field)
	for _, tfToken := range sortedMapKeys(fromMap) {
		fromHistory := fromMap[tfToken]
		if !tokenAppearsInHistory(fromHistory, token) {
			continue
		}

		fromPaths := flattenTokenFields(fromHistory)
		toPaths := flattenTokenFields(toMap[tfToken])
		for _, candidate := range fieldCandidates {
			fromValue, ok := fromPaths[candidate]
			if !ok {
				continue
			}

			sourceMatched = true
			toValue, ok := toPaths[candidate]
			if !ok {
				weakTargetMiss = true
				continue
			}
			transition := ClassifyMaxItemsOneTransition(fromValue, toValue)
			// Singular fallback is intended for cardinality/shape transitions.
			// Keep conservative behavior by ignoring fallback-only unchanged matches.
			if candidate != field && transition == MaxItemsOneTransitionUnchanged {
				continue
			}
			exactMatch = true
			exactTransitions[transition] = struct{}{}
		}
	}

	if !sourceMatched {
		return fieldLookupNoneResult()
	}

	if !exactMatch {
		return fieldLookupNoneResult()
	}

	if weakTargetMiss || len(exactTransitions) != 1 {
		return FieldLookupResult{
			Outcome:    TokenLookupOutcomeAmbiguous,
			Candidates: []string{field},
		}
	}

	for transition := range exactTransitions {
		return FieldLookupResult{
			Outcome:    TokenLookupOutcomeResolved,
			Field:      field,
			Transition: transition,
			Candidates: []string{},
		}
	}
	return fieldLookupNoneResult()
}

// lookupFieldPathCandidates returns deterministic metadata-path lookup candidates
// for a schema field path. Candidates include:
// 1) as-provided
// 2) Terraform-style snake_case path
// 3) singularized final segment variants of 1 and 2
//
// Example:
// input: "eksProperties[*].podProperties[*].containers"
// output order:
// - "eksProperties[\"*\"].podProperties[\"*\"].containers"
// - "eks_properties[\"*\"].pod_properties[\"*\"].containers"
func lookupFieldPathCandidates(path string) []string {
	if path == "" {
		return nil
	}

	normalized := normalizeFieldPath(path)
	candidates := []string{normalized}
	terraform := toTerraformFieldPath(normalized)
	if terraform != normalized {
		candidates = append(candidates, terraform)
	}

	singularized := singularizeFieldPath(normalized)
	if singularized != normalized {
		candidates = append(candidates, singularized)
	}
	singularizedTerraform := singularizeFieldPath(terraform)
	if singularizedTerraform != terraform && singularizedTerraform != singularized {
		candidates = append(candidates, singularizedTerraform)
	}

	return dedupeOrderedStrings(candidates)
}

// toTerraformFieldPath converts each dot-separated segment to Terraform-style
// snake_case while preserving list markers (`[*]`).
//
// Example:
// input:  "eksProperties[*].podProperties[*].containers"
// output: "eks_properties[\"*\"].pod_properties[\"*\"].containers"
func toTerraformFieldPath(path string) string {
	parsed, err := resource.ParsePropertyPath(path)
	if err != nil {
		return path
	}
	converted := make(resource.PropertyPath, len(parsed))
	for i, segment := range parsed {
		switch part := segment.(type) {
		case string:
			if part == "*" {
				converted[i] = part
				continue
			}
			converted[i] = tfbridge.PulumiToTerraformName(part, nil, nil)
		default:
			converted[i] = segment
		}
	}
	return converted.String()
}

// singularizeFieldPath singularizes only the final path segment.
//
// Example:
// input:  "loggings"
// output: "logging"
func singularizeFieldPath(path string) string {
	parsed, err := resource.ParsePropertyPath(path)
	if err != nil {
		return path
	}

	for i := len(parsed) - 1; i >= 0; i-- {
		name, ok := parsed[i].(string)
		if !ok || name == "*" {
			continue
		}
		singular := singularizeName(name)
		if singular == name {
			return path
		}
		updated := make(resource.PropertyPath, len(parsed))
		copy(updated, parsed)
		updated[i] = singular
		return updated.String()
	}
	return path
}

// dedupeOrderedStrings removes duplicates while preserving first-seen order.
//
// Example:
// input:  ["logging", "logging", "loggings"]
// output: ["logging", "loggings"]
func dedupeOrderedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// singularizeName converts one identifier to its singular form using Pulumi's
// inflector rules.
func singularizeName(name string) string {
	if name == "" {
		return ""
	}
	return inflector.Singularize(name)
}

// resolveFieldDirectionMaps returns source/target token-history maps for a
// requested lookup direction and metadata scope.
func resolveFieldDirectionMaps(oldMetadata, newMetadata *MetadataEnvelope, direction fieldLookupDirection, scope string) (map[string]*TokenHistory, map[string]*TokenHistory) {
	switch direction {
	case fieldLookupDirectionOldToNew:
		return readHistoryMap(oldMetadata, scope), readHistoryMap(newMetadata, scope)
	case fieldLookupDirectionNewToOld:
		return readHistoryMap(newMetadata, scope), readHistoryMap(oldMetadata, scope)
	default:
		return nil, nil
	}
}

// flattenTokenFields returns flattened field history for one token history node.
func flattenTokenFields(history *TokenHistory) map[string]*bool {
	if history == nil {
		return map[string]*bool{}
	}
	return FlattenFieldHistory(history.Fields)
}

// flattenFieldHistoryNode recursively flattens one FieldHistory subtree into
// deterministic property-path keys.
func flattenFieldHistoryNode(path resource.PropertyPath, history *FieldHistory, out map[string]*bool) {
	if history == nil {
		return
	}

	out[path.String()] = cloneBoolPtr(history.MaxItemsOne)

	for _, fieldName := range sortedMapKeys(history.Fields) {
		flattenFieldHistoryNode(appendPropertyPath(path, fieldName), history.Fields[fieldName], out)
	}
	if history.Elem != nil {
		flattenFieldHistoryNode(appendPropertyPath(path, "*"), history.Elem, out)
	}
}

func appendPropertyPath(path resource.PropertyPath, segment interface{}) resource.PropertyPath {
	next := make(resource.PropertyPath, len(path)+1)
	copy(next, path)
	next[len(path)] = segment
	return next
}

func normalizeFieldPath(path string) string {
	parsed, err := resource.ParsePropertyPath(path)
	if err != nil {
		return path
	}
	return parsed.String()
}

// fieldLookupNoneResult returns a canonical "none" lookup outcome.
func fieldLookupNoneResult() FieldLookupResult {
	return FieldLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}}
}

// cloneBoolPtr returns a new pointer copy for value-preserving pointer semantics.
func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

// sortedMapKeys returns map keys in stable lexical order.
func sortedMapKeys[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
