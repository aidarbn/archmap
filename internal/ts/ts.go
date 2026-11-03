// Package ts is the shared tree-sitter layer: language setup, parsing and small
// traversal helpers used by the language parsers.
//
// Library: github.com/tree-sitter/go-tree-sitter (official cgo bindings).
// Grammars: official tree-sitter-{python,go,javascript,typescript}, plus
// tree-sitter-grammars/tree-sitter-kotlin and alex-pinkus/tree-sitter-swift.
package ts

import (
	"fmt"
	"sync"
	"unsafe"

	swift "github.com/alex-pinkus/tree-sitter-swift/bindings/go"
	kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Lang identifies a grammar.
type Lang string

// Supported grammars.
const (
	Python     Lang = "python"
	Go         Lang = "go"
	JavaScript Lang = "javascript" // also JSX
	TypeScript Lang = "typescript"
	TSX        Lang = "tsx"
	Swift      Lang = "swift"
	Kotlin     Lang = "kotlin"
)

// All lists every supported grammar.
var All = []Lang{Python, Go, JavaScript, TypeScript, TSX, Swift, Kotlin}

var (
	mu    sync.Mutex
	cache = map[Lang]*sitter.Language{}
)

// Language returns the (cached) grammar for l.
func Language(l Lang) (*sitter.Language, error) {
	mu.Lock()
	defer mu.Unlock()
	if v, ok := cache[l]; ok {
		return v, nil
	}
	var p unsafe.Pointer
	switch l {
	case Python:
		p = python.Language()
	case Go:
		p = golang.Language()
	case JavaScript:
		p = javascript.Language()
	case TypeScript:
		p = typescript.LanguageTypescript()
	case TSX:
		p = typescript.LanguageTSX()
	case Swift:
		p = swift.Language()
	case Kotlin:
		p = kotlin.Language()
	default:
		return nil, fmt.Errorf("ts: unknown language %q", l)
	}
	v := sitter.NewLanguage(p)
	cache[l] = v
	return v, nil
}

// Tree is a parsed file together with its source, so nodes can be read as text.
type Tree struct {
	*sitter.Tree
	Src []byte
}

// Parse parses src with the grammar for l. The caller must Close the tree.
// Tree-sitter recovers from syntax errors; the returned tree may contain ERROR nodes.
func Parse(l Lang, src []byte) (*Tree, error) {
	lg, err := Language(l)
	if err != nil {
		return nil, err
	}
	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(lg); err != nil {
		return nil, fmt.Errorf("ts: set language %s: %w", l, err)
	}
	t := p.Parse(src, nil)
	if t == nil {
		return nil, fmt.Errorf("ts: parse %s failed", l)
	}
	return &Tree{Tree: t, Src: src}, nil
}

// Root returns the root node.
func (t *Tree) Root() *sitter.Node { return t.RootNode() }

// Text returns the source text of n ("" for nil).
func (t *Tree) Text(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	return string(t.Src[n.StartByte():n.EndByte()])
}

// OwnLine reports that nothing but whitespace precedes n on its first line, so a
// comment node is a comment of its own and not the tail of another statement
// (`#[cfg(unix)] // note`, `const x = 1; // note`).
func OwnLine(src []byte, n *sitter.Node) bool {
	for i := int(n.StartByte()) - 1; i >= 0 && src[i] != '\n'; i-- {
		if src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
			return false
		}
	}
	return true
}

// LastRow is the 0-based row holding the last character of n; unlike
// EndPosition it ignores a trailing newline some grammars put inside comment nodes.
func LastRow(src []byte, n *sitter.Node) uint {
	row := n.EndPosition().Row
	if e := n.EndByte(); e > n.StartByte() && src[e-1] == '\n' && row > n.StartPosition().Row {
		row--
	}
	return row
}

// FieldText returns the text of the named field child of n ("" if absent).
func (t *Tree) FieldText(n *sitter.Node, field string) string {
	return t.Text(n.ChildByFieldName(field))
}

// StartLine is the 1-based first line of n.
func StartLine(n *sitter.Node) int { return int(n.StartPosition().Row) + 1 }

// EndLine is the 1-based last line of n.
func EndLine(n *sitter.Node) int { return int(n.EndPosition().Row) + 1 }

// Children returns all (named and anonymous) children of n.
func Children(n *sitter.Node) []*sitter.Node {
	out := make([]*sitter.Node, 0, n.ChildCount())
	for i := uint(0); i < n.ChildCount(); i++ {
		out = append(out, n.Child(i))
	}
	return out
}

// NamedChildren returns the named children of n.
func NamedChildren(n *sitter.Node) []*sitter.Node {
	out := make([]*sitter.Node, 0, n.NamedChildCount())
	for i := uint(0); i < n.NamedChildCount(); i++ {
		out = append(out, n.NamedChild(i))
	}
	return out
}

// Walk visits n and its descendants in source order (pre-order). If fn returns
// false the node's children are skipped.
func Walk(n *sitter.Node, fn func(*sitter.Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		Walk(n.Child(i), fn)
	}
}
