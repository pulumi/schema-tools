package compare

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pulumi/schema-tools/internal/normalize"
)

func TestSchemasRealAWSExamplesExpectedNormalization(t *testing.T) {
	oldSchema := mustReadFixtureSchema(t, "aws-real-v6.83.0-schema-snippet.json")
	newSchema := mustReadFixtureSchema(t, "aws-real-v7.0.0-schema-snippet.json")
	oldMetadata := mustReadFixtureMetadata(t, "aws-real-v6.83.0-bridge-metadata-snippet.json")
	newMetadata := mustReadFixtureMetadata(t, "aws-real-v7.0.0-bridge-metadata-snippet.json")

	result := Schemas(oldSchema, newSchema, Options{
		Provider:    "aws",
		MaxChanges:  -1,
		OldMetadata: oldMetadata,
		NewMetadata: newMetadata,
	})
	var out bytes.Buffer
	if err := RenderText(&out, result); err != nil {
		t.Fatalf("RenderText failed: %v", err)
	}
	rendered := out.String()

	assertContains(t, rendered, `"loggings" renamed to "logging" and type changed from "array" to "#/types/aws:s3/BucketLogging:BucketLogging"`)
	assertContains(t, rendered, `"containers" type changed from "#/types/aws:batch/JobDefinitionEksPropertiesPodPropertiesContainers:JobDefinitionEksPropertiesPodPropertiesContainers" to "array<#/types/aws:batch/JobDefinitionEksPropertiesPodPropertiesContainer:JobDefinitionEksPropertiesPodPropertiesContainer>"`)
	assertContains(t, rendered, `"forward" renamed to "forwards" and type changed from "#/types/aws:lb/getListenerRuleActionForward:getListenerRuleActionForward" to "array<#/types/aws:lb/getListenerRuleActionForward:getListenerRuleActionForward>"`)
	assertContains(t, rendered, `"stickiness" renamed to "stickinesses" and type changed from "#/types/aws:lb/getListenerRuleActionForwardStickiness:getListenerRuleActionForwardStickiness" to "array<#/types/aws:lb/getListenerRuleActionForwardStickiness:getListenerRuleActionForwardStickiness>"`)

	assertNotContains(t, rendered, `inputs: "loggings" missing`)
	assertNotContains(t, rendered, `properties: "forward" missing`)
	assertNotContains(t, rendered, `properties: "stickiness" missing`)
}

func assertContains(t *testing.T, rendered, needle string) {
	t.Helper()
	if !strings.Contains(rendered, needle) {
		t.Fatalf("expected rendered output to contain %q\nactual:\n%s", needle, rendered)
	}
}

func assertNotContains(t *testing.T, rendered, needle string) {
	t.Helper()
	if strings.Contains(rendered, needle) {
		t.Fatalf("expected rendered output to not contain %q\nactual:\n%s", needle, rendered)
	}
}

func mustReadFixtureMetadata(t testing.TB, name string) *normalize.MetadataEnvelope {
	t.Helper()
	data := mustReadTestdataFile(t, name)
	metadata, err := normalize.ParseMetadata(data)
	if err != nil {
		t.Fatalf("parse metadata fixture %q: %v", name, err)
	}
	return metadata
}
