package diagtree

import "testing"

func TestPathTitles(t *testing.T) {
	root := &Node{}
	leaf := root.Label("Resources").Value("pkg:index:Res").Label("inputs").Value("name")
	leaf.SetDescription(Warn, "missing")

	parts := leaf.PathTitles()
	want := []string{"Resources", `"pkg:index:Res"`, "inputs", `"name"`}
	if len(parts) != len(want) {
		t.Fatalf("unexpected length %d (%v)", len(parts), parts)
	}
	for i := range parts {
		if parts[i] != want[i] {
			t.Fatalf("unexpected part[%d]=%q want %q (%v)", i, parts[i], want[i], parts)
		}
	}
}

func TestWalkDisplayed(t *testing.T) {
	root := &Node{}
	shown := root.Label("Resources").Value("r1")
	shown.SetDescription(Danger, "missing")
	root.Label("Resources").Value("r2")

	count := 0
	root.WalkDisplayed(func(n *Node) {
		if n.Title != "" {
			count++
		}
	})

	if count != 2 {
		t.Fatalf("expected 2 displayed titled nodes, got %d", count)
	}
}
