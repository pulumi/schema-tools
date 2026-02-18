package compare

import (
	"fmt"
	"io"
	"strings"
)

// RenderText writes human-readable compare output from Result only.
func RenderText(out io.Writer, result Result) error {
	write := func(format string, args ...any) error {
		if _, err := fmt.Fprintf(out, format, args...); err != nil {
			return fmt.Errorf("write compare text output: %w", err)
		}
		return nil
	}
	writeln := func(args ...any) error {
		if _, err := fmt.Fprintln(out, args...); err != nil {
			return fmt.Errorf("write compare text output: %w", err)
		}
		return nil
	}

	if err := write("### Does the PR have any schema changes?\n\n"); err != nil {
		return err
	}
	switch len(result.BreakingChanges) {
	case 0:
		if err := writeln("Looking good! No breaking changes found."); err != nil {
			return err
		}
	case 1:
		if err := writeln("Found 1 breaking change:"); err != nil {
			return err
		}
	default:
		if err := write("Found %d breaking changes:\n", len(result.BreakingChanges)); err != nil {
			return err
		}
	}
	if len(result.BreakingChanges) > 0 {
		if err := writeln(strings.Join(result.BreakingChanges, "\n")); err != nil {
			return err
		}
	}

	newResources := result.NewResources
	newFunctions := result.NewFunctions

	if len(newResources) > 0 {
		if err := writeln("\n#### New resources:"); err != nil {
			return err
		}
		if err := writeln(""); err != nil {
			return err
		}
		for _, v := range newResources {
			if err := write("- `%s`\n", v); err != nil {
				return err
			}
		}
	}

	if len(newFunctions) > 0 {
		if err := writeln("\n#### New functions:"); err != nil {
			return err
		}
		if err := writeln(""); err != nil {
			return err
		}
		for _, v := range newFunctions {
			if err := write("- `%s`\n", v); err != nil {
				return err
			}
		}
	}

	if len(newResources) == 0 && len(newFunctions) == 0 {
		if err := writeln("No new resources/functions."); err != nil {
			return err
		}
	}
	return nil
}
