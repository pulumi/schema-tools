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

type diffCategory string

const (
	diffMissingResource              diffCategory = "missing-resource"
	diffMissingFunction              diffCategory = "missing-function"
	diffMissingType                  diffCategory = "missing-type"
	diffMissingInput                 diffCategory = "missing-input"
	diffMissingOutput                diffCategory = "missing-output"
	diffMissingProperty              diffCategory = "missing-property"
	diffTypeChangedInput             diffCategory = "type-changed-input"
	diffTypeChangedOutput            diffCategory = "type-changed-output"
	diffTypeChangedIntToNumberInput  diffCategory = "type-changed-int-to-number-input"
	diffTypeChangedIntToNumberOutput diffCategory = "type-changed-int-to-number-output"
	diffOptionalToRequiredInput      diffCategory = "optional-to-required-input"
	diffOptionalToRequiredOutput     diffCategory = "optional-to-required-output"
	diffRequiredToOptionalInput      diffCategory = "required-to-optional-input"
	diffRequiredToOptionalOutput     diffCategory = "required-to-optional-output"
	diffSignatureChanged             diffCategory = "signature-changed"
)

var categoryOrder = []diffCategory{
	diffMissingResource,
	diffMissingFunction,
	diffMissingType,
	diffMissingInput,
	diffMissingOutput,
	diffMissingProperty,
	diffTypeChangedInput,
	diffTypeChangedOutput,
	diffTypeChangedIntToNumberInput,
	diffTypeChangedIntToNumberOutput,
	diffOptionalToRequiredInput,
	diffOptionalToRequiredOutput,
	diffRequiredToOptionalInput,
	diffRequiredToOptionalOutput,
	diffSignatureChanged,
}

type diffFilter struct {
	counts map[diffCategory]int
}

func newDiffFilter() *diffFilter {
	return &diffFilter{
		counts: map[diffCategory]int{},
	}
}

func (f *diffFilter) record(cat diffCategory, node *diagtree.Node, level diagtree.Severity, msg string, a ...any) {
	f.counts[cat]++
	node.SetDescription(level, msg, a...)
}

func (f *diffFilter) hasCounts() bool {
	return len(f.counts) > 0
}

func (f *diffFilter) summaryLines() []string {
	lines := []string{}
	for _, cat := range categoryOrder {
		count := f.counts[cat]
		if count == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %d", cat, count))
	}
	return lines
}

type usageKind int

const (
	usageInput usageKind = iota
	usageOutput
)

type typeUsage struct {
	input  bool
	output bool
}

func (u typeUsage) typeChangeCategory() diffCategory {
	if u.input {
		return diffTypeChangedInput
	}
	return diffTypeChangedOutput
}

func intToNumberCategory(typeCat diffCategory) diffCategory {
	switch typeCat {
	case diffTypeChangedInput:
		return diffTypeChangedIntToNumberInput
	case diffTypeChangedOutput:
		return diffTypeChangedIntToNumberOutput
	default:
		return typeCat
	}
}

func (u typeUsage) optionalToRequiredCategory() diffCategory {
	if u.input {
		return diffOptionalToRequiredInput
	}
	return diffOptionalToRequiredOutput
}

func (u typeUsage) requiredToOptionalCategory() diffCategory {
	if u.input {
		return diffRequiredToOptionalInput
	}
	return diffRequiredToOptionalOutput
}

func localTypeName(ref string) string {
	const marker = "#/types/"
	idx := strings.Index(ref, marker)
	if idx == -1 {
		return ""
	}
	return ref[idx+len(marker):]
}

func buildTypeUsage(spec schema.PackageSpec) map[string]typeUsage {
	usage := map[string]typeUsage{}
	visitedInput := map[string]bool{}
	visitedOutput := map[string]bool{}

	var visitTypeRef func(name string, ctx usageKind)
	var collectRefs func(ts *schema.TypeSpec, ctx usageKind)

	visitTypeRef = func(name string, ctx usageKind) {
		if name == "" {
			return
		}
		switch ctx {
		case usageInput:
			if visitedInput[name] {
				return
			}
			visitedInput[name] = true
		case usageOutput:
			if visitedOutput[name] {
				return
			}
			visitedOutput[name] = true
		}

		current := usage[name]
		if ctx == usageInput {
			current.input = true
		} else {
			current.output = true
		}
		usage[name] = current

		typ, ok := spec.Types[name]
		if !ok {
			return
		}
		for _, prop := range typ.Properties {
			collectRefs(&prop.TypeSpec, ctx)
		}
	}

	collectRefs = func(ts *schema.TypeSpec, ctx usageKind) {
		if ts == nil {
			return
		}
		if name := localTypeName(ts.Ref); name != "" {
			visitTypeRef(name, ctx)
		}
		if ts.Items != nil {
			collectRefs(ts.Items, ctx)
		}
		if ts.AdditionalProperties != nil {
			collectRefs(ts.AdditionalProperties, ctx)
		}
		for i := range ts.OneOf {
			collectRefs(&ts.OneOf[i], ctx)
		}
	}

	for _, res := range spec.Resources {
		for _, prop := range res.InputProperties {
			collectRefs(&prop.TypeSpec, usageInput)
		}
		for _, prop := range res.Properties {
			collectRefs(&prop.TypeSpec, usageOutput)
		}
	}

	for _, fn := range spec.Functions {
		if fn.Inputs != nil {
			for _, prop := range fn.Inputs.Properties {
				collectRefs(&prop.TypeSpec, usageInput)
			}
		}
		if fn.Outputs != nil {
			for _, prop := range fn.Outputs.Properties {
				collectRefs(&prop.TypeSpec, usageOutput)
			}
		}
	}

	return usage
}

