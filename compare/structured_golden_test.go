package compare

import (
	"bytes"
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

	requiredResourcesBoth := []string{
		"aws:s3/bucket:Bucket",
		"aws:paymentcryptography/key:Key",
		"aws:cur/reportDefinition:ReportDefinition",
		"aws:eks/cluster:Cluster",
	}
	for _, token := range requiredResourcesBoth {
		if _, ok := oldSchema.Resources[token]; !ok {
			t.Fatalf("old schema missing resource token %q", token)
		}
		if _, ok := newSchema.Resources[token]; !ok {
			t.Fatalf("new schema missing resource token %q", token)
		}
	}
	requiredResourcesOldOnly := []string{
		"aws:chime/voiceConnectorOrganization:VoiceConnectorOrganization",
	}
	for _, token := range requiredResourcesOldOnly {
		if _, ok := oldSchema.Resources[token]; !ok {
			t.Fatalf("old schema missing resource token %q", token)
		}
		if _, ok := newSchema.Resources[token]; ok {
			t.Fatalf("new schema should not contain old-only resource token %q", token)
		}
	}

	requiredFunctions := []string{
		"aws:lb/getListenerRule:getListenerRule",
		"aws:bedrock/getInferenceProfiles:getInferenceProfiles",
	}
	for _, token := range requiredFunctions {
		if _, ok := oldSchema.Functions[token]; !ok {
			t.Fatalf("old schema missing function token %q", token)
		}
		if _, ok := newSchema.Functions[token]; !ok {
			t.Fatalf("new schema missing function token %q", token)
		}
	}

	requiredTypes := []string{
		"aws:paymentcryptography/KeyKeyAttributes:KeyKeyAttributes",
		"aws:eks/ClusterCertificateAuthority:ClusterCertificateAuthority",
		"aws:ssoadmin/getApplicationPortalOption:getApplicationPortalOption",
	}
	for _, token := range requiredTypes {
		if _, ok := oldSchema.Types[token]; !ok {
			t.Fatalf("old schema missing type token %q", token)
		}
		if _, ok := newSchema.Types[token]; !ok {
			t.Fatalf("new schema missing type token %q", token)
		}
	}
}

func TestStructuredAWSMiniGoldens(t *testing.T) {
	oldSchema := mustReadAWSMiniSchema(t, "schema-v6.83.0.json")
	newSchema := mustReadAWSMiniSchema(t, "schema-v7.0.0.json")
	oldMetadata := mustParseMetadataCompare(t, string(mustReadAWSMiniFixtureFile(t, "metadata-v6.83.0.json")))
	newMetadata := mustParseMetadataCompare(t, string(mustReadAWSMiniFixtureFile(t, "metadata-v7.0.0.json")))

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "aws",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	gotJSON, err := json.MarshalIndent(NewFullJSONOutput(result), "", "  ")
	if err != nil {
		t.Fatalf("marshal aws-mini structured JSON: %v", err)
	}
	wantJSON := mustReadCompareStructuredGolden(t, "aws-mini.golden.json")
	if strings.TrimSpace(string(gotJSON)) != strings.TrimSpace(string(wantJSON)) {
		t.Fatalf("aws-mini.golden.json mismatch:\n--- got ---\n%s\n--- want ---\n%s", string(gotJSON), string(wantJSON))
	}

	var rendered bytes.Buffer
	if err := RenderText(&rendered, result); err != nil {
		t.Fatalf("render aws-mini structured text: %v", err)
	}
	wantText := mustReadCompareStructuredGolden(t, "aws-mini.golden.txt")
	if strings.TrimSpace(rendered.String()) != strings.TrimSpace(string(wantText)) {
		t.Fatalf("aws-mini.golden.txt mismatch:\n--- got ---\n%s\n--- want ---\n%s", rendered.String(), string(wantText))
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

func mustReadCompareStructuredGolden(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "structured", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return data
}
