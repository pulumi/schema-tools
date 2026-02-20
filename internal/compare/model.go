package compare

import "github.com/pulumi/schema-tools/internal/util/diagtree"

// Diagnostic is a flattened engine violation entry.
type Diagnostic struct {
	Scope       string
	Token       string
	Location    string
	Path        string
	Description string
}

// Report captures the compare engine output used by text and JSON renderers.
type Report struct {
	Violations   *diagtree.Node
	Diagnostics  []Diagnostic
	NewResources []string
	NewFunctions []string
}
