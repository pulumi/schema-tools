package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/compare"
	"github.com/pulumi/schema-tools/internal/normalize"
	"github.com/pulumi/schema-tools/internal/pkg"
)

type compareDeps struct {
	currentUser          func() (*user.User, error)
	downloadSchema       func(context.Context, string, string, string) (schema.PackageSpec, error)
	downloadRepoFile     func(context.Context, string, string, string, string) ([]byte, error)
	loadLocalPackageSpec func(string) (schema.PackageSpec, error)
	parseMetadata        func([]byte) (*normalize.MetadataEnvelope, error)
	normalizeSchemas     func(schema.PackageSpec, schema.PackageSpec, *normalize.MetadataEnvelope, *normalize.MetadataEnvelope) (normalize.Result, error)
}

type compareInput struct {
	provider    string
	repository  string
	oldCommit   string
	newCommit   string
	oldPath     string
	newPath     string
	maxChanges  int
	jsonMode    bool
	summaryMode bool
}

func defaultCompareDeps() compareDeps {
	return compareDeps{
		currentUser:          user.Current,
		downloadSchema:       pkg.DownloadSchema,
		downloadRepoFile:     pkg.DownloadRepoFile,
		loadLocalPackageSpec: pkg.LoadLocalPackageSpec,
		parseMetadata:        normalize.ParseMetadata,
		normalizeSchemas:     normalize.Normalize,
	}
}

var ErrCompareMetadataRequired = errors.New("compare metadata required")

type compareMetadataRequiredError struct {
	Side   string
	Source string
	Path   string
	Commit string
	Err    error
}

func (e *compareMetadataRequiredError) Error() string {
	return fmt.Sprintf("compare %s metadata required: %s@%s:%s", e.Side, e.Source, e.Commit, e.Path)
}

func (e *compareMetadataRequiredError) Unwrap() error {
	return e.Err
}

func (e *compareMetadataRequiredError) Is(target error) bool {
	return target == ErrCompareMetadataRequired
}

func compareCmd() *cobra.Command {
	var provider, repository, oldCommit, newCommit string
	var oldPath, newPath string
	var maxChanges int
	var jsonMode, summaryMode bool

	command := &cobra.Command{
		Use:   "compare",
		Short: "Compare two versions of a Pulumi schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			if newCommit == "" && newPath == "" {
				return fmt.Errorf("either --new-commit or --new-path must be set")
			}
			if newCommit != "" && newPath != "" {
				return fmt.Errorf("--new-commit and --new-path are mutually exclusive")
			}
			if oldCommit != "" && oldPath != "" {
				return fmt.Errorf("--old-commit and --old-path are mutually exclusive")
			}
			return runCompareCmd(compareInput{
				provider:    provider,
				repository:  repository,
				oldCommit:   oldCommit,
				newCommit:   newCommit,
				oldPath:     oldPath,
				newPath:     newPath,
				maxChanges:  maxChanges,
				jsonMode:    jsonMode,
				summaryMode: summaryMode,
			})
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "", "the provider whose schema we are comparing")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&repository, "repository", "r",
		"github://api.github.com/pulumi", "the Git repository to download the schema file from")

	command.Flags().StringVarP(&oldCommit, "old-commit", "o", "",
		"the old commit to compare with (defaults to master when no --old-path is set)")
	command.Flags().StringVar(&oldPath, "old-path", "",
		"path to a local schema file to use as the old version")

	command.Flags().StringVarP(&newCommit, "new-commit", "n", "",
		"the new commit to compare against the old commit")
	command.Flags().StringVar(&newPath, "new-path", "",
		"path to a local schema file to use as the new version")

	command.Flags().IntVarP(&maxChanges, "max-changes", "m", 500,
		"the maximum number of breaking changes to display. Pass -1 to display all changes")
	command.Flags().BoolVar(&jsonMode, "json", false, "render compare output as JSON")
	command.Flags().BoolVar(&summaryMode, "summary", false,
		"render summary-only output (text counts; with --json, includes entry details)")

	return command
}

func runCompareCmd(input compareInput) error {
	return runCompareCmdWithDeps(input, defaultCompareDeps())
}

