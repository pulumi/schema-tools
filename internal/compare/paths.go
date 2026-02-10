package compare

import (
	"strings"

	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

func NodePath(node *diagtree.Node) string {
	if node == nil {
		return ""
	}
	return strings.Join(node.PathTitles(), ": ")
}

func NodeEntry(node *diagtree.Node) string {
	if node == nil {
		return ""
	}
	path := NodePath(node)
	if node.Description == "" {
		return path
	}
	if path == "" {
		return node.Description
	}
	return path + " " + node.Description
}
