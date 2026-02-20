package normalize

import (
	"reflect"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

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
		tfToken, ok := resolveCanonicalTFToken(scopeResources, token, context.tokenRemap, resourceIndex)
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
		tfToken, ok := resolveCanonicalTFToken(scopeDataSources, token, context.tokenRemap, functionIndex)
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
		if pathEvidence.Transition != MaxItemsOneTransitionChanged {
			continue
		}
		pathParts, ok := parseFieldPath(path)
		if !ok {
			continue
		}

		oldType, ok := lookupTypeSpecAtPath(oldSchema, oldProps, pathParts)
		if !ok {
			continue
		}
		newType, ok := lookupTypeSpecAtPath(*normalizedNew, updated, pathParts)
		if !ok {
			continue
		}
		if !isMaxItemsOneTypeChange(&oldType, &newType) {
			continue
		}

		nextProps, ok := setTypeSpecAtPath(normalizedNew, updated, pathParts, oldType, localTypeRefUseIndex)
		if !ok {
			continue
		}
		updated = nextProps
		changes = append(changes, MaxItemsOneChange{
			Scope:    scope,
			Token:    token,
			Location: location,
			Field:    path,
			OldType:  typeIdentifier(&oldType),
			NewType:  typeIdentifier(&newType),
		})
	}

	return updated, changes
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

func lookupTypeSpecAtPath(
	pkg schema.PackageSpec,
	props map[string]schema.PropertySpec,
	path []fieldPathPart,
) (schema.TypeSpec, bool) {
	if len(path) == 0 {
		return schema.TypeSpec{}, false
	}
	prop, ok := props[path[0].Name]
	if !ok {
		return schema.TypeSpec{}, false
	}
	return lookupTypeSpecFromProperty(pkg, prop, path[0], path[1:])
}

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

	prop, ok := props[path[0].Name]
	if !ok {
		return props, false
	}
	updatedProp, ok := setTypeSpecFromProperty(pkg, prop, path[0], path[1:], replacement, localTypeRefUseIndex)
	if !ok {
		return props, false
	}

	out := clonePropertySpecMap(props)
	out[path[0].Name] = updatedProp
	return out, true
}

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

func typeIdentifier(ts *schema.TypeSpec) string {
	if ts == nil {
		return ""
	}
	if ts.Ref != "" {
		return ts.Ref
	}
	return ts.Type
}

func isArrayType(ts *schema.TypeSpec) bool {
	return ts != nil && ts.Type == "array"
}

func sameTypeSpec(a, b *schema.TypeSpec) bool {
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(*a, *b)
}

func cloneTypeSpecPtr(ts schema.TypeSpec) *schema.TypeSpec {
	clone := ts
	return &clone
}

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

func incrementLocalTypeRefsFromPropertyMap(props map[string]schema.PropertySpec, refCountsByTypeToken map[string]int) {
	for _, prop := range props {
		incrementLocalTypeRefsFromTypeSpec(prop.TypeSpec, refCountsByTypeToken)
	}
}

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

func localTypeExternalRefUseCount(refCountsByTypeToken map[string]int, typeToken string) int {
	return refCountsByTypeToken[typeToken]
}

func propertyMapRefCount(props map[string]schema.PropertySpec, targetRef string) int {
	count := 0
	for _, prop := range props {
		count += typeSpecRefCount(prop.TypeSpec, targetRef)
	}
	return count
}

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
