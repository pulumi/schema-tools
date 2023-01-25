package cmd

import (
	"fmt"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/pkg"
	"github.com/spf13/cobra"
	"os/user"
	"path/filepath"
	"strings"
)

func experimentalCmd() *cobra.Command {
	var oldRes, newRes, res string
	command := &cobra.Command{
		Use:   "squeeze",
		Short: "Utilities to compare Azure Native versions on backward compatibility",
		RunE: func(cmd *cobra.Command, args []string) error {
			if oldRes != "" && newRes != "" {
				return compareTwo(oldRes, newRes)
			}
			if res != "" {
				return compareGroup(res)
			}
			return compareAll()
		},
	}
	command.Flags().StringVarP(&oldRes, "old", "o", "", "old resource name")
	command.Flags().StringVarP(&newRes, "new", "n", "", "new resource name")
	command.Flags().StringVarP(&res, "resource", "r", "", "resource (default) name")

	return command
}

func compareTwo(oldName string, newName string) error {
	sch, err := readSchema()
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

func compareGroup(groupName string) error {
	sch, err := readSchema()
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

func compareAll() error {
	sch, err := readSchema()
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
	for _, name := range sortedKeys {
		group := resourceMap[name]
		unique := calculateUniqueVersions(sch, group)
		fmt.Printf("%s can be reduced from %d to %d\n", name, len(group), len(unique))
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

		vs := validateTypesDeep(sch, &prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q input %q", newName, propName))
		violations = append(violations, vs...)
	}

	for propName, prop := range oldRes.Properties {
		newProp, ok := newRes.Properties[propName]
		if !ok {
			violations = append(violations, fmt.Sprintf("Resource %q missing output %q", newName, propName))
			continue
		}

		vs := validateTypesDeep(sch, &prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Resource %q output %q", newName, propName))
		violations = append(violations, vs...)
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

func validateTypesDeep(sch *schema.PackageSpec, old *schema.TypeSpec, new *schema.TypeSpec, prefix string) (violations []string) {
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
		if strings.HasPrefix(oldType, "#/types/azure-native") &&
			strings.HasPrefix(newType, "#/types/azure-native") &&
			pkg.VersionlessName(oldType) == pkg.VersionlessName(newType) {
			// Both are reference types, let's do a deep comparison
			oldTypeRef := sch.Types[oldType]
			newTypeRef := sch.Types[newType]
			for propName, prop := range oldTypeRef.Properties {
				newProp, ok := newTypeRef.Properties[propName]
				if !ok {
					violations = append(violations, fmt.Sprintf("Type %q missing input %q", newType, propName))
					continue
				}

				vs := validateTypesDeep(sch, &prop.TypeSpec, &newProp.TypeSpec, fmt.Sprintf("Type %q input %q", newType, propName))
				violations = append(violations, vs...)
			}
		} else {
			violations = append(violations, fmt.Sprintf("%s type changed from %q to %q", prefix, oldType, newType))
		}
	}
	violations = append(violations, validateTypesDeep(sch, old.Items, new.Items, prefix+" items")...)
	violations = append(violations, validateTypesDeep(sch, old.AdditionalProperties, new.AdditionalProperties, prefix+" additional properties")...)
	return
}

func readSchema() (*schema.PackageSpec, error) {
	var sch schema.PackageSpec
	usr, _ := user.Current()
	basePath := fmt.Sprintf("%s/go/src/github.com/pulumi", usr.HomeDir)
	path := fmt.Sprintf("pulumi-%s/provider/cmd/pulumi-resource-%[1]s", "azure-native")
	schemaPath := filepath.Join(basePath, path, "schema-full.json")
	sch, err := pkg.LoadLocalPackageSpec(schemaPath)
	if err != nil {
		return nil, err
	}
	return &sch, nil
}
