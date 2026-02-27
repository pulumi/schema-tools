package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/compare"
	"github.com/pulumi/schema-tools/internal/normalize"
)

const updateStructuredGoldenEnv = "SCHEMA_TOOLS_UPDATE_GOLDEN"

func TestCompareV2StructuredGoldens(t *testing.T) {
	result := mustBuildAWSMiniCompareResult(t)

	t.Run("json", func(t *testing.T) {
		var out bytes.Buffer
		if err := renderCompareOutput(&out, result, true, false); err != nil {
			t.Fatalf("render structured json: %v", err)
		}
		assertGoldenBytes(t, compareStructuredGoldenPath("aws-mini.golden.json"), out.Bytes())
	})

	t.Run("text", func(t *testing.T) {
		var out bytes.Buffer
		if err := renderCompareOutput(&out, result, false, false); err != nil {
			t.Fatalf("render structured text: %v", err)
		}
		assertGoldenBytes(t, compareStructuredGoldenPath("aws-mini.golden.txt"), out.Bytes())
	})
}

func mustBuildAWSMiniCompareResult(t testing.TB) compare.Result {
	t.Helper()

	oldSchema := mustReadAWSMiniSchema(t, "schema-v6.83.0.json")
	newSchema := mustReadAWSMiniSchema(t, "schema-v7.0.0.json")
	oldMetadata := mustReadAWSMiniMetadataEnvelope(t, "metadata-v6.83.0.json")
	newMetadata := mustReadAWSMiniMetadataEnvelope(t, "metadata-v7.0.0.json")

	return compare.Schemas(oldSchema, newSchema, compare.Options{
		Provider:    "aws",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})
}

func mustReadAWSMiniSchema(t testing.TB, name string) schema.PackageSpec {
	t.Helper()
	data := mustReadAWSMiniFixtureFile(t, name)
	var spec schema.PackageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse aws mini schema %q: %v", name, err)
	}
	return spec
}

func mustReadAWSMiniMetadataEnvelope(t testing.TB, name string) *normalize.MetadataEnvelope {
	t.Helper()
	data := mustReadAWSMiniFixtureFile(t, name)
	metadata, err := normalize.ParseMetadata(data)
	if err != nil {
		t.Fatalf("parse aws mini metadata %q: %v", name, err)
	}
	return metadata
}

func mustReadAWSMiniFixtureFile(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "compare-v2", "aws-mini", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func compareStructuredGoldenPath(name string) string {
	return filepath.Join("..", "..", "compare", "testdata", "structured", name)
}

func assertGoldenBytes(t testing.TB, path string, got []byte) {
	t.Helper()
	if os.Getenv(updateStructuredGoldenEnv) == "1" {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("update golden %s: %v", path, err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf("golden mismatch: %s\nset %s=1 to update\nwant (%d bytes) != got (%d bytes)\n%s",
		path, updateStructuredGoldenEnv, len(want), len(got), firstDiffPreview(want, got))
}

func firstDiffPreview(want, got []byte) string {
	max := len(want)
	if len(got) < max {
		max = len(got)
	}
	for i := 0; i < max; i++ {
		if want[i] != got[i] {
			start := i - 40
			if start < 0 {
				start = 0
			}
			endWant := i + 40
			if endWant > len(want) {
				endWant = len(want)
			}
			endGot := i + 40
			if endGot > len(got) {
				endGot = len(got)
			}
			return fmt.Sprintf("first diff at byte %d\nwant: %q\ngot:  %q", i, want[start:endWant], got[start:endGot])
		}
	}
	if len(want) != len(got) {
		return fmt.Sprintf("common prefix length %d; size differs", max)
	}
	return "contents differ"
}
