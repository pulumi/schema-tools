package compare

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// RenderText writes the human-readable compare report.
func RenderText(out io.Writer, report Report, maxChanges int) {
	fmt.Fprintf(out, "### Does the PR have any schema changes?\n\n")
	displayedViolations := new(bytes.Buffer)
	lenViolations := report.Violations.Display(displayedViolations, maxChanges)
	switch lenViolations {
	case 0:
		fmt.Fprintln(out, "Looking good! No breaking changes found.")
	case 1:
		fmt.Fprintln(out, "Found 1 breaking change: ")
	default:
		fmt.Fprintf(out, "Found %d breaking changes:\n", lenViolations)
	}

	_, err := out.Write(displayedViolations.Bytes())
	contract.AssertNoErrorf(err, "writing to a bytes.Buffer failing indicates OOM")

	if len(report.NewResources) > 0 {
		fmt.Fprintln(out, "\n#### New resources:")
		fmt.Fprintln(out, "")
		sort.Strings(report.NewResources)
		for _, v := range report.NewResources {
			fmt.Fprintf(out, "- `%s`\n", v)
		}
	}

	if len(report.NewFunctions) > 0 {
		fmt.Fprintln(out, "\n#### New functions:")
		fmt.Fprintln(out, "")
		sort.Strings(report.NewFunctions)
		for _, v := range report.NewFunctions {
			fmt.Fprintf(out, "- `%s`\n", v)
		}
	}

	if len(report.NewResources) == 0 && len(report.NewFunctions) == 0 {
		fmt.Fprintln(out, "No new resources/functions.")
	}
}
