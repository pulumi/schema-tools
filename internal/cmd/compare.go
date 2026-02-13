package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/spf13/cobra"

	"github.com/pulumi/schema-tools/compare"
	"github.com/pulumi/schema-tools/internal/pkg"
)

type compareDeps struct {
	currentUser          func() (*user.User, error)
	downloadSchema       func(context.Context, string, string, string) (schema.PackageSpec, error)
	loadLocalPackageSpec func(string) (schema.PackageSpec, error)
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
		loadLocalPackageSpec: pkg.LoadLocalPackageSpec,
	}
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
	_ = command.MarkFlagRequired("provider")

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
	command.Flags().BoolVar(&summaryMode, "summary", false, "render summary-only output")

	return command
}

func runCompareCmd(input compareInput) error {
	return runCompareCmdWithDeps(input, defaultCompareDeps())
}

func runCompareCmdWithDeps(input compareInput, deps compareDeps) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loadLocal := func(path string) (schema.PackageSpec, error) {
		schemaPath, err := filepath.Abs(path)
		if err != nil {
			return schema.PackageSpec{}, fmt.Errorf("unable to construct absolute path to schema.json: %w", err)
		}
		return deps.loadLocalPackageSpec(schemaPath)
	}

	var schOld schema.PackageSpec
	schOldDone := make(chan error, 1)
	go func() {
		var err error
		switch {
		case input.oldPath != "":
			schOld, err = loadLocal(input.oldPath)
		case input.oldCommit != "":
			schOld, err = deps.downloadSchema(ctx, input.repository, input.provider, input.oldCommit)
		default:
			schOld, err = deps.downloadSchema(ctx, input.repository, input.provider, "master")
		}
		if err != nil {
			cancel()
		}
		schOldDone <- err
	}()

	var schNew schema.PackageSpec
	if input.newPath != "" {
		var err error
		schNew, err = loadLocal(input.newPath)
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(input.newCommit, "--local-path=") {
		fmt.Fprintln(os.Stderr, "Warning: --local-path= in --new-commit is deprecated, use --new-path instead")
		parts := strings.Split(input.newCommit, "=")
		if len(parts) < 2 || parts[1] == "" {
			return fmt.Errorf("invalid --local-path value: %q", input.newCommit)
		}
		var err error
		schNew, err = loadLocal(parts[1])
		if err != nil {
			return err
		}
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

	result := compare.Schemas(schOld, schNew, compare.Options{
		Provider:   input.provider,
		MaxChanges: input.maxChanges,
	})
	return renderCompareOutput(os.Stdout, result, input.jsonMode, input.summaryMode)
}

func renderCompareOutput(out io.Writer, result compare.Result, jsonMode bool, summaryMode bool) error {
	if jsonMode {
		return compare.RenderJSON(out, result, summaryMode)
	}
	if summaryMode {
		return compare.RenderSummary(out, result)
	}
	compare.RenderText(out, result)
	return nil
}
