package normalize

import "strings"

// ResolveEquivalentTypeChange resolves field evidence (old->new) and classifies
// whether the provided source/target types are maxItems-like equivalents.
//
// Example:
//
//	result := ResolveEquivalentTypeChange(oldMeta, newMeta, "resources", token, "spec", "array<string>", "string")
func ResolveEquivalentTypeChange(
	oldMetadata, newMetadata *MetadataEnvelope,
	scope, oldToken, oldField, oldType, newType string,
) EquivalentTypeChangeResult {
	return resolveEquivalentTypeChange(
		ResolveField(oldMetadata, newMetadata, scope, oldToken, oldField),
		oldType,
		newType,
	)
}

// ResolveNewEquivalentTypeChange resolves field evidence (new->old) and classifies
// whether the provided source/target types are maxItems-like equivalents.
//
// Example:
//
//	result := ResolveNewEquivalentTypeChange(oldMeta, newMeta, "resources", token, "spec", "string", "array<string>")
func ResolveNewEquivalentTypeChange(
	oldMetadata, newMetadata *MetadataEnvelope,
	scope, newToken, newField, newType, oldType string,
) EquivalentTypeChangeResult {
	return resolveEquivalentTypeChange(
		ResolveNewField(oldMetadata, newMetadata, scope, newToken, newField),
		newType,
		oldType,
	)
}

// resolveEquivalentTypeChange applies field lookup outcome and transition evidence
// to compute a conservative equivalence decision for type changes.
func resolveEquivalentTypeChange(fieldResult FieldLookupResult, sourceType, targetType string) EquivalentTypeChangeResult {
	result := EquivalentTypeChangeResult{
		Outcome:    fieldResult.Outcome,
		Field:      fieldResult.Field,
		Candidates: append([]string{}, fieldResult.Candidates...),
	}

	if fieldResult.Outcome != TokenLookupOutcomeResolved {
		return result
	}

	if fieldResult.Transition == MaxItemsOneTransitionChanged {
		result.Equivalent = maxItemsLikeTypeEquivalent(sourceType, targetType)
	}
	return result
}

// maxItemsLikeTypeEquivalent returns true when source/target differ only in
// array-vs-single cardinality for the same base type.
func maxItemsLikeTypeEquivalent(sourceType, targetType string) bool {
	sourceBase, sourceArray, sourceOK := parseTypeCardinality(sourceType)
	targetBase, targetArray, targetOK := parseTypeCardinality(targetType)
	if !sourceOK || !targetOK {
		return false
	}

	return sourceBase == targetBase && sourceArray != targetArray
}

// parseTypeCardinality parses normalized type text into a base type and whether
// it is represented as an array-like form.
// Supported array forms: `array<T>` and `T[]`.
func parseTypeCardinality(raw string) (base string, isArray bool, ok bool) {
	typeName := strings.TrimSpace(raw)
	if typeName == "" {
		return "", false, false
	}

	if strings.HasPrefix(typeName, "array<") && strings.HasSuffix(typeName, ">") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(typeName, "array<"), ">"))
		if inner == "" {
			return "", false, false
		}
		return inner, true, true
	}

	if strings.HasSuffix(typeName, "[]") {
		inner := strings.TrimSpace(strings.TrimSuffix(typeName, "[]"))
		if inner == "" {
			return "", false, false
		}
		return inner, true, true
	}

	return typeName, false, true
}
