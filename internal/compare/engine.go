package compare

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pulumi/inflector"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/normalize"
	"github.com/pulumi/schema-tools/internal/util/set"
)

const (
	scopeResources   = "resources"
	scopeDatasources = "datasources"
)

// Analyze computes typed changes and newly introduced resources/functions.
func Analyze(provider string, oldSchema, newSchema schema.PackageSpec, oldMetadata, newMetadata *normalize.MetadataEnvelope) Report {
	changes, newResources, newFunctions := buildChanges(provider, oldSchema, newSchema, oldMetadata, newMetadata)
	sortChanges(changes)
	return Report{
		Changes:      changes,
		NewResources: newResources,
		NewFunctions: newFunctions,
	}
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func buildChanges(provider string, oldSchema, newSchema schema.PackageSpec, oldMetadata, newMetadata *normalize.MetadataEnvelope) ([]Change, []string, []string) {
	changes := []Change{}
	newResources := []string{}
	newFunctions := []string{}
	matchedResourceTokens := map[string]struct{}{}
	matchedFunctionTokens := map[string]struct{}{}

	for _, resName := range sortedMapKeys(oldSchema.Resources) {
		res := oldSchema.Resources[resName]
		resolvedToken, decision, ok := resolveOldToken(resName, scopeResources, newSchema.Resources, oldMetadata, newMetadata)
		if !ok {
			changes = append(changes, newChangeWithReason(resourcesCategory, resName, nil, ChangeKindMissingResource, missingWithLookup("missing", decision), decision))
			continue
		}
		newRes := newSchema.Resources[resolvedToken]
		matchedResourceTokens[resolvedToken] = struct{}{}
		if decision != nil && decision.Outcome == NormalizationOutcomeResolved && resolvedToken != resName {
			changes = append(changes, tokenRemapChange(resourcesCategory, resName, resolvedToken, decision, false))
		}

		for _, propName := range sortedMapKeys(res.InputProperties) {
			prop := res.InputProperties[propName]
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				if description, ok := renamedPropertyTypeChangeDescription(propName, &prop.TypeSpec, newRes.InputProperties); ok {
					changes = append(changes, newChange(resourcesCategory, resName, []string{"inputs", propName}, ChangeKindTypeChanged, description))
					continue
				}
				changes = append(changes, newChange(resourcesCategory, resName, []string{"inputs", propName}, ChangeKindMissingInput, "missing"))
				continue
			}
			appendTypeChanges(&changes, resourcesCategory, resName, []string{"inputs", propName}, &prop.TypeSpec, &newProp.TypeSpec, oldMetadata, newMetadata)
		}

		for _, propName := range sortedMapKeys(res.Properties) {
			prop := res.Properties[propName]
			newProp, ok := newRes.Properties[propName]
			if !ok {
				if description, ok := renamedPropertyTypeChangeDescription(propName, &prop.TypeSpec, newRes.Properties); ok {
					changes = append(changes, newChange(resourcesCategory, resName, []string{"properties", propName}, ChangeKindTypeChanged, description))
					continue
				}
				changes = append(changes, newChange(resourcesCategory, resName, []string{"properties", propName}, ChangeKindMissingOutput, fmt.Sprintf("missing output %q", propName)))
				continue
			}
			appendTypeChanges(&changes, resourcesCategory, resName, []string{"properties", propName}, &prop.TypeSpec, &newProp.TypeSpec, oldMetadata, newMetadata)
		}

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			if !oldRequiredInputs.Has(input) {
				changes = append(changes, newChange(resourcesCategory, resName, []string{"required inputs", input}, ChangeKindOptionalToRequired, changedToRequired("input")))
			}
		}

		newRequiredProperties := set.FromSlice(newRes.Required)
		for _, prop := range res.Required {
			_, stillExists := newRes.Properties[prop]
			if !newRequiredProperties.Has(prop) && stillExists {
				changes = append(changes, newChange(resourcesCategory, resName, []string{"required", prop}, ChangeKindRequiredToOptional, changedToOptional("property")))
			}
		}
	}
	for _, resName := range sortedMapKeys(newSchema.Resources) {
		if _, ok := oldSchema.Resources[resName]; ok {
			continue
		}
		if _, ok := matchedResourceTokens[resName]; ok {
			continue
		}

		decision := resolveNewTokenDecision(resName, scopeResources, oldMetadata, newMetadata)
		if decision.Outcome == NormalizationOutcomeResolved {
			if _, ok := oldSchema.Resources[decision.Token]; ok {
				if !isRetainedInCodegenAlias(newMetadata, scopeResources, decision.Token, resName) {
					continue
				}
				changes = append(changes, tokenRemapChange(resourcesCategory, decision.Token, resName, decision, true))
			}
		}

		changes = append(changes, newChangeWithReason(resourcesCategory, resName, nil, ChangeKindNewResource, "added", decision))
		newResources = append(newResources, formatName(provider, resName))
	}

	for _, funcName := range sortedMapKeys(oldSchema.Functions) {
		f := oldSchema.Functions[funcName]
		resolvedToken, decision, ok := resolveOldToken(funcName, scopeDatasources, newSchema.Functions, oldMetadata, newMetadata)
		if !ok {
			changes = append(changes, newChangeWithReason(functionsCategory, funcName, nil, ChangeKindMissingFunction, missingWithLookup("missing", decision), decision))
			continue
		}
		newFunc := newSchema.Functions[resolvedToken]
		matchedFunctionTokens[resolvedToken] = struct{}{}
		if decision != nil && decision.Outcome == NormalizationOutcomeResolved && resolvedToken != funcName {
			changes = append(changes, tokenRemapChange(functionsCategory, funcName, resolvedToken, decision, false))
		}

		if f.Inputs != nil {
			for _, propName := range sortedMapKeys(f.Inputs.Properties) {
				prop := f.Inputs.Properties[propName]
				if newFunc.Inputs == nil {
					changes = append(changes, newChange(functionsCategory, funcName, []string{"inputs", propName}, ChangeKindMissingInput, fmt.Sprintf("missing input %q", propName)))
					continue
				}
				newProp, ok := newFunc.Inputs.Properties[propName]
				if !ok {
					if description, renamed := renamedPropertyTypeChangeDescription(propName, &prop.TypeSpec, newFunc.Inputs.Properties); renamed {
						changes = append(changes, newChange(functionsCategory, funcName, []string{"inputs", propName}, ChangeKindTypeChanged, description))
						continue
					}
					changes = append(changes, newChange(functionsCategory, funcName, []string{"inputs", propName}, ChangeKindMissingInput, fmt.Sprintf("missing input %q", propName)))
					continue
				}
				appendTypeChanges(&changes, functionsCategory, funcName, []string{"inputs", propName}, &prop.TypeSpec, &newProp.TypeSpec, oldMetadata, newMetadata)
			}
			if newFunc.Inputs != nil {
				oldRequired := set.FromSlice(f.Inputs.Required)
				for _, req := range newFunc.Inputs.Required {
					if !oldRequired.Has(req) {
						changes = append(changes, newChange(functionsCategory, funcName, []string{"inputs", "required", req}, ChangeKindOptionalToRequired, changedToRequired("input")))
					}
				}
			}
		}

		isNonZeroArgs := func(ts *schema.ObjectTypeSpec) bool {
			if ts == nil {
				return false
			}
			return len(ts.Properties) > 0
		}
		type nonZeroArgs struct{ old, new bool }
		switch (nonZeroArgs{old: isNonZeroArgs(f.Inputs), new: isNonZeroArgs(newFunc.Inputs)}) {
		case nonZeroArgs{false, true}:
			changes = append(changes, newChange(functionsCategory, funcName, nil, ChangeKindSignatureChanged,
				"signature change (pulumi.InvokeOptions)->T => (Args, pulumi.InvokeOptions)->T"))
		case nonZeroArgs{true, false}:
			changes = append(changes, newChange(functionsCategory, funcName, nil, ChangeKindSignatureChanged,
				"signature change (Args, pulumi.InvokeOptions)->T => (pulumi.InvokeOptions)->T"))
		}

		if f.Outputs != nil {
			for _, propName := range sortedMapKeys(f.Outputs.Properties) {
				prop := f.Outputs.Properties[propName]
				if newFunc.Outputs == nil {
					changes = append(changes, newChange(functionsCategory, funcName, []string{"outputs", propName}, ChangeKindMissingOutput, "missing output"))
					continue
				}
				newProp, ok := newFunc.Outputs.Properties[propName]
				if !ok {
					if description, renamed := renamedPropertyTypeChangeDescription(propName, &prop.TypeSpec, newFunc.Outputs.Properties); renamed {
						changes = append(changes, newChange(functionsCategory, funcName, []string{"outputs", propName}, ChangeKindTypeChanged, description))
						continue
					}
					changes = append(changes, newChange(functionsCategory, funcName, []string{"outputs", propName}, ChangeKindMissingOutput, "missing output"))
					continue
				}
				appendTypeChanges(&changes, functionsCategory, funcName, []string{"outputs", propName}, &prop.TypeSpec, &newProp.TypeSpec, oldMetadata, newMetadata)
			}
			var newRequired set.Set[string]
			if newFunc.Outputs != nil {
				newRequired = set.FromSlice(newFunc.Outputs.Required)
			}
			for _, req := range f.Outputs.Required {
				stillExists := false
				if newFunc.Outputs != nil {
					_, stillExists = newFunc.Outputs.Properties[req]
				}
				if !newRequired.Has(req) && stillExists {
					changes = append(changes, newChange(functionsCategory, funcName, []string{"outputs", "required", req}, ChangeKindRequiredToOptional, changedToOptional("property")))
				}
			}
		}
	}
	for _, funcName := range sortedMapKeys(newSchema.Functions) {
		if _, ok := oldSchema.Functions[funcName]; ok {
			continue
		}
		if _, ok := matchedFunctionTokens[funcName]; ok {
			continue
		}

		decision := resolveNewTokenDecision(funcName, scopeDatasources, oldMetadata, newMetadata)
		if decision.Outcome == NormalizationOutcomeResolved {
			if _, ok := oldSchema.Functions[decision.Token]; ok {
				if !isRetainedInCodegenAlias(newMetadata, scopeDatasources, decision.Token, funcName) {
					continue
				}
				changes = append(changes, tokenRemapChange(functionsCategory, decision.Token, funcName, decision, true))
			}
		}

		changes = append(changes, newChangeWithReason(functionsCategory, funcName, nil, ChangeKindNewFunction, "added", decision))
		newFunctions = append(newFunctions, formatName(provider, funcName))
	}

	for _, typName := range sortedMapKeys(oldSchema.Types) {
		typ := oldSchema.Types[typName]
		newTyp, ok := newSchema.Types[typName]
		if !ok {
			changes = append(changes, newChange(typesCategory, typName, nil, ChangeKindMissingType, "missing"))
			continue
		}

		for _, propName := range sortedMapKeys(typ.Properties) {
			prop := typ.Properties[propName]
			newProp, ok := newTyp.Properties[propName]
			if !ok {
				if description, renamed := renamedPropertyTypeChangeDescription(propName, &prop.TypeSpec, newTyp.Properties); renamed {
					changes = append(changes, newChange(typesCategory, typName, []string{"properties", propName}, ChangeKindTypeChanged, description))
					continue
				}
				changes = append(changes, newChange(typesCategory, typName, []string{"properties", propName}, ChangeKindMissingProperty, "missing"))
				continue
			}
			appendTypeChanges(&changes, typesCategory, typName, []string{"properties", propName}, &prop.TypeSpec, &newProp.TypeSpec, nil, nil)
		}

		newRequired := set.FromSlice(newTyp.Required)
		for _, r := range typ.Required {
			_, stillExists := newTyp.Properties[r]
			if !newRequired.Has(r) && stillExists {
				changes = append(changes, newChange(typesCategory, typName, []string{"required", r}, ChangeKindRequiredToOptional, changedToOptional("property")))
			}
		}
		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if !required.Has(r) {
				changes = append(changes, newChange(typesCategory, typName, []string{"required", r}, ChangeKindOptionalToRequired, changedToRequired("property")))
			}
		}
	}

	return changes, newResources, newFunctions
}

