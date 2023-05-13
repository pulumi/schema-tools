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
	Subfields   []*Node

	doDisplay bool
	parent    *Node
}

func (m *Node) subfield(name string) *Node {
	contract.Assertf(name != "", "we cannot display an empty name")
	for _, v := range m.Subfields {
		if v.Title == name {
			return v
		}
	}
	v := &Node{
		Title:  name,
		parent: m,
	}
	m.Subfields = append(m.Subfields, v)
	return v
}

func (m *Node) Label(name string) *Node {
	return m.subfield(name)
}

func (m *Node) Value(value string) *Node {
	return m.subfield(fmt.Sprintf("%q", value))
}

func (m *Node) levelPrefix(level int) string {
	switch level {
	case 0:
		return "###"
	case 1:
		return "####"
	}
	if level < 0 {
		return ""
	}
	return strings.Repeat("  ", (level-2)*2) + "-"
}

func (m *Node) Display(out io.Writer, max int) int {
	return m.display(out, 0, true, max)
}

func (m *Node) display(out io.Writer, level int, prefix bool, max int) int {
	if m == nil || !m.doDisplay || max <= 0 {
		// Nothing to display
		return 0
	}

	var displayed int
	var display string
	if m.Title != "" {
		if prefix {
			display = fmt.Sprintf("%s %s ",
				m.levelPrefix(level),
				m.severity())
		}
		display += m.Title + ": "
		if m.Description != "" {
			displayed += 1
			display += m.Description
		}

		out.Write([]byte(display))
	}

	if level > 1 && m.Severity == None {
		if s := m.uniqueSuccessor(); s != nil {
			return s.display(out, level, false, max-displayed) + displayed
		}
	}

	out.Write([]byte{'\n'})

	order := make([]int, len(m.Subfields))
	for i := range order {
		order[i] = i
	}
	if level > 0 {
		// Obtain an ordering on the subfields without mutating `.Subfields`.
		sort.Slice(order, func(i, j int) bool {
			return m.Subfields[order[i]].Title < m.Subfields[order[j]].Title
		})
	}

	for _, f := range order {
		n := m.Subfields[f].display(out, level+1, true, max-displayed)
		displayed += n
	}
	return displayed
}

func (m *Node) uniqueSuccessor() *Node {
	var us *Node
	for _, s := range m.Subfields {
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

func (m *Node) severity() Severity {
	for m != nil {
		s := m.uniqueSuccessor()
		if s == nil {
			return m.Severity
		}
		m = s
	}
	return None
}

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
