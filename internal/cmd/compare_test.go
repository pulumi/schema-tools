package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/compare"
	"github.com/pulumi/schema-tools/internal/normalize"
	"github.com/pulumi/schema-tools/internal/pkg"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderCompareOutputModes(t *testing.T) {
	result := compare.Result{
		Summary:         []compare.SummaryItem{{Category: "missing-input", Count: 1, Entries: []string{"e1"}}},
		BreakingChanges: []string{"line-1"},
		NewResources:    []string{"r1"},
		NewFunctions:    []string{"f1"},
	}

	t.Run("json", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), `"breaking_changes": [`)
		assert.Contains(t, out.String(), `"line-1"`)
		assert.True(t, strings.HasSuffix(out.String(), "\n"))
	})

	t.Run("summary text", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, false, true)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "Summary by category:")
		assert.Contains(t, out.String(), "- missing-input: 1")
		assert.NotContains(t, out.String(), "e1")
	})

	t.Run("json summary", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, true)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), `"summary": [`)
		assert.Contains(t, out.String(), `"missing-input"`)
		assert.Contains(t, out.String(), `"entries": [`)
		assert.NotContains(t, out.String(), `"line-1"`)
		assert.NotContains(t, out.String(), `"r1"`)
		assert.NotContains(t, out.String(), `"f1"`)
		assert.NotContains(t, out.String(), `"breaking_changes":`)
		assert.NotContains(t, out.String(), `"new_resources":`)
		assert.NotContains(t, out.String(), `"new_functions":`)
	})

	t.Run("summary write error", func(t *testing.T) {
		err := renderCompareOutput(errorWriter{}, result, false, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write summary output")
	})

	t.Run("json write error", func(t *testing.T) {
		err := renderCompareOutput(errorWriter{}, result, true, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write compare JSON")
	})

	t.Run("text write error", func(t *testing.T) {
		err := renderCompareOutput(errorWriter{}, result, false, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write compare text output")
	})
}

func TestCompareCmdProviderFlagRequired(t *testing.T) {
	command := compareCmd()
	flag := command.Flag("provider")
	require.NotNil(t, flag)
	require.Contains(t, flag.Annotations, cobra.BashCompOneRequiredFlag)
	require.Equal(t, []string{"true"}, flag.Annotations[cobra.BashCompOneRequiredFlag])
}

func TestAddNormalizationRenamesAddsBreakingChangesAndSummaryCategories(t *testing.T) {
	result := compare.Result{
		Summary: []compare.SummaryItem{
			{Category: "type-changed", Count: 1, Entries: []string{`Resources: "pkg:index:Widget": inputs: "name" type changed from "string" to "integer"`}},
		},
		BreakingChanges: []string{`type changed from "string" to "integer"`},
	}
	renames := []normalize.TokenRename{
		{Scope: "resources", OldToken: "pkg:index:Widget", NewToken: "pkg:index:RenamedWidget"},
		{Scope: "datasources", OldToken: "pkg:index:getWidget", NewToken: "pkg:index:getRenamedWidget"},
	}

	got := addNormalizationRenames(result, renames)

	assert.Equal(t, []string{
		"`🔴` Functions: \"pkg:index:getWidget\" renamed to \"pkg:index:getRenamedWidget\"",
		"`🔴` Resources: \"pkg:index:Widget\" renamed to \"pkg:index:RenamedWidget\"",
		`type changed from "string" to "integer"`,
	}, got.BreakingChanges)

	categories := map[string]compare.SummaryItem{}
	for _, item := range got.Summary {
		categories[item.Category] = item
	}
	assert.Equal(t, 1, categories["renamed-function"].Count)
	assert.Equal(t, []string{`Functions: "pkg:index:getWidget" renamed to "pkg:index:getRenamedWidget"`}, categories["renamed-function"].Entries)
	assert.Equal(t, 1, categories["renamed-resource"].Count)
	assert.Equal(t, []string{`Resources: "pkg:index:Widget" renamed to "pkg:index:RenamedWidget"`}, categories["renamed-resource"].Entries)
	assert.Equal(t, 1, categories["type-changed"].Count)
}

func TestAddNormalizationMaxItemsOneAddsBreakingChangesAndSummaryCategory(t *testing.T) {
	result := compare.Result{
		Summary:         []compare.SummaryItem{},
		BreakingChanges: []string{},
	}
	changes := []normalize.MaxItemsOneChange{
		{
			Scope:    "resources",
			Token:    "pkg:index:Widget",
			Location: "inputs",
			Field:    "filter",
			OldType:  "string",
			NewType:  "array",
		},
	}

	got := addNormalizationMaxItemsOne(result, changes)

	assert.Equal(t, []string{
		"`🔴` Resources: \"pkg:index:Widget\": inputs: \"filter\" maxItemsOne changed from \"string\" to \"array\"",
	}, got.BreakingChanges)

	categories := map[string]compare.SummaryItem{}
	for _, item := range got.Summary {
		categories[item.Category] = item
	}
	assert.Equal(t, 1, categories["max-items-one-changed"].Count)
	assert.Equal(t, []string{
		`Resources: "pkg:index:Widget": inputs: "filter" maxItemsOne changed from "string" to "array"`,
	}, categories["max-items-one-changed"].Entries)
}

func TestApplyMaxChangesLimit(t *testing.T) {
	t.Run("no cap when unlimited", func(t *testing.T) {
		result := compare.Result{
			BreakingChanges: []string{"a", "b", "c"},
		}
		assert.Equal(t, []string{"a", "b", "c"}, applyMaxChangesLimit(result, -1).BreakingChanges)
	})

	t.Run("caps final output after normalization additions", func(t *testing.T) {
		result := compare.Result{
			BreakingChanges: []string{"legacy-1", "legacy-2"},
		}
		renamed := addNormalizationRenames(result, []normalize.TokenRename{
			{Scope: "resources", OldToken: "pkg:index:A", NewToken: "pkg:index:B"},
			{Scope: "datasources", OldToken: "pkg:index:getA", NewToken: "pkg:index:getB"},
		})

		capped := applyMaxChangesLimit(renamed, 2)
		assert.Len(t, capped.BreakingChanges, 2)
		assert.Contains(t, capped.BreakingChanges[0], "renamed")
		assert.Contains(t, capped.BreakingChanges[1], "renamed")
	})
}

func TestCompareEngineMaxChanges(t *testing.T) {
	t.Run("remote compare uses uncapped engine output", func(t *testing.T) {
		assert.Equal(t, -1, compareEngineMaxChanges(true, 10))
		assert.Equal(t, -1, compareEngineMaxChanges(true, 0))
	})

	t.Run("local compare preserves requested cap", func(t *testing.T) {
		assert.Equal(t, 10, compareEngineMaxChanges(false, 10))
		assert.Equal(t, 0, compareEngineMaxChanges(false, 0))
		assert.Equal(t, -1, compareEngineMaxChanges(false, -1))
	})
}

func TestCompareRequiresProvider(t *testing.T) {
	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, _ string) (schema.PackageSpec, error) {
			t.Fatalf("downloadSchema should not be called when provider is missing")
			return schema.PackageSpec{}, nil
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:   " ",
		repository: "github://api.github.com/pulumi",
		oldCommit:  "old-sha",
		newCommit:  "new-sha",
	}, deps)

	require.Error(t, err)
	assert.Equal(t, "--provider must be set", err.Error())
}