func runCompareCmdWithDeps(input compareInput, deps compareDeps) error {
	deps = applyCompareDepsDefaults(deps)
	if strings.TrimSpace(input.provider) == "" {
		return fmt.Errorf("--provider must be set")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loadLocal := func(path string) (schema.PackageSpec, error) {
		schemaPath, err := filepath.Abs(path)
		if err != nil {
			return schema.PackageSpec{}, fmt.Errorf("unable to construct absolute path to schema.json: %w", err)
		}
		return deps.loadLocalPackageSpec(schemaPath)
	}

	oldCommit := input.oldCommit
	if oldCommit == "" && input.oldPath == "" {
		oldCommit = "master"
	}

	var schOld schema.PackageSpec
	schOldDone := make(chan error, 1)
	go func() {
		var err error
		switch {
		case input.oldPath != "":
			schOld, err = loadLocal(input.oldPath)
		default:
			schOld, err = deps.downloadSchema(ctx, input.repository, input.provider, oldCommit)
		}
		if err != nil {
			cancel()
		}
		schOldDone <- err
	}()

	var schNew schema.PackageSpec
	newCommit := input.newCommit
	newIsLocal := false
	if input.newPath != "" {
		var err error
		schNew, err = loadLocal(input.newPath)
		if err != nil {
			return err
		}
		newIsLocal = true
	} else if strings.HasPrefix(input.newCommit, "--local-path=") {
		fmt.Fprintln(os.Stderr, "Warning: --local-path= in --new-commit is deprecated, use --new-path instead")
		_, localPath, ok := strings.Cut(input.newCommit, "=")
		if !ok || localPath == "" {
			return fmt.Errorf("invalid --local-path value: %q", input.newCommit)
		}
		var err error
		schNew, err = loadLocal(localPath)
		if err != nil {
			return err
		}
		newCommit = ""
		newIsLocal = true
	} else if input.newCommit == "--local" {
		fmt.Fprintln(os.Stderr, "Warning: --local in --new-commit is deprecated, use --new-path instead")
		usr, err := deps.currentUser()
		if err != nil {
			return fmt.Errorf("get current user: %w", err)
		}
		basePath := fmt.Sprintf("%s/go/src/github.com/pulumi/%s", usr.HomeDir, input.provider)
		schemaFile := pkg.StandardSchemaPath(input.provider)
		schemaPath := filepath.Join(basePath, schemaFile)
		schNew, err = deps.loadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
		newCommit = ""
		newIsLocal = true
	} else {
		var err error
		schNew, err = deps.downloadSchema(ctx, input.repository, input.provider, input.newCommit)
		if err != nil {
			return err
		}
	}

	if err := <-schOldDone; err != nil {
		return err
	}

	var oldMetadata, newMetadata *normalize.MetadataEnvelope
	// Normalize only when both sides are remote commit-based inputs.
	// Examples:
	// - compare -o v1.0.0 -n v1.1.0   => true (remote normalization applies)
	// - compare --old-path a.json ... => false (legacy local/hybrid flow)
	remoteCompare := input.oldPath == "" && !newIsLocal && !isFileRepositoryURL(input.repository)
	if remoteCompare {
		var err error
		oldMetadata, err = resolveCompareMetadataSource(
			ctx, deps, "old", input.provider, input.repository, oldCommit,
		)
		if err != nil {
			return err
		}
		newMetadata, err = resolveCompareMetadataSource(
			ctx, deps, "new", input.provider, input.repository, newCommit,
		)
		if err != nil {
			return err
		}
	}

	normalizedOld, normalizedNew := schOld, schNew
	normalizedRenames := []normalize.TokenRename{}
	normalizedMaxItemsOne := []normalize.MaxItemsOneChange{}
	if remoteCompare {
		normalized, err := deps.normalizeSchemas(schOld, schNew, oldMetadata, newMetadata)
		if err != nil {
			return fmt.Errorf("normalize schemas: %w", err)
		}
		normalizedOld, normalizedNew = normalized.OldSchema, normalized.NewSchema
		normalizedRenames = normalized.Renames
		normalizedMaxItemsOne = normalized.MaxItemsOne
	}

	result := compare.Schemas(normalizedOld, normalizedNew, compare.Options{
		Provider:   input.provider,
		MaxChanges: compareEngineMaxChanges(remoteCompare, input.maxChanges),
	})
	result = addNormalizationRenames(result, normalizedRenames)
	result = addNormalizationMaxItemsOne(result, normalizedMaxItemsOne)
	// Intentionally cap after normalization injections so synthetic breaking changes
	// (rename/maxItemsOne) participate in the same final max-changes policy.
	// maxChanges == 0 is valid and yields zero rendered breaking changes.
	result = applyMaxChangesLimit(result, input.maxChanges)
	return renderCompareOutput(os.Stdout, result, input.jsonMode, input.summaryMode)
}

// applyCompareDepsDefaults fills unset injected dependencies for test seams.
func applyCompareDepsDefaults(deps compareDeps) compareDeps {
	defaults := defaultCompareDeps()
	if deps.currentUser == nil {
		deps.currentUser = defaults.currentUser
	}
	if deps.downloadSchema == nil {
		deps.downloadSchema = defaults.downloadSchema
	}
	if deps.downloadRepoFile == nil {
		deps.downloadRepoFile = defaults.downloadRepoFile
	}
	if deps.loadLocalPackageSpec == nil {
		deps.loadLocalPackageSpec = defaults.loadLocalPackageSpec
	}
	if deps.parseMetadata == nil {
		deps.parseMetadata = defaults.parseMetadata
	}
	if deps.normalizeSchemas == nil {
		deps.normalizeSchemas = defaults.normalizeSchemas
	}
	return deps
}

// resolveCompareMetadataSource loads and parses bridge metadata for one compare side.
// Missing metadata is converted into ErrCompareMetadataRequired for strict remote mode.
func resolveCompareMetadataSource(
	ctx context.Context,
	deps compareDeps,
	side, provider, repository, commit string,
) (*normalize.MetadataEnvelope, error) {
	metadataRepoPath := pkg.StandardMetadataPath(provider)

	payload, err := deps.downloadRepoFile(ctx, repository, provider, commit, metadataRepoPath)
	if err != nil {
		if errors.Is(err, pkg.ErrRepoFileNotFound) {
			return nil, &compareMetadataRequiredError{
				Side:   side,
				Source: repository,
				Path:   metadataRepoPath,
				Commit: commit,
				Err:    err,
			}
		}
		return nil, fmt.Errorf("download %s metadata: %w", side, err)
	}

	metadata, err := deps.parseMetadata(payload)
	if err != nil {
		if errors.Is(err, normalize.ErrMetadataRequired) {
			return nil, &compareMetadataRequiredError{
				Side:   side,
				Source: repository,
				Path:   metadataRepoPath,
				Commit: commit,
				Err:    err,
			}
		}
		return nil, fmt.Errorf("parse %s metadata: %w", side, err)
	}
	return metadata, nil
}

// applyMaxChangesLimit enforces the final rendered cap.
// Example: maxChanges == 0 returns zero breaking change lines.
func applyMaxChangesLimit(result compare.Result, maxChanges int) compare.Result {
	if maxChanges < 0 || len(result.BreakingChanges) <= maxChanges {
		return result
	}
	result.BreakingChanges = result.BreakingChanges[:maxChanges]
	return result
}

// compareEngineMaxChanges controls pre-capping in compare.Schemas.
// Remote mode returns -1 so normalization-injected lines are included before final capping.
func compareEngineMaxChanges(remoteCompare bool, maxChanges int) int {
	// Remote compares can inject normalization-derived breaking changes after
	// compare.Schemas, so avoid pre-capping in the engine and apply one final cap.
	if remoteCompare {
		return -1
	}
	return maxChanges
}

// isFileRepositoryURL reports whether --repository uses the file: scheme.
func isFileRepositoryURL(repository string) bool {
	repoURL, err := url.Parse(repository)
	return err == nil && repoURL.Scheme == "file"
}

// renderCompareOutput writes compare results in text/json and full/summary modes.
func renderCompareOutput(out io.Writer, result compare.Result, jsonMode bool, summaryMode bool) error {
	if jsonMode {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")

		var payload any
		if summaryMode {
			payload = compare.NewSummaryJSONOutput(result)
		} else {
			payload = compare.NewFullJSONOutput(result)
		}
		if err := encoder.Encode(payload); err != nil {
			return fmt.Errorf("write compare JSON: %w", err)
		}
		return nil
	}
	if summaryMode {
		return compare.RenderSummary(out, result)
	}
	return compare.RenderText(out, result)
}

// addNormalizationRenames injects normalization-derived token rename changes into
// both breaking-change lines and summary categories.
func addNormalizationRenames(result compare.Result, renames []normalize.TokenRename) compare.Result {
	if len(renames) == 0 {
		return result
	}

	entriesByCategory := map[string][]string{}
	breakingLines := []string{}
	seen := map[string]struct{}{}
	for _, rename := range renames {
		category, scopeLabel := normalizationRenameCategory(rename.Scope)
		if category == "" || strings.TrimSpace(rename.OldToken) == "" || strings.TrimSpace(rename.NewToken) == "" {
			continue
		}

		entry := fmt.Sprintf(`%s: %q renamed to %q`, scopeLabel, rename.OldToken, rename.NewToken)
		key := category + "|" + entry
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		entriesByCategory[category] = append(entriesByCategory[category], entry)
		breakingLines = append(breakingLines, fmt.Sprintf("`🔴` %s", entry))
	}

	if len(breakingLines) == 0 {
		return result
	}
	result = prependNormalizationBreakingLines(result, breakingLines)
	result = mergeNormalizationSummaryEntries(result, entriesByCategory)
	return result
}

// addNormalizationMaxItemsOne injects normalization-derived maxItemsOne transition
// changes into both breaking-change lines and summary categories.
func addNormalizationMaxItemsOne(result compare.Result, changes []normalize.MaxItemsOneChange) compare.Result {
	if len(changes) == 0 {
		return result
	}

	const category = "max-items-one-changed"
	entries := []string{}
	breakingLines := []string{}
	seen := map[string]struct{}{}
	for _, change := range changes {
		scopeLabel := normalizationScopeLabel(change.Scope)
		if scopeLabel == "" || strings.TrimSpace(change.Token) == "" || strings.TrimSpace(change.Field) == "" {
			continue
		}

		location := change.Location
		if strings.TrimSpace(location) == "" {
			location = "properties"
		}
		entry := fmt.Sprintf(`%s: %q: %s: %q maxItemsOne changed from %q to %q`,
			scopeLabel, change.Token, location, change.Field, change.OldType, change.NewType)
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
		breakingLines = append(breakingLines, fmt.Sprintf("`🔴` %s", entry))
	}
	if len(entries) == 0 {
		return result
	}

	result = prependNormalizationBreakingLines(result, breakingLines)
	result = mergeNormalizationSummaryEntries(result, map[string][]string{
		category: entries,
	})
	return result
}

// prependNormalizationBreakingLines inserts synthetic normalization lines ahead of
// engine-produced breaking changes, with deterministic ordering.
func prependNormalizationBreakingLines(result compare.Result, lines []string) compare.Result {
	if len(lines) == 0 {
		return result
	}
	sort.Strings(lines)
	result.BreakingChanges = append(lines, result.BreakingChanges...)
	return result
}

// mergeNormalizationSummaryEntries merges synthetic normalization categories into
// an existing compare summary while preserving stable ordering.
func mergeNormalizationSummaryEntries(result compare.Result, entriesByCategory map[string][]string) compare.Result {
	if len(entriesByCategory) == 0 {
		return result
	}
	summaryByCategory := map[string]int{}
	for i := range result.Summary {
		summaryByCategory[result.Summary[i].Category] = i
	}
	for category := range entriesByCategory {
		sort.Strings(entriesByCategory[category])
	}
	for _, category := range sortedSummaryCategories(entriesByCategory) {
		entries := entriesByCategory[category]
		if idx, ok := summaryByCategory[category]; ok {
			result.Summary[idx].Entries = append(result.Summary[idx].Entries, entries...)
			sort.Strings(result.Summary[idx].Entries)
			result.Summary[idx].Count += len(entries)
			continue
		}
		result.Summary = append(result.Summary, compare.SummaryItem{
			Category: category,
			Count:    len(entries),
			Entries:  entries,
		})
	}

	sort.Slice(result.Summary, func(i, j int) bool {
		return result.Summary[i].Category < result.Summary[j].Category
	})
	return result
}

// normalizationScopeLabel maps normalization scopes to compare output labels.
func normalizationScopeLabel(scope string) string {
	switch scope {
	case "resources":
		return "Resources"
	case "datasources":
		return "Functions"
	default:
		return ""
	}
}

// normalizationRenameCategory maps normalization scopes to rename summary categories.
func normalizationRenameCategory(scope string) (string, string) {
	switch scope {
	case "resources":
		return "renamed-resource", "Resources"
	case "datasources":
		return "renamed-function", "Functions"
	default:
		return "", ""
	}
}

// sortedSummaryCategories returns category keys in lexical order.
func sortedSummaryCategories(entriesByCategory map[string][]string) []string {
	if len(entriesByCategory) == 0 {
		return nil
	}
	categories := make([]string, 0, len(entriesByCategory))
	for category := range entriesByCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}
