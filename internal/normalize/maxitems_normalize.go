package normalize

import (
	"reflect"
	"strings"

	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// fieldPathPart represents one segment of a flattened field-history path.
type fieldPathPart struct {
	Name string
	Elem bool
}

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

	for token, newResource := range normalizedNew.Resources {
		oldResource, ok := oldSchema.Resources[token]
		if !ok {
			continue
		}
		tfToken, ok := resolveSchemaTokenTFToken(
			scopeResources, token, context.tokenRemap, resourceIndex, oldMetadata, newMetadata,
		)
		if !ok {
			continue
		}
		evidence := context.fieldEvidence.Resources[tfToken]
		if len(evidence) == 0 {
			continue
		}

		var resourceChanges []MaxItemsOneChange
		newResource.InputProperties, resourceChanges = normalizePropertyMapByFieldEvidence(
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
		changes = append(changes, resourceChanges...)

		newResource.Properties, resourceChanges = normalizePropertyMapByFieldEvidence(
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
		changes = append(changes, resourceChanges...)

		normalizedNew.Resources[token] = newResource
	}

	for token, newFunction := range normalizedNew.Functions {
		oldFunction, ok := oldSchema.Functions[token]
		if !ok {
			continue
		}
		tfToken, ok := resolveSchemaTokenTFToken(
			scopeDataSources, token, context.tokenRemap, functionIndex, oldMetadata, newMetadata,
		)
		if !ok {
			continue
		}
		evidence := context.fieldEvidence.Datasources[tfToken]
		if len(evidence) == 0 {
			continue
		}

		if oldFunction.Inputs != nil && newFunction.Inputs != nil {
			updated, functionChanges := normalizePropertyMapByFieldEvidence(
				oldSchema,
				normalizedNew,
				localTypeRefUseIndex,
				scopeDataSources,
				token,
				"inputs",
				oldFunction.Inputs.Properties,
				newFunction.Inputs.Properties,
				evidence,
			)
			inputs := *newFunction.Inputs
			inputs.Properties = updated
			newFunction.Inputs = &inputs
			changes = append(changes, functionChanges...)
		}

		if oldFunction.Outputs != nil && newFunction.Outputs != nil {
			updated, functionChanges := normalizePropertyMapByFieldEvidence(
				oldSchema,
				normalizedNew,
				localTypeRefUseIndex,
				scopeDataSources,
				token,
				"outputs",
				oldFunction.Outputs.Properties,
				newFunction.Outputs.Properties,
				evidence,
			)
			outputs := *newFunction.Outputs
			outputs.Properties = updated
			newFunction.Outputs = &outputs
			changes = append(changes, functionChanges...)
		}

		normalizedNew.Functions[token] = newFunction
	}

	return changes
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
			if renamedProps, renamed := renameResolvedLeafProperty(
				normalizedNew,
				updated,
				renameFromPath,
				renameToPath,
				localTypeRefUseIndex,
			); renamed {
				updated = renamedProps
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

// parseFieldPath parses flattened field-history paths.
// Example: "settings[*].name" => [{Name:"settings",Elem:true}, {Name:"name",Elem:false}]
func parseFieldPath(path string) ([]fieldPathPart, bool) {
	if strings.TrimSpace(path) == "" {
		return nil, false
	}

	raw := strings.Split(path, ".")
	parts := make([]fieldPathPart, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		elem := false
		if strings.HasSuffix(part, "[*]") {
			elem = true
			part = strings.TrimSuffix(part, "[*]")
		}
		if part == "" || strings.Contains(part, "[*]") {
			return nil, false
		}
		parts = append(parts, fieldPathPart{Name: part, Elem: elem})
	}
	return parts, true
}

// cloneFieldPathParts returns a copy of parsed path segments.
func cloneFieldPathParts(parts []fieldPathPart) []fieldPathPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]fieldPathPart, len(parts))
	copy(out, parts)
	return out
}

// leafPathName returns the terminal field name from a parsed path.
func leafPathName(parts []fieldPathPart) string {
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1].Name
}

// fieldPathString renders parsed path segments back into flattened form.
func fieldPathString(parts []fieldPathPart) string {
	if len(parts) == 0 {
		return ""
	}
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		segment := part.Name
		if part.Elem {
			segment += "[*]"
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, ".")
}

