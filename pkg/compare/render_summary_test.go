package compare

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRenderSummaryIncludesCountsOnly(t *testing.T) {
	result := CompareResult{
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
	result := CompareResult{
		Summary: []SummaryItem{{Category: "missing-input", Count: 1}},
	}

	err := RenderSummary(failingWriter{}, result)
	if err == nil {
		t.Fatal("expected write error")
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("boom")
}
