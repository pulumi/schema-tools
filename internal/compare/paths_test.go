package compare

import (
	"testing"

	"github.com/pulumi/schema-tools/internal/util/diagtree"
)

func TestNodePathAndEntry(t *testing.T) {
	root := &diagtree.Node{}
	leaf := root.Label("Resources").Value("pkg:index:Res").Label("inputs").Value("name")
	leaf.SetDescription(diagtree.Warn, "missing")

	path := nodePath(leaf)
	if path != `Resources: "pkg:index:Res": inputs: "name"` {
		t.Fatalf("unexpected path: %q", path)
	}
	entry := nodeEntry(leaf)
	if entry != `Resources: "pkg:index:Res": inputs: "name" missing` {
		t.Fatalf("unexpected entry: %q", entry)
	}
}
