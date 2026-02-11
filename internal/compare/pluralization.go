package compare

import (
	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/util/set"
)

// pluralizationCandidates returns singular/plural variants for a property name.
func pluralizationCandidates(name string) []string {
	if name == "" {
		return nil
	}

	candidates := []string{}
	seen := map[string]bool{}
	add := func(candidate string) {
		if candidate == "" || candidate == name || seen[candidate] {
			return
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}

	add(inflector.Pluralize(name))
	add(inflector.Singularize(name))

	return candidates
}

func maxItemsOneRename(oldName string, oldProp schema.PropertySpec, newProps map[string]schema.PropertySpec) (string, bool) {
	for _, candidate := range pluralizationCandidates(oldName) {
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

func isMaxItemsOneRenameRequired(newName string, oldRequired set.Set[string], oldProps, newProps map[string]schema.PropertySpec) bool {
	if newName == "" {
		return false
	}
	newProp, ok := newProps[newName]
	if !ok {
		return false
	}
	for _, candidate := range pluralizationCandidates(newName) {
		if !oldRequired.Has(candidate) {
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
	for _, candidate := range pluralizationCandidates(oldName) {
		if !newRequired.Has(candidate) {
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
