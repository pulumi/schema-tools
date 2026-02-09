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

	comparepkg "github.com/pulumi/schema-tools/internal/compare"
	"github.com/pulumi/schema-tools/internal/pkg"
	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

func compareCmd() *cobra.Command {
	var provider, repository, oldCommit, newCommit string
	var maxChanges int

	command := &cobra.Command{
		Use:   "compare",
		Short: "Compare two versions of a Pulumi schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			return compare(provider, repository, oldCommit, newCommit, maxChanges)
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

	return command
}

func compare(provider string, repository string, oldCommit string, newCommit string, maxChanges int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var schOld schema.PackageSpec
	schOldDone := make(chan error)
	go func() {
		var err error
		schOld, err = pkg.DownloadSchema(ctx, repository, provider, oldCommit)
		if err != nil {
			cancel()
		}
		schOldDone <- err
	}()

	var schNew schema.PackageSpec
	if newCommit == "--local" {
		usr, _ := user.Current()
		basePath := fmt.Sprintf("%s/go/src/github.com/pulumi/%s", usr.HomeDir, provider)
		schemaFile := pkg.StandardSchemaPath(provider)
		schemaPath := filepath.Join(basePath, schemaFile)
		var err error
		schNew, err = pkg.LoadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(newCommit, "--local-path=") {
		parts := strings.Split(newCommit, "=")
		schemaPath, err := filepath.Abs(parts[1])
		if err != nil {
			return fmt.Errorf("unable to construct absolute path to schema.json: %w", err)
		}
		schNew, err = pkg.LoadLocalPackageSpec(schemaPath)
		if err != nil {
			return err
		}
	} else {
		var err error
		schNew, err = pkg.DownloadSchema(ctx, repository, provider, newCommit)
		if err != nil {
			return err
		}
	}

	if err := <-schOldDone; err != nil {
		return err
	}

	compareSchemas(os.Stdout, provider, schOld, schNew, maxChanges)
	return nil
}

func breakingChanges(oldSchema, newSchema schema.PackageSpec) *diagtree.Node {
	return comparepkg.BreakingChanges(oldSchema, newSchema)
}

func compareSchemas(out io.Writer, provider string, oldSchema, newSchema schema.PackageSpec, maxChanges int) {
	report := comparepkg.Analyze(provider, oldSchema, newSchema)
	comparepkg.RenderText(out, report, maxChanges)
}
