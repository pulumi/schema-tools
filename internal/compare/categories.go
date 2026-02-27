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

func kindSeverity(kind ChangeKind, path []string) Severity {
	switch kind {
	case ChangeKindMissingResource, ChangeKindMissingFunction, ChangeKindMissingType, ChangeKindSignatureChanged:
		return SeverityDanger
	case ChangeKindMissingInput, ChangeKindMissingOutput, ChangeKindMissingProperty, ChangeKindTypeChanged:
		return SeverityWarn
	case ChangeKindOptionalToRequired:
		if isInputRequiredPath(path) {
			return SeverityDanger
		}
		return SeverityWarn
	case ChangeKindRequiredToOptional, ChangeKindNewResource, ChangeKindNewFunction:
		return SeverityInfo
	case ChangeKindTokenRemapped:
		return SeverityWarn
	}
	panic("unsupported change kind severity mapping")
}

func kindBreaking(kind ChangeKind) bool {
	switch kind {
	case ChangeKindNewResource, ChangeKindNewFunction:
		return false
	case ChangeKindRequiredToOptional:
		return false
	case ChangeKindMissingResource, ChangeKindMissingFunction, ChangeKindMissingType,
		ChangeKindMissingInput, ChangeKindMissingOutput, ChangeKindMissingProperty,
		ChangeKindTypeChanged, ChangeKindOptionalToRequired, ChangeKindTokenRemapped,
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
		Severity:    kindSeverity(kind, path),
		Breaking:    kindBreaking(kind),
		Description: description,
	}
}

func isInputRequiredPath(path []string) bool {
	if len(path) == 0 {
		return false
	}
	return path[0] == "required inputs" || (len(path) >= 2 && path[0] == "inputs" && path[1] == "required")
}
