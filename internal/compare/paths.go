package compare

import (
	"strings"

	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

func nodePath(node *diagtree.Node) string {
	if node == nil {
		return ""
	}
	return strings.Join(node.PathTitles(), ": ")
}

func nodeEntry(node *diagtree.Node) string {
	if node == nil {
		return ""
	}
	path := nodePath(node)
	if node.Description == "" {
		return path
	}
	if path == "" {
		return node.Description
	}
	return path + " " + node.Description
}
