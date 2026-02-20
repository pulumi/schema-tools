package compare

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
)

// RenderText writes human-readable compare output from Result only.
func RenderText(out io.Writer, result Result, maxChanges int) error {
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

	displayed := selectTextChanges(result.Changes, maxChanges)
	breakingCount := 0
	for _, change := range displayed {
		if change.Breaking {
			breakingCount++
		}
	}

	if err := write("### Does the PR have any schema changes?\n\n"); err != nil {
		return err
	}
	switch breakingCount {
	case 0:
		if err := writeln("Looking good! No breaking changes found."); err != nil {
			return err
		}
	case 1:
		if err := writeln("Found 1 breaking change:"); err != nil {
			return err
		}
	default:
		if err := write("Found %d breaking changes:\n", breakingCount); err != nil {
			return err
		}
	}
	if err := writeGroupedSections(write, writeln, displayed); err != nil {
		return err
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

func selectTextChanges(changes []Change, maxChanges int) []Change {
	if len(changes) == 0 {
		return []Change{}
	}
	if maxChanges < 0 || len(changes) <= maxChanges {
		return changes
	}
	if maxChanges == 0 {
		return []Change{}
	}
	return changes[:maxChanges]
}

func writeGroupedSections(
	write func(string, ...any) error,
	writeln func(...any) error,
	changes []Change,
) error {
	grouped := groupChanges(changes)
	if err := writeGroupedSection(write, writeln, "Resources", grouped.Resources); err != nil {
		return err
	}
	if err := writeGroupedSection(write, writeln, "Functions", grouped.Functions); err != nil {
		return err
	}
	if err := writeGroupedSection(write, writeln, "Types", grouped.Types); err != nil {
		return err
	}
	return nil
}

func writeGroupedSection(
	write func(string, ...any) error,
	writeln func(...any) error,
	section string,
	group map[string]map[string][]Change,
) error {
	if len(group) == 0 {
		return nil
	}
	if err := writeln("\n#### " + section); err != nil {
		return err
	}

	for _, token := range sortedMapKeys(group) {
		if err := write("- %q:\n", token); err != nil {
			return err
		}
		locations := sortedTextLocations(group[token])
		for _, location := range locations {
			if err := write("    - %s:\n", location); err != nil {
				return err
			}
			for _, change := range sortChanges(group[token][location]) {
				if err := write("        - %s %s\n", severityIcon(change.Severity), textChangeMessage(change)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func sortedTextLocations(byLocation map[string][]Change) []string {
	locations := make([]string, 0, len(byLocation))
	for location := range byLocation {
		locations = append(locations, location)
	}
	slices.SortFunc(locations, func(a, b string) int {
		if d := cmp.Compare(locationSortOrder(a), locationSortOrder(b)); d != 0 {
			return d
		}
		return cmp.Compare(a, b)
	})
	return locations
}

func locationSortOrder(location string) int {
	switch location {
	case "inputs":
		return 0
	case "properties":
		return 1
	case "outputs":
		return 2
	case "signature":
		return 3
	case "required inputs":
		return 4
	case "required":
		return 5
	case "general":
		return 99
	default:
		return 50
	}
}

func textChangeMessage(change Change) string {
	message := strings.TrimSpace(change.Message)
	if message == "" {
		return strings.TrimSpace(change.Path)
	}
	if change.Location == "" || change.Location == "general" {
		return message
	}

	label := trailingQuotedValue(change.Path)
	if label == "" || label == change.Token || strings.Contains(message, `"`+label+`"`) {
		return message
	}
	return fmt.Sprintf("%q %s", label, message)
}

func trailingQuotedValue(path string) string {
	if path == "" {
		return ""
	}
	lastQuote := strings.LastIndex(path, `"`)
	if lastQuote <= 0 {
		return ""
	}
	start := strings.LastIndex(path[:lastQuote], `"`)
	if start == -1 || start+1 >= lastQuote {
		return ""
	}
	return path[start+1 : lastQuote]
}

func severityIcon(severity ChangeSeverity) string {
	switch severity {
	case SeverityError:
		return "`🔴`"
	case SeverityWarn:
		return "`🟡`"
	case SeverityInfo:
		return "`🟢`"
	default:
		return "`🟡`"
	}
}
