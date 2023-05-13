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

type message struct {
	Title       string
	Description string
	Severity    Severity
	Subfields   []*message

	doDisplay bool
	parent    *message
}

func (m *message) subfield(name string) *message {
	contract.Assertf(name != "", "we cannot display an empty name")
	for _, v := range m.Subfields {
		if v.Title == name {
			return v
		}
	}
	v := &message{
		Title:  name,
		parent: m,
	}
	m.Subfields = append(m.Subfields, v)
	return v
}

func (m *message) label(name string) *message {
	return m.subfield(name)
}

func (m *message) value(value string) *message {
	return m.subfield(fmt.Sprintf("%q", value))
}

func (m *message) levelPrefix(level int) string {
	switch level {
	case 0:
		return "###"
	case 1:
		return "####"
	}
	if level < 0 {
		return ""
	}
	return strings.Repeat("  ", (level-2)*2) + "-"
}

func (m *message) display(out io.Writer, level int, prefix bool, max int) int {
	if m == nil || !m.doDisplay || max <= 0 {
		// Nothing to display
		return 0
	}

	var displayed int
	var display string
	if m.Title != "" {
		if prefix {
			display = fmt.Sprintf("%s %s ",
				m.levelPrefix(level),
				m.severity())
		}
		display += m.Title + ": "
		if m.Description != "" {
			displayed += 1
			display += m.Description
		}

		out.Write([]byte(display))
	}

	if level > 1 && m.Severity == None {
		if s := m.uniqueSuccessor(); s != nil {
			return s.display(out, level, false, max-displayed) + displayed
		}
	}

	out.Write([]byte{'\n'})

	order := make([]int, len(m.Subfields))
	for i := range order {
		order[i] = i
	}
	if level > 0 {
		// Obtain an ordering on the subfields without mutating `.Subfields`.
		sort.Slice(order, func(i, j int) bool {
			return m.Subfields[order[i]].Title < m.Subfields[order[j]].Title
		})
	}

	for _, f := range order {
		n := m.Subfields[f].display(out, level+1, true, max-displayed)
		displayed += n
	}
	return displayed
}

func (m *message) uniqueSuccessor() *message {
	var us *message
	for _, s := range m.Subfields {
		if !s.doDisplay {
			continue
		}
		if us != nil {
			return nil
		}
		us = s
	}
	return us
}

func (m *message) severity() Severity {
	for m != nil {
		s := m.uniqueSuccessor()
		if s == nil {
			return m.Severity
		}
		m = s
	}
	return None
}

type Severity string

const (
	None   Severity = ""
	Info   Severity = "`🟢`"
	Warn   Severity = "`🟡`"
	Danger Severity = "`🔴`"
)

func (m *message) SetDescription(level Severity, msg string, a ...any) {
	for v := m.parent; v != nil && !v.doDisplay; v = v.parent {
		v.doDisplay = true
	}
	m.doDisplay = true
	m.Description = fmt.Sprintf(msg, a...)
	m.Severity = level
}