func TestCompareLocalCurrentUserErrorCancelsOldSchemaDownload(t *testing.T) {
	deps := compareDeps{
		currentUser: func() (*user.User, error) {
			return nil, errors.New("whoami failed")
		},
	}
	oldDownloadCanceled := make(chan struct{})
	deps.downloadSchema = func(
		ctx context.Context, repository string, provider string, ref string,
	) (schema.PackageSpec, error) {
		<-ctx.Done()
		close(oldDownloadCanceled)
		return schema.PackageSpec{}, ctx.Err()
	}
	deps.loadLocalPackageSpec = func(path string) (schema.PackageSpec, error) {
		t.Fatalf("loadLocalPackageSpec should not be called, got path=%s", path)
		return schema.PackageSpec{}, nil
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "aws",
		repository:  "github://api.github.com/pulumi",
		oldCommit:   "old",
		newCommit:   "--local",
		maxChanges:  100,
		jsonMode:    false,
		summaryMode: false,
	}, deps)
	if assert.Error(t, err) {
		assert.True(t, strings.Contains(err.Error(), "get current user"))
	}

	select {
	case <-oldDownloadCanceled:
	case <-time.After(time.Second):
		t.Fatal("old schema download goroutine was not canceled")
	}
}

type errorWriter struct{}

