package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/pkg"
	"github.com/spf13/cobra"
)

func squeezeCmd() *cobra.Command {
	var oldRes, newRes, res, source, out string
	command := &cobra.Command{
		Use:   "squeeze",
		Short: "Utilities to compare Azure Native versions on backward compatibility",
		RunE: func(cmd *cobra.Command, args []string) error {
			if source == "" {
				return fmt.Errorf("source path is required")
			}
			if oldRes != "" && newRes != "" {
				return compareTwo(source, oldRes, newRes)
			}
			if res != "" {
				return compareGroup(source, res)
			}
			return compareAll(source, out)
		},
	}
	command.Flags().StringVarP(&oldRes, "old", "o", "", "old resource name")
	command.Flags().StringVarP(&newRes, "new", "n", "", "new resource name")
	command.Flags().StringVarP(&source, "source", "s", "", "source schema path")
	command.Flags().StringVarP(&res, "resource", "r", "", "resource (default) name")
	command.Flags().StringVar(&out, "out", "", "replacements output path (when comparing all resources)")

	return command
}

func compareTwo(path, oldName, newName string) error {
	sch, err := readSchema(path)
	if err != nil {
		return err
	}

	violations, err := compareResources(sch, oldName, newName)
	if err != nil {
		return err
	}
	switch len(violations) {
	case 0:
		fmt.Println("Looking good! No breaking changes found.")
	case 1:
		fmt.Println("Found 1 breaking change:")
	default:
		fmt.Printf("Found %d breaking changes:\n", len(violations))
	}

	var violationDetails []string
	if len(violations) > 500 {
		violationDetails = violations[0:499]
	} else {
		violationDetails = violations
	}

	for _, v := range violationDetails {
		fmt.Println(v)
	}
	return nil
}

func compareGroup(path, groupName string) error {
	sch, err := readSchema(path)
	if err != nil {
		return err
	}

	resVersions := codegen.StringSet{}
	for name := range sch.Resources {
		if !strings.Contains(name, "/") {
			continue
		}
		if groupName == pkg.VersionlessName(name) {
			resVersions.Add(name)
		}
	}

	uniqueVersions := calculateUniqueVersions(sch, resVersions)

	fmt.Println("All versions:")
	for _, name := range resVersions.SortedValues() {
		fmt.Printf("%s\n", name)
	}
	fmt.Println("Not forward-compatible versions:")
	for _, name := range uniqueVersions.SortedValues() {
		fmt.Printf("%s\n", name)
	}

	return nil
}

func compareAll(path, out string) error {
	sch, err := readSchema(path)
	if err != nil {
		return err
	}

	resourceMap := map[string]codegen.StringSet{}
	for name := range sch.Resources {
		vls := pkg.VersionlessName(name)
		if existing, ok := resourceMap[vls]; ok {
			existing.Add(name)
		} else {
			resourceMap[vls] = codegen.NewStringSet(name)
		}
	}

	sortedKeys := codegen.SortedKeys(resourceMap)
	replacements := map[string]string{}
	for _, name := range sortedKeys {
		group := resourceMap[name]
		unique := calculateUniqueVersions(sch, group)
		reduced := group.Subtract(unique)
		for r := range reduced {
			fmt.Println(r)
		}
		for k := range reduced {
			for _, a := range codegen.SortedKeys(unique) {
				if a > k {
					replacements[k] = a
					break
				}
			}
		}
	}

	if out != "" {
		return writeJSONToFile(out, replacements)
	}
	return nil
}