func mergeTypeUsage(dst, src map[string]typeUsage) map[string]typeUsage {
	if dst == nil {
		dst = map[string]typeUsage{}
	}
	for name, u := range src {
		current := dst[name]
		current.input = current.input || u.input
		current.output = current.output || u.output
		dst[name] = current
	}
	return dst
}

func compareCmd() *cobra.Command {
	var provider, repository, oldCommit, newCommit string
	var maxChanges int

	command := &cobra.Command{
		Use:   "compare",
		Short: "Compare two versions of a Pulumi schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			return compare(provider, repository, oldCommit, newCommit, maxChanges)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "", "the provider whose schema we are comparing")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&repository, "repository", "r",
		"github://api.github.com/pulumi", "the Git repository to download the schema file from")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&oldCommit, "old-commit", "o", "master",
		"the old commit to compare with (defaults to master)")

	command.Flags().StringVarP(&newCommit, "new-commit", "n", "",
		"the new commit to compare against the old commit")
	_ = command.MarkFlagRequired("new-commit")

	command.Flags().IntVarP(&maxChanges, "max-changes", "m", 500,
		"the maximum number of breaking changes to display. Pass -1 to display all changes")

	return command
}

func compare(provider string, repository string, oldCommit string, newCommit string, maxChanges int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var schOld schema.PackageSpec
	schOldDone := make(chan error)
	go func() {
		var err error
		schOld, err = pkg.DownloadSchema(ctx, repository, provider, oldCommit)
		if err != nil {
			cancel()
		}
		schOldDone <- err
	}()

	var schNew schema.PackageSpec
	if newCommit == "--local" {
		usr, _ := user.Current()
		basePath := fmt.Sprintf("%s/go/src/github.com/pulumi/%s", usr.HomeDir, provider)
		schemaFile := pkg.StandardSchemaPath(provider)
		schemaPath := filepath.Join(basePath, schemaFile)
		var err error
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

func breakingChanges(oldSchema, newSchema schema.PackageSpec, filter *diffFilter) *diagtree.Node {
	msg := &diagtree.Node{Title: ""}
	typeUsage := mergeTypeUsage(buildTypeUsage(oldSchema), buildTypeUsage(newSchema))

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
			filter.record(diffMissingResource, msg, diagtree.Danger, "missing")
			continue
		}

		for propName, prop := range res.InputProperties {
			msg := msg.Label("inputs").Value(propName)
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				filter.record(diffMissingInput, msg, diagtree.Warn, "missing")
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg, diffTypeChangedInput, filter)
		}

		for propName, prop := range res.Properties {
			msg := msg.Label("properties").Value(propName)
			newProp, ok := newRes.Properties[propName]
			if !ok {
				filter.record(diffMissingOutput, msg, diagtree.Warn, "missing output %q", propName)
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg, diffTypeChangedOutput, filter)
		}

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			msg := msg.Label("required inputs").Value(input)
			if !oldRequiredInputs.Has(input) {
				filter.record(diffOptionalToRequiredInput, msg, diagtree.Info, changedToRequired("input"))
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
				filter.record(diffRequiredToOptionalOutput, msg, diagtree.Info, changedToOptional("property"))
			}
		}
	}

	for funcName, f := range oldSchema.Functions {
		msg := msg.Label("Functions").Value(funcName)
		newFunc, ok := newSchema.Functions[funcName]
		if !ok {
			filter.record(diffMissingFunction, msg, diagtree.Danger, "missing")
			continue
		}

		if f.Inputs != nil {
			msg := msg.Label("inputs")
			for propName, prop := range f.Inputs.Properties {
				msg := msg.Value(propName)
				if newFunc.Inputs == nil {
					filter.record(diffMissingInput, msg, diagtree.Warn, "missing input %q", propName)
					continue
				}

				newProp, ok := newFunc.Inputs.Properties[propName]
				if !ok {
					filter.record(diffMissingInput, msg, diagtree.Warn, "missing input %q", propName)
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg, diffTypeChangedInput, filter)
			}

			if newFunc.Inputs != nil {
				msg := msg.Label("required")
				oldRequired := set.FromSlice(f.Inputs.Required)
				for _, req := range newFunc.Inputs.Required {
					if !oldRequired.Has(req) {
						filter.record(diffOptionalToRequiredInput, msg.Value(req), diagtree.Info,
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
			filter.record(diffSignatureChanged, msg, diagtree.Danger,
				"signature change (pulumi.InvokeOptions)->T => (Args, pulumi.InvokeOptions)->T")
		case nonZeroArgs{true, false}:
			filter.record(diffSignatureChanged, msg, diagtree.Danger,
				"signature change (Args, pulumi.InvokeOptions)->T => (pulumi.InvokeOptions)->T")
		}

		if f.Outputs != nil {
			msg := msg.Label("outputs")
			for propName, prop := range f.Outputs.Properties {
				msg := msg.Value(propName)
				if newFunc.Outputs == nil {
					filter.record(diffMissingOutput, msg, diagtree.Warn, "missing output")
					continue
				}

				newProp, ok := newFunc.Outputs.Properties[propName]
				if !ok {
					filter.record(diffMissingOutput, msg, diagtree.Warn, "missing output")
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg, diffTypeChangedOutput, filter)
			}

			var newRequired set.Set[string]
			if newFunc.Outputs != nil {
				newRequired = set.FromSlice(newFunc.Outputs.Required)
			}
			msg = msg.Label("required")
			for _, req := range f.Outputs.Required {
				_, stillExists := f.Outputs.Properties[req]
				if !newRequired.Has(req) && stillExists {
					filter.record(diffRequiredToOptionalOutput, msg.Value(req),
						diagtree.Info, changedToOptional("property"))
				}
			}
		}
	}

	for typName, typ := range oldSchema.Types {
		msg := msg.Label("Types").Value(typName)
		newTyp, ok := newSchema.Types[typName]
		if !ok {
			filter.record(diffMissingType, msg, diagtree.Danger, "missing")
			continue
		}

		for propName, prop := range typ.Properties {
			msg := msg.Label("properties").Value(propName)
			newProp, ok := newTyp.Properties[propName]
			if !ok {
				filter.record(diffMissingProperty, msg, diagtree.Warn, "missing")
				continue
			}

			usage := typeUsage[typName]
			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg, usage.typeChangeCategory(), filter)
		}

		// Since we don't know if this type will be consumed by pulumi (as an
		// input) or by the user (as an output), this inherits the strictness of
		// both inputs and outputs.
		newRequired := set.FromSlice(newTyp.Required)
		for _, r := range typ.Required {
			_, stillExists := typ.Properties[r]
			if !newRequired.Has(r) && stillExists {
				usage := typeUsage[typName]
				filter.record(usage.requiredToOptionalCategory(), msg.Label("required").Value(r),
					diagtree.Info, changedToOptional("property"))
			}
		}
		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if !required.Has(r) {
				usage := typeUsage[typName]
				filter.record(usage.optionalToRequiredCategory(), msg.Label("required").Value(r),
					diagtree.Info, changedToRequired("property"))
			}
		}
	}

	msg.Prune()
	return msg
}

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec, maxChanges int) {
	fmt.Fprintf(out, "### Does the PR have any schema changes?\n\n")
	filter := newDiffFilter()
	violations := breakingChanges(oldSchema, newSchema, filter)
	if filter.hasCounts() {
		fmt.Fprintln(out, "Summary by category:")
		for _, line := range filter.summaryLines() {
			fmt.Fprintf(out, "- %s\n", line)
		}
		fmt.Fprintln(out, "")
	}
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

func validateTypes(old *schema.TypeSpec, new *schema.TypeSpec, msg *diagtree.Node, typeChangeCategory diffCategory, filter *diffFilter) {
	switch {
	case old == nil && new == nil:
		return
	case old != nil && new == nil:
		filter.record(typeChangeCategory, msg, diagtree.Warn, "had %+v but now has no type", old)
		return
	case old == nil && new != nil:
		filter.record(typeChangeCategory, msg, diagtree.Warn, "had no type but now has %+v", new)
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
		if oldType == "integer" && newType == "number" {
			filter.record(intToNumberCategory(typeChangeCategory), msg, diagtree.Warn, "type changed from %q to %q", oldType, newType)
			return
		}
		filter.record(typeChangeCategory, msg, diagtree.Warn, "type changed from %q to %q", oldType, newType)
	}

	validateTypes(old.Items, new.Items, msg.Label("items"), typeChangeCategory, filter)
	validateTypes(old.AdditionalProperties, new.AdditionalProperties, msg.Label("additional properties"), typeChangeCategory, filter)
}

func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
}
