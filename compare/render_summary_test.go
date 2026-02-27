package compare

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestRenderSummaryIncludesCountsOnly(t *testing.T) {
	result := Result{
		Summary: []SummaryItem{{
			Category: "missing-input",
			Count:    2,
			Entries:  []string{"e1", "e2"},
		}},
	}

	var out bytes.Buffer
	if err := RenderSummary(&out, result); err != nil {
		t.Fatalf("RenderSummary returned error: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Summary by category:") {
		t.Fatalf("missing summary header: %s", text)
	}
	if !strings.Contains(text, "- missing-input: 2") {
		t.Fatalf("missing category/count line: %s", text)
	}
	if strings.Contains(text, "e1") || strings.Contains(text, "e2") {
		t.Fatalf("did not expect entries in summary text output: %s", text)
	}
}

func TestRenderSummaryWriteError(t *testing.T) {
	result := Result{
		Summary: []SummaryItem{{Category: "missing-input", Count: 1}},
	}

	err := RenderSummary(failingWriter{}, result)
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestRenderSummaryEmpty(t *testing.T) {
	var out bytes.Buffer
	err := RenderSummary(&out, Result{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "No breaking changes found.\n" {
		t.Fatalf("unexpected empty summary output: %q", out.String())
	}
}

func TestRenderSummaryResolvedTokenRemapIncludesTokenCategory(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v2:Widget": {},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget"}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v2:Widget","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}]}}}}`)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1, OldMetadata: oldMetadata, NewMetadata: newMetadata})

	var out bytes.Buffer
	if err := RenderSummary(&out, result); err != nil {
		t.Fatalf("RenderSummary returned error: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Summary by category:") {
		t.Fatalf("expected summary header, got %q", text)
	}
	if !strings.Contains(text, "- token-remapped: 1") {
		t.Fatalf("expected token-remapped summary count, got %q", text)
	}
}
