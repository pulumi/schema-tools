package testhelpers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// LoadCompareFixtureSchemas loads the shared compare fixture pair.
func LoadCompareFixtureSchemas() (schema.PackageSpec, schema.PackageSpec, error) {
	oldSchema, err := LoadCompareFixtureSchema("schema-old.json")
	if err != nil {
		return schema.PackageSpec{}, schema.PackageSpec{}, err
	}
	newSchema, err := LoadCompareFixtureSchema("schema-new.json")
	if err != nil {
		return schema.PackageSpec{}, schema.PackageSpec{}, err
	}
	return oldSchema, newSchema, nil
}

// LoadCompareFixtureSchema loads one schema fixture from testdata/compare.
func LoadCompareFixtureSchema(name string) (schema.PackageSpec, error) {
	fixturesDir, err := compareFixturesDir()
	if err != nil {
		return schema.PackageSpec{}, err
	}
	path := filepath.Join(fixturesDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.PackageSpec{}, fmt.Errorf("read compare fixture %q: %w", path, err)
	}

	var spec schema.PackageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return schema.PackageSpec{}, fmt.Errorf("unmarshal compare fixture %q: %w", path, err)
	}
	return spec, nil
}

func compareFixturesDir() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve testhelpers source location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "compare")), nil
}