func (errorWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestCompareSchemasFixtureTextOutput(t *testing.T) {
	oldSchema, newSchema := mustLoadCompareFixtureSchemas(t)

	var out bytes.Buffer
	result := compare.Schemas(oldSchema, newSchema, compare.Options{
		Provider:   "my-pkg",
		MaxChanges: -1,
	})
	assert.NoError(t, compare.RenderText(&out, result))

	text := out.String()
	assert.Contains(t, text, "Found 14 breaking changes:")
	assert.Contains(t, text, `"my-pkg:index:RemovedResource" missing`)
	assert.Contains(t, text, `"my-pkg:index:removedFunction" missing`)
	assert.Contains(t, text, `type changed from "string" to "integer"`)
	assert.Contains(t, text, `input has changed to Required`)
	assert.Contains(t, text, `property is no longer Required`)
}

func TestRenderCompareOutputFixtureJSON(t *testing.T) {
	oldSchema, newSchema := mustLoadCompareFixtureSchemas(t)
	result := compare.Schemas(oldSchema, newSchema, compare.Options{
		Provider:   "my-pkg",
		MaxChanges: -1,
	})

	t.Run("full", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, false)
		assert.NoError(t, err)

		var payload struct {
			Summary []struct {
				Category string `json:"category"`
				Count    int    `json:"count"`
				Entries  []string
			} `json:"summary"`
			BreakingChanges []string `json:"breaking_changes"`
			NewResources    []string `json:"new_resources"`
			NewFunctions    []string `json:"new_functions"`
		}
		assert.NoError(t, json.Unmarshal(out.Bytes(), &payload))

		gotCounts := map[string]int{}
		for _, item := range payload.Summary {
			gotCounts[item.Category] = item.Count
			assert.Empty(t, item.Entries)
		}
		assert.Equal(t, map[string]int{
			"missing-function":     1,
			"missing-resource":     1,
			"optional-to-required": 3,
			"required-to-optional": 2,
			"type-changed":         1,
		}, gotCounts)
		assert.Equal(t, result.BreakingChanges, payload.BreakingChanges)
		assert.Empty(t, payload.NewResources)
		assert.Empty(t, payload.NewFunctions)
	})

	t.Run("summary", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, true)
		assert.NoError(t, err)

		var payload struct {
			Summary []struct {
				Category string   `json:"category"`
				Count    int      `json:"count"`
				Entries  []string `json:"entries"`
			} `json:"summary"`
		}
		assert.NoError(t, json.Unmarshal(out.Bytes(), &payload))

		entriesByCategory := map[string][]string{}
		for _, item := range payload.Summary {
			entriesByCategory[item.Category] = item.Entries
		}
		assert.Equal(t, []string{
			`Functions: "my-pkg:index:MyFunction": inputs: required: "arg" input has changed to Required`,
			`Resources: "my-pkg:index:MyResource": required inputs: "count" input has changed to Required`,
			`Types: "my-pkg:index:MyType": required: "count" property has changed to Required`,
		}, entriesByCategory["optional-to-required"])
	})
}

func mustLoadCompareFixtureSchemas(t testing.TB) (schema.PackageSpec, schema.PackageSpec) {
	t.Helper()
	// Keep in sync with compare/compare_test.go fixture loaders by design:
	// package boundaries prevent sharing local *_test.go helpers directly.
	return mustLoadCompareFixtureSchema(t, "schema-old.json"),
		mustLoadCompareFixtureSchema(t, "schema-new.json")
}

func mustLoadCompareFixtureSchema(t testing.TB, name string) schema.PackageSpec {
	t.Helper()
	data, err := os.ReadFile("../../testdata/compare/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	var spec schema.PackageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal fixture %q: %v", name, err)
	}
	return spec
}

func TestCompareMetadataNormalizeE2EStrictRemoteUsesRepoMetadata(t *testing.T) {
	metadataCalls := []struct {
		commit string
		path   string
	}{}
	var metadataCallsMu sync.Mutex
	normalizeCalled := false

	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, _ string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, commit string, repoPath string) ([]byte, error) {
			metadataCallsMu.Lock()
			metadataCalls = append(metadataCalls, struct {
				commit string
				path   string
			}{commit: commit, path: repoPath})
			metadataCallsMu.Unlock()
			return []byte(`{"auto-aliasing":{"resources":{},"datasources":{}}}`), nil
		},
		normalizeSchemas: func(
			oldSchema, newSchema schema.PackageSpec,
			oldMetadata, newMetadata *normalize.MetadataEnvelope,
		) (normalize.Result, error) {
			normalizeCalled = true
			assert.NotNil(t, oldMetadata)
			assert.NotNil(t, newMetadata)
			return normalize.Result{OldSchema: oldSchema, NewSchema: newSchema}, nil
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "my-pkg",
		repository:  "github://api.github.com/pulumi",
		oldCommit:   "old-sha",
		newCommit:   "new-sha",
		maxChanges:  1,
		summaryMode: true,
	}, deps)

	assert.NoError(t, err)
	assert.True(t, normalizeCalled, "normalization should run when metadata is available")
	metadataCallsMu.Lock()
	assert.ElementsMatch(t, []struct {
		commit string
		path   string
	}{
		{commit: "old-sha", path: "provider/cmd/pulumi-resource-my-pkg/bridge-metadata.json"},
		{commit: "new-sha", path: "provider/cmd/pulumi-resource-my-pkg/bridge-metadata.json"},
	}, metadataCalls)
	metadataCallsMu.Unlock()
}