// resolveTypeSpecAtPath resolves a flattened field path through a property map
// and returns both resolved Pulumi property names and the final TypeSpec.
func resolveTypeSpecAtPath(
	pkg schema.PackageSpec,
	props map[string]schema.PropertySpec,
	path []fieldPathPart,
) ([]fieldPathPart, schema.TypeSpec, bool) {
	if len(path) == 0 {
		return nil, schema.TypeSpec{}, false
	}

	currentProps := props
	resolved := make([]fieldPathPart, 0, len(path))
	for i, part := range path {
		propKey, ok := resolvePropertyName(currentProps, part.Name)
		if !ok {
			return nil, schema.TypeSpec{}, false
		}

		resolvedPart := fieldPathPart{Name: propKey, Elem: part.Elem}
		resolved = append(resolved, resolvedPart)

		prop := currentProps[propKey]
		currentType := prop.TypeSpec
		if part.Elem {
			if currentType.Type != "array" || currentType.Items == nil {
				return nil, schema.TypeSpec{}, false
			}
			currentType = *currentType.Items
		}
		if i == len(path)-1 {
			return resolved, currentType, true
		}

		nextProps, ok := objectPropertiesForTypeSpec(pkg, currentType)
		if !ok {
			return nil, schema.TypeSpec{}, false
		}
		currentProps = nextProps
	}

	return nil, schema.TypeSpec{}, false
}

// lookupTypeSpecAtPath resolves and returns only the terminal TypeSpec.
func lookupTypeSpecAtPath(
	pkg schema.PackageSpec,
	props map[string]schema.PropertySpec,
	path []fieldPathPart,
) (schema.TypeSpec, bool) {
	_, ts, ok := resolveTypeSpecAtPath(pkg, props, path)
	return ts, ok
}

// lookupTypeSpecFromProperty resolves nested path segments starting from one
// property's TypeSpec.
func lookupTypeSpecFromProperty(
	pkg schema.PackageSpec,
	prop schema.PropertySpec,
	part fieldPathPart,
	remaining []fieldPathPart,
) (schema.TypeSpec, bool) {
	current := prop.TypeSpec
	if part.Elem {
		if current.Type != "array" || current.Items == nil {
			return schema.TypeSpec{}, false
		}
		current = *current.Items
	}
	if len(remaining) == 0 {
		return current, true
	}

	properties, ok := objectPropertiesForTypeSpec(pkg, current)
	if !ok {
		return schema.TypeSpec{}, false
	}
	return lookupTypeSpecAtPath(pkg, properties, remaining)
}

// objectPropertiesForTypeSpec resolves local object type refs to their property map.
func objectPropertiesForTypeSpec(pkg schema.PackageSpec, ts schema.TypeSpec) (map[string]schema.PropertySpec, bool) {
	typeToken, ok := localTypeToken(ts.Ref)
	if !ok {
		return nil, false
	}
	typeSpec, ok := pkg.Types[typeToken]
	if !ok {
		return nil, false
	}
	return typeSpec.Properties, true
}

// setTypeSpecAtPath replaces the terminal TypeSpec at a resolved field path.
func setTypeSpecAtPath(
	pkg *schema.PackageSpec,
	props map[string]schema.PropertySpec,
	path []fieldPathPart,
	replacement schema.TypeSpec,
	localTypeRefUseIndex map[string]int,
) (map[string]schema.PropertySpec, bool) {
	if len(path) == 0 {
		return props, false
	}

	propKey, ok := resolvePropertyName(props, path[0].Name)
	if !ok {
		return props, false
	}
	prop := props[propKey]
	head := path[0]
	head.Name = propKey
	updatedProp, ok := setTypeSpecFromProperty(pkg, prop, head, path[1:], replacement, localTypeRefUseIndex)
	if !ok {
		return props, false
	}

	out := clonePropertySpecMap(props)
	out[propKey] = updatedProp
	return out, true
}

