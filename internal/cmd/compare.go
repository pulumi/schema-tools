package cmd

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/internal/pkg"
	"github.com/pulumi/schema-tools/internal/util/set"
)

func compareCmd() *cobra.Command {
	var provider, oldCommit, newCommit string

	command := &cobra.Command{
		Use:   "compare",
		Short: "Compare two versions of a Pulumi schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			return compare(provider, oldCommit, newCommit)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "", "the provider whose schema we are comparing")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&oldCommit, "old-commit", "o", "master",
		"the old commit to compare with (defaults to master)")

	command.Flags().StringVarP(&newCommit, "new-commit", "n", "",
		"the new commit to compare against the old commit")
	_ = command.MarkFlagRequired("new-commit")

	return command
}

func compare(provider string, oldCommit string, newCommit string) error {
	schemaUrlOld := fmt.Sprintf("https://raw.githubusercontent.com/pulumi/pulumi-%s/%s/provider/cmd/pulumi-resource-%[1]s/schema.json", provider, oldCommit)
	schOld, err := pkg.DownloadSchema(schemaUrlOld)
	if err != nil {
		return err
	}

	var schNew schema.PackageSpec

	if newCommit == "--local" {
		usr, _ := user.Current()
		basePath := fmt.Sprintf("%s/go/src/github.com/pulumi", usr.HomeDir)
		path := fmt.Sprintf("pulumi-%s/provider/cmd/pulumi-resource-%[1]s", provider)
		schemaPath := filepath.Join(basePath, path, "schema.json")
		schNew, err = pkg.LoadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(newCommit, "--local-path=") {
		parts := strings.Split(newCommit, "=")
		schemaPath, err := filepath.Abs(parts[1])
		if err != nil {
			return fmt.Errorf("unable to construct absolute path to schema.json: %w", err)
		}
		schNew, err = pkg.LoadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else {
		schemaUrlNew := fmt.Sprintf("https://raw.githubusercontent.com/pulumi/pulumi-%s/%s/provider/cmd/pulumi-resource-%[1]s/schema.json", provider, newCommit)
		schNew, err = pkg.DownloadSchema(schemaUrlNew)
		if err != nil {
			return err
		}
	}
	compareSchemas(os.Stdout, provider, schOld, schNew)
	return nil
}

func breakingChanges(oldSchema, newSchema schema.PackageSpec) []string {
	var violations []string

	changedToRequired := func(kind, name string) string {
		return fmt.Sprintf("%s %q has changed to Required", kind, name)
	}
	changedToOptional := func(kind, name string) string {
		return fmt.Sprintf("%s %q is no longer Required", kind, name)
	}

	for resName, res := range oldSchema.Resources {
		violation := func(msg string, args ...any) {
			violations = append(violations, fmt.Sprintf("Resource %q "+msg, append([]any{resName}, args...)...))
		}
		newRes, ok := newSchema.Resources[resName]
		if !ok {
			violation("missing")
			continue
		}

		for propName, prop := range res.InputProperties {
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				violation("missing input %q", propName)
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q input %q", resName, propName))
			violations = append(violations, vs...)
		}

		for propName, prop := range res.Properties {
			newProp, ok := newRes.Properties[propName]
			if !ok {
				violation("missing output %q", propName)
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q output %q", resName, propName))
			violations = append(violations, vs...)
		}

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			if !oldRequiredInputs.Has(input) {
				violation(changedToRequired("input", input))
			}
		}

		newRequiredProperties := set.FromSlice(newRes.Required)
		for _, prop := range res.Required {
			// It is a breaking change to move an output property from
			// required to optional.
			//
			// If the property was removed, but that breaking change will
			// already warned on, so we don't need to warn here.
			_, stillExists := newRes.Properties[prop]
			if !newRequiredProperties.Has(prop) && stillExists {
				violation(changedToOptional("property", prop))
			}
		}
	}

	for funcName, f := range oldSchema.Functions {
		violation := func(msg string, args ...any) {
			violations = append(violations, fmt.Sprintf("Function %q "+msg, append([]any{funcName}, args...)...))
		}
		newFunc, ok := newSchema.Functions[funcName]
		if !ok {
			violation("missing")
			continue
		}

		if f.Inputs != nil {
			for propName, prop := range f.Inputs.Properties {
				if newFunc.Inputs == nil {
					violation("missing input %q", propName)
					continue
				}

				newProp, ok := newFunc.Inputs.Properties[propName]
				if !ok {
					violation("missing input %q", propName)
					continue
				}

				vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Function %q input %q", funcName, propName))
				violations = append(violations, vs...)
			}

			if newFunc.Inputs != nil {
				oldRequired := set.FromSlice(f.Inputs.Required)
				for _, req := range newFunc.Inputs.Required {
					if !oldRequired.Has(req) {
						violation(changedToRequired("input", req))
					}
				}
			}
		}

		if f.Outputs != nil {
			for propName, prop := range f.Outputs.Properties {
				if newFunc.Outputs == nil {
					violation("missing output %q", propName)
					continue
				}

				newProp, ok := newFunc.Outputs.Properties[propName]
				if !ok {
					violation("missing output %q", propName)
					continue
				}

				vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Function %q output %q", funcName, propName))
				violations = append(violations, vs...)
			}

			var newRequired set.Set[string]
			if newFunc.Outputs != nil {
				newRequired = set.FromSlice(newFunc.Outputs.Required)
			}
			for _, req := range f.Outputs.Required {
				_, stillExists := f.Outputs.Properties[req]
				if !newRequired.Has(req) && stillExists {
					violation(changedToOptional("property", req))
				}
			}
		}
	}

	for typName, typ := range oldSchema.Types {
		violation := func(msg string, a ...any) {
			violations = append(violations, fmt.Sprintf("Type %q "+msg, append([]any{typName}, a...)...))
		}
		newTyp, ok := newSchema.Types[typName]
		if !ok {
			violation("missing")
			continue
		}

		for propName, prop := range typ.Properties {
			newProp, ok := newTyp.Properties[propName]
			if !ok {
				violation("missing property %q", propName)
				continue
			}

			vs := validateTypes(&prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Type %q property %q", typName, propName))
			violations = append(violations, vs...)
		}

		// Since we don't know if this type will be consumed by pulumi (as an
		// input) or by the user (as an output), this inherits the strictness of
		// both inputs and outputs.
		newRequired := set.FromSlice(newTyp.Required)
		for _, r := range typ.Required {
			_, stillExists := typ.Properties[r]
			if !newRequired.Has(r) && stillExists {
				violation(changedToOptional("property", r))
			}
		}
		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if !required.Has(r) {
				violation(changedToRequired("property", r))
			}
		}
	}

	return violations
}

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec) {
	violations := breakingChanges(oldSchema, newSchema)
	switch len(violations) {
	case 0:
		fmt.Fprintln(out, "Looking good! No breaking changes found.")
	case 1:
		fmt.Fprintln(out, "Found 1 breaking change:")
	default:
		fmt.Fprintf(out, "Found %d breaking changes:\n", len(violations))
	}

	var violationDetails []string
	if len(violations) > 500 {
		violationDetails = violations[0:499]
	} else {
		violationDetails = violations
	}

	for _, v := range violationDetails {
		fmt.Fprintln(out, v)
	}

	var newResources, newFunctions []string
	for resName := range newSchema.Resources {
		if _, ok := oldSchema.Resources[resName]; !ok {
			newResources = append(newResources, formatName(provider, resName))
		}
	}
	for resName := range newSchema.Functions {
		if _, ok := oldSchema.Functions[resName]; !ok {
			newFunctions = append(newFunctions, formatName(provider, resName))
		}
	}

	if len(newResources) > 0 {
		fmt.Fprintln(out, "\n#### New resources:")
		fmt.Fprintln(out, "")
		sort.Strings(newResources)
		for _, v := range newResources {
			fmt.Fprintf(out, "- `%s`\n", v)
		}
	}

	if len(newFunctions) > 0 {
		fmt.Fprintln(out, "\n#### New functions:")
		fmt.Fprintln(out, "")
		sort.Strings(newFunctions)
		for _, v := range newFunctions {
			fmt.Fprintf(out, "- `%s`\n", v)
		}
	}

	if len(newResources) == 0 && len(newFunctions) == 0 {
		fmt.Fprintln(out, "No new resources/functions.")
	}
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
