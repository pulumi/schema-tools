package compare

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/util/diagtree"
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
		Violations:   BreakingChanges(oldSchema, newSchema),
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

// BreakingChanges builds the diagnostics tree for schema incompatibilities.
func BreakingChanges(oldSchema, newSchema schema.PackageSpec) *diagtree.Node {
	msg := &diagtree.Node{Title: ""}

	for resName, res := range oldSchema.Resources {
		msg := msg.Label(resourcesCategory).Value(resName)
		newRes, ok := newSchema.Resources[resName]
		if !ok {
			msg.SetDescription(diagtree.Danger, "missing")
			continue
		}

		for propName, prop := range res.InputProperties {
			msg := msg.Label("inputs").Value(propName)
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				msg.SetDescription(diagtree.Warn, "missing")
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		for propName, prop := range res.Properties {
			msg := msg.Label("properties").Value(propName)
			newProp, ok := newRes.Properties[propName]
			if !ok {
				msg.SetDescription(diagtree.Warn, "missing output %q", propName)
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			msg := msg.Label("required inputs").Value(input)
			if !oldRequiredInputs.Has(input) {
				msg.SetDescription(diagtree.Info, changedToRequired("input"))
			}
		}

		newRequiredProperties := set.FromSlice(newRes.Required)
		for _, prop := range res.Required {
			msg := msg.Label("required").Value(prop)
			// It is a breaking change to move an output property from
			// required to optional.
			//
			// If the property was removed, that breaking change is
			// already warned on, so we don't need to warn here.
			_, stillExists := newRes.Properties[prop]
			if !newRequiredProperties.Has(prop) && stillExists {
				msg.SetDescription(diagtree.Info, changedToOptional("property"))
			}
		}
	}

	for funcName, f := range oldSchema.Functions {
		msg := msg.Label(functionsCategory).Value(funcName)
		newFunc, ok := newSchema.Functions[funcName]
		if !ok {
			msg.SetDescription(diagtree.Danger, "missing")
			continue
		}

		if f.Inputs != nil {
			msg := msg.Label("inputs")
			for propName, prop := range f.Inputs.Properties {
				msg := msg.Value(propName)
				if newFunc.Inputs == nil {
					msg.SetDescription(diagtree.Warn, "missing input %q", propName)
					continue
				}

				newProp, ok := newFunc.Inputs.Properties[propName]
				if !ok {
					msg.SetDescription(diagtree.Warn, "missing input %q", propName)
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
			}

			if newFunc.Inputs != nil {
				msg := msg.Label("required")
				oldRequired := set.FromSlice(f.Inputs.Required)
				for _, req := range newFunc.Inputs.Required {
					if !oldRequired.Has(req) {
						msg.Value(req).SetDescription(diagtree.Info,
							changedToRequired("input"))
					}
				}
			}
		}

		// The upstream issue is tracked at
		// https://github.com/pulumi/pulumi/issues/13563.
		isNonZeroArgs := func(ts *schema.ObjectTypeSpec) bool {
			if ts == nil {
				return false
			}
			return len(ts.Properties) > 0
		}
		type nonZeroArgs struct{ old, new bool }
		switch (nonZeroArgs{old: isNonZeroArgs(f.Inputs), new: isNonZeroArgs(newFunc.Inputs)}) {
		case nonZeroArgs{false, true}:
			msg.SetDescription(diagtree.Danger,
				"signature change (pulumi.InvokeOptions)->T => (Args, pulumi.InvokeOptions)->T")
		case nonZeroArgs{true, false}:
			msg.SetDescription(diagtree.Danger,
				"signature change (Args, pulumi.InvokeOptions)->T => (pulumi.InvokeOptions)->T")
		}

		if f.Outputs != nil {
			msg := msg.Label("outputs")
			for propName, prop := range f.Outputs.Properties {
				msg := msg.Value(propName)
				if newFunc.Outputs == nil {
					msg.SetDescription(diagtree.Warn, "missing output")
					continue
				}

				newProp, ok := newFunc.Outputs.Properties[propName]
				if !ok {
					msg.SetDescription(diagtree.Warn, "missing output")
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
			}

			var newRequired set.Set[string]
			if newFunc.Outputs != nil {
				newRequired = set.FromSlice(newFunc.Outputs.Required)
			}
			msg = msg.Label("required")
			for _, req := range f.Outputs.Required {
				stillExists := false
				if newFunc.Outputs != nil {
					_, stillExists = newFunc.Outputs.Properties[req]
				}
				if !newRequired.Has(req) && stillExists {
					msg.Value(req).SetDescription(
						diagtree.Info, changedToOptional("property"))
				}
			}
		}
	}

	for typName, typ := range oldSchema.Types {
		msg := msg.Label(typesCategory).Value(typName)
		newTyp, ok := newSchema.Types[typName]
		if !ok {
			msg.SetDescription(diagtree.Danger, "missing")
			continue
		}

		for propName, prop := range typ.Properties {
			msg := msg.Label("properties").Value(propName)
			newProp, ok := newTyp.Properties[propName]
			if !ok {
				msg.SetDescription(diagtree.Warn, "missing")
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		// Since we don't know if this type will be consumed by pulumi (as an
		// input) or by the user (as an output), this inherits the strictness of
		// both inputs and outputs.
		newRequired := set.FromSlice(newTyp.Required)
		for _, r := range typ.Required {
			_, stillExists := newTyp.Properties[r]
			if !newRequired.Has(r) && stillExists {
				msg.Label("required").Value(r).SetDescription(
					diagtree.Info, changedToOptional("property"))
			}
		}
		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if !required.Has(r) {
				msg.Label("required").Value(r).SetDescription(
					diagtree.Info, changedToRequired("property"))
			}
		}
	}

	msg.Prune()
	return msg
}

// validateTypes recursively compares schema type shapes and records differences.
func validateTypes(old *schema.TypeSpec, new *schema.TypeSpec, msg *diagtree.Node) {
	switch {
	case old == nil && new == nil:
		return
	case old != nil && new == nil:
		msg.SetDescription(diagtree.Warn, "had %+v but now has no type", old)
		return
	case old == nil && new != nil:
		msg.SetDescription(diagtree.Warn, "had no type but now has %+v", new)
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
		msg.SetDescription(diagtree.Warn, "type changed from %q to %q", oldType, newType)
	}

	validateTypes(old.Items, new.Items, msg.Label("items"))
	validateTypes(old.AdditionalProperties, new.AdditionalProperties, msg.Label("additional properties"))
}
