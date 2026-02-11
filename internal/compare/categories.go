package compare

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

const (
	resourcesCategory = "Resources"
	functionsCategory = "Functions"
	typesCategory     = "Types"
)

// changedToRequired returns the message used when a field becomes required.
func changedToRequired(kind string) string {
	return fmt.Sprintf("%s has changed to Required", kind)
}

// changedToOptional returns the message used when a field becomes optional.
func changedToOptional(kind string) string {
	return fmt.Sprintf("%s is no longer Required", kind)
}

// changedToMaxItemsOne returns the message used for max-items-one shape changes.
func changedToMaxItemsOne(oldType, newType string) string {
	return fmt.Sprintf(`type changed from %q to %q (max-items-one)`, oldType, newType)
}

// changedToMaxItemsOneRename returns the message used for pluralization-based rename detection.
func changedToMaxItemsOneRename(oldType, newType, newName string) string {
	return fmt.Sprintf(`type changed from %q to %q (max-items-one; renamed to %q)`, oldType, newType, newName)
}

// formatName rewrites a token into provider-relative dot notation for output.
func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
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

func sameTypeIdentifier(a, b *schema.TypeSpec) bool {
	if a == nil || b == nil {
		return false
	}
	aID := typeIdentifier(a)
	if aID == "" {
		return false
	}
	return aID == typeIdentifier(b)
}

// isMaxItemsOneChange returns true for scalar<->array transitions with stable element types.
func isMaxItemsOneChange(old, new *schema.TypeSpec) bool {
	if old == nil || new == nil {
		return false
	}
	if isArrayType(old) && !isArrayType(new) {
		return sameTypeIdentifier(old.Items, new)
	}
	if !isArrayType(old) && isArrayType(new) {
		return sameTypeIdentifier(new.Items, old)
	}
	return false
}