func appendTypeChanges(changes *[]Change, category, name string, path []string, old, new *schema.TypeSpec, oldMetadata, newMetadata *normalize.MetadataEnvelope) {
	switch {
	case old == nil && new == nil:
		return
	case old != nil && new == nil:
		*changes = append(*changes, newChange(category, name, path, ChangeKindTypeChanged, fmt.Sprintf("had %+v but now has no type", old)))
		return
	case old == nil && new != nil:
		*changes = append(*changes, newChange(category, name, path, ChangeKindTypeChanged, fmt.Sprintf("had no type but now has %+v", new)))
		return
	}

	if oldTypeText, newTypeText, ok := refArrayBoundaryTypeChangeText(old, new); ok {
		*changes = append(*changes, newChange(category, name, path, ChangeKindTypeChanged, fmt.Sprintf("type changed from %q to %q", oldTypeText, newTypeText)))
		return
	}

	oldType := old.Type
	if old.Ref != "" {
		oldType = old.Ref
	}
	newType := new.Type
	if new.Ref != "" {
		newType = new.Ref
	}
	if oldType != newType {
		*changes = append(*changes, newChange(category, name, path, ChangeKindTypeChanged, fmt.Sprintf("type changed from %q to %q", oldType, newType)))
		if isMaxItemsEquivalentTransition(category, name, path, old, new, oldMetadata, newMetadata) {
			return
		}
	}

	appendTypeChanges(changes, category, name, append(slices.Clone(path), "items"), old.Items, new.Items, oldMetadata, newMetadata)
	appendTypeChanges(changes, category, name, append(slices.Clone(path), "additional properties"), old.AdditionalProperties, new.AdditionalProperties, oldMetadata, newMetadata)
}

