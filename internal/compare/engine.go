package compare

import (
	"strings"

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
	collectResourceViolations(msg, oldSchema.Resources, newSchema.Resources)
	collectFunctionViolations(msg, oldSchema.Functions, newSchema.Functions)
	collectTypeViolations(msg, oldSchema.Types, newSchema.Types)

	msg.Prune()
	return msg
}

func collectResourceViolations(
	root *diagtree.Node,
	oldResources map[string]schema.ResourceSpec,
	newResources map[string]schema.ResourceSpec,
) {
	for resName, res := range oldResources {
		msg := root.Label(resourcesCategory).Value(resName)
		newRes, ok := newResources[resName]
		if !ok {
			msg.SetDescription(diagtree.Danger, "missing")
			continue
		}

		collectPropertyTypeViolations(msg.Label("inputs"), res.InputProperties, newRes.InputProperties, "missing")
		collectPropertyTypeViolations(msg.Label("properties"), res.Properties, newRes.Properties, "missing output %q")

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			requiredMsg := msg.Label("required inputs").Value(input)
			if oldRequiredInputs.Has(input) {
				continue
			}
			if isMaxItemsOneRenameRequired(input, oldRequiredInputs, res.InputProperties, newRes.InputProperties) {
				continue
			}
			requiredMsg.SetDescription(diagtree.Info, changedToRequired("input"))
		}

		newRequiredProperties := set.FromSlice(newRes.Required)
		for _, prop := range res.Required {
			requiredMsg := msg.Label("required").Value(prop)
			_, stillExists := newRes.Properties[prop]
			if !newRequiredProperties.Has(prop) && stillExists {
				if isMaxItemsOneRenameRequiredToOptional(prop, newRequiredProperties, res.Properties, newRes.Properties) {
					continue
				}
				requiredMsg.SetDescription(diagtree.Info, changedToOptional("property"))
			}
		}
	}
}

func collectFunctionViolations(
	root *diagtree.Node,
	oldFunctions map[string]schema.FunctionSpec,
	newFunctions map[string]schema.FunctionSpec,
) {
	for funcName, f := range oldFunctions {
		msg := root.Label(functionsCategory).Value(funcName)
		newFunc, ok := newFunctions[funcName]
		if !ok {
			msg.SetDescription(diagtree.Danger, "missing")
			continue
		}

		collectFunctionInputViolations(msg, f, newFunc)
		collectFunctionSignatureViolations(msg, f, newFunc)
		collectFunctionOutputViolations(msg, f, newFunc)
	}
}

func collectFunctionInputViolations(msg *diagtree.Node, oldFunc, newFunc schema.FunctionSpec) {
	if oldFunc.Inputs == nil {
		return
	}

	inputsMsg := msg.Label("inputs")
	for propName, prop := range oldFunc.Inputs.Properties {
		propMsg := inputsMsg.Value(propName)
		if newFunc.Inputs == nil {
			propMsg.SetDescription(diagtree.Warn, "missing input %q", propName)
			continue
		}

		newProp, ok := newFunc.Inputs.Properties[propName]
		if !ok {
			if candidate, ok := maxItemsOneRename(propName, prop, newFunc.Inputs.Properties); ok {
				renamed := newFunc.Inputs.Properties[candidate]
				oldType := typeIdentifier(&prop.TypeSpec)
				newType := typeIdentifier(&renamed.TypeSpec)
				propMsg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
				continue
			}
			propMsg.SetDescription(diagtree.Warn, "missing input %q", propName)
			continue
		}
		validateTypes(&prop.TypeSpec, &newProp.TypeSpec, propMsg)
	}

	if newFunc.Inputs == nil {
		return
	}

	requiredMsg := inputsMsg.Label("required")
	oldRequired := set.FromSlice(oldFunc.Inputs.Required)
	for _, req := range newFunc.Inputs.Required {
		if oldRequired.Has(req) {
			continue
		}
		if isMaxItemsOneRenameRequired(req, oldRequired, oldFunc.Inputs.Properties, newFunc.Inputs.Properties) {
			continue
		}
		requiredMsg.Value(req).SetDescription(diagtree.Info, changedToRequired("input"))
	}
}

func collectFunctionSignatureViolations(msg *diagtree.Node, oldFunc, newFunc schema.FunctionSpec) {
	// The upstream issue is tracked at
	// https://github.com/pulumi/pulumi/issues/13563.
	isNonZeroArgs := func(ts *schema.ObjectTypeSpec) bool {
		if ts == nil {
			return false
		}
		return len(ts.Properties) > 0
	}
	type nonZeroArgs struct{ old, new bool }
	switch (nonZeroArgs{old: isNonZeroArgs(oldFunc.Inputs), new: isNonZeroArgs(newFunc.Inputs)}) {
	case nonZeroArgs{false, true}:
		msg.SetDescription(diagtree.Danger,
			"signature change (pulumi.InvokeOptions)->T => (Args, pulumi.InvokeOptions)->T")
	case nonZeroArgs{true, false}:
		msg.SetDescription(diagtree.Danger,
			"signature change (Args, pulumi.InvokeOptions)->T => (pulumi.InvokeOptions)->T")
	}
}

