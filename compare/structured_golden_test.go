package compare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestStructuredGoldenBaselines(t *testing.T) {
	jsonData := mustReadStructuredGoldenFile(t, "aws-mini.golden.json")
	textData := string(mustReadStructuredGoldenFile(t, "aws-mini.golden.txt"))

	var payload struct {
		Summary      []SummaryItem  `json:"summary"`
		Changes      []Change       `json:"changes"`
		Grouped      GroupedChanges `json:"grouped"`
		NewResources []string       `json:"new_resources"`
		NewFunctions []string       `json:"new_functions"`
	}
	if err := json.Unmarshal(jsonData, &payload); err != nil {
		t.Fatalf("failed to parse structured golden JSON: %v", err)
	}

	if len(payload.Summary) == 0 || len(payload.Changes) == 0 {
		t.Fatalf("structured golden JSON must include summary and changes: %+v", payload)
	}

	assertGoldenHasChange(t, payload.Changes, "aws:s3/bucket:Bucket", "max-items-one-changed")
	assertGoldenHasChange(t, payload.Changes, "aws:paymentcryptography/key:Key", "max-items-one-changed")
	assertGoldenHasChange(t, payload.Changes, "aws:lb/getListenerRule:getListenerRule", "max-items-one-changed")
	assertGoldenHasChange(t, payload.Changes, "aws:s3/bucketAclV2:BucketAclV2", "deprecated-resource-alias")

	typeChange := assertGoldenHasChange(t, payload.Changes,
		"aws:paymentcryptography/KeyKeyAttributes:KeyKeyAttributes", "type-changed")
	if len(typeChange.ImpactedBy) == 0 || typeChange.ImpactCount == 0 {
		t.Fatalf("type change must carry impact metadata: %+v", typeChange)
	}

	if _, ok := payload.Grouped.Resources["aws:s3/bucket:Bucket"]["inputs"]; !ok {
		t.Fatalf("grouped resources bucket inputs missing: %+v", payload.Grouped.Resources)
	}
	if _, ok := payload.Grouped.Functions["aws:lb/getListenerRule:getListenerRule"]["inputs"]; !ok {
		t.Fatalf("grouped function listener rule inputs missing: %+v", payload.Grouped.Functions)
	}
	if _, ok := payload.Grouped.Types["aws:paymentcryptography/KeyKeyAttributes:KeyKeyAttributes"]["properties"]; !ok {
		t.Fatalf("grouped type properties missing: %+v", payload.Grouped.Types)
	}

	requiredTextSnippets := []string{
		"#### Resources",
		"aws:s3/bucket:Bucket",
		"aws:paymentcryptography/key:Key",
		"#### Functions",
		"aws:lb/getListenerRule:getListenerRule",
		"#### Types",
		"impacted by 1 token",
	}
	for _, snippet := range requiredTextSnippets {
		if !strings.Contains(textData, snippet) {
			t.Fatalf("structured golden text missing %q", snippet)
		}
	}
}

func assertGoldenHasChange(t testing.TB, changes []Change, token, kind string) Change {
	t.Helper()
	for _, change := range changes {
		if change.Token == token && change.Kind == kind {
			return change
		}
	}
	t.Fatalf("missing change token=%q kind=%q in %+v", token, kind, changes)
	return Change{}
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

func mustReadStructuredGoldenFile(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "structured", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return data
}
