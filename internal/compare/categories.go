package compare

import (
	"fmt"
	"strings"
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

// formatName rewrites a token into provider-relative dot notation for output.
func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
}
