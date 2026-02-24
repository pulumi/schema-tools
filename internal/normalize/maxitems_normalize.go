package normalize

import (
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// applyMaxItemsOneNormalization rewrites new-schema field types to old equivalents
// when metadata field evidence shows a true maxItemsOne transition.
func applyMaxItemsOneNormalization(
	oldSchema schema.PackageSpec,
	normalizedNew *schema.PackageSpec,
	context *NormalizationContext,
	oldMetadata, newMetadata *MetadataEnvelope,
) []MaxItemsOneChange {
	if context == nil || normalizedNew == nil {
		return nil
	}

	changes := []MaxItemsOneChange{}
	localTypeRefUseIndex := buildLocalTypeRefUseCountIndex(*normalizedNew)
	resourceIndex := canonicalTFTokenIndex(scopeResources, context.tokenRemap, oldMetadata, newMetadata)
	functionIndex := canonicalTFTokenIndex(scopeDataSources, context.tokenRemap, oldMetadata, newMetadata)

	for token := range normalizedNew.Resources {
		changes = append(changes, applyMaxItemsOneResource(
			oldSchema,
			normalizedNew,
			localTypeRefUseIndex,
			context,
			resourceIndex,
			oldMetadata,
			newMetadata,
			token,
		)...)
	}

	for token := range normalizedNew.Functions {
		changes = append(changes, applyMaxItemsOneFunction(
			oldSchema,
			normalizedNew,
			localTypeRefUseIndex,
			context,
			functionIndex,
			oldMetadata,
			newMetadata,
			token,
		)...)
	}

	return changes
}

func applyMaxItemsOneResource(
	oldSchema schema.PackageSpec,
	normalizedNew *schema.PackageSpec,
	localTypeRefUseIndex map[string]int,
	context *NormalizationContext,
	resourceIndex map[string]string,
	oldMetadata, newMetadata *MetadataEnvelope,
	token string,
) []MaxItemsOneChange {
	oldResource, ok := oldSchema.Resources[token]
	if !ok {
		return nil
	}
	newResource, ok := normalizedNew.Resources[token]
	if !ok {
		return nil
	}

	tfToken, ok := resolveSchemaTokenTFToken(
		scopeResources, token, context.tokenRemap, resourceIndex, oldMetadata, newMetadata,
	)
	if !ok {
		return nil
	}
	evidence := context.fieldEvidence.Resources[tfToken]
	if len(evidence) == 0 {
		return nil
	}

	changes := []MaxItemsOneChange{}
	newResource.InputProperties, changes = normalizePropertyMapByFieldEvidence(
		oldSchema,
		normalizedNew,
		localTypeRefUseIndex,
		scopeResources,
		token,
		"inputs",
		oldResource.InputProperties,
		newResource.InputProperties,
		evidence,
	)
	if currentResource, ok := normalizedNew.Resources[token]; ok {
		newResource.RequiredInputs = currentResource.RequiredInputs
	}

	var propertyChanges []MaxItemsOneChange
	newResource.Properties, propertyChanges = normalizePropertyMapByFieldEvidence(
		oldSchema,
		normalizedNew,
		localTypeRefUseIndex,
		scopeResources,
		token,
		"properties",
		oldResource.Properties,
		newResource.Properties,
		evidence,
	)
	if currentResource, ok := normalizedNew.Resources[token]; ok {
		newResource.Required = currentResource.Required
	}
	changes = append(changes, propertyChanges...)

	normalizedNew.Resources[token] = newResource
	return changes
}

func applyMaxItemsOneFunction(
	oldSchema schema.PackageSpec,
	normalizedNew *schema.PackageSpec,
	localTypeRefUseIndex map[string]int,
	context *NormalizationContext,
	functionIndex map[string]string,
	oldMetadata, newMetadata *MetadataEnvelope,
	token string,
) []MaxItemsOneChange {
	oldFunction, ok := oldSchema.Functions[token]
	if !ok {
		return nil
	}
	newFunction, ok := normalizedNew.Functions[token]
	if !ok {
		return nil
	}

	tfToken, ok := resolveSchemaTokenTFToken(
		scopeDataSources, token, context.tokenRemap, functionIndex, oldMetadata, newMetadata,
	)
	if !ok {
		return nil
	}
	evidence := context.fieldEvidence.Datasources[tfToken]
	if len(evidence) == 0 {
		return nil
	}

	changes := []MaxItemsOneChange{}
	if oldFunction.Inputs != nil && newFunction.Inputs != nil {
		updatedInputs, inputChanges := normalizeFunctionObjectProperties(
			oldSchema,
			normalizedNew,
			localTypeRefUseIndex,
			token,
			"inputs",
			oldFunction.Inputs,
			newFunction.Inputs,
			evidence,
		)
		newFunction.Inputs = updatedInputs
		changes = append(changes, inputChanges...)
	}
	if oldFunction.Outputs != nil && newFunction.Outputs != nil {
		updatedOutputs, outputChanges := normalizeFunctionObjectProperties(
			oldSchema,
			normalizedNew,
			localTypeRefUseIndex,
			token,
			"outputs",
			oldFunction.Outputs,
			newFunction.Outputs,
			evidence,
		)
		newFunction.Outputs = updatedOutputs
		changes = append(changes, outputChanges...)
	}

	normalizedNew.Functions[token] = newFunction
	return changes
}

func normalizeFunctionObjectProperties(
	oldSchema schema.PackageSpec,
	normalizedNew *schema.PackageSpec,
	localTypeRefUseIndex map[string]int,
	token, location string,
	oldObject, newObject *schema.ObjectTypeSpec,
	evidence map[string]FieldPathEvidence,
) (*schema.ObjectTypeSpec, []MaxItemsOneChange) {
	updatedProperties, changes := normalizePropertyMapByFieldEvidence(
		oldSchema,
		normalizedNew,
		localTypeRefUseIndex,
		scopeDataSources,
		token,
		location,
		oldObject.Properties,
		newObject.Properties,
		evidence,
	)

	updatedObject := *newObject
	updatedObject.Properties = updatedProperties
	if currentFunction, ok := normalizedNew.Functions[token]; ok {
		switch location {
		case "inputs":
			if currentFunction.Inputs != nil {
				updatedObject.Required = currentFunction.Inputs.Required
			}
		case "outputs":
			if currentFunction.Outputs != nil {
				updatedObject.Required = currentFunction.Outputs.Required
			}
		}
	}
	return &updatedObject, changes
}

// normalizePropertyMapByFieldEvidence applies maxItemsOne rewrites for one
// property map (resource inputs/properties or function inputs/outputs).
func normalizePropertyMapByFieldEvidence(
	oldSchema schema.PackageSpec,
	normalizedNew *schema.PackageSpec,
	localTypeRefUseIndex map[string]int,
	scope, token, location string,
	oldProps, newProps map[string]schema.PropertySpec,
	evidence map[string]FieldPathEvidence,
) (map[string]schema.PropertySpec, []MaxItemsOneChange) {
	if len(newProps) == 0 || len(evidence) == 0 {
		return newProps, nil
	}

	updated := newProps
	changes := []MaxItemsOneChange{}
	for _, path := range SortedEvidencePaths(evidence) {
		pathEvidence := evidence[path]
		if !shouldNormalizeByFieldEvidence(pathEvidence) {
			continue
		}
		pathParts, ok := parseFieldPath(path)
		if !ok {
			continue
		}

		oldResolvedPath, oldType, ok := resolveTypeSpecAtPath(oldSchema, oldProps, pathParts)
		if !ok {
			continue
		}
		newResolvedPath, newType, ok := resolveTypeSpecAtPath(*normalizedNew, updated, pathParts)
		if !ok {
			continue
		}
		if !isMaxItemsOneTypeChange(&oldType, &newType) {
			continue
		}

		renameFromPath := cloneFieldPathParts(newResolvedPath)
		renameToPath := cloneFieldPathParts(oldResolvedPath)
		if leafPathName(renameFromPath) != leafPathName(renameToPath) {
			fromKey := leafPathName(renameFromPath)
			toKey := leafPathName(renameToPath)
			if renamedProps, renamed := renameResolvedLeafProperty(
				normalizedNew,
				updated,
				renameFromPath,
				renameToPath,
				localTypeRefUseIndex,
			); renamed {
				updated = renamedProps
				if len(renameToPath) == 1 {
					applyTopLevelRequiredRename(normalizedNew, scope, token, location, fromKey, toKey)
				}
			}
		}

		nextProps, ok := setTypeSpecAtPath(
			normalizedNew,
			updated,
			oldResolvedPath,
			oldType,
			localTypeRefUseIndex,
		)
		if !ok {
			continue
		}
		updated = nextProps
		change := MaxItemsOneChange{
			Scope:    scope,
			Token:    token,
			Location: location,
			Field:    fieldPathString(oldResolvedPath),
			OldType:  typeIdentifier(&oldType),
			NewType:  typeIdentifier(&newType),
		}
		newFieldPath := fieldPathString(newResolvedPath)
		if change.Field != "" && newFieldPath != "" && change.Field != newFieldPath {
			change.NewField = newFieldPath
		}
		changes = append(changes, change)
	}

	return updated, changes
}

// shouldNormalizeByFieldEvidence gates rewrites to paths where metadata indicates
// a maxItemsOne transition (or an unknown one-sided legacy entry).
func shouldNormalizeByFieldEvidence(pathEvidence FieldPathEvidence) bool {
	if pathEvidence.Transition == MaxItemsOneTransitionChanged {
		return true
	}

	// Older bridge metadata can omit field entries on one side of history. Treat
	// one-sided unknown values as candidate transitions and let schema type checks
	// decide whether this is a real maxItemsOne array<->single rewrite.
	if pathEvidence.Transition != MaxItemsOneTransitionUnknown {
		return false
	}
	return (pathEvidence.Old == nil) != (pathEvidence.New == nil)
}

// canonicalTFTokenIndex resolves canonical identities back to TF token keys. When
// multiple TF tokens claim the same canonical identity, the canonical is marked
// ambiguous and omitted from normalization.
func canonicalTFTokenIndex(
	scope string,
	remap TokenRemap,
	oldMetadata, newMetadata *MetadataEnvelope,
) map[string]string {
	index := map[string]string{}
	ambiguous := map[string]struct{}{}

	add := func(canonical, tfToken string) {
		if canonical == "" || tfToken == "" {
			return
		}
		if _, conflict := ambiguous[canonical]; conflict {
			return
		}
		if existing, ok := index[canonical]; ok && existing != tfToken {
			delete(index, canonical)
			ambiguous[canonical] = struct{}{}
			return
		}
		index[canonical] = tfToken
	}

	for tfToken, history := range readHistoryMap(oldMetadata, scope == scopeResources) {
		if history == nil || strings.TrimSpace(history.Current) == "" {
			continue
		}
		if canonical, ok := remap.CanonicalForOld(scope, history.Current); ok {
			add(canonical, tfToken)
		}
	}
	for tfToken, history := range readHistoryMap(newMetadata, scope == scopeResources) {
		if history == nil || strings.TrimSpace(history.Current) == "" {
			continue
		}
		if canonical, ok := remap.CanonicalForNew(scope, history.Current); ok {
			add(canonical, tfToken)
		}
	}

	return index
}

// resolveCanonicalTFToken finds the TF token key for a schema token through
// canonical remap identity.
func resolveCanonicalTFToken(
	scope, token string,
	remap TokenRemap,
	index map[string]string,
) (string, bool) {
	canonical, ok := remap.CanonicalForOld(scope, token)
	if !ok {
		canonical, ok = remap.CanonicalForNew(scope, token)
		if !ok {
			return "", false
		}
	}
	tfToken, ok := index[canonical]
	return tfToken, ok
}

// resolveSchemaTokenTFToken resolves schema token -> TF token using canonical
// identity when unambiguous, with a metadata-history fallback for cases where
// multiple TF tokens intentionally share overlapping token aliases.
func resolveSchemaTokenTFToken(
	scope, token string,
	remap TokenRemap,
	index map[string]string,
	oldMetadata, newMetadata *MetadataEnvelope,
) (string, bool) {
	if tfToken, ok := resolveCanonicalTFToken(scope, token, remap, index); ok {
		return tfToken, true
	}

	type ranked struct {
		tfToken string
		rank    int
	}
	candidates := map[string]ranked{}
	addCandidate := func(tfToken string, rank int) {
		if strings.TrimSpace(tfToken) == "" {
			return
		}
		if existing, ok := candidates[tfToken]; ok && existing.rank <= rank {
			return
		}
		candidates[tfToken] = ranked{tfToken: tfToken, rank: rank}
	}

	collect := func(historyByTFToken map[string]*TokenHistory, isNewSnapshot bool) {
		for _, tfToken := range sortedKeys(historyByTFToken) {
			history := historyByTFToken[tfToken]
			if history == nil {
				continue
			}
			current := strings.TrimSpace(history.Current)
			if current == token {
				// Prefer new-current matches over old-current matches.
				if isNewSnapshot {
					addCandidate(tfToken, 1)
				} else {
					addCandidate(tfToken, 2)
				}
				continue
			}
			for _, alias := range history.Past {
				if strings.TrimSpace(alias.Name) != token {
					continue
				}
				if isNewSnapshot {
					addCandidate(tfToken, 3)
				} else {
					addCandidate(tfToken, 4)
				}
				break
			}
		}
	}

	collect(readHistoryMap(newMetadata, scope == scopeResources), true)
	collect(readHistoryMap(oldMetadata, scope == scopeResources), false)

	if len(candidates) == 0 {
		return "", false
	}
	minRank := 999
	for _, candidate := range candidates {
		if candidate.rank < minRank {
			minRank = candidate.rank
		}
	}
	matched := []string{}
	for _, candidate := range candidates {
		if candidate.rank == minRank {
			matched = append(matched, candidate.tfToken)
		}
	}
	if len(matched) != 1 {
		return "", false
	}
	return matched[0], true
}
