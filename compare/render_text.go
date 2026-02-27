package compare

import (
	"fmt"
	"io"
	"sort"
	"strconv"
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
	displayed := breakingOnlyChanges(result.Changes)
	remaps := tokenRemapOnlyChanges(result.Changes)
	breakingCount := len(displayed)
	totalBreakingCount := result.totalBreakingCount(breakingCount)
	grouped := groupStructuredChanges(displayed)
	switch totalBreakingCount {
	case 0:
		if err := writeln("Looking good! No breaking changes found."); err != nil {
			return err
		}
	case 1:
		if err := writeln("Found 1 breaking change:"); err != nil {
			return err
		}
	default:
		if err := write("Found %d breaking changes:\n", totalBreakingCount); err != nil {
			return err
		}
	}
	if breakingCount > 0 {
		if err := writeGroupedText(write, grouped); err != nil {
			return err
		}
	}
	if totalBreakingCount > breakingCount {
		if err := write("Showing %d of %d breaking changes.\n", breakingCount, totalBreakingCount); err != nil {
			return err
		}
	}
	if len(remaps) > 0 {
		if err := write("\n#### Token remaps\n"); err != nil {
			return err
		}
		remapGrouped := groupStructuredChanges(remaps)
		if err := writeGroupedText(write, remapGrouped); err != nil {
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

func breakingChangeEntryCount(lines []string) int {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isBreakingDiagnosticLine(trimmed) {
			count++
		}
	}
	return count
}

func isBreakingDiagnosticLine(line string) bool {
	return strings.HasPrefix(line, "- `") || strings.HasPrefix(line, "#### `")
}
func (result Result) totalBreakingCount(displayed int) int {
	if result.totalBreaking > displayed {
		return result.totalBreaking
	}
	return displayed
}

func breakingOnlyChanges(changes []Change) []Change {
	if len(changes) == 0 {
		return []Change{}
	}
	out := make([]Change, 0, len(changes))
	for _, change := range changes {
		if change.Breaking {
			out = append(out, change)
		}
	}
	return sortStructuredChanges(out)
}

func tokenRemapOnlyChanges(changes []Change) []Change {
	if len(changes) == 0 {
		return []Change{}
	}
	out := make([]Change, 0, len(changes))
	for _, change := range changes {
		if change.Kind == "token-remapped" {
			out = append(out, change)
		}
	}
	return sortStructuredChanges(out)
}

func writeGroupedText(write func(string, ...any) error, grouped GroupedChanges) error {
	sections := []struct {
		title string
		data  map[string]map[string][]Change
	}{
		{title: "Resources", data: grouped.Resources},
		{title: "Functions", data: grouped.Functions},
		{title: "Types", data: grouped.Types},
	}
	for _, section := range sections {
		if len(section.data) == 0 {
			continue
		}
		if err := write("\n#### %s\n", section.title); err != nil {
			return err
		}
		tokens := sortedTokens(section.data)
		for _, token := range tokens {
			byLocation := section.data[token]
			locations := sortedLocations(byLocation)

			if len(locations) == 1 && locations[0] == "general" {
				for _, change := range sortStructuredChanges(byLocation["general"]) {
					if err := write("- %s %s\n", severityIcon(change.Severity), nodeEntry(strconv.Quote(token), textChangeMessage(change))); err != nil {
						return err
					}
				}
				continue
			}

			if err := write("- %q:\n", token); err != nil {
				return err
			}
			for _, location := range locations {
				if location == "general" {
					for _, change := range sortStructuredChanges(byLocation[location]) {
						if err := write("    - %s %s\n", severityIcon(change.Severity), textChangeMessage(change)); err != nil {
							return err
						}
					}
					continue
				}
				if err := write("    - %s:\n", location); err != nil {
					return err
				}
				for _, change := range sortStructuredChanges(byLocation[location]) {
					if err := write("        - %s %s\n", severityIcon(change.Severity), textChangeMessage(change)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func sortedTokens(grouped map[string]map[string][]Change) []string {
	tokens := make([]string, 0, len(grouped))
	for token := range grouped {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}

func sortedLocations(byLocation map[string][]Change) []string {
	locations := make([]string, 0, len(byLocation))
	for location := range byLocation {
		locations = append(locations, location)
	}
	sort.Strings(locations)
	return locations
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
