package compare

import (
	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/util/set"
)

// pluralizationCandidates returns singular/plural variants for a property name.
func pluralizationCandidates(name string) []string {
	first, second := pluralizationCandidatePair(name)
	switch {
	case first == "":
		return nil
	case second == "":
		return []string{first}
	default:
		return []string{first, second}
	}
}

func pluralizationCandidatePair(name string) (first string, second string) {
	if name == "" {
		return "", ""
	}
	plural := inflector.Pluralize(name)
	singular := inflector.Singularize(name)

	add := func(candidate string) {
		if candidate == "" || candidate == name || candidate == first {
			return
		}
		if first == "" {
			first = candidate
			return
		}
		if second == "" {
			second = candidate
		}
	}
	add(plural)
	add(singular)
	return first, second
}

func maxItemsOneRename(oldName string, oldProp schema.PropertySpec, newProps map[string]schema.PropertySpec) (string, bool) {
	first, second := pluralizationCandidatePair(oldName)
	for _, candidate := range [2]string{first, second} {
		if candidate == "" {
			continue
		}
		newProp, ok := newProps[candidate]
		if !ok {
			continue
		}
		if isMaxItemsOneChange(&oldProp.TypeSpec, &newProp.TypeSpec) {
			return candidate, true
		}
	}
	return "", false
}

func isTrueRename(oldName, newName string, oldProps, newProps map[string]schema.PropertySpec) bool {
	if oldName == "" || newName == "" || oldName == newName {
		return false
	}
	if _, ok := oldProps[oldName]; !ok {
		return false
	}
	if _, ok := newProps[newName]; !ok {
		return false
	}
	// Rename suppression should apply only when the old key was removed
	// and the new key did not already exist in the old schema.
	if _, stillInNew := newProps[oldName]; stillInNew {
		return false
	}
	if _, existedInOld := oldProps[newName]; existedInOld {
		return false
	}
	return true
}

func isMaxItemsOneRenameRequired(newName string, oldRequired set.Set[string], oldProps, newProps map[string]schema.PropertySpec) bool {
	if newName == "" {
		return false
	}
	newProp, ok := newProps[newName]
	if !ok {
		return false
	}
	first, second := pluralizationCandidatePair(newName)
	for _, candidate := range [2]string{first, second} {
		if candidate == "" {
			continue
		}
		if !oldRequired.Has(candidate) {
			continue
		}
		if !isTrueRename(candidate, newName, oldProps, newProps) {
			continue
		}
		oldProp, ok := oldProps[candidate]
		if !ok {
			continue
		}
		if isMaxItemsOneChange(&oldProp.TypeSpec, &newProp.TypeSpec) {
			return true
		}
	}
	return false
}

func isMaxItemsOneRenameRequiredToOptional(oldName string, newRequired set.Set[string], oldProps, newProps map[string]schema.PropertySpec) bool {
	if oldName == "" {
		return false
	}
	oldProp, ok := oldProps[oldName]
	if !ok {
		return false
	}
	first, second := pluralizationCandidatePair(oldName)
	for _, candidate := range [2]string{first, second} {
		if candidate == "" {
			continue
		}
		if !newRequired.Has(candidate) {
			continue
		}
		if !isTrueRename(oldName, candidate, oldProps, newProps) {
			continue
		}
		newProp, ok := newProps[candidate]
		if !ok {
			continue
		}
		if isMaxItemsOneChange(&oldProp.TypeSpec, &newProp.TypeSpec) {
			return true
		}
	}
	return false
}
