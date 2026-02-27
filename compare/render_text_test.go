package compare

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func TestRenderTextNoBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("expected no-breaking message, got:\n%s", text)
	}
}

func TestRenderTextPreservesNewResourcesAndFunctionsOrder(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		NewResources: []string{"zeta.Resource", "alpha.Resource"},
		NewFunctions: []string{"zeta.fn", "alpha.fn"},
	}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if first, second := strings.Index(text, "- `zeta.Resource`"), strings.Index(text, "- `alpha.Resource`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected resources to preserve input order, got:\n%s", text)
	}
	if first, second := strings.Index(text, "- `zeta.fn`"), strings.Index(text, "- `alpha.fn`"); first == -1 || second == -1 || first > second {
		t.Fatalf("expected functions to preserve input order, got:\n%s", text)
	}
}

func TestRenderTextOneBreakingChange(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:Res",
				Path:     `Resources: "pkg:index:Res"`,
				Kind:     "missing-resource",
				Severity: SeverityError,
				Breaking: true,
				Message:  "missing",
			},
		},
	}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Found 1 breaking change:\n") {
		t.Fatalf("expected singular breaking-change header, got:\n%s", text)
	}
	if !strings.Contains(text, "`🔴`") || !strings.Contains(text, `"pkg:index:Res" missing`) {
		t.Fatalf("expected rendered change details, got:\n%s", text)
	}
}

func TestRenderTextManyBreakingChanges(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeFunction,
				Token:    "pkg:index:getA",
				Path:     `Functions: "pkg:index:getA"`,
				Kind:     "signature-changed",
				Severity: SeverityError,
				Breaking: true,
				Message:  "signature change",
			},
			{
				Scope:    ScopeType,
				Token:    "pkg:index:Type",
				Location: "properties",
				Path:     `Types: "pkg:index:Type": properties: "field"`,
				Kind:     "type-changed",
				Severity: SeverityWarn,
				Breaking: true,
				Message:  `type changed from "string" to "integer"`,
			},
		},
	}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Found 2 breaking changes:\n") {
		t.Fatalf("expected plural breaking-change header, got:\n%s", text)
	}
	if !strings.Contains(text, "signature change") || !strings.Contains(text, `type changed from "string" to "integer"`) {
		t.Fatalf("expected all rendered change messages, got:\n%s", text)
	}
}

func TestRenderTextGroupsDirectAndNestedChanges(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeFunction,
				Token:    "my-pkg:index:MyFunction",
				Path:     `Functions: "my-pkg:index:MyFunction"`,
				Kind:     "signature-changed",
				Severity: SeverityError,
				Breaking: true,
				Message:  "signature change",
			},
			{
				Scope:    ScopeFunction,
				Token:    "my-pkg:index:MyFunction",
				Location: "inputs",
				Path:     `Functions: "my-pkg:index:MyFunction": inputs: required: "arg"`,
				Kind:     "optional-to-required",
				Severity: SeverityInfo,
				Breaking: true,
				Message:  "input has changed to Required",
			},
		},
	}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "#### Functions") {
		t.Fatalf("expected functions section header, got:\n%s", text)
	}
	if !strings.Contains(text, `- "my-pkg:index:MyFunction":`) {
		t.Fatalf("expected token bullet header, got:\n%s", text)
	}
	if !strings.Contains(text, "    - inputs:") {
		t.Fatalf("expected nested location bucket, got:\n%s", text)
	}
	if !strings.Contains(text, `        - `+"`🟢`"+` "arg" input has changed to Required`) {
		t.Fatalf("expected nested input detail in output, got:\n%s", text)
	}
}

func TestRenderTextMaxChangesZeroUsesTotalBreakingCount(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{totalBreaking: 2}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("unexpected no-breaking message when total > 0:\n%s", text)
	}
	if !strings.Contains(text, "Found 2 breaking changes:\n") {
		t.Fatalf("expected total-breaking header, got:\n%s", text)
	}
	if !strings.Contains(text, "Showing 0 of 2 breaking changes.") {
		t.Fatalf("expected truncation message, got:\n%s", text)
	}
}

func TestRenderTextShowsDisplayedAndTotalBreakingCountWhenCapped(t *testing.T) {
	var out bytes.Buffer
	if err := RenderText(&out, Result{
		Changes: []Change{
			{
				Scope:    ScopeFunction,
				Token:    "my-pkg:index:MyFunction",
				Location: "inputs",
				Path:     `Functions: "my-pkg:index:MyFunction": inputs: required: "arg"`,
				Kind:     "optional-to-required",
				Severity: SeverityInfo,
				Breaking: true,
				Message:  "input has changed to Required",
			},
		},
		totalBreaking: 8,
	}); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Found 8 breaking changes:\n") {
		t.Fatalf("expected total-breaking header, got:\n%s", text)
	}
	if !strings.Contains(text, "Showing 1 of 8 breaking changes.") {
		t.Fatalf("expected displayed/total truncation message, got:\n%s", text)
	}
}