// renameResolvedLeafProperty renames the terminal field key when old/new paths
// differ only by leaf name.
func renameResolvedLeafProperty(
	pkg *schema.PackageSpec,
	props map[string]schema.PropertySpec,
	fromPath []fieldPathPart,
	toPath []fieldPathPart,
	localTypeRefUseIndex map[string]int,
) (map[string]schema.PropertySpec, bool) {
	if len(fromPath) == 0 || len(fromPath) != len(toPath) {
		return props, false
	}
	for i := 0; i < len(fromPath)-1; i++ {
		if fromPath[i].Name != toPath[i].Name || fromPath[i].Elem != toPath[i].Elem {
			return props, false
		}
	}

	fromKey := fromPath[len(fromPath)-1].Name
	toKey := toPath[len(toPath)-1].Name
	if fromKey == toKey {
		return props, true
	}
	return renameResolvedPropertyKeyAtPath(pkg, props, fromPath[:len(fromPath)-1], fromKey, toKey, localTypeRefUseIndex)
}

// renameResolvedPropertyKeyAtPath renames a property key at root or within nested
// local type refs when doing so is safe (not shared across multiple tokens).
func renameResolvedPropertyKeyAtPath(
	pkg *schema.PackageSpec,
	props map[string]schema.PropertySpec,
	path []fieldPathPart,
	fromKey, toKey string,
	localTypeRefUseIndex map[string]int,
) (map[string]schema.PropertySpec, bool) {
	if len(path) == 0 {
		prop, ok := props[fromKey]
		if !ok {
			return props, false
		}
		if _, exists := props[toKey]; exists {
			return props, false
		}
		out := clonePropertySpecMap(props)
		out[toKey] = prop
		delete(out, fromKey)
		return out, true
	}

	part := path[0]
	prop, ok := props[part.Name]
	if !ok {
		return props, false
	}

	current := prop.TypeSpec
	if part.Elem {
		if current.Type != "array" || current.Items == nil {
			return props, false
		}
		current = *current.Items
	}

	typeToken, ok := localTypeToken(current.Ref)
	if !ok {
		return props, false
	}
	typeSpec, ok := pkg.Types[typeToken]
	if !ok {
		return props, false
	}
	// Avoid cross-token side effects when nested local object refs are shared.
	if localTypeExternalRefUseCount(localTypeRefUseIndex, typeToken) > 1 {
		return props, false
	}

	updatedTypeProps, ok := renameResolvedPropertyKeyAtPath(
		pkg,
		typeSpec.Properties,
		path[1:],
		fromKey,
		toKey,
		localTypeRefUseIndex,
	)
	if !ok {
		return props, false
	}
	typeSpec.Properties = updatedTypeProps
	pkg.Types[typeToken] = typeSpec
	return props, true
}

// setTypeSpecFromProperty applies replacement to a property type, including
// element rewrites for array paths.
func setTypeSpecFromProperty(
	pkg *schema.PackageSpec,
	prop schema.PropertySpec,
	part fieldPathPart,
	remaining []fieldPathPart,
	replacement schema.TypeSpec,
	localTypeRefUseIndex map[string]int,
) (schema.PropertySpec, bool) {
	current := prop.TypeSpec
	if part.Elem {
		if current.Type != "array" || current.Items == nil {
			return prop, false
		}
		item := *current.Items
		if len(remaining) == 0 {
			current.Items = cloneTypeSpecPtr(replacement)
			prop.TypeSpec = current
			return prop, true
		}

		updatedItem, ok := setTypeSpecFromNestedType(pkg, item, remaining, replacement, localTypeRefUseIndex)
		if !ok {
			return prop, false
		}
		current.Items = cloneTypeSpecPtr(updatedItem)
		prop.TypeSpec = current
		return prop, true
	}

	if len(remaining) == 0 {
		prop.TypeSpec = replacement
		return prop, true
	}

	updatedCurrent, ok := setTypeSpecFromNestedType(pkg, current, remaining, replacement, localTypeRefUseIndex)
	if !ok {
		return prop, false
	}
	prop.TypeSpec = updatedCurrent
	return prop, true
}

// setTypeSpecFromNestedType rewrites nested local type definitions when the
// referenced type is not externally shared.
func setTypeSpecFromNestedType(
	pkg *schema.PackageSpec,
	ts schema.TypeSpec,
	path []fieldPathPart,
	replacement schema.TypeSpec,
	localTypeRefUseIndex map[string]int,
) (schema.TypeSpec, bool) {
	typeToken, ok := localTypeToken(ts.Ref)
	if !ok {
		return ts, false
	}
	typeSpec, ok := pkg.Types[typeToken]
	if !ok {
		return ts, false
	}
	// Avoid cross-token side effects when this type token is referenced from multiple
	// schema locations. We only normalize nested refs when the ref is not shared.
	if localTypeExternalRefUseCount(localTypeRefUseIndex, typeToken) > 1 {
		return ts, false
	}
	updatedProps, ok := setTypeSpecAtPath(pkg, typeSpec.Properties, path, replacement, localTypeRefUseIndex)
	if !ok {
		return ts, false
	}
	typeSpec.Properties = updatedProps
	pkg.Types[typeToken] = typeSpec
	return ts, true
}

