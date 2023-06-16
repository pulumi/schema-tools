package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/internal/pkg"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
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

func breakingChanges(oldSchema, newSchema schema.PackageSpec) *diagtree.Node {
	msg := &diagtree.Node{Title: ""}

	changedToRequired := func(kind string) string {
		return fmt.Sprintf("%s has changed to Required", kind)
	}
	changedToOptional := func(kind string) string {
		return fmt.Sprintf("%s is no longer Required", kind)
	}

	for resName, res := range oldSchema.Resources {
		msg := msg.Label("Resources").Value(resName)
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
		msg := msg.Label("Functions").Value(funcName)
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
				_, stillExists := f.Outputs.Properties[req]
				if !newRequired.Has(req) && stillExists {
					msg.Value(req).SetDescription(
						diagtree.Info, changedToOptional("property"))
				}
			}
		}
	}

	for typName, typ := range oldSchema.Types {
		msg := msg.Label("Types").Value(typName)
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
			_, stillExists := typ.Properties[r]
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

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec) {
	fmt.Fprintf(out, "### Does the PR have any schema changes?\n\n")
	violations := breakingChanges(oldSchema, newSchema)
	displayedViolations := new(bytes.Buffer)
	lenViolations := violations.Display(displayedViolations, 500)
	switch lenViolations {
	case 0:
		fmt.Fprintln(out, "Looking good! No breaking changes found.")
	case 1:
		fmt.Fprintln(out, "Found 1 breaking change: ")
	default:
		fmt.Fprintf(out, "Found %d breaking changes:\n", lenViolations)
	}

	_, err := out.Write(displayedViolations.Bytes())
	contract.AssertNoErrorf(err, "writing to a bytes.Buffer failing indicates OOM")

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

func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
}
