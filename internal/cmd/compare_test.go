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
		Summary: []compare.SummaryItem{{Category: "missing-input", Count: 1, Entries: []string{"e1"}}},
		Changes: []compare.Change{
			{
				Scope:    compare.ScopeResource,
				Token:    "pkg:index:Res",
				Location: "inputs",
				Path:     `Resources: "pkg:index:Res": inputs: "name"`,
				Kind:     "missing-input",
				Severity: compare.SeverityWarn,
				Breaking: true,
				Message:  "missing input",
			},
		},
		Grouped: compare.GroupedChanges{
			Resources: map[string]map[string][]compare.Change{
				"pkg:index:Res": {
					"inputs": {
						{
							Scope:    compare.ScopeResource,
							Token:    "pkg:index:Res",
							Location: "inputs",
							Path:     `Resources: "pkg:index:Res": inputs: "name"`,
							Kind:     "missing-input",
							Severity: compare.SeverityWarn,
							Breaking: true,
							Message:  "missing input",
						},
					},
				},
			},
			Functions: map[string]map[string][]compare.Change{},
			Types:     map[string]map[string][]compare.Change{},
		},
		NewResources: []string{"r1"},
		NewFunctions: []string{"f1"},
	}

	t.Run("json", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), `"changes": [`)
		assert.Contains(t, out.String(), `"grouped": {`)
		assert.Contains(t, out.String(), `"scope": "resource"`)
		assert.NotContains(t, out.String(), `"breaking_changes":`)
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
		assert.NotContains(t, out.String(), `"entries": [`)
		assert.NotContains(t, out.String(), `"scope":`)
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

func TestRenderCompareOutputTextPreservesBreakingDiagnosticLines(t *testing.T) {
	result := compare.Result{
		BreakingChanges: []string{
			"### Resources",
			`#### "my-pkg:index:Widget"`,
			`- ` + "`🟡`" + ` inputs: "list" type changed from "array" to "string"`,
		},
	}

	var out bytes.Buffer
	err := renderCompareOutput(&out, result, false, false)
	assert.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, `type changed from "array" to "string"`)
	assert.NotContains(t, text, "maxItemsOne")
}

func TestRenderCompareOutputTextUsesOnePassDisplayedEntryCountWhenCapped(t *testing.T) {
	result := compare.Result{
		BreakingChanges: []string{
			"### Resources",
			`#### "my-pkg:index:Widget"`,
			`- ` + "`🟢`" + ` inputs: required: "list" input has changed to Required`,
		},
	}

	var out bytes.Buffer
	err := renderCompareOutput(&out, result, false, false)
	assert.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Found 1 breaking change:")
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
	assert.Contains(t, text, "Found 8 breaking changes:")
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
				Category string   `json:"category"`
				Count    int      `json:"count"`
				Entries  []string `json:"entries,omitempty"`
			} `json:"summary"`
			Changes      []compare.Change       `json:"changes"`
			Grouped      compare.GroupedChanges `json:"grouped"`
			NewResources []string               `json:"new_resources"`
			NewFunctions []string               `json:"new_functions"`
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
		assert.Equal(t, result.Changes, payload.Changes)
		assert.Equal(t, result.Grouped, payload.Grouped)
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
				Entries  []string `json:"entries,omitempty"`
			} `json:"summary"`
		}
		assert.NoError(t, json.Unmarshal(out.Bytes(), &payload))

		countsByCategory := map[string]int{}
		for _, item := range payload.Summary {
			assert.Empty(t, item.Entries)
			countsByCategory[item.Category] = item.Count
		}
		assert.Equal(t, map[string]int{
			"missing-function":     1,
			"missing-resource":     1,
			"optional-to-required": 3,
			"required-to-optional": 2,
			"type-changed":         1,
		}, countsByCategory)
	})
}

func TestRenderCompareOutputFixtureJSONGoldens(t *testing.T) {
	oldSchema, newSchema := mustLoadCompareFixtureSchemas(t)
	result := compare.Schemas(oldSchema, newSchema, compare.Options{
		Provider:   "my-pkg",
		MaxChanges: -1,
	})

	t.Run("full", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, false)
		assert.NoError(t, err)
		assertJSONGoldenString(t, out.String(), "compare-full.golden.json")
	})

	t.Run("summary", func(t *testing.T) {
		var out bytes.Buffer
		err := renderCompareOutput(&out, result, true, true)
		assert.NoError(t, err)
		assertJSONGoldenString(t, out.String(), "compare-summary.golden.json")
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
	data := mustReadCompareFixtureFile(t, name)
	var spec schema.PackageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal fixture %q: %v", name, err)
	}
	return spec
}

func mustReadCompareFixtureFile(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/compare/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

func assertJSONGoldenString(t testing.TB, got, goldenName string) {
	t.Helper()
	want := string(mustReadCompareFixtureFile(t, goldenName))
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", goldenName, strings.TrimSpace(got), strings.TrimSpace(want))
	}
}
