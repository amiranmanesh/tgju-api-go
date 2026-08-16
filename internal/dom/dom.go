// Package dom is a thin query layer over golang.org/x/net/html.
//
// It exists so the scraper can be written in terms of "the first th of this
// row" instead of hand rolled tree walks, without pulling in a full CSS
// selector engine. The API is deliberately tiny: find an element, read an
// attribute, read the text.
package dom

import (
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Node is an element of a parsed document. It is an alias rather than a wrapper
// so that this package adds a vocabulary without adding a layer, and so that
// callers never have to import golang.org/x/net/html themselves.
type Node = html.Node

// Parse reads an HTML document and returns its root node.
func Parse(r io.Reader) (*Node, error) { return html.Parse(r) }

// Matcher reports whether a node is interesting. [Find] and [FindAll] walk the
// tree in document order and hand every element node to it.
type Matcher func(*Node) bool

// Tag matches element nodes with the given tag name.
func Tag(name string) Matcher {
	a := atom.Lookup([]byte(name))
	return func(n *Node) bool {
		if a != 0 {
			return n.DataAtom == a
		}
		return n.Data == name
	}
}

// TagClass matches element nodes with the given tag name that also carry the
// given class. An empty tag name matches any element.
func TagClass(name, class string) Matcher {
	tag := Tag(name)
	return func(n *Node) bool {
		if name != "" && !tag(n) {
			return false
		}
		return HasClass(n, class)
	}
}

// Find returns the first descendant of n matching m, or nil.
func Find(n *Node, m Matcher) *Node {
	var found *Node
	walk(n, func(c *Node) bool {
		if m(c) {
			found = c
			return false // stop
		}
		return true
	})
	return found
}

// FindAll returns every descendant of n matching m, in document order.
//
// The walk does not descend into a node that already matched, so nested tables
// are reported once as the outer table rather than twice.
func FindAll(n *Node, m Matcher) []*Node {
	var out []*Node
	var visit func(*Node)
	visit = func(cur *Node) {
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && m(c) {
				out = append(out, c)
				continue
			}
			visit(c)
		}
	}
	visit(n)
	return out
}

// Children returns the direct element children of n matching m.
func Children(n *Node, m Matcher) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && m(c) {
			out = append(out, c)
		}
	}
	return out
}

// Attr returns the value of the named attribute, or "" when it is absent.
func Attr(n *Node, name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// HasClass reports whether n carries the given class token.
func HasClass(n *Node, class string) bool {
	if n == nil {
		return false
	}
	for _, c := range strings.Fields(Attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// Classes returns the class tokens of n.
func Classes(n *Node) []string { return strings.Fields(Attr(n, "class")) }

// Text returns the concatenated text of n and its descendants, with runs of
// whitespace collapsed into single spaces and the result trimmed.
//
// An element boundary counts as whitespace, so the two header cells of a row
// read as "عنوان قیمت زنده" rather than running into each other. Numbers split
// across inline tags survive this: the separators are stripped again by the
// number parser.
func Text(n *Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var visit func(*Node)
	visit = func(cur *Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
			return
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				b.WriteByte(' ')
			}
			visit(c)
			if c.Type == html.ElementNode {
				b.WriteByte(' ')
			}
		}
	}
	visit(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// walk visits every element descendant of n in document order until fn returns
// false.
func walk(n *Node, fn func(*Node) bool) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !fn(c) {
			return false
		}
		if !walk(c, fn) {
			return false
		}
	}
	return true
}
