package compare

import (
	"fmt"
	"io"
)

// RenderSummary writes summary category counts only.
func RenderSummary(out io.Writer, result CompareResult) error {
	if len(result.Summary) == 0 {
		if _, err := fmt.Fprintln(out, "No breaking changes found."); err != nil {
			return fmt.Errorf("write summary output: %w", err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(out, "Summary by category:"); err != nil {
		return fmt.Errorf("write summary output: %w", err)
	}
	for _, item := range result.Summary {
		if _, err := fmt.Fprintf(out, "- %s: %d\n", item.Category, item.Count); err != nil {
			return fmt.Errorf("write summary output: %w", err)
		}
	}
	return nil
}
