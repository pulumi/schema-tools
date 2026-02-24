package normalize

import "github.com/pulumi/pulumi/pkg/v3/codegen/schema"

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