// localTypeToken parses "#/types/<token>" refs.
func localTypeToken(ref string) (string, bool) {
	const prefix = "#/types/"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	typeToken := strings.TrimPrefix(ref, prefix)
	if strings.TrimSpace(typeToken) == "" {
		return "", false
	}
	return typeToken, true
}

// isMaxItemsOneTypeChange reports array<->single transitions with equivalent
// element/object inner type.
func isMaxItemsOneTypeChange(oldType, newType *schema.TypeSpec) bool {
	if oldType == nil || newType == nil {
		return false
	}
	if isArrayType(oldType) && !isArrayType(newType) {
		return sameTypeSpec(oldType.Items, newType)
	}
	if !isArrayType(oldType) && isArrayType(newType) {
		return sameTypeSpec(newType.Items, oldType)
	}
	return false
}

// typeIdentifier returns a display identifier for TypeSpec values.
func typeIdentifier(ts *schema.TypeSpec) string {
	if ts == nil {
		return ""
	}
	if ts.Ref != "" {
		return ts.Ref
	}
	return ts.Type
}

// isArrayType reports whether a TypeSpec pointer is an array type.
func isArrayType(ts *schema.TypeSpec) bool {
	return ts != nil && ts.Type == "array"
}

// sameTypeSpec performs structural equality for two non-nil TypeSpecs.
func sameTypeSpec(a, b *schema.TypeSpec) bool {
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(*a, *b)
}

// cloneTypeSpecPtr returns a pointer to a copied TypeSpec value.
func cloneTypeSpecPtr(ts schema.TypeSpec) *schema.TypeSpec {
	clone := ts
	return &clone
}

// clonePropertySpecMap copies a property map.
func clonePropertySpecMap(props map[string]schema.PropertySpec) map[string]schema.PropertySpec {
	if props == nil {
		return nil
	}
	out := make(map[string]schema.PropertySpec, len(props))
	for key, value := range props {
		out[key] = value
	}
	return out
}