// renamedPropertyTypeChangeDescription detects simple singular/plural property
// renames and formats a combined rename+type-change message.
//
// Example:
// old property map: {"loggings": array<#/types/.../BucketLogging>}
// new property map: {"logging": #/types/.../BucketLogging}
// result: `renamed to "logging" and type changed from "array" to "#/types/.../BucketLogging"`
func renamedPropertyTypeChangeDescription(oldName string, oldType *schema.TypeSpec, newProps map[string]schema.PropertySpec) (string, bool) {
	if oldType == nil {
		return "", false
	}
	for _, candidate := range renameCandidates(oldName) {
		newProp, ok := newProps[candidate]
		if !ok || candidate == oldName {
			continue
		}
		oldTypeText, newTypeText, ok := refArrayBoundaryTypeChangeText(oldType, &newProp.TypeSpec)
		if !ok {
			continue
		}
		return fmt.Sprintf("renamed to %q and type changed from %q to %q", candidate, oldTypeText, newTypeText), true
	}
	return "", false
}

// renameCandidates returns deterministic singular/plural name alternatives used
// when exact property lookup misses.
//
// Example:
// input: "forwards"
// output: ["forward", "forwards"]
func renameCandidates(name string) []string {
	candidates := []string{singularizeName(name), pluralizeName(name)}
	out := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

// singularizeName converts one identifier to its singular form using Pulumi's
// inflector rules.
func singularizeName(name string) string {
	if name == "" {
		return ""
	}
	return inflector.Singularize(name)
}

// pluralizeName converts one identifier to its plural form using Pulumi's
// inflector rules.
func pluralizeName(name string) string {
	if name == "" {
		return ""
	}
	return inflector.Pluralize(name)
}

// refArrayBoundaryTypeChangeText normalizes specific ref/array<ref> transitions
// into one before/after text pair so callers can emit a single diagnostic.
//
// Examples:
//   - old: #/types/pkg:index/Foo:Foo
//     new: array<#/types/pkg:index/Foo:Foo>
//     out: ("#/types/pkg:index/Foo:Foo", "array<#/types/pkg:index/Foo:Foo>")
//   - old: array<#/types/pkg:index/Foos:Foos>
//     new: #/types/pkg:index/Foo:Foo
//     out: ("array", "#/types/pkg:index/Foo:Foo")
func refArrayBoundaryTypeChangeText(old, new *schema.TypeSpec) (string, string, bool) {
	if old == nil || new == nil {
		return "", "", false
	}

	switch {
	case old.Ref != "" && new.Type == "array" && new.Items != nil && new.Items.Ref != "":
		return old.Ref, typeLookupText(new), true
	case old.Type == "array" && old.Items != nil && old.Items.Ref != "" && new.Ref != "":
		return "array", new.Ref, true
	default:
		return "", "", false
	}
}

// isMaxItemsEquivalentTransition reports whether old/new types represent a
// metadata-backed maxItems-like transition for resource/function fields.
func isMaxItemsEquivalentTransition(
	category, token string,
	path []string,
	old, new *schema.TypeSpec,
	oldMetadata, newMetadata *normalize.MetadataEnvelope,
) bool {
	scope, field, ok := scopeAndFieldPath(category, path)
	if !ok {
		return false
	}
	result := normalize.ResolveEquivalentTypeChange(
		oldMetadata,
		newMetadata,
		scope,
		token,
		field,
		typeLookupText(old),
		typeLookupText(new),
	)
	return result.Outcome == normalize.TokenLookupOutcomeResolved && result.Equivalent
}

// scopeAndFieldPath converts a typed change path into a normalize lookup scope
// and field path.
//
// Example:
//
//	scope, field, ok := scopeAndFieldPath(resourcesCategory, []string{"inputs", "list", "items"})
//	// scope == "resources", field == "list[*]", ok == true
func scopeAndFieldPath(category string, path []string) (string, string, bool) {
	if len(path) < 2 {
		return "", "", false
	}
	if path[0] != "inputs" && path[0] != "outputs" && path[0] != "properties" {
		return "", "", false
	}

	var scope string
	switch category {
	case resourcesCategory:
		scope = scopeResources
	case functionsCategory:
		scope = scopeDatasources
	default:
		return "", "", false
	}

	field := path[1]
	if field == "" {
		return "", "", false
	}
	for _, segment := range path[2:] {
		switch segment {
		case "items":
			field += "[*]"
		default:
			return "", "", false
		}
	}

	return scope, field, true
}

// typeLookupText converts a TypeSpec into normalized text for compare
// diagnostics and metadata equivalence checks.
//
// Example:
//
//	oldText := typeLookupText(&schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}})
//	// oldText == "array<string>"
func typeLookupText(spec *schema.TypeSpec) string {
	if spec == nil {
		return ""
	}
	if spec.Ref != "" {
		return spec.Ref
	}
	if spec.Type == "array" {
		if spec.Items == nil {
			return "array"
		}
		itemType := typeLookupText(spec.Items)
		if itemType == "" {
			return "array"
		}
		return fmt.Sprintf("array<%s>", itemType)
	}
	if spec.Type == "object" && spec.AdditionalProperties != nil {
		valueType := typeLookupText(spec.AdditionalProperties)
		if valueType == "" {
			return "map"
		}
		return fmt.Sprintf("map<%s>", valueType)
	}
	return spec.Type
}

func sortChanges(changes []Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Category != changes[j].Category {
			return changes[i].Category < changes[j].Category
		}
		if changes[i].Name != changes[j].Name {
			return changes[i].Name < changes[j].Name
		}
		if cmp := strings.Compare(strings.Join(changes[i].Path, "\x00"), strings.Join(changes[j].Path, "\x00")); cmp != 0 {
			return cmp < 0
		}
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		if changes[i].Severity != changes[j].Severity {
			return changes[i].Severity < changes[j].Severity
		}
		if changes[i].Description != changes[j].Description {
			return changes[i].Description < changes[j].Description
		}
		return reasonSortKey(changes[i].Reason) < reasonSortKey(changes[j].Reason)
	})
}

