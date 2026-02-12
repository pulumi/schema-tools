package compare

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

// RenderText writes human-readable compare output from Result only.
func RenderText(out io.Writer, result Result) {
	fmt.Fprintf(out, "### Does the PR have any schema changes?\n\n")
	switch len(result.BreakingChanges) {
	case 0:
		fmt.Fprintln(out, "Looking good! No breaking changes found.")
	case 1:
		fmt.Fprintln(out, "Found 1 breaking change: ")
	default:
		fmt.Fprintf(out, "Found %d breaking changes:\n", len(result.BreakingChanges))
	}
	if len(result.BreakingChanges) > 0 {
		fmt.Fprintln(out, strings.Join(result.BreakingChanges, "\n"))
	}

	newResources := ensureSlice(slices.Clone(result.NewResources))
	newFunctions := ensureSlice(slices.Clone(result.NewFunctions))
	sort.Strings(newResources)
	sort.Strings(newFunctions)

	if len(newResources) > 0 {
		fmt.Fprintln(out, "\n#### New resources:")
		fmt.Fprintln(out, "")
		for _, v := range newResources {
			fmt.Fprintf(out, "- `%s`\n", v)
		}
	}

	if len(newFunctions) > 0 {
		fmt.Fprintln(out, "\n#### New functions:")
		fmt.Fprintln(out, "")
		for _, v := range newFunctions {
			fmt.Fprintf(out, "- `%s`\n", v)
		}
	}

	if len(newResources) == 0 && len(newFunctions) == 0 {
		fmt.Fprintln(out, "No new resources/functions.")
	}
}
