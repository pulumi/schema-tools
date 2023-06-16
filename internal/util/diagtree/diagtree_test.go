package diagtree_test

import (
	"bytes"
	"testing"

	"github.com/pulumi/schema-tools/internal/util/diagtree"
	"github.com/stretchr/testify/assert"
)

func TestPrunedDisplay(t *testing.T) {
	t.Parallel()
	n := func(f func(*diagtree.Node)) *diagtree.Node {
		n := &diagtree.Node{Title: "Top Level"}
		f(n)
		n.Prune()
		return n
	}
	nn := func(f func(*diagtree.Node)) *diagtree.Node {
		return n(func(n *diagtree.Node) {
			f(n.Label("l1").Label("l2"))
		})
	}

	tests := []testCase{
		{
			input: n(func(n *diagtree.Node) {
				n.SetDescription(diagtree.Info, "A top level value (%d)", 1)
			}),
			expected:      "### `🟢` Top Level A top level value (1)\n",
			maxItems:      10,
			expectedCount: 1,
		},
		{
			input: nn(func(n *diagtree.Node) {
				n = n.Label("l3")
				n.SetDescription(diagtree.Info, "A nested value")
			}),
			expected:      "### Top Level\n#### l1\n- `🟢` l2: l3 A nested value\n",
			maxItems:      10,
			expectedCount: 1,
		},
		{
			input: nn(func(n *diagtree.Node) {
				n.SetDescription(diagtree.Info, "nested descriptions")
				n.Label("value1").SetDescription(diagtree.Warn, "warn")
				n.Label("value2").SetDescription(diagtree.Danger, "danger")
			}),
			expected:      "### Top Level\n#### l1\n- `🟢` l2 nested descriptions:\n    - `🟡` value1 warn\n    - `🔴` value2 danger\n",
			maxItems:      10,
			expectedCount: 3,
		},
		{
			input: nn(func(n *diagtree.Node) {
				n.Label("no data").Value("still nothing")
				bt := n.Label("branching tree")
				bt.Label("empty branch")
				bt.Value("has description").SetDescription(diagtree.Info, "a description")
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
			tt.check(t)
		})
	}
}

type testCase struct {
	input *diagtree.Node

	expected      string
	expectedCount int
	maxItems      int
}

func (tt testCase) check(t *testing.T) {
	actual := new(bytes.Buffer)
	actualCount := tt.input.Display(actual, tt.maxItems)
	assert.Equal(t, tt.expectedCount, actualCount)
	assert.Equal(t, tt.expected, actual.String())
}