func TestCompareMetadataE2EStrictRemoteMissingMetadataFailsTyped(t *testing.T) {
	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, _ string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
			return nil, &pkg.RepoFileNotFoundError{
				RepositoryURL:  "github://api.github.com/pulumi",
				Commit:         "sha",
				RepositoryPath: "provider/cmd/pulumi-resource-my-pkg/bridge-metadata.json",
				Err:            errors.New("not found"),
			}
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "my-pkg",
		repository:  "github://api.github.com/pulumi",
		oldCommit:   "old-sha",
		newCommit:   "new-sha",
		maxChanges:  1,
		summaryMode: true,
	}, deps)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCompareMetadataRequired))
}

func TestCompareNormalizeE2ELocalModeUnchangedSkipsNormalizationAndMetadata(t *testing.T) {
	normalizeCalled := false
	metadataFetchCount := 0
	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, _ string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		loadLocalPackageSpec: func(path string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
			metadataFetchCount++
			return nil, errors.New("unexpected metadata download")
		},
		normalizeSchemas: func(
			oldSchema, newSchema schema.PackageSpec,
			oldMetadata, newMetadata *normalize.MetadataEnvelope,
		) (normalize.Result, error) {
			normalizeCalled = true
			return normalize.Result{OldSchema: oldSchema, NewSchema: newSchema}, nil
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "my-pkg",
		repository:  "gitlab://gitlab.com/pulumi",
		oldCommit:   "old-sha",
		newPath:     "/tmp/local-new-schema.json",
		maxChanges:  1,
		summaryMode: true,
	}, deps)

	assert.NoError(t, err)
	assert.False(t, normalizeCalled, "local mode should remain unchanged")
	assert.Equal(t, 0, metadataFetchCount, "local mode should not fetch repo metadata")
}

func TestCompareNormalizeE2EHybridModeOldLocalNewRemoteSkipsNormalizationAndMetadata(t *testing.T) {
	normalizeCalled := false
	metadataFetchCount := 0
	deps := compareDeps{
		loadLocalPackageSpec: func(path string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadSchema: func(_ context.Context, _ string, _ string, _ string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
			metadataFetchCount++
			return nil, errors.New("unexpected metadata download")
		},
		normalizeSchemas: func(
			oldSchema, newSchema schema.PackageSpec,
			oldMetadata, newMetadata *normalize.MetadataEnvelope,
		) (normalize.Result, error) {
			normalizeCalled = true
			return normalize.Result{OldSchema: oldSchema, NewSchema: newSchema}, nil
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "my-pkg",
		repository:  "github://api.github.com/pulumi",
		oldPath:     "/tmp/local-old-schema.json",
		newCommit:   "new-sha",
		maxChanges:  1,
		summaryMode: true,
	}, deps)

	assert.NoError(t, err)
	assert.False(t, normalizeCalled, "hybrid local/remote mode should remain unchanged")
	assert.Equal(t, 0, metadataFetchCount, "hybrid local/remote mode should not fetch repo metadata")
}

func TestCompareNormalizeE2EFileRepositorySkipsNormalizationAndMetadata(t *testing.T) {
	normalizeCalled := false
	metadataFetchCount := 0
	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, _ string) (schema.PackageSpec, error) {
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
			metadataFetchCount++
			return nil, errors.New("unexpected metadata download")
		},
		normalizeSchemas: func(
			oldSchema, newSchema schema.PackageSpec,
			oldMetadata, newMetadata *normalize.MetadataEnvelope,
		) (normalize.Result, error) {
			normalizeCalled = true
			return normalize.Result{OldSchema: oldSchema, NewSchema: newSchema}, nil
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "my-pkg",
		repository:  "file:" + filepath.Join(t.TempDir(), "schema.json"),
		oldCommit:   "old-sha",
		newCommit:   "new-sha",
		maxChanges:  1,
		summaryMode: true,
	}, deps)

	assert.NoError(t, err)
	assert.False(t, normalizeCalled, "file repository compare should remain unchanged")
	assert.Equal(t, 0, metadataFetchCount, "file repository compare should not fetch repo metadata")
}

