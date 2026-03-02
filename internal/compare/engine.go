package compare

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/util/set"
)

// Analyze computes typed changes and newly introduced resources/functions.
func Analyze(provider string, oldSchema, newSchema schema.PackageSpec) Report {
	changes, newResources, newFunctions := buildChanges(provider, oldSchema, newSchema)
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

func buildChanges(provider string, oldSchema, newSchema schema.PackageSpec) ([]Change, []string, []string) {
	changes := []Change{}
	newResources := []string{}
	newFunctions := []string{}

	for _, resName := range sortedMapKeys(oldSchema.Resources) {
		res := oldSchema.Resources[resName]
		newRes, ok := newSchema.Resources[resName]
		if !ok {
			changes = append(changes, newChange(resourcesCategory, resName, nil, ChangeKindMissingResource, "missing"))
			continue
		}

		for _, propName := range sortedMapKeys(res.InputProperties) {
			prop := res.InputProperties[propName]
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				changes = append(changes, newChange(resourcesCategory, resName, []string{"inputs", propName}, ChangeKindMissingInput, "missing"))
				continue
			}
			appendTypeChanges(&changes, resourcesCategory, resName, []string{"inputs", propName}, &prop.TypeSpec, &newProp.TypeSpec)
		}

		for _, propName := range sortedMapKeys(res.Properties) {
			prop := res.Properties[propName]
			newProp, ok := newRes.Properties[propName]
			if !ok {
				changes = append(changes, newChange(resourcesCategory, resName, []string{"properties", propName}, ChangeKindMissingOutput, fmt.Sprintf("missing output %q", propName)))
				continue
			}
			appendTypeChanges(&changes, resourcesCategory, resName, []string{"properties", propName}, &prop.TypeSpec, &newProp.TypeSpec)
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
		if _, ok := oldSchema.Resources[resName]; !ok {
			changes = append(changes, newChange(resourcesCategory, resName, nil, ChangeKindNewResource, "added"))
			newResources = append(newResources, formatName(provider, resName))
		}
	}

	for _, funcName := range sortedMapKeys(oldSchema.Functions) {
		f := oldSchema.Functions[funcName]
		newFunc, ok := newSchema.Functions[funcName]
		if !ok {
			changes = append(changes, newChange(functionsCategory, funcName, nil, ChangeKindMissingFunction, "missing"))
			continue
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
					changes = append(changes, newChange(functionsCategory, funcName, []string{"inputs", propName}, ChangeKindMissingInput, fmt.Sprintf("missing input %q", propName)))
					continue
				}
				appendTypeChanges(&changes, functionsCategory, funcName, []string{"inputs", propName}, &prop.TypeSpec, &newProp.TypeSpec)
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
					changes = append(changes, newChange(functionsCategory, funcName, []string{"outputs", propName}, ChangeKindMissingOutput, "missing output"))
					continue
				}
				appendTypeChanges(&changes, functionsCategory, funcName, []string{"outputs", propName}, &prop.TypeSpec, &newProp.TypeSpec)
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
		if _, ok := oldSchema.Functions[funcName]; !ok {
			changes = append(changes, newChange(functionsCategory, funcName, nil, ChangeKindNewFunction, "added"))
			newFunctions = append(newFunctions, formatName(provider, funcName))
		}
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
				changes = append(changes, newChange(typesCategory, typName, []string{"properties", propName}, ChangeKindMissingProperty, "missing"))
				continue
			}
			appendTypeChanges(&changes, typesCategory, typName, []string{"properties", propName}, &prop.TypeSpec, &newProp.TypeSpec)
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

func appendTypeChanges(changes *[]Change, category, name string, path []string, old, new *schema.TypeSpec) {
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
	}

	appendTypeChanges(changes, category, name, append(slices.Clone(path), "items"), old.Items, new.Items)
	appendTypeChanges(changes, category, name, append(slices.Clone(path), "additional properties"), old.AdditionalProperties, new.AdditionalProperties)
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
		return changes[i].Description < changes[j].Description
	})
}
