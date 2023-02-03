package pkg

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func Compare(provider string, schOld, schNew schema.PackageSpec) string {
	var output bytes.Buffer
	var violations []string
	for resName, res := range schOld.Resources {
		newRes, ok := schNew.Resources[resName]
		if !ok {
			violations = append(violations, fmt.Sprintf("Resource %q missing", resName))
			continue
		}

		for propName, prop := range res.InputProperties {
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				violations = append(violations, fmt.Sprintf("Resource %q missing input %q", resName, propName))
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q input %q", resName, propName))
			violations = append(violations, vs...)
		}

		for propName, prop := range res.Properties {
			newProp, ok := newRes.Properties[propName]
			if !ok {
				violations = append(violations, fmt.Sprintf("Resource %q missing output %q", resName, propName))
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q output %q", resName, propName))
			violations = append(violations, vs...)
		}
	}

	for funcName, f := range schOld.Functions {
		newFunc, ok := schNew.Functions[funcName]
		if !ok {
			violations = append(violations, fmt.Sprintf("Function %q missing", funcName))
			continue
		}

		if f.Inputs == nil {
			continue
		}

		for propName, prop := range f.Inputs.Properties {
			if newFunc.Inputs == nil {
				violations = append(violations, fmt.Sprintf("Function %q missing input %q", funcName, propName))
				continue
			}

			newProp, ok := newFunc.Inputs.Properties[propName]
			if !ok {
				violations = append(violations, fmt.Sprintf("Function %q missing input %q", funcName, propName))
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Function %q input %q", funcName, propName))
			violations = append(violations, vs...)
		}

		for propName, prop := range f.Outputs.Properties {
			newProp, ok := newFunc.Outputs.Properties[propName]
			if !ok {
				violations = append(violations, fmt.Sprintf("Function %q missing output %q", funcName, propName))
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Function %q output %q", funcName, propName))
			violations = append(violations, vs...)
		}
	}

	for typName, typ := range schOld.Types {
		newTyp, ok := schNew.Types[typName]
		if !ok {
			violations = append(violations, fmt.Sprintf("Type %q missing", typName))
			continue
		}

		for propName, prop := range typ.Properties {
			newProp, ok := newTyp.Properties[propName]
			if !ok {
				violations = append(violations, fmt.Sprintf("Type %q missing property %q", typName, propName))
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Type %q input %q", typName, propName))
			violations = append(violations, vs...)
		}
	}

	switch len(violations) {
	case 0:
		output.WriteString("Looking good! No breaking changes found.\n")
	case 1:
		output.WriteString("Found 1 breaking change:\n")
	default:
		output.WriteString(fmt.Sprintf("Found %d breaking changes:\n", len(violations)))
	}

	var violationDetails []string
	if len(violations) > 500 {
		violationDetails = violations[0:499]
	} else {
		violationDetails = violations
	}

	for _, v := range violationDetails {
		output.WriteString(v + "\n")
	}

	var newResources, newFunctions []string
	for resName := range schNew.Resources {
		if _, ok := schOld.Resources[resName]; !ok {
			newResources = append(newResources, formatName(provider, resName))
		}
	}
	for resName := range schNew.Functions {
		if _, ok := schOld.Functions[resName]; !ok {
			newFunctions = append(newFunctions, formatName(provider, resName))
		}
	}

	if len(newResources) > 0 {
		output.WriteString("\n#### New resources:\n\n")
		sort.Strings(newResources)
		for _, v := range newResources {
			output.WriteString(fmt.Sprintf("- `%s`\n", v))
		}
	}

	if len(newFunctions) > 0 {
		output.WriteString("\n#### New functions:\n\n")
		sort.Strings(newFunctions)
		for _, v := range newFunctions {
			output.WriteString(fmt.Sprintf("- `%s`\n", v))
		}
	}

	if len(newResources) == 0 && len(newFunctions) == 0 {
		output.WriteString("No new resources/functions.\n")
	}

	return output.String()
}

func validateTypes(old *schema.TypeSpec, new *schema.TypeSpec, prefix string) (violations []string) {
	switch {
	case old == nil && new == nil:
		return
	case old != nil && new == nil:
		violations = append(violations, fmt.Sprintf("had %+v but now has no type", old))
		return
	case old == nil && new != nil:
		violations = append(violations, fmt.Sprintf("had no type but now has %+v", new))
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
		violations = append(violations, fmt.Sprintf("%s type changed from %q to %q", prefix, oldType, newType))
	}
	violations = append(violations, validateTypes(old.Items, new.Items, prefix+" items")...)
	violations = append(violations, validateTypes(old.AdditionalProperties, new.AdditionalProperties, prefix+" additional properties")...)
	return
}

func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
}