// resolveOldToken resolves old-snapshot token matching for missing/rename detection.
func resolveOldToken[T any](oldToken, scope string, newTokens map[string]T, oldMetadata, newMetadata *normalize.MetadataEnvelope) (string, *NormalizationReason, bool) {
	if _, ok := newTokens[oldToken]; ok {
		return oldToken, nil, true
	}

	lookup := normalize.ResolveToken(oldMetadata, newMetadata, scope, oldToken)
	reason := normalizationReasonFromLookup("ResolveToken", lookup)
	if lookup.Outcome != normalize.TokenLookupOutcomeResolved {
		return "", reason, false
	}
	if _, ok := newTokens[lookup.Token]; !ok {
		return "", reason, false
	}
	return lookup.Token, reason, true
}

// resolveNewTokenDecision resolves new-snapshot token matching for additions suppression.
func resolveNewTokenDecision(newToken, scope string, oldMetadata, newMetadata *normalize.MetadataEnvelope) *NormalizationReason {
	lookup := normalize.ResolveNewToken(oldMetadata, newMetadata, scope, newToken)
	return normalizationReasonFromLookup("ResolveNewToken", lookup)
}

// isRetainedInCodegenAlias reports whether oldToken remains as an in-codegen alias for newToken.
func isRetainedInCodegenAlias(metadata *normalize.MetadataEnvelope, scope, oldToken, newToken string) bool {
	if metadata == nil || metadata.AutoAliasing == nil {
		return false
	}

	var history map[string]*normalize.TokenHistory
	switch scope {
	case scopeResources:
		history = metadata.AutoAliasing.Resources
	case scopeDatasources:
		history = metadata.AutoAliasing.Datasources
	default:
		return false
	}

	for _, entry := range history {
		if entry == nil || entry.Current != newToken {
			continue
		}
		for _, past := range entry.Past {
			if past.Name == oldToken && past.InCodegen {
				return true
			}
		}
	}

	return false
}

