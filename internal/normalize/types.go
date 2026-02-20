package normalize

import "errors"

var (
	// ErrMetadataRequired indicates normalization metadata is missing.
	ErrMetadataRequired = errors.New("metadata required")
	// ErrMetadataInvalid indicates metadata payload shape/content is malformed.
	ErrMetadataInvalid = errors.New("metadata invalid")
	// ErrMetadataVersionUnsupported indicates a known-but-unsupported metadata version.
	ErrMetadataVersionUnsupported = errors.New("metadata version unsupported")
)

// SupportedAutoAliasingVersion is the highest metadata auto-aliasing version
// currently understood by this normalizer.
const SupportedAutoAliasingVersion = 1

const (
	// scopeResources identifies resource token maps in normalization logic.
	scopeResources = "resources"
	// scopeDataSources identifies datasource/function token maps.
	scopeDataSources = "datasources"
)

// MetadataEnvelope models bridge-metadata.json fields needed by normalization.
type MetadataEnvelope struct {
	AutoAliasing *AutoAliasing `json:"auto-aliasing,omitempty"`
}

// TokenRename captures a token rename discovered by metadata normalization.
type TokenRename struct {
	Scope    string
	OldToken string
	NewToken string
}

// MaxItemsOneChange captures a metadata-driven maxItemsOne transition.
type MaxItemsOneChange struct {
	Scope    string
	Token    string
	Location string
	Field    string
	OldType  string
	NewType  string
}

// AutoAliasing mirrors the bridge auto-aliasing payload.
type AutoAliasing struct {
	Version     *int                     `json:"version,omitempty"`
	Resources   map[string]*TokenHistory `json:"resources,omitempty"`
	Datasources map[string]*TokenHistory `json:"datasources,omitempty"`
}

// TokenHistory tracks current/past token names and field history for a TF token.
type TokenHistory struct {
	Current      string                   `json:"current"`
	Past         []TokenAlias             `json:"past,omitempty"`
	MajorVersion int                      `json:"majorVersion,omitempty"`
	Fields       map[string]*FieldHistory `json:"fields,omitempty"`
}

// TokenAlias records one historic Pulumi token for a Terraform token.
type TokenAlias struct {
	Name         string `json:"name"`
	InCodegen    bool   `json:"inCodegen"`
	MajorVersion int    `json:"majorVersion"`
}

// FieldHistory stores recursive maxItemsOne history for fields and element blocks.
type FieldHistory struct {
	MaxItemsOne *bool                    `json:"maxItemsOne,omitempty"`
	Fields      map[string]*FieldHistory `json:"fields,omitempty"`
	Elem        *FieldHistory            `json:"elem,omitempty"`
}
