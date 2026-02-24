package normalize

import (
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// fieldPathPart represents one segment of a flattened field-history path.
type fieldPathPart struct {
	Name string
	Elem bool
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
	if len(path) == 1 {
		typeSpec.Required = renameRequiredName(typeSpec.Required, fromKey, toKey)
	}
	typeSpec.Properties = updatedTypeProps
	pkg.Types[typeToken] = typeSpec
	return props, true
}

func applyTopLevelRequiredRename(pkg *schema.PackageSpec, scope, token, location, fromName, toName string) {
	if pkg == nil || fromName == "" || toName == "" || fromName == toName {
		return
	}
	switch scope {
	case scopeResources:
		resource, ok := pkg.Resources[token]
		if !ok {
			return
		}
		switch location {
		case "inputs":
			resource.RequiredInputs = renameRequiredName(resource.RequiredInputs, fromName, toName)
		case "properties":
			resource.Required = renameRequiredName(resource.Required, fromName, toName)
		default:
			return
		}
		pkg.Resources[token] = resource
	case scopeDataSources:
		function, ok := pkg.Functions[token]
		if !ok {
			return
		}
		switch location {
		case "inputs":
			if function.Inputs == nil {
				return
			}
			inputs := *function.Inputs
			inputs.Required = renameRequiredName(inputs.Required, fromName, toName)
			function.Inputs = &inputs
		case "outputs":
			if function.Outputs == nil {
				return
			}
			outputs := *function.Outputs
			outputs.Required = renameRequiredName(outputs.Required, fromName, toName)
			function.Outputs = &outputs
		default:
			return
		}
		pkg.Functions[token] = function
	}
}

func renameRequiredName(required []string, fromName, toName string) []string {
	if len(required) == 0 || fromName == "" || toName == "" || fromName == toName {
		return required
	}
	updated := append([]string(nil), required...)
	for i, name := range updated {
		if name == fromName {
			updated[i] = toName
		}
	}
	return updated
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
