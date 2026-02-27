package normalize

import "sort"

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
		flattenFieldHistoryNode(fieldName, fields[fieldName], out)
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
	for _, tfToken := range sortedMapKeys(fromMap) {
		fromHistory := fromMap[tfToken]
		if !tokenAppearsInHistory(fromHistory, token) {
			continue
		}

		fromPaths := flattenTokenFields(fromHistory)
		fromValue, ok := fromPaths[field]
		if !ok {
			continue
		}

		sourceMatched = true
		toPaths := flattenTokenFields(toMap[tfToken])
		toValue, ok := toPaths[field]
		if !ok {
			weakTargetMiss = true
			continue
		}
		exactMatch = true
		exactTransitions[ClassifyMaxItemsOneTransition(fromValue, toValue)] = struct{}{}
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
// deterministic dot-path keys, using "[*]" for elem traversal.
func flattenFieldHistoryNode(path string, history *FieldHistory, out map[string]*bool) {
	if history == nil {
		return
	}

	out[path] = cloneBoolPtr(history.MaxItemsOne)

	for _, fieldName := range sortedMapKeys(history.Fields) {
		flattenFieldHistoryNode(path+"."+fieldName, history.Fields[fieldName], out)
	}
	if history.Elem != nil {
		flattenFieldHistoryNode(path+"[*]", history.Elem, out)
	}
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
