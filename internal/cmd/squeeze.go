package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/internal/pkg"
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
		if !pkg.IsVersionedName(name) {
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
		if !pkg.IsVersionedName(name) {
			continue
		}

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

	sortedVersions := resVersions.SortedValues()
	sortApiVersions(sortedVersions)

outer:
	for _, oldName := range sortedVersions {
		for _, newName := range sortedVersions {
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

func apiVersionToDate(apiVersion string) (time.Time, error) {
	if len(apiVersion) < 9 {
		return time.Time{}, fmt.Errorf("invalid API version %q", apiVersion)
	}
	// The API version is in the format YYYY-MM-DD - ignore suffixes like "-preview".
	return time.Parse("20060102", apiVersion[1:9])
}

func compareApiVersions(a, b string) int {
	timeA, err := apiVersionToDate(a)
	if err != nil {
		return strings.Compare(a, b)
	}
	timeB, err := apiVersionToDate(b)
	if err != nil {
		return strings.Compare(a, b)
	}
	timeDiff := timeA.Compare(timeB)
	if timeDiff != 0 {
		return timeDiff
	}

	// Sort private first, preview second, stable last.
	aPrivate := isPrivate(a)
	bPrivate := isPrivate(b)
	if aPrivate != bPrivate {
		if aPrivate {
			return -1
		}
		return 1
	}
	aPreview := isPreview(a)
	bPreview := isPreview(b)
	if aPreview != bPreview {
		if aPreview {
			return -1
		}
		return 1
	}
	return 0
}

func isPreview(apiVersion string) bool {
	lower := strings.ToLower(apiVersion)
	return strings.Contains(lower, "preview") || strings.Contains(lower, "beta")
}

func isPrivate(apiVersion string) bool {
	lower := strings.ToLower(apiVersion)
	return strings.Contains(lower, "private")
}

func sortApiVersions(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		return compareApiVersions(versions[i], versions[j]) < 0
	})
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

	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("error writing JSON data to file: %w", err)
	}

	return nil
}