func breakingChanges(oldSchema, newSchema schema.PackageSpec) *message {
	msg := &message{Title: ""}

	changedToRequired := func(kind, name string) string {
		return fmt.Sprintf("%s %q has changed to Required", kind, name)
	}
	changedToOptional := func(kind, name string) string {
		return fmt.Sprintf("%s %q is no longer Required", kind, name)
	}

	for resName, res := range oldSchema.Resources {
		msg := msg.label("Resources").value(resName)
		newRes, ok := newSchema.Resources[resName]
		if !ok {
			msg.SetDescription(Danger, "missing")
			continue
		}

		for propName, prop := range res.InputProperties {
			msg := msg.label("inputs").value(propName)
			newProp, ok := newRes.InputProperties[propName]
			if !ok {
				msg.SetDescription(Warn, "missing")
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		for propName, prop := range res.Properties {
			msg := msg.label("properties").value(propName)
			newProp, ok := newRes.Properties[propName]
			if !ok {
				msg.SetDescription("missing output %q", propName)
				continue
			}

			validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
		}

		oldRequiredInputs := set.FromSlice(res.RequiredInputs)
		for _, input := range newRes.RequiredInputs {
			msg := msg.label("required inputs").value(input)
			if !oldRequiredInputs.Has(input) {
				msg.SetDescription(Info, changedToRequired("input", input))
			}
		}

		newRequiredProperties := set.FromSlice(newRes.Required)
		for _, prop := range res.Required {
			msg := msg.label("required").value(prop)
			// It is a breaking change to move an output property from
			// required to optional.
			//
			// If the property was removed, that breaking change is
			// already warned on, so we don't need to warn here.
			_, stillExists := newRes.Properties[prop]
			if !newRequiredProperties.Has(prop) && stillExists {
				msg.SetDescription(Info, changedToOptional("property", prop))
			}
		}
	}

	for funcName, f := range oldSchema.Functions {
		msg := msg.label("Functions").value(funcName)
		newFunc, ok := newSchema.Functions[funcName]
		if !ok {
			msg.SetDescription(Danger, "missing")
			continue
		}

		if f.Inputs != nil {
			msg := msg.label("inputs")
			for propName, prop := range f.Inputs.Properties {
				msg := msg.label("properties").value(propName)
				if newFunc.Inputs == nil {
					msg.SetDescription("missing input %q", propName)
					continue
				}

				newProp, ok := newFunc.Inputs.Properties[propName]
				if !ok {
					msg.SetDescription("missing input %q", propName)
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
			}

			if newFunc.Inputs != nil {
				msg := msg.label("required")
				oldRequired := set.FromSlice(f.Inputs.Required)
				for _, req := range newFunc.Inputs.Required {
					msg.value(req)
					if !oldRequired.Has(req) {
						msg.SetDescription(Info, changedToRequired("input", req))
					}
				}
			}
		}

		if f.Outputs != nil {
			msg := msg.label("outputs")
			for propName, prop := range f.Outputs.Properties {
				msg := msg.label("properties").value(propName)
				if newFunc.Outputs == nil {
					msg.SetDescription(Warn, "missing output")
					continue
				}

				newProp, ok := newFunc.Outputs.Properties[propName]
				if !ok {
					msg.SetDescription(Warn, "missing output")
					continue
				}

				validateTypes(&prop.TypeSpec, &newProp.TypeSpec, msg)
			}

			var newRequired set.Set[string]
			if newFunc.Outputs != nil {
				newRequired = set.FromSlice(newFunc.Outputs.Required)
			}
			for _, req := range f.Outputs.Required {
				_, stillExists := f.Outputs.Properties[req]
				if !newRequired.Has(req) && stillExists {
					msg.label("required").value(req).SetDescription(Info, changedToOptional("property", req))
				}
			}
		}
	}

	for typName, typ := range oldSchema.Types {
		msg := msg.label("Types").value(typName)
		newTyp, ok := newSchema.Types[typName]
		if !ok {
			msg.SetDescription(Danger, "missing")
			continue
		}

		for propName, prop := range typ.Properties {
			msg := msg.label("properties").value(propName)
			newProp, ok := newTyp.Properties[propName]
			if !ok {
				msg.SetDescription(Warn, "missing")
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
				msg.label("required").value(r).SetDescription(Info, changedToOptional("property", r))
			}
		}
		required := set.FromSlice(typ.Required)
		for _, r := range newTyp.Required {
			if !required.Has(r) {
				msg.label("required").value(r).SetDescription(Info, changedToRequired("property", r))
			}
		}
	}

	return msg
}

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec) {
	fmt.Fprintf(out, "### Does the PR have any schema changes?\n\n")
	violations := breakingChanges(oldSchema, newSchema)
	displayedViolations := new(bytes.Buffer)
	lenViolations := violations.display(displayedViolations, 0, true, 500)
	switch lenViolations {
	case 0:
		fmt.Fprintln(out, "Looking good! No breaking changes found.")
	case 1:
		fmt.Fprintln(out, "Found 1 breaking change: ")
	default:
		fmt.Fprintf(out, "Found %d breaking changes:\n", lenViolations)
	}

	out.Write(displayedViolations.Bytes())

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

func validateTypes(old *schema.TypeSpec, new *schema.TypeSpec, msg *message) {
	switch {
	case old == nil && new == nil:
		return
	case old != nil && new == nil:
		msg.SetDescription(Warn, "had %+v but now has no type", old)
		return
	case old == nil && new != nil:
		msg.SetDescription(Warn, "had no type but now has %+v", new)
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
		msg.SetDescription("type changed from %q to %q", oldType, newType)
	}

	validateTypes(old.Items, new.Items, msg.label("items"))
	validateTypes(old.AdditionalProperties, new.AdditionalProperties, msg.label("additional properties"))
}

func formatName(provider, s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, fmt.Sprintf("%s:", provider)), ":", ".")
}
