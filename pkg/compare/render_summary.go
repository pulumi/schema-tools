package compare

import (
	"fmt"
	"io"
)

// RenderSummary writes summary category counts only.
func RenderSummary(out io.Writer, result CompareResult) {
	if len(result.Summary) == 0 {
		fmt.Fprintln(out, "No breaking changes found.")
		return
	}

	fmt.Fprintln(out, "Summary by category:")
	for _, item := range result.Summary {
		fmt.Fprintf(out, "- %s: %d\n", item.Category, item.Count)
	}
}
