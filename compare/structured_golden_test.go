package compare

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStructuredAWSMiniTextGolden(t *testing.T) {
	var payload FullJSONOutput
	if err := json.Unmarshal(mustReadStructuredGolden(t, "aws-mini.golden.json"), &payload); err != nil {
		t.Fatalf("parse structured JSON golden: %v", err)
	}

	totalBreaking := 0
	for _, change := range payload.Changes {
		if change.Breaking {
			totalBreaking++
		}
	}

	result := Result{
		Summary:       payload.Summary,
		Changes:       payload.Changes,
		Grouped:       payload.Grouped,
		NewResources:  payload.NewResources,
		NewFunctions:  payload.NewFunctions,
		totalBreaking: totalBreaking,
	}

	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("render structured text: %v", err)
	}

	want := mustReadStructuredGolden(t, "aws-mini.golden.txt")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("structured text golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), string(want))
	}
}

func mustReadStructuredGolden(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "structured", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
