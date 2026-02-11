package compare

import (
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/util/diagtree"
	"github.com/pulumi/schema-tools/internal/util/set"
)

// Analyze computes violations and newly introduced resources/functions.
func Analyze(provider string, oldSchema, newSchema schema.PackageSpec) Report {
	var newResources, newFunctions []string
	for resName := range newSchema.Resources {
		if _, ok := oldSchema.Resources[resName]; !ok {
			newResources = append(newResources, formatName(provider, resName))
		}
	}
	for funcName := range newSchema.Functions {
		if _, ok := oldSchema.Functions[funcName]; !ok {
			newFunctions = append(newFunctions, formatName(provider, funcName))
		}
	}

	return Report{
		Violations:   BreakingChanges(oldSchema, newSchema),
		NewResources: newResources,
		NewFunctions: newFunctions,
	}
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
				if candidate, ok := maxItemsOneRename(propName, prop, newRes.InputProperties); ok {
					renamed := newRes.InputProperties[candidate]
					oldType := typeIdentifier(&prop.TypeSpec)
					newType := typeIdentifier(&renamed.TypeSpec)
					msg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
					continue
				}
				msg.SetDescription(diagtree.Warn, "missing")
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		for propName, prop := range res.Properties {
			msg := msg.Label("properties").Value(propName)
			newProp, ok := newRes.Properties[propName]
			if !ok {
				if candidate, ok := maxItemsOneRename(propName, prop, newRes.Properties); ok {
					renamed := newRes.Properties[candidate]
					oldType := typeIdentifier(&prop.TypeSpec)
					newType := typeIdentifier(&renamed.TypeSpec)
					msg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
					continue
				}
				msg.SetDescription(diagtree.Warn, "missing output %q", propName)
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			msg := msg.Label("required inputs").Value(input)
			if !oldRequiredInputs.Has(input) {
				if isMaxItemsOneRenameRequired(input, oldRequiredInputs, res.InputProperties, newRes.InputProperties) {
					continue
				}
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
				if isMaxItemsOneRenameRequiredToOptional(prop, newRequiredProperties, res.Properties, newRes.Properties) {
					continue
				}
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
					if candidate, ok := maxItemsOneRename(propName, prop, newFunc.Inputs.Properties); ok {
						renamed := newFunc.Inputs.Properties[candidate]
						oldType := typeIdentifier(&prop.TypeSpec)
						newType := typeIdentifier(&renamed.TypeSpec)
						msg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
						continue
					}
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
						if isMaxItemsOneRenameRequired(req, oldRequired, f.Inputs.Properties, newFunc.Inputs.Properties) {
							continue
						}
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
					if candidate, ok := maxItemsOneRename(propName, prop, newFunc.Outputs.Properties); ok {
						renamed := newFunc.Outputs.Properties[candidate]
						oldType := typeIdentifier(&prop.TypeSpec)
						newType := typeIdentifier(&renamed.TypeSpec)
						msg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
						continue
					}
					msg.SetDescription(diagtree.Warn, "missing output")
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
			}

			var newRequired set.Set[string]
			var newOutputProperties map[string]schema.PropertySpec
			if newFunc.Outputs != nil {
				newRequired = set.FromSlice(newFunc.Outputs.Required)
				newOutputProperties = newFunc.Outputs.Properties
			}
			msg = msg.Label("required")
			for _, req := range f.Outputs.Required {
				_, stillExists := f.Outputs.Properties[req]
				if !newRequired.Has(req) && stillExists {
					if isMaxItemsOneRenameRequiredToOptional(req, newRequired, f.Outputs.Properties, newOutputProperties) {
						continue
					}
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
				if candidate, ok := maxItemsOneRename(propName, prop, newTyp.Properties); ok {
					renamed := newTyp.Properties[candidate]
					oldType := typeIdentifier(&prop.TypeSpec)
					newType := typeIdentifier(&renamed.TypeSpec)
					msg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
					continue
				}
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
			_, stillExists := typ.Properties[r]
			if !newRequired.Has(r) && stillExists {
				if isMaxItemsOneRenameRequiredToOptional(r, newRequired, typ.Properties, newTyp.Properties) {
					continue
				}
				msg.Label("required").Value(r).SetDescription(
					diagtree.Info, changedToOptional("property"))
			}
		}
		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if !required.Has(r) {
				if isMaxItemsOneRenameRequired(r, required, typ.Properties, newTyp.Properties) {
					continue
				}
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
		if isMaxItemsOneChange(old, new) {
			msg.SetDescription(diagtree.Warn, changedToMaxItemsOne(oldType, newType))
			return
		}
		msg.SetDescription(diagtree.Warn, "type changed from %q to %q", oldType, newType)
	}

	validateTypes(old.Items, new.Items, msg.Label("items"))
	validateTypes(old.AdditionalProperties, new.AdditionalProperties, msg.Label("additional properties"))
}
