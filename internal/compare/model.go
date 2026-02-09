package compare

import "github.com/pulumi/schema-tools/internal/util/diagtree"

// Report captures the compare engine output used by text and JSON renderers.
type Report struct {
	Violations   *diagtree.Node
	NewResources []string
	NewFunctions []string
}