func TestRenderTextWriteError(t *testing.T) {
	err := RenderText(failingWriter{}, Result{
		Changes: []Change{
			{
				Scope:    ScopeResource,
				Token:    "pkg:index:Res",
				Path:     `Resources: "pkg:index:Res"`,
				Kind:     "missing-resource",
				Severity: SeverityError,
				Breaking: true,
				Message:  "missing",
			},
		},
	})
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestRenderTextResolvedTokenRemapShowsGuidance(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v2:Widget": {},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget"}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v2:Widget","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}]}}}}`)
	result := Schemas(oldSchema, newSchema, Options{Provider: "my-pkg", MaxChanges: -1, OldMetadata: oldMetadata, NewMetadata: newMetadata})

	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("expected no-breaking message for remap-only changes, got:\n%s", text)
	}
	if !strings.Contains(text, "#### Token remaps") {
		t.Fatalf("expected token remap section, got:\n%s", text)
	}
	if !strings.Contains(text, `token remapped: migrate from "my-pkg:index/v1:Widget" to "my-pkg:index/v2:Widget"`) {
		t.Fatalf("expected remap guidance in text output, got:\n%s", text)
	}
}

func TestRenderTextSuppressesEquivalentMaxItemsTypeChangeInOnePass(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"list": {TypeSpec: schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"list": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index:Widget","fields":{"list":{"maxItemsOne":true}}}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index:Widget","fields":{"list":{"maxItemsOne":false}}}}}}`)

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "my-pkg",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("expected no-breaking message for equivalent maxItems transition, got:\n%s", text)
	}
	if strings.Contains(text, "type changed") {
		t.Fatalf("unexpected type changed diagnostic for equivalent maxItems transition:\n%s", text)
	}
}

func TestRenderTextKeepsTypeChangeWhenMaxItemsLookupAmbiguous(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"spec": {TypeSpec: schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v2:WidgetA": {
				InputProperties: map[string]schema.PropertySpec{
					"spec": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
			"my-pkg:index/v2:WidgetB": {
				InputProperties: map[string]schema.PropertySpec{
					"spec": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index/v1:Widget","fields":{"spec":{"maxItemsOne":true}}}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{
		"tf_widget_a":{"current":"my-pkg:index/v2:WidgetA","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}],"fields":{"spec":{"maxItemsOne":false}}},
		"tf_widget_b":{"current":"my-pkg:index/v2:WidgetB","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}],"fields":{"spec":{"maxItemsOne":false}}}
	}}}`)

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "my-pkg",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Found 1 breaking change:") {
		t.Fatalf("expected one breaking change when lookup is ambiguous, got:\n%s", text)
	}
	if !strings.Contains(text, `lookup ambiguous: my-pkg:index/v2:WidgetA, my-pkg:index/v2:WidgetB`) {
		t.Fatalf("expected deterministic ambiguous lookup diagnostic, got:\n%s", text)
	}
}

func TestRenderTextKeepsNestedMapValueTypeChangeWithMaxItemsTransition(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"list": {
						TypeSpec: schema.TypeSpec{
							Type: "array",
							Items: &schema.TypeSpec{
								Type: "object",
								AdditionalProperties: &schema.TypeSpec{
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"list": {
						TypeSpec: schema.TypeSpec{
							Type: "object",
							AdditionalProperties: &schema.TypeSpec{
								Type: "number",
							},
						},
					},
				},
			},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index:Widget","fields":{"list":{"maxItemsOne":true}}}}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{"tf_widget":{"current":"my-pkg:index:Widget","fields":{"list":{"maxItemsOne":false}}}}}}`)

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "my-pkg",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, `inputs: "list" type changed from "array" to "object"`) {
		t.Fatalf("expected root list type change to remain visible, got:\n%s", text)
	}
	if !strings.Contains(text, `inputs: "list": additional properties had no type but now has`) {
		t.Fatalf("expected nested map value-type diagnostic to remain visible, got:\n%s", text)
	}
}

func TestRenderTextKeepsTypeChangeWhenFieldEquivalenceIsAmbiguousAfterTokenResolution(t *testing.T) {
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v1:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"spec": {TypeSpec: schema.TypeSpec{Type: "array", Items: &schema.TypeSpec{Type: "string"}}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			"my-pkg:index/v2:Widget": {
				InputProperties: map[string]schema.PropertySpec{
					"spec": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	oldMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{
		"tf_widget_a":{"current":"my-pkg:index/v1:Widget","fields":{"spec":{"maxItemsOne":true}}},
		"tf_widget_b":{"current":"my-pkg:index/v1:Widget","fields":{"spec":{"maxItemsOne":true}}}
	}}}`)
	newMetadata := mustParseMetadataCompare(t, `{"auto-aliasing":{"version":1,"resources":{
		"tf_widget_a":{"current":"my-pkg:index/v2:Widget","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}],"fields":{"spec":{"maxItemsOne":false}}},
		"tf_widget_b":{"current":"my-pkg:index/v2:Widget","past":[{"name":"my-pkg:index/v1:Widget","inCodegen":false,"majorVersion":1}],"fields":{"spec":{"maxItemsOne":true}}}
	}}}`)

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "my-pkg",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})

	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	text := out.String()
	if strings.Contains(text, "Looking good! No breaking changes found.") {
		t.Fatalf("expected breaking type changes when field-equivalence is ambiguous, got:\n%s", text)
	}
	if !strings.Contains(text, `type changed from "array" to "string"`) {
		t.Fatalf("expected type-change line when field-equivalence is ambiguous, got:\n%s", text)
	}
}
