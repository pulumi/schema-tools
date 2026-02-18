package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/compare"
	"github.com/stretchr/testify/assert"
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
		assert.True(t, reflect.DeepEqual(gotCounts, map[string]int{
			"missing-function":     1,
			"missing-resource":     1,
			"optional-to-required": 3,
			"required-to-optional": 2,
			"type-changed":         1,
		}))
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
