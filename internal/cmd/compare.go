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

// compareDeps holds injectable command dependencies for deterministic tests.
type compareDeps struct {
	currentUser          func() (*user.User, error)
	downloadSchema       func(context.Context, string, string, string) (schema.PackageSpec, error)
	downloadRepoFile     func(context.Context, string, string, string, string) ([]byte, error)
	loadLocalPackageSpec func(string) (schema.PackageSpec, error)
	parseMetadata        func([]byte) (*normalize.MetadataEnvelope, error)
	normalizeSchemas     func(schema.PackageSpec, schema.PackageSpec, *normalize.MetadataEnvelope, *normalize.MetadataEnvelope) (normalize.Result, error)
}

// compareInput captures normalized compare flag values.
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

// defaultCompareDeps returns production implementations for compare command
// dependencies. Tests can replace any field through compareDeps.
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

// ErrCompareMetadataRequired identifies strict mode failures when required
// bridge metadata cannot be loaded for one compare side.
var ErrCompareMetadataRequired = errors.New("compare metadata required")

// compareCmd constructs the "compare" CLI command and binds all flags.
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

// runCompareCmd executes compare with default production dependencies.
func runCompareCmd(input compareInput) error {
	return runCompareCmdWithDeps(input, defaultCompareDeps())
}

// runCompareCmdWithDeps executes compare using injectable dependencies. It
// handles schema loading, optional metadata normalization, then output rendering.
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

	result := compare.Schemas(normalizedOld, normalizedNew, compare.Options{Provider: input.provider})
	result = compare.MergeChanges(result, buildNormalizationChanges(normalizedRenames, normalizedMaxItemsOne))
	return renderCompareOutput(os.Stdout, result, input.jsonMode, input.summaryMode, input.maxChanges)
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
			return nil, compareMetadataRequired(side, repository, commit, metadataRepoPath, err)
		}
		return nil, fmt.Errorf("download %s metadata: %w", side, err)
	}

	metadata, err := deps.parseMetadata(payload)
	if err != nil {
		if errors.Is(err, normalize.ErrMetadataRequired) {
			return nil, compareMetadataRequired(side, repository, commit, metadataRepoPath, err)
		}
		return nil, fmt.Errorf("parse %s metadata: %w", side, err)
	}
	return metadata, nil
}

func compareMetadataRequired(side, repository, commit, metadataRepoPath string, err error) error {
	return fmt.Errorf(
		"compare %s metadata required: %s@%s:%s: %w",
		side, repository, commit, metadataRepoPath, errors.Join(ErrCompareMetadataRequired, err),
	)
}

// isFileRepositoryURL reports whether --repository uses the file: scheme.
func isFileRepositoryURL(repository string) bool {
	repoURL, err := url.Parse(repository)
	return err == nil && repoURL.Scheme == "file"
}

// renderCompareOutput writes compare results in text/json and full/summary modes.
func renderCompareOutput(out io.Writer, result compare.Result, jsonMode bool, summaryMode bool, maxChanges int) error {
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
	return compare.RenderText(out, result, maxChanges)
}

// buildNormalizationChanges converts normalization evidence into compare.Change
// records and returns them in deterministic deduplicated order.
func buildNormalizationChanges(
	renames []normalize.TokenRename,
	maxItemsOne []normalize.MaxItemsOneChange,
) []compare.Change {
	changes := []compare.Change{}
	changes = append(changes, buildNormalizationRenameChanges(renames)...)
	changes = append(changes, buildNormalizationMaxItemsOneChanges(maxItemsOne)...)
	if len(changes) == 0 {
		return changes
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Scope != changes[j].Scope {
			return changes[i].Scope < changes[j].Scope
		}
		if changes[i].Token != changes[j].Token {
			return changes[i].Token < changes[j].Token
		}
		if changes[i].Location != changes[j].Location {
			return changes[i].Location < changes[j].Location
		}
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		return changes[i].Message < changes[j].Message
	})
	out := make([]compare.Change, 0, len(changes))
	seen := map[string]struct{}{}
	for _, change := range changes {
		key := string(change.Scope) + "|" + change.Token + "|" + change.Location + "|" + change.Path + "|" + change.Kind + "|" + change.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, change)
	}
	return out
}

// buildNormalizationRenameChanges turns token-rename evidence into user-facing
// compare changes, including in-codegen alias migration annotations.
func buildNormalizationRenameChanges(renames []normalize.TokenRename) []compare.Change {
	changes := []compare.Change{}
	for _, rename := range renames {
		scope, scopeLabel := normalizationScope(rename.Scope)
		if scopeLabel == "" || strings.TrimSpace(rename.OldToken) == "" || strings.TrimSpace(rename.NewToken) == "" {
			continue
		}

		kind := "renamed-resource"
		if scope == compare.ScopeFunction {
			kind = "renamed-function"
		}
		severity := compare.SeverityError
		breaking := true
		message := fmt.Sprintf(`renamed to %q`, rename.NewToken)
		if rename.Kind == normalize.TokenRenameKindInCodegenAlias {
			if scope == compare.ScopeResource {
				kind = "deprecated-resource-alias"
			} else {
				kind = "deprecated-function-alias"
			}
			severity = compare.SeverityInfo
			breaking = false
			message = fmt.Sprintf(`retained as deprecated alias; migrate to %q`, rename.NewToken)
		}

		changes = append(changes, compare.Change{
			Scope:    scope,
			Token:    rename.OldToken,
			Path:     fmt.Sprintf(`%s: %q`, scopeLabel, rename.OldToken),
			Kind:     kind,
			Severity: severity,
			Breaking: breaking,
			Source:   compare.SourceNormalize,
			Message:  message,
		})
	}
	return changes
}

// buildNormalizationMaxItemsOneChanges converts maxItemsOne normalization
// rewrites into explicit compare changes for report visibility.
func buildNormalizationMaxItemsOneChanges(changes []normalize.MaxItemsOneChange) []compare.Change {
	out := []compare.Change{}
	for _, change := range changes {
		scope, scopeLabel := normalizationScope(change.Scope)
		if scopeLabel == "" || strings.TrimSpace(change.Token) == "" || strings.TrimSpace(change.Field) == "" {
			continue
		}

		location := change.Location
		if strings.TrimSpace(location) == "" {
			location = "properties"
		}

		oldField := strings.TrimSpace(change.Field)
		newField := strings.TrimSpace(change.NewField)
		message := fmt.Sprintf(`%q type changed from %q to %q`, oldField, change.OldType, change.NewType)
		if newField != "" && newField != oldField {
			message = fmt.Sprintf(`%q renamed to %q and type changed from %q to %q`,
				oldField, newField, change.OldType, change.NewType)
		}

		out = append(out, compare.Change{
			Scope:    scope,
			Token:    change.Token,
			Location: location,
			Path:     fmt.Sprintf(`%s: %q: %s: %q`, scopeLabel, change.Token, location, oldField),
			Kind:     "max-items-one-changed",
			Severity: compare.SeverityError,
			Breaking: true,
			Source:   compare.SourceNormalize,
			Message:  message,
		})
	}
	return out
}

// normalizationScope maps normalization metadata scope labels into compare scope
// enums and display labels used in formatted paths.
func normalizationScope(scope string) (compare.ChangeScope, string) {
	switch scope {
	case "resources":
		return compare.ScopeResource, "Resources"
	case "datasources":
		return compare.ScopeFunction, "Functions"
	default:
		return compare.ScopeUnknown, ""
	}
}