func compareResources(sch *schema.PackageSpec, oldName string, newName string) ([]string, error) {
	var violations []string
	oldRes, ok := sch.Resources[oldName]
	if !ok {
		return nil, fmt.Errorf("resource %q missing", oldName)
	}
	newRes, ok := sch.Resources[newName]
	if !ok {
		return nil, fmt.Errorf("resource %q missing", newName)
	}

	for propName, prop := range oldRes.InputProperties {
		newProp, ok := newRes.InputProperties[propName]
		if !ok {
			violations = append(violations, fmt.Sprintf("Resource %q missing input %q", newName, propName))
			continue
		}

		vs := validateTypesDeep(sch, &prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q input %q", newName, propName), true)
		violations = append(violations, vs...)
	}

	for propName, prop := range oldRes.Properties {
		newProp, ok := newRes.Properties[propName]
		if !ok {
			violations = append(violations, fmt.Sprintf("Resource %q missing output %q", newName, propName))
			continue
		}

		vs := validateTypesDeep(sch, &prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q output %q", newName, propName), false)
		violations = append(violations, vs...)
	}

	oldRequiredSet := codegen.NewStringSet(oldRes.RequiredInputs...)
	for _, propName := range newRes.RequiredInputs {
		if !oldRequiredSet.Has(propName) {
			violations = append(violations, fmt.Sprintf("Resource %q has a new required input %q", newName, propName))
		}
	}

	newRequiredSet := codegen.NewStringSet(newRes.Required...)
	for _, propName := range oldRes.Required {
		if !newRequiredSet.Has(propName) {
			violations = append(violations, fmt.Sprintf("Resource %q has output %q that is not required anymore", newName, propName))
		}
	}

	return violations, nil
}

func calculateUniqueVersions(sch *schema.PackageSpec, resVersions codegen.StringSet) codegen.StringSet {
	uniqueVersions := codegen.StringSet{}

outer:
	for _, oldName := range resVersions.SortedValues() {
		for _, newName := range resVersions.SortedValues() {
			if oldName >= newName {
				continue
			}
			violations, err := compareResources(sch, oldName, newName)
			if err == nil && len(violations) == 0 {
				continue outer
			}
		}
		uniqueVersions.Add(oldName)
	}
	return uniqueVersions
}

func validateTypesDeep(sch *schema.PackageSpec, old *schema.TypeSpec, new *schema.TypeSpec, prefix string, input bool) (violations []string) {
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
		if strings.HasPrefix(oldType, "#/types/azure-native") && //azure-native:resources/v20210101:MyType
			strings.HasPrefix(newType, "#/types/azure-native") &&
			pkg.VersionlessName(oldType) == pkg.VersionlessName(newType) { // resources:MyType
			// Both are reference types, let's do a deep comparison
			oldTypeRef := sch.Types[oldType]
			newTypeRef := sch.Types[newType]
			for propName, prop := range oldTypeRef.Properties {
				newProp, ok := newTypeRef.Properties[propName]
				if !ok {
					violations = append(violations, fmt.Sprintf("Type %q missing input %q", newType, propName))
					continue
				}

				vs := validateTypesDeep(sch, &prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Type %q input %q", newType, propName), input)
				violations = append(violations, vs...)
			}

			if input {
				oldRequiredSet := codegen.NewStringSet(oldTypeRef.Required...)
				for _, propName := range newTypeRef.Required {
					if !oldRequiredSet.Has(propName) {
						violations = append(violations, fmt.Sprintf("Type %q has a new required input %q", newType, propName))
					}
				}
			} else {
				newRequiredSet := codegen.NewStringSet(newTypeRef.Required...)
				for _, propName := range oldTypeRef.Required {
					if !newRequiredSet.Has(propName) {
						violations = append(violations, fmt.Sprintf("Type %q has output %q that is not required anymore", newType, propName))
					}
				}
			}
		} else {
			violations = append(violations, fmt.Sprintf("%s type changed from %q to %q", prefix, oldType, newType))
		}
	}
	violations = append(violations, validateTypesDeep(sch, old.Items, new.Items, prefix+" items", input)...)
	violations = append(violations, validateTypesDeep(sch, old.AdditionalProperties, new.AdditionalProperties, prefix+" additional properties", input)...)
	return
}

func readSchema(path string) (*schema.PackageSpec, error) {
	sch, err := pkg.LoadLocalPackageSpec(path)
	if err != nil {
		return nil, err
	}
	return &sch, nil
}

func writeJSONToFile(filename string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializing to JSON: %w", err)
	}

	err = ioutil.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("error writing JSON data to file: %w", err)
	}

	return nil
}