// resolvePropertyName maps metadata field names to actual Pulumi schema keys.
func resolvePropertyName(props map[string]schema.PropertySpec, metadataFieldName string) (string, bool) {
	if len(props) == 0 || strings.TrimSpace(metadataFieldName) == "" {
		return "", false
	}
	for _, candidate := range metadataFieldNameCandidates(metadataFieldName) {
		if _, ok := props[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

// metadataFieldNameCandidates generates candidate Pulumi field names from a
// metadata field token (exact, terraform->pulumi, plural/singular forms).
func metadataFieldNameCandidates(name string) []string {
	seen := map[string]struct{}{}
	candidates := []string{}
	add := func(v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		candidates = append(candidates, v)
	}

	// Use bridge naming first (snake_case -> lowerCamel), then bridge inflector variants.
	add(name)
	add(tfbridge.TerraformToPulumiNameV2(name, nil, nil))

	seed := append([]string(nil), candidates...)
	for _, base := range seed {
		add(inflector.Pluralize(base))
		add(inflector.Singularize(base))
	}
	return candidates
}

// buildLocalTypeRefUseCountIndex counts external references to each local type
// so nested rewrites can skip shared definitions.
func buildLocalTypeRefUseCountIndex(pkg schema.PackageSpec) map[string]int {
	// Count external uses of local #/types refs so nested rewrites can skip
	// shared type definitions and avoid cross-token side effects.
	refCountsByTypeToken := map[string]int{}

	incrementLocalTypeRefsFromPropertyMap(pkg.Config.Variables, refCountsByTypeToken)
	incrementLocalTypeRefsFromPropertyMap(pkg.Provider.InputProperties, refCountsByTypeToken)
	incrementLocalTypeRefsFromPropertyMap(pkg.Provider.Properties, refCountsByTypeToken)
	if pkg.Provider.StateInputs != nil {
		incrementLocalTypeRefsFromPropertyMap(pkg.Provider.StateInputs.Properties, refCountsByTypeToken)
	}

	for _, resource := range pkg.Resources {
		incrementLocalTypeRefsFromPropertyMap(resource.InputProperties, refCountsByTypeToken)
		incrementLocalTypeRefsFromPropertyMap(resource.Properties, refCountsByTypeToken)
		if resource.StateInputs != nil {
			incrementLocalTypeRefsFromPropertyMap(resource.StateInputs.Properties, refCountsByTypeToken)
		}
	}

	for _, function := range pkg.Functions {
		if function.Inputs != nil {
			incrementLocalTypeRefsFromPropertyMap(function.Inputs.Properties, refCountsByTypeToken)
		}
		if function.Outputs != nil {
			incrementLocalTypeRefsFromPropertyMap(function.Outputs.Properties, refCountsByTypeToken)
		}
		if function.ReturnType != nil {
			if function.ReturnType.ObjectTypeSpec != nil {
				incrementLocalTypeRefsFromPropertyMap(function.ReturnType.ObjectTypeSpec.Properties, refCountsByTypeToken)
			}
			if function.ReturnType.TypeSpec != nil {
				incrementLocalTypeRefsFromTypeSpec(*function.ReturnType.TypeSpec, refCountsByTypeToken)
			}
		}
	}

	for _, typeSpec := range pkg.Types {
		incrementLocalTypeRefsFromPropertyMap(typeSpec.Properties, refCountsByTypeToken)
	}

	for typeToken, typeSpec := range pkg.Types {
		target := "#/types/" + typeToken
		selfRefCount := propertyMapRefCount(typeSpec.Properties, target)
		refCountsByTypeToken[typeToken] -= selfRefCount
		if refCountsByTypeToken[typeToken] < 0 {
			refCountsByTypeToken[typeToken] = 0
		}
	}

	return refCountsByTypeToken
}

// incrementLocalTypeRefsFromPropertyMap adds local ref counts from one property map.
func incrementLocalTypeRefsFromPropertyMap(props map[string]schema.PropertySpec, refCountsByTypeToken map[string]int) {
	for _, prop := range props {
		incrementLocalTypeRefsFromTypeSpec(prop.TypeSpec, refCountsByTypeToken)
	}
}

// incrementLocalTypeRefsFromTypeSpec recursively counts local type refs in one TypeSpec.
func incrementLocalTypeRefsFromTypeSpec(ts schema.TypeSpec, refCountsByTypeToken map[string]int) {
	if typeToken, ok := localTypeToken(ts.Ref); ok {
		refCountsByTypeToken[typeToken]++
	}
	if ts.Items != nil {
		incrementLocalTypeRefsFromTypeSpec(*ts.Items, refCountsByTypeToken)
	}
	if ts.AdditionalProperties != nil {
		incrementLocalTypeRefsFromTypeSpec(*ts.AdditionalProperties, refCountsByTypeToken)
	}
	for _, branch := range ts.OneOf {
		incrementLocalTypeRefsFromTypeSpec(branch, refCountsByTypeToken)
	}
}

// localTypeExternalRefUseCount returns external use count for one local type token.
func localTypeExternalRefUseCount(refCountsByTypeToken map[string]int, typeToken string) int {
	return refCountsByTypeToken[typeToken]
}

// propertyMapRefCount counts references to targetRef in a property map.
func propertyMapRefCount(props map[string]schema.PropertySpec, targetRef string) int {
	count := 0
	for _, prop := range props {
		count += typeSpecRefCount(prop.TypeSpec, targetRef)
	}
	return count
}

// typeSpecRefCount counts recursive references to targetRef within one TypeSpec.
func typeSpecRefCount(ts schema.TypeSpec, targetRef string) int {
	count := 0
	if ts.Ref == targetRef {
		count++
	}
	if ts.Items != nil {
		count += typeSpecRefCount(*ts.Items, targetRef)
	}
	if ts.AdditionalProperties != nil {
		count += typeSpecRefCount(*ts.AdditionalProperties, targetRef)
	}
	for _, branch := range ts.OneOf {
		count += typeSpecRefCount(branch, targetRef)
	}
	return count
}
