package stats

import (
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	v2 "github.com/pulumi/schema-tools/internal/pkg/stats/format-v2"
)

func V2(schema schema.PackageSpec) v2.Stats {
	stats := v2.Stats{
		Resources: make(map[string]v2.Resource, len(schema.Resources)),
		Functions: make(map[string]v2.Function, len(schema.Functions)),
	}

	visitor := &v2Builder{stats}

	visitSchema(schema, visitor)

	visitor.summarize()

	return visitor.stats

}

// The kind of inputness in the property
type propKind string

const (
	// If the type isn't used as an input or an output
	unknownKind propKind = ""
	// If the type is used exclusively as an input property
	inputKind propKind = "input"
	// If the type is used exclusively as an output property
	outputKind propKind = "output"
	// If the type is used as an input and as an output property
	inputOutputKind propKind = "input&output"
)

type schemaVisitor interface {
	visitResource(token string, res schema.ResourceSpec)
	visitFunction(token string, fun schema.FunctionSpec)
	visitType(token string, typ schema.ComplexTypeSpec, kind propKind)
	visitProperty(prop schema.PropertySpec, kind propKind)
}

type kindMap map[string]propKind

const typePrefix = "#/types/"

func (k kindMap) declareOutput(token string) bool {
	token = strings.TrimPrefix(token, typePrefix)
	switch k[token] {
	case unknownKind:
		k[token] = outputKind
	case inputKind:
		k[token] = inputOutputKind
	}
}

func (k kindMap) declareInput(token string) bool {
	token = strings.TrimPrefix(token, typePrefix)
	switch k[token] {
	case unknownKind:
		k[token] = inputKind
	case outputKind:
		k[token] = inputOutputKind
	}
}

func (k kindMap) kindOf(token string) propKind {
	token = strings.TrimPrefix(token, typePrefix)
	return k[token]
}

func visitSchema(pulumiSchema schema.PackageSpec, visitor schemaVisitor) {
	var typeKind kindMap = make(map[string]propKind, len(pulumiSchema.Types))

	visitProp := func(prop schema.PropertySpec, kind propKind) {
		visitor.visitProperty(prop, kind)

		decl := func(string) bool { return false }
		switch kind {
		case inputKind:
			decl = typeKind.declareInput
		case outputKind:
			decl = typeKind.declareOutput
		}

		if prop.Ref != "" {
			decl(prop.Ref)
		}
		if items := prop.Items; items != nil {
			if items.Ref != "" {
				decl(items.Ref)
			}
		}
		if addnl := prop.AdditionalProperties; addnl != nil {
			if addnl.Ref != "" {
				decl(addnl.Ref)
			}
		}
	}

	for tk, r := range pulumiSchema.Resources {
		visitor.visitResource(tk, r)
		for _, prop := range r.InputProperties {
			visitProp(prop, inputKind)
		}
		for _, prop := range r.Properties {
			visitProp(prop, outputKind)
		}
	}
	for tk, r := range pulumiSchema.Functions {
		visitor.visitFunction(tk, r)
		if r.Inputs != nil {
			for _, prop := range r.Inputs.Properties {
				visitProp(prop, inputKind)
			}
		}
		if r.Outputs != nil {
			for _, prop := range r.Outputs.Properties {
				visitProp(prop, outputKind)
			}
		}
	}
}

type v2Builder struct {
	stats v2.Stats
}

func (b *v2Builder) visitResource(token string, res schema.ResourceSpec)               {}
func (b *v2Builder) visitFunction(token string, fun schema.FunctionSpec)               {}
func (b *v2Builder) visitType(token string, typ schema.ComplexTypeSpec, kind propKind) {}
func (b *v2Builder) visitProperty(prop schema.PropertySpec, kind propKind)             {}
