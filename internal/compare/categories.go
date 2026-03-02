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

func kindSeverity(kind ChangeKind) Severity {
	switch kind {
	case ChangeKindMissingResource, ChangeKindMissingFunction, ChangeKindMissingType, ChangeKindSignatureChanged:
		return SeverityDanger
	case ChangeKindMissingInput, ChangeKindMissingOutput, ChangeKindMissingProperty, ChangeKindTypeChanged:
		return SeverityWarn
	case ChangeKindOptionalToRequired, ChangeKindRequiredToOptional, ChangeKindNewResource, ChangeKindNewFunction:
		return SeverityInfo
	}
	panic("unsupported change kind severity mapping")
}

func kindBreaking(kind ChangeKind) bool {
	switch kind {
	case ChangeKindNewResource, ChangeKindNewFunction:
		return false
	case ChangeKindMissingResource, ChangeKindMissingFunction, ChangeKindMissingType,
		ChangeKindMissingInput, ChangeKindMissingOutput, ChangeKindMissingProperty,
		ChangeKindTypeChanged, ChangeKindOptionalToRequired, ChangeKindRequiredToOptional,
		ChangeKindSignatureChanged:
		return true
	}
	panic("unsupported change kind breaking mapping")
}

func newChange(category, name string, path []string, kind ChangeKind, description string) Change {
	return Change{
		Category:    category,
		Name:        name,
		Path:        path,
		Kind:        kind,
		Severity:    kindSeverity(kind),
		Breaking:    kindBreaking(kind),
		Description: description,
	}
}
