package cmd

import (
	"bytes"
	"context"
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
	var provider, repository, oldCommit, newCommit string
	var oldPath, newPath string
	var maxChanges int

	command := &cobra.Command{
		Use:   "compare",
		Short: "Compare two versions of a Pulumi schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			if newCommit == "" && newPath == "" {
				return fmt.Errorf("either --new-commit or --new-path must be set")
			}
			if newCommit != "" && newPath != "" {
				return fmt.Errorf("--new-commit and --new-path are mutually exclusive")
			}
			if oldCommit != "" && oldPath != "" {
				return fmt.Errorf("--old-commit and --old-path are mutually exclusive")
			}
			return compare(provider, repository, oldCommit, newCommit, oldPath, newPath, maxChanges)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "", "the provider whose schema we are comparing")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&repository, "repository", "r",
		"github://api.github.com/pulumi", "the Git repository to download the schema file from")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&oldCommit, "old-commit", "o", "",
		"the old commit to compare with (defaults to master when no --old-path is set)")
	command.Flags().StringVar(&oldPath, "old-path", "",
		"path to a local schema file to use as the old version")

	command.Flags().StringVarP(&newCommit, "new-commit", "n", "",
		"the new commit to compare against the old commit")
	command.Flags().StringVar(&newPath, "new-path", "",
		"path to a local schema file to use as the new version")

	command.Flags().IntVarP(&maxChanges, "max-changes", "m", 500,
		"the maximum number of breaking changes to display. Pass -1 to display all changes")

	return command
}

func compare(provider string, repository string, oldCommit string, newCommit string, oldPath string, newPath string, maxChanges int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loadLocal := func(path string) (schema.PackageSpec, error) {
		schemaPath, err := filepath.Abs(path)
		if err != nil {
			return schema.PackageSpec{}, fmt.Errorf("unable to construct absolute path to schema.json: %w", err)
		}
		return pkg.LoadLocalPackageSpec(schemaPath)
	}

	var schOld schema.PackageSpec
	schOldDone := make(chan error)
	go func() {
		var err error
		switch {
		case oldPath != "":
			schOld, err = loadLocal(oldPath)
		case oldCommit != "":
			schOld, err = pkg.DownloadSchema(ctx, repository, provider, oldCommit)
		default:
			schOld, err = pkg.DownloadSchema(ctx, repository, provider, "master")
		}
		if err != nil {
			cancel()
		}
		schOldDone <- err
	}()

	var schNew schema.PackageSpec
	if newPath != "" {
		var err error
		schNew, err = loadLocal(newPath)
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(newCommit, "--local-path=") {
		fmt.Fprintln(os.Stderr, "Warning: --local-path= in --new-commit is deprecated, use --new-path instead")
		parts := strings.Split(newCommit, "=")
		if len(parts) < 2 || parts[1] == "" {
			return fmt.Errorf("invalid --local-path value: %q", newCommit)
		}
		var err error
		schNew, err = loadLocal(parts[1])
		if err != nil {
			return err
		}
	} else if newCommit == "--local" {
		fmt.Fprintln(os.Stderr, "Warning: --local in --new-commit is deprecated, use --new-path instead")
		usr, _ := user.Current()
		basePath := fmt.Sprintf("%s/go/src/github.com/pulumi/%s", usr.HomeDir, provider)
		schemaFile := pkg.StandardSchemaPath(provider)
		schemaPath := filepath.Join(basePath, schemaFile)
		var err error
		schNew, err = pkg.LoadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else {
		var err error
		schNew, err = pkg.DownloadSchema(ctx, repository, provider, newCommit)
		if err != nil {
			return err
		}
	}

	if err := <-schOldDone; err != nil {
		return err
	}

	compareSchemas(os.Stdout, provider, schOld, schNew, maxChanges)
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

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec, maxChanges int) {
	fmt.Fprintf(out, "### Does the PR have any schema changes?\n\n")
	violations := breakingChanges(oldSchema, newSchema)
	displayedViolations := new(bytes.Buffer)
	lenViolations := violations.Display(displayedViolations, maxChanges)
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
