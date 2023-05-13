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

	subfields []*Node
	doDisplay bool
	parent    *Node
}

func (m *Node) subfield(name string) *Node {
	contract.Assertf(name != "", "we cannot display an empty name")
	for _, v := range m.subfields {
		if v.Title == name {
			return v
		}
	}
	v := &Node{
		Title:  name,
		parent: m,
	}
	m.subfields = append(m.subfields, v)
	return v
}

func (m *Node) Label(name string) *Node {
	return m.subfield(name)
}

func (m *Node) Value(value string) *Node {
	return m.subfield(fmt.Sprintf("%q", value))
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

func (m *Node) Display(out io.Writer, max int) int {
	return m.display(out, 0, true, max)
}

func (m *Node) display(out io.Writer, level int, prefix bool, max int) int {
	write := func(s string) {
		_, err := out.Write([]byte(s))
		contract.AssertNoErrorf(err, "failed to write display")
	}
	if m == nil || !m.doDisplay || max <= 0 {
		// Nothing to display
		return 0
	}

	var displayed int
	var display string
	if m.Title != "" {
		if prefix {
			display = m.levelPrefix(level) + m.severity()
		}
		display += m.Title
		if m.Description != "" {
			displayed += 1
			display += " " + m.Description
		}

		write(display)
	}

	if level > 1 && m.Severity == None {
		if s := m.uniqueSuccessor(); s != nil {
			write(": ")
			return s.display(out, level, false, max-displayed) + displayed
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
		n := m.subfields[i].display(out, level+1, true, max-displayed)
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
			if m.Severity == "" {
				return ""
			}
			return string(m.Severity) + " "
		}
		m = s
	}
	return ""
}

// The severity of a node.
//
// Nodes with their own (non-None) severity are always displayed on their own level.
type Severity string

const (
	None   Severity = ""
	Info   Severity = "`🟢`"
	Warn   Severity = "`🟡`"
	Danger Severity = "`🔴`"
)

func (m *Node) SetDescription(level Severity, msg string, a ...any) {
	for v := m.parent; v != nil && !v.doDisplay; v = v.parent {
		v.doDisplay = true
	}
	m.doDisplay = true
	m.Description = fmt.Sprintf(msg, a...)
	m.Severity = level
}
