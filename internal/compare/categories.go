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

func changedToRequired(kind string) string {
	return fmt.Sprintf("%s has changed to Required", kind)
}

func changedToOptional(kind string) string {
	return fmt.Sprintf("%s is no longer Required", kind)
}

func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
}
