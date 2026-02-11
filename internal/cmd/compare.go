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

	internalcompare "github.com/pulumi/schema-tools/internal/compare"
	"github.com/pulumi/schema-tools/internal/pkg"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
	comparepkg "github.com/pulumi/schema-tools/pkg/compare"
)

var (
	currentUser          = user.Current
	downloadSchema       = pkg.DownloadSchema
	loadLocalPackageSpec = pkg.LoadLocalPackageSpec
)

func compareCmd() *cobra.Command {
	var provider, repository, oldCommit, newCommit string
	var maxChanges int
	var jsonMode, summaryMode bool

	command := &cobra.Command{
		Use:   "compare",
		Short: "Compare two versions of a Pulumi schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			return compare(provider, repository, oldCommit, newCommit, maxChanges, jsonMode, summaryMode)
		},
	}

	command.Flags().StringVarP(&provider, "provider", "p", "", "the provider whose schema we are comparing")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&repository, "repository", "r",
		"github://api.github.com/pulumi", "the Git repository to download the schema file from")
	_ = command.MarkFlagRequired("provider")

	command.Flags().StringVarP(&oldCommit, "old-commit", "o", "master",
		"the old commit to compare with (defaults to master)")

	command.Flags().StringVarP(&newCommit, "new-commit", "n", "",
		"the new commit to compare against the old commit")
	_ = command.MarkFlagRequired("new-commit")

	command.Flags().IntVarP(&maxChanges, "max-changes", "m", 500,
		"the maximum number of breaking changes to display. Pass -1 to display all changes")
	command.Flags().BoolVar(&jsonMode, "json", false, "render compare output as JSON")
	command.Flags().BoolVar(&summaryMode, "summary", false, "render summary-only output")

	return command
}

func compare(
	provider string,
	repository string,
	oldCommit string,
	newCommit string,
	maxChanges int,
	jsonMode bool,
	summaryMode bool,
) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var schOld schema.PackageSpec
	schOldDone := make(chan error, 1)
	go func() {
		var err error
		schOld, err = downloadSchema(ctx, repository, provider, oldCommit)
		if err != nil {
			cancel()
		}
		schOldDone <- err
	}()

	var schNew schema.PackageSpec
	if newCommit == "--local" {
		usr, err := currentUser()
		if err != nil {
			return fmt.Errorf("get current user: %w", err)
		}
		basePath := fmt.Sprintf("%s/go/src/github.com/pulumi/%s", usr.HomeDir, provider)
		schemaFile := pkg.StandardSchemaPath(provider)
		schemaPath := filepath.Join(basePath, schemaFile)
		schNew, err = loadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(newCommit, "--local-path=") {
		parts := strings.Split(newCommit, "=")
		schemaPath, err := filepath.Abs(parts[1])
		if err != nil {
			return fmt.Errorf("unable to construct absolute path to schema.json: %w", err)
		}
		schNew, err = loadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else {
		var err error
		schNew, err = downloadSchema(ctx, repository, provider, newCommit)
		if err != nil {
			return err
		}
	}

	if err := <-schOldDone; err != nil {
		return err
	}

	result := comparepkg.Compare(schOld, schNew, comparepkg.CompareOptions{
		Provider:   provider,
		MaxChanges: maxChanges,
	})
	return renderCompareOutput(os.Stdout, result, jsonMode, summaryMode)
}

func renderCompareOutput(out io.Writer, result comparepkg.CompareResult, jsonMode bool, summaryMode bool) error {
	if jsonMode {
		return comparepkg.RenderJSON(out, result, summaryMode)
	}
	if summaryMode {
		return comparepkg.RenderSummary(out, result)
	}
	comparepkg.RenderText(out, result)
	return nil
}

func breakingChanges(oldSchema, newSchema schema.PackageSpec) *diagtree.Node {
	return internalcompare.BreakingChanges(oldSchema, newSchema)
}

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec, maxChanges int) {
	result := comparepkg.Compare(oldSchema, newSchema, comparepkg.CompareOptions{
		Provider:   provider,
		MaxChanges: maxChanges,
	})
	comparepkg.RenderText(out, result)
}
