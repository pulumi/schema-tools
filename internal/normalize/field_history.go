package normalize

import "sort"

type MaxItemsOneTransition string

const (
	MaxItemsOneTransitionChanged   MaxItemsOneTransition = "changed"
	MaxItemsOneTransitionUnchanged MaxItemsOneTransition = "unchanged"
	MaxItemsOneTransitionUnknown   MaxItemsOneTransition = "unknown"
)

type FieldPathEvidence struct {
	Old        *bool
	New        *bool
	Transition MaxItemsOneTransition
}

type FieldHistoryEvidence struct {
	Resources   map[string]map[string]FieldPathEvidence
	Datasources map[string]map[string]FieldPathEvidence
}

func BuildFieldHistoryEvidence(oldMetadata, newMetadata *MetadataEnvelope) FieldHistoryEvidence {
	return FieldHistoryEvidence{
		Resources:   buildTokenFieldEvidence(readHistoryMap(oldMetadata, true), readHistoryMap(newMetadata, true)),
		Datasources: buildTokenFieldEvidence(readHistoryMap(oldMetadata, false), readHistoryMap(newMetadata, false)),
	}
}

func FlattenFieldHistory(fields map[string]*FieldHistory) map[string]*bool {
	if len(fields) == 0 {
		return map[string]*bool{}
	}

	out := map[string]*bool{}
	for _, fieldName := range sortedKeys(fields) {
		flattenFieldHistoryNode(fieldName, fields[fieldName], out)
	}
	return out
}

func ClassifyMaxItemsOneTransition(oldValue, newValue *bool) MaxItemsOneTransition {
	if oldValue == nil || newValue == nil {
		return MaxItemsOneTransitionUnknown
	}
	if *oldValue == *newValue {
		return MaxItemsOneTransitionUnchanged
	}
	return MaxItemsOneTransitionChanged
}

func SortedEvidencePaths(evidence map[string]FieldPathEvidence) []string {
	if len(evidence) == 0 {
		return nil
	}
	paths := make([]string, 0, len(evidence))
	for path := range evidence {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func buildTokenFieldEvidence(oldMap, newMap map[string]*TokenHistory) map[string]map[string]FieldPathEvidence {
	tokens := map[string]struct{}{}
	for token := range oldMap {
		tokens[token] = struct{}{}
	}
	for token := range newMap {
		tokens[token] = struct{}{}
	}

	out := map[string]map[string]FieldPathEvidence{}
	for _, token := range sortedKeys(tokens) {
		oldPaths := flattenTokenFields(oldMap[token])
		newPaths := flattenTokenFields(newMap[token])
		pathSet := map[string]struct{}{}
		for path := range oldPaths {
			pathSet[path] = struct{}{}
		}
		for path := range newPaths {
			pathSet[path] = struct{}{}
		}

		if len(pathSet) == 0 {
			continue
		}

		tokenEvidence := map[string]FieldPathEvidence{}
		for _, path := range sortedKeys(pathSet) {
			oldValue := cloneBoolPtr(oldPaths[path])
			newValue := cloneBoolPtr(newPaths[path])
			tokenEvidence[path] = FieldPathEvidence{
				Old:        oldValue,
				New:        newValue,
				Transition: ClassifyMaxItemsOneTransition(oldValue, newValue),
			}
		}
		out[token] = tokenEvidence
	}

	return out
}

func flattenTokenFields(history *TokenHistory) map[string]*bool {
	if history == nil {
		return map[string]*bool{}
	}
	return FlattenFieldHistory(history.Fields)
}

func flattenFieldHistoryNode(path string, history *FieldHistory, out map[string]*bool) {
	if history == nil {
		return
	}

	out[path] = cloneBoolPtr(history.MaxItemsOne)

	for _, fieldName := range sortedKeys(history.Fields) {
		flattenFieldHistoryNode(path+"."+fieldName, history.Fields[fieldName], out)
	}
	if history.Elem != nil {
		flattenFieldHistoryNode(path+"[*]", history.Elem, out)
	}
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	clonedValue := *v
	return &clonedValue
}
