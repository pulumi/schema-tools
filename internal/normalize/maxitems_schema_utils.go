package normalize

import (
	"reflect"
	"strings"

	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

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
