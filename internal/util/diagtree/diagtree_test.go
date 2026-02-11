package diagtree

import (
	"bytes"
	"testing"
)

func TestPrunedDisplay(t *testing.T) {
	t.Parallel()
	n := func(f func(*Node)) *Node {
		n := &Node{Title: "Top Level"}
		f(n)
		n.Prune()
		return n
	}
	nn := func(f func(*Node)) *Node {
		return n(func(n *Node) {
			f(n.Label("l1").Label("l2"))
		})
	}

	tests := []struct {
		input         *Node
		expected      string
		expectedCount int
		maxItems      int
	}{
		{
			input: n(func(n *Node) {
				n.SetDescription(Info, "A top level value (%d)", 1)
			}),
			expected:      "### `🟢` Top Level A top level value (1)\n",
			maxItems:      10,
			expectedCount: 1,
		},
		{
			input: nn(func(n *Node) {
				n = n.Label("l3")
				n.SetDescription(Info, "A nested value")
			}),
			expected:      "### Top Level\n#### l1\n- `🟢` l2: l3 A nested value\n",
			maxItems:      10,
			expectedCount: 1,
		},
		{
			input: nn(func(n *Node) {
				n.SetDescription(Info, "nested descriptions")
				n.Label("value1").SetDescription(Warn, "warn")
				n.Label("value2").SetDescription(Danger, "danger")
			}),
			expected:      "### Top Level\n#### l1\n- `🟢` l2 nested descriptions:\n    - `🟡` value1 warn\n    - `🔴` value2 danger\n",
			maxItems:      10,
			expectedCount: 3,
		},
		{
			input: nn(func(n *Node) {
				n.Label("no data").Value("still nothing")
				bt := n.Label("branching tree")
				bt.Label("empty branch")
				bt.Value("has description").SetDescription(Info, "a description")
				n.Label("another top level branch")
			}),
			expected:      "### Top Level\n#### l1\n- `🟢` l2: branching tree: \"has description\" a description\n",
			maxItems:      10,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()
			actual := new(bytes.Buffer)
			actualCount := tt.input.Display(actual, tt.maxItems)
			if actualCount != tt.expectedCount {
				t.Fatalf("displayed count: got %d, want %d", actualCount, tt.expectedCount)
			}
			if actual.String() != tt.expected {
				t.Fatalf("display output mismatch:\n got: %q\nwant: %q", actual.String(), tt.expected)
			}
		})
	}
}

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