// normalizationReasonFromLookup converts lookup API outcomes to typed change attribution.
func normalizationReasonFromLookup(lookupName string, lookup normalize.TokenLookupResult) *NormalizationReason {
	candidates := make([]string, 0, len(lookup.Candidates))
	candidates = append(candidates, lookup.Candidates...)

	switch lookup.Outcome {
	case normalize.TokenLookupOutcomeResolved:
		return &NormalizationReason{Outcome: NormalizationOutcomeResolved, Lookup: lookupName, Token: lookup.Token, Candidates: candidates}
	case normalize.TokenLookupOutcomeAmbiguous:
		return &NormalizationReason{Outcome: NormalizationOutcomeAmbiguous, Lookup: lookupName, Candidates: candidates}
	default:
		return &NormalizationReason{Outcome: NormalizationOutcomeNone, Lookup: lookupName, Candidates: candidates}
	}
}

// newChangeWithReason attaches typed normalization attribution to one emitted change.
func newChangeWithReason(category, name string, path []string, kind ChangeKind, description string, reason *NormalizationReason) Change {
	change := newChange(category, name, path, kind, description)
	change.Reason = cloneReason(reason)
	return change
}

// tokenRemapChange emits a typed non-breaking token migration event for resolved remaps.
func tokenRemapChange(category, oldToken, newToken string, reason *NormalizationReason, retainedAlias bool) Change {
	description := fmt.Sprintf("token remapped: migrate from %q to %q", oldToken, newToken)
	if retainedAlias {
		description = fmt.Sprintf("token deprecated: prefer %q instead of %q", newToken, oldToken)
	}
	return Change{
		Category:    category,
		Name:        oldToken,
		Kind:        ChangeKindTokenRemapped,
		Severity:    SeverityInfo,
		Breaking:    false,
		Description: description,
		Reason:      cloneReason(reason),
	}
}

