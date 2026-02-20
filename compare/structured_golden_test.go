package compare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestStructuredAWSMiniFixturesLoad(t *testing.T) {
	oldSchema := mustReadAWSMiniSchema(t, "schema-v6.83.0.json")
	newSchema := mustReadAWSMiniSchema(t, "schema-v7.0.0.json")
	mustReadAWSMiniMetadata(t, "metadata-v6.83.0.json")
	mustReadAWSMiniMetadata(t, "metadata-v7.0.0.json")

	requiredResources := []string{
		"aws:s3/bucket:Bucket",
		"aws:paymentcryptography/key:Key",
	}
	for _, token := range requiredResources {
		if _, ok := oldSchema.Resources[token]; !ok {
			t.Fatalf("old schema missing resource token %q", token)
		}
		if _, ok := newSchema.Resources[token]; !ok {
			t.Fatalf("new schema missing resource token %q", token)
		}
	}

	requiredFunctions := []string{"aws:lb/getListenerRule:getListenerRule"}
	for _, token := range requiredFunctions {
		if _, ok := oldSchema.Functions[token]; !ok {
			t.Fatalf("old schema missing function token %q", token)
		}
		if _, ok := newSchema.Functions[token]; !ok {
			t.Fatalf("new schema missing function token %q", token)
		}
	}

	requiredTypes := []string{"aws:paymentcryptography/KeyKeyAttributes:KeyKeyAttributes"}
	for _, token := range requiredTypes {
		if _, ok := oldSchema.Types[token]; !ok {
			t.Fatalf("old schema missing type token %q", token)
		}
		if _, ok := newSchema.Types[token]; !ok {
			t.Fatalf("new schema missing type token %q", token)
		}
	}
}

func mustReadAWSMiniSchema(t testing.TB, name string) schema.PackageSpec {
	t.Helper()
	data := mustReadAWSMiniFixtureFile(t, name)
	var pkg schema.PackageSpec
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("failed to parse aws mini schema %q: %v", name, err)
	}
	return pkg
}

func mustReadAWSMiniMetadata(t testing.TB, name string) map[string]any {
	t.Helper()
	data := mustReadAWSMiniFixtureFile(t, name)
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to parse aws mini metadata %q: %v", name, err)
	}
	return payload
}

func mustReadAWSMiniFixtureFile(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "testdata", "compare-v2", "aws-mini", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return data
}
