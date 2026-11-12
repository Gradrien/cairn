package state

import (
	"regexp"
	"strings"
)

// The plan is written by agents, so it is read leniently, as an outline: a
// list item owns every line under it that is indented more than it is (its
// sub-items, the paragraphs after a blank line, its code blocks) and every
// line that continues its text. Nothing under an item is dropped or turned
// into an item of its own.
//
// Strict CommonMark is not used on purpose: it only nests a sub-item when it
// is indented to its parent's text column, so "1. Question ?\n  - (a) …"
// gives two separate lists, and the options would become questions.

// node is an item ("- text", "1. text", "(a) text") or a paragraph of a section.
type node struct {
	Text   string   // the text, continuation lines joined with a space
	Lines  []string // the text's lines, trimmed
	Line   int      // body line of its first line
	Indent int
	Marker string // "-", "*", "+", "1.", "a)", "(a)"; "" for a paragraph
	Box    byte   // ' ', 'x', 'X', '~' or '-' for "[ ]" items, 0 otherwise
	Code   bool   // a line of a fenced code block
	Kids   []*node
}

func (n *node) item() bool { return n.Marker != "" }

// plain is a paragraph: shown without a bullet.
func (n *node) plain() bool { return !n.item() && !n.Code }

// ordered reports a numbered or lettered marker, which the text keeps.
func (n *node) ordered() bool { return n.item() && !strings.ContainsAny(n.Marker[:1], "-*+") }

// label is the text as shown: an ordered marker stays in front.
func (n *node) label() string {
	if n.ordered() {
		return n.Marker + " " + n.Text
	}
	return n.Text
}

func (n *node) done() bool  { return n.Box == 'x' || n.Box == 'X' }
func (n *node) doing() bool { return n.Box == '~' || n.Box == '-' }

var (
	outItem  = regexp.MustCompile(`^(\s*)([-*+]|\d{1,3}[.)]|[a-zA-Z][.)]|\([a-zA-Z0-9]{1,2}\))\s+(.*)$`)
	outBox   = regexp.MustCompile(`^\[([ xX~-])\]\s+(.*)$`)
	outFence = regexp.MustCompile("^\\s*(```|~~~)")
)

func indentOf(l string) int {
	n := 0
	for _, r := range l {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n
		}
	}
	return n
}

// outline reads the lines of a section (first is the body line of lines[0]).
func outline(lines []string, first int) []*node {
	var roots []*node
	var stack []*node // open items, by increasing indent
	var cur *node     // the text a next line continues, nil after a blank
	fence := ""
	attach := func(n *node) {
		for len(stack) > 0 && stack[len(stack)-1].Indent >= n.Indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			p := stack[len(stack)-1]
			p.Kids = append(p.Kids, n)
		}
	}
	for i, l := range lines {
		at := first + i
		t := strings.TrimSpace(l)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
				continue
			}
			n := &node{Text: strings.TrimRight(l, " "), Lines: []string{l}, Line: at, Indent: indentOf(l), Code: true}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.Kids = append(p.Kids, n)
			} else {
				roots = append(roots, n)
			}
			continue
		}
		if m := outFence.FindStringSubmatch(l); m != nil {
			fence, cur = m[1], nil
			continue
		}
		if t == "" {
			cur = nil
			continue
		}
		if m := outItem.FindStringSubmatch(l); m != nil {
			text := strings.TrimSpace(m[3])
			n := &node{Line: at, Indent: indentOf(m[1]), Marker: m[2]}
			if b := outBox.FindStringSubmatch(text); b != nil {
				n.Box, text = b[1][0], strings.TrimSpace(b[2])
			}
			n.Text, n.Lines = text, []string{text}
			attach(n)
			stack = append(stack, n)
			cur = n
			continue
		}
		if cur != nil {
			// a line right under a text continues it, whatever its indent
			cur.Text += " " + t
			cur.Lines = append(cur.Lines, t)
			continue
		}
		n := &node{Text: t, Lines: []string{t}, Line: at, Indent: indentOf(l)}
		for len(stack) > 0 && stack[len(stack)-1].Indent >= n.Indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			p := stack[len(stack)-1]
			p.Kids = append(p.Kids, n)
		}
		cur = n
	}
	return roots
}

// body flattens what sits under an item into display items, the ones with
// a box excepted (they are tasks of their own).
func body(kids []*node, depth int) []Item {
	var out []Item
	for _, k := range kids {
		if k.Box != 0 {
			continue
		}
		out = append(out, Item{Text: k.label(), Depth: min(depth, 2), Plain: k.plain(), Code: k.Code})
		out = append(out, body(k.Kids, depth+1)...)
	}
	return out
}

// boxed calls f for every item with a box, at any depth, in order.
func boxed(ns []*node, f func(*node)) {
	for _, n := range ns {
		if n.Box != 0 {
			f(n)
		}
		boxed(n.Kids, f)
	}
}
