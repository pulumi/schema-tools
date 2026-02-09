package compare

import "github.com/pulumi/schema-tools/internal/util/diagtree"

type Report struct {
	Violations   *diagtree.Node
	NewResources []string
	NewFunctions []string
}