func TestCompareResolveMetadataSourceRepoCommit(t *testing.T) {
	deps := defaultCompareDeps()
	deps.downloadRepoFile = func(_ context.Context, repository string, provider string, commit string, repoPath string) ([]byte, error) {
		assert.Equal(t, "github://api.github.com/pulumi", repository)
		assert.Equal(t, "aws", provider)
		assert.Equal(t, "v1.2.3", commit)
		assert.Equal(t, "provider/cmd/pulumi-resource-aws/bridge-metadata.json", repoPath)
		return []byte(`{"auto-aliasing":{"resources":{},"datasources":{}}}`), nil
	}

	metadata, err := resolveCompareMetadataSource(
		context.Background(), deps, "new", "aws", "github://api.github.com/pulumi", "v1.2.3",
	)

	assert.NoError(t, err)
	assert.NotNil(t, metadata)
}

func TestCompareMetadataSourceStrictMissingRepoMetadata(t *testing.T) {
	deps := defaultCompareDeps()
	deps.downloadRepoFile = func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
		return nil, &pkg.RepoFileNotFoundError{
			RepositoryURL:  "github://api.github.com/pulumi",
			Commit:         "main",
			RepositoryPath: "provider/cmd/pulumi-resource-aws/bridge-metadata.json",
			Err:            errors.New("not found"),
		}
	}

	_, err := resolveCompareMetadataSource(
		context.Background(), deps, "new", "aws", "github://api.github.com/pulumi", "main",
	)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCompareMetadataRequired))

	var requiredErr *compareMetadataRequiredError
	assert.True(t, errors.As(err, &requiredErr))
	assert.Equal(t,
		"compare new metadata required: github://api.github.com/pulumi@main:provider/cmd/pulumi-resource-aws/bridge-metadata.json",
		requiredErr.Error())
}

func TestCompareRemoteDefaultOldCommitUsesMaster(t *testing.T) {
	schemaCommits := []string{}
	var schemaCommitsMu sync.Mutex
	metadataCommits := []string{}
	var metadataCommitsMu sync.Mutex
	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, commit string) (schema.PackageSpec, error) {
			schemaCommitsMu.Lock()
			schemaCommits = append(schemaCommits, commit)
			schemaCommitsMu.Unlock()
			return schema.PackageSpec{Name: "my-pkg", Resources: map[string]schema.ResourceSpec{}}, nil
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, commit string, _ string) ([]byte, error) {
			metadataCommitsMu.Lock()
			metadataCommits = append(metadataCommits, commit)
			metadataCommitsMu.Unlock()
			return []byte(`{"auto-aliasing":{"resources":{},"datasources":{}}}`), nil
		},
	}

	err := runCompareCmdWithDeps(compareInput{
		provider:    "my-pkg",
		repository:  "github://api.github.com/pulumi",
		oldCommit:   "",
		newCommit:   "new-sha",
		maxChanges:  1,
		summaryMode: true,
	}, deps)
	require.NoError(t, err)
	schemaCommitsMu.Lock()
	require.Len(t, schemaCommits, 2)
	assert.Contains(t, schemaCommits, "master")
	assert.Contains(t, schemaCommits, "new-sha")
	schemaCommitsMu.Unlock()
	metadataCommitsMu.Lock()
	assert.ElementsMatch(t, []string{"master", "new-sha"}, metadataCommits)
	metadataCommitsMu.Unlock()
}

func TestCompareMetadataSourceStrictParseMetadataRequired(t *testing.T) {
	deps := defaultCompareDeps()
	deps.downloadRepoFile = func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
		return []byte(`{}`), nil
	}
	deps.parseMetadata = func(_ []byte) (*normalize.MetadataEnvelope, error) {
		return nil, normalize.ErrMetadataRequired
	}

	_, err := resolveCompareMetadataSource(
		context.Background(), deps, "new", "aws", "github://api.github.com/pulumi", "main",
	)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCompareMetadataRequired))
}

