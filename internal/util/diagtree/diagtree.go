package diagtree

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type Node struct {
	Title       string
	Description string
	Severity    Severity

	subfields       []*Node
	subfieldByTitle map[string]*Node
	doDisplay       bool
	parent          *Node
}

func (m *Node) subfield(name string) *Node {
	contract.Assertf(name != "", "we cannot display an empty name")
	if m.subfieldByTitle != nil {
		if v, ok := m.subfieldByTitle[name]; ok {
			return v
		}
	}
	v := &Node{
		Title:  name,
		parent: m,
	}
	if m.subfieldByTitle == nil {
		m.subfieldByTitle = map[string]*Node{}
	}
	m.subfieldByTitle[name] = v
	m.subfields = append(m.subfields, v)
	return v
}

func (m *Node) Label(name string) *Node {
	return m.subfield(name)
}

func (m *Node) Value(value string) *Node {
	return m.subfield(fmt.Sprintf("%q", value))
}

func (m *Node) PathTitles() []string {
	if m == nil {
		return nil
	}

	parts := []string{}
	for n := m; n != nil; n = n.parent {
		if n.Title == "" {
			continue
		}
		parts = append(parts, n.Title)
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return parts
}

func (m *Node) WalkDisplayed(visit func(*Node)) {
	if m == nil || visit == nil {
		return
	}
	m.walkDisplayed(visit)
}

func (m *Node) walkDisplayed(visit func(*Node)) {
	if m == nil || !m.doDisplay {
		return
	}
	visit(m)
	for _, child := range m.subfields {
		child.walkDisplayed(visit)
	}
}

func (m *Node) Prune() {
	sfs := []*Node{}
	for _, v := range m.subfields {
		if !v.doDisplay {
			continue
		}
		sfs = append(sfs, v)
		v.Prune()
	}
	if len(sfs) == 0 {
		sfs = nil
	}
	m.subfields = sfs
	if len(sfs) == 0 {
		m.subfieldByTitle = nil
		return
	}
	m.subfieldByTitle = make(map[string]*Node, len(sfs))
	for _, child := range sfs {
		m.subfieldByTitle[child.Title] = child
	}
}

func (m *Node) levelPrefix(level int) string {
	switch level {
	case 0:
		return "### "
	case 1:
		return "#### "
	}
	if level < 0 {
		return ""
	}
	return strings.Repeat("  ", (level-2)*2) + "- "
}

type cappedWriter struct {
	// The number of remaining writes before we hit the cap.
	remaining int
	out       io.Writer
}

func (c *cappedWriter) incr() {
	if c.remaining > 0 {
		// We never step past 0, because -1 indicates that we should always print
		c.remaining--
	}
}

func (c *cappedWriter) Write(p []byte) (n int, err error) {
	if c.remaining > 0 || c.remaining == -1 {
		return c.out.Write(p)
	}
	// We pretend we finished the write, but we do nothing.
	return len(p), nil
}

func (m *Node) Display(out io.Writer, max int) int {
	writer := &cappedWriter{max, out}
	return m.display(writer, 0, true)
}

func (m *Node) display(out *cappedWriter, level int, prefix bool) int {
	write := func(s string) {
		_, err := out.Write([]byte(s))
		contract.AssertNoErrorf(err, "failed to write display")
	}
	if m == nil || !m.doDisplay {
		// Nothing to display
		return 0
	}

	var displayed int
	var display string
	if m.Title != "" {
		if prefix {
			display = m.levelPrefix(level)
			// levels 0 & 1 are always top level, so we special case them
			// here.
			if level > 1 || m.Severity != None {
				display += m.severity()
			}
		}
		display += m.Title
		if m.Description != "" {
			displayed += 1
			display += " " + m.Description
		}

		write(display)
		out.incr()
	}

	if level > 1 && m.Severity == None {
		if s := m.uniqueSuccessor(); s != nil {
			write(": ")
			return s.display(out, level, false) + displayed
		}
	}

	order := make([]int, len(m.subfields))
	for i := range order {
		order[i] = i
	}
	if level > 0 {
		// Obtain an ordering on the subfields without mutating `.Subfields`.
		sort.Slice(order, func(i, j int) bool {
			return m.subfields[order[i]].Title < m.subfields[order[j]].Title
		})
	}

	var didEndLine bool
	for _, i := range order {
		if m.subfields[i].doDisplay && !didEndLine {
			if level > 1 {
				write(":\n")
			} else {
				write("\n")
			}
			didEndLine = true
		}
		n := m.subfields[i].display(out, level+1, true)
		displayed += n
	}

	if !didEndLine {
		write("\n")
	}

	return displayed
}

// Find the unique successor node for m.
//
// If there is no successor or if there are multiple successors, nil is returned.
func (m *Node) uniqueSuccessor() *Node {
	var us *Node
	for _, s := range m.subfields {
		if !s.doDisplay {
			continue
		}
		if us != nil {
			return nil
		}
		us = s
	}
	return us
}

// Get the string to display the severity of a node.
//
// If a node has a unique successor, it's severity is used. This is applied recursively.
func (m *Node) severity() string {
	for m != nil {
		s := m.uniqueSuccessor()
		if s == nil {
			if m.Severity == None {
				return ""
			}
			return m.Severity.String() + " "
		}
		m = s
	}
	return ""
}

// The severity of a node.
//
// Nodes with their own (non-None) severity are always displayed on their own level.
type Severity struct{ s string }

var (
	None   = Severity{""}
	Info   = Severity{"`🟢`"}
	Warn   = Severity{"`🟡`"}
	Danger = Severity{"`🔴`"}
)

func (s Severity) String() string {
	return s.s
}

func (m *Node) SetDescription(level Severity, msg string, a ...any) {
	for v := m; v != nil && !v.doDisplay; v = v.parent {
		v.doDisplay = true
	}
	m.Description = fmt.Sprintf(msg, a...)
	m.Severity = level
}