// missingWithLookup appends deterministic lookup context for missing diagnostics.
func missingWithLookup(base string, reason *NormalizationReason) string {
	if reason == nil {
		return base
	}
	switch reason.Outcome {
	case NormalizationOutcomeResolved:
		if reason.Token == "" {
			return base
		}
		return fmt.Sprintf("%s (lookup resolved to %q)", base, reason.Token)
	case NormalizationOutcomeAmbiguous:
		if len(reason.Candidates) == 0 {
			return base + " (lookup ambiguous)"
		}
		return fmt.Sprintf("%s (lookup ambiguous: %s)", base, strings.Join(reason.Candidates, ", "))
	default:
		return base
	}
}

// cloneReason copies normalization attribution so emitted changes remain immutable.
func cloneReason(reason *NormalizationReason) *NormalizationReason {
	if reason == nil {
		return nil
	}
	cloned := *reason
	cloned.Candidates = append([]string{}, reason.Candidates...)
	return &cloned
}

// reasonSortKey builds a deterministic lexical key for attribution-based tie breaks.
func reasonSortKey(reason *NormalizationReason) string {
	if reason == nil {
		return ""
	}
	return string(reason.Outcome) + "\x00" + reason.Lookup + "\x00" + reason.Token + "\x00" + strings.Join(reason.Candidates, "\x00")
}