func TestCompareMetadataSourceStrictParseMetadataRequiredWrapped(t *testing.T) {
	deps := defaultCompareDeps()
	deps.downloadRepoFile = func(_ context.Context, _ string, _ string, _ string, _ string) ([]byte, error) {
		return []byte(`{}`), nil
	}
	deps.parseMetadata = func(_ []byte) (*normalize.MetadataEnvelope, error) {
		return nil, fmt.Errorf("parse failed: %w", normalize.ErrMetadataRequired)
	}

	_, err := resolveCompareMetadataSource(
		context.Background(), deps, "new", "aws", "github://api.github.com/pulumi", "main",
	)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCompareMetadataRequired))

	var requiredErr *compareMetadataRequiredError
	assert.True(t, errors.As(err, &requiredErr))
	assert.Equal(t, "new", requiredErr.Side)
}

func TestApplyMaxChangesLimitZero(t *testing.T) {
	result := compare.Result{
		BreakingChanges: []string{"a", "b"},
	}
	capped := applyMaxChangesLimit(result, 0)
	assert.Empty(t, capped.BreakingChanges)
}

func TestCompareNormalizationComposeRenameAndMaxItemsOneWithCap(t *testing.T) {
	oldToken := "pkg:index:Widget"
	newToken := "pkg:index:RenamedWidget"
	oldSchema := schema.PackageSpec{
		Name: "pkg",
		Resources: map[string]schema.ResourceSpec{
			oldToken: {
				InputProperties: map[string]schema.PropertySpec{
					"name":   {TypeSpec: schema.TypeSpec{Type: "string"}},
					"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Name: "pkg",
		Resources: map[string]schema.ResourceSpec{
			newToken: {
				InputProperties: map[string]schema.PropertySpec{
					"name": {
						TypeSpec: schema.TypeSpec{Type: "integer"},
					},
					"filter": {
						TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						},
					},
				},
			},
		},
	}

	oldMetadata := []byte(`{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {"filter": {"maxItemsOne": true}}
				}
			},
			"datasources": {}
		}
	}`)
	newMetadata := []byte(`{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:RenamedWidget",
					"past": [{"name":"pkg:index:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {"filter": {"maxItemsOne": false}}
				}
			},
			"datasources": {}
		}
	}`)

	deps := compareDeps{
		downloadSchema: func(_ context.Context, _ string, _ string, commit string) (schema.PackageSpec, error) {
			switch commit {
			case "old-sha":
				return oldSchema, nil
			case "new-sha":
				return newSchema, nil
			default:
				return schema.PackageSpec{}, fmt.Errorf("unexpected commit: %s", commit)
			}
		},
		downloadRepoFile: func(_ context.Context, _ string, _ string, commit string, _ string) ([]byte, error) {
			switch commit {
			case "old-sha":
				return oldMetadata, nil
			case "new-sha":
				return newMetadata, nil
			default:
				return nil, fmt.Errorf("unexpected metadata commit: %s", commit)
			}
		},
	}

	run := func(maxChanges int) map[string]any {
		stdout := captureStdout(t, func() {
			err := runCompareCmdWithDeps(compareInput{
				provider:   "pkg",
				repository: "github://api.github.com/pulumi",
				oldCommit:  "old-sha",
				newCommit:  "new-sha",
				maxChanges: maxChanges,
				jsonMode:   true,
			}, deps)
			require.NoError(t, err)
		})
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
		return payload
	}

	uncapped := run(-1)
	uncappedChanges := uncapped["breaking_changes"].([]any)
	uncappedText := []string{}
	for _, line := range uncappedChanges {
		uncappedText = append(uncappedText, line.(string))
	}
	require.GreaterOrEqual(t, len(uncappedText), 3)
	joinedUncapped := strings.Join(uncappedText, "\n")
	assert.Contains(t, joinedUncapped, "maxItemsOne changed")
	assert.Contains(t, joinedUncapped, "renamed")
	assert.Contains(t, joinedUncapped, "type changed")

	capped := run(2)
	cappedChanges := capped["breaking_changes"].([]any)
	require.Len(t, cappedChanges, 2)
	assert.Contains(t, cappedChanges[0].(string), "maxItemsOne changed")
	assert.Contains(t, cappedChanges[1].(string), "renamed")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	// This helper mutates process-global stdout; callers must not run in parallel.

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
		_ = r.Close()
		_ = w.Close()
	}()

	readDone := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		readDone <- string(data)
	}()

	fn()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	return <-readDone
}