func collectFunctionOutputViolations(msg *diagtree.Node, oldFunc, newFunc schema.FunctionSpec) {
	if oldFunc.Outputs == nil {
		return
	}

	outputsMsg := msg.Label("outputs")
	for propName, prop := range oldFunc.Outputs.Properties {
		propMsg := outputsMsg.Value(propName)
		if newFunc.Outputs == nil {
			propMsg.SetDescription(diagtree.Warn, "missing output")
			continue
		}

		newProp, ok := newFunc.Outputs.Properties[propName]
		if !ok {
			if candidate, ok := maxItemsOneRename(propName, prop, newFunc.Outputs.Properties); ok {
				renamed := newFunc.Outputs.Properties[candidate]
				oldType := typeIdentifier(&prop.TypeSpec)
				newType := typeIdentifier(&renamed.TypeSpec)
				propMsg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
				continue
			}
			propMsg.SetDescription(diagtree.Warn, "missing output")
			continue
		}
		validateTypes(&prop.TypeSpec, &newProp.TypeSpec, propMsg)
	}

	var newRequired set.Set[string]
	var newOutputProperties map[string]schema.PropertySpec
	if newFunc.Outputs != nil {
		newRequired = set.FromSlice(newFunc.Outputs.Required)
		newOutputProperties = newFunc.Outputs.Properties
	}
	requiredMsg := outputsMsg.Label("required")
	for _, req := range oldFunc.Outputs.Required {
		_, stillExists := newOutputProperties[req]
		if !newRequired.Has(req) && stillExists {
			if isMaxItemsOneRenameRequiredToOptional(req, newRequired, oldFunc.Outputs.Properties, newOutputProperties) {
				continue
			}
			requiredMsg.Value(req).SetDescription(diagtree.Info, changedToOptional("property"))
		}
	}
}

func collectTypeViolations(
	root *diagtree.Node,
	oldTypes map[string]schema.ComplexTypeSpec,
	newTypes map[string]schema.ComplexTypeSpec,
) {
	for typName, typ := range oldTypes {
		msg := root.Label(typesCategory).Value(typName)
		newTyp, ok := newTypes[typName]
		if !ok {
			msg.SetDescription(diagtree.Danger, "missing")
			continue
		}

		collectPropertyTypeViolations(msg.Label("properties"), typ.Properties, newTyp.Properties, "missing")

		// Since we don't know if this type will be consumed by pulumi (as an
		// input) or by the user (as an output), this inherits the strictness of
		// both inputs and outputs.
		newRequired := set.FromSlice(newTyp.Required)
		requiredMsg := msg.Label("required")
		for _, r := range typ.Required {
			_, stillExists := newTyp.Properties[r]
			if !newRequired.Has(r) && stillExists {
				if isMaxItemsOneRenameRequiredToOptional(r, newRequired, typ.Properties, newTyp.Properties) {
					continue
				}
				requiredMsg.Value(r).SetDescription(diagtree.Info, changedToOptional("property"))
			}
		}

		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if required.Has(r) {
				continue
			}
			if isMaxItemsOneRenameRequired(r, required, typ.Properties, newTyp.Properties) {
				continue
			}
			requiredMsg.Value(r).SetDescription(diagtree.Info, changedToRequired("property"))
		}
	}
}

func collectPropertyTypeViolations(
	base *diagtree.Node,
	oldProps map[string]schema.PropertySpec,
	newProps map[string]schema.PropertySpec,
	missingFormat string,
) {
	for propName, prop := range oldProps {
		propMsg := base.Value(propName)
		newProp, ok := newProps[propName]
		if !ok {
			if candidate, ok := maxItemsOneRename(propName, prop, newProps); ok {
				renamed := newProps[candidate]
				oldType := typeIdentifier(&prop.TypeSpec)
				newType := typeIdentifier(&renamed.TypeSpec)
				propMsg.SetDescription(diagtree.Warn, changedToMaxItemsOneRename(oldType, newType, candidate))
				continue
			}
			if strings.Contains(missingFormat, "%") {
				propMsg.SetDescription(diagtree.Warn, missingFormat, propName)
			} else {
				propMsg.SetDescription(diagtree.Warn, missingFormat)
			}
			continue
		}
		validateTypes(&prop.TypeSpec, &newProp.TypeSpec, propMsg)
	}
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
