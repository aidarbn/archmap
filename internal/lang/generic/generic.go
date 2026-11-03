// Package generic is a language-agnostic parser driven by tree-sitter tags
// queries. A language is described by a Spec: name, extensions, grammar, a tags
// query and optionally an import query. The hand-written parsers (python, go,
// js, swift, kotlin) are more precise and stay; generic covers the rest.
//
// # Tags query contract (the standard tags.scm captures)
//
//   - @definition.function / @definition.method with @name: a Func. The captured
//     node is the whole definition (Start/End); the body is its "body" field (if
//     the grammar has none, the whole node).
//   - @definition.class / .interface / .module / .impl with @name: a container.
//     Func.Cls is the name of the innermost container whose range contains the
//     function (for Rust impl blocks: the implemented type).
//   - @reference.call with @name: a call. It belongs to the innermost Func whose
//     body contains it. Recv is the source text between the start of the call
//     (or of the name's parent node: member/field/scoped expression) and the
//     name, without the trailing ".", "::", "->" or "&."; "" for a bare call,
//     "?" when the receiver is not a plain dotted/scoped name; "::" becomes ".".
//   - @recv (optional, with @reference.call): the first node of the receiver
//     chain, for grammars where a call is a run of sibling nodes (Dart). The
//     receiver is then the source text from it to the name.
//   - @doc (optional) with the definition: the doc comment; otherwise the
//     comment block directly above the definition (attributes/annotations in
//     between are skipped).
//
// # Import query
//
// @import is the module path text, @import.relative a path relative to the
// file ("./" is prepended unless it starts with a dot), @import.mod a child
// module name ("self::<name>"), and a @wild capture in the same match appends
// ".*". Spec.ExpandImport, if set, turns one captured text into several paths.
package generic

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/aidarbn/archmap/internal/lang"
)

// Spec describes one language.
type Spec struct {
	Name       string
	Extensions []string
	// Grammar returns the C pointer of the tree-sitter language (the grammar
	// binding's Language()).
	Grammar func() unsafe.Pointer
	Tags    string // tags query source
	Imports string // import query source, may be empty
	// ExpandImport optionally maps a captured @import text to import paths.
	ExpandImport func(string) []string
	// SiblingBody, if set, is the node kind of a body that is the next sibling
	// of the definition node instead of its "body" field (Dart:
	// function_signature + function_body). The definition then spans both.
	SiblingBody string
}

// Parser implements lang.Parser for a Spec.
type Parser struct {
	spec Spec
	once sync.Once
	lg   *sitter.Language
	tags *sitter.Query
	imps *sitter.Query
	err  error
}

var _ lang.Parser = (*Parser)(nil)

// New returns a parser for the spec. Grammar and queries are compiled lazily on
// the first Parse.
func New(s Spec) *Parser { return &Parser{spec: s} }

// Name returns the language key.
func (p *Parser) Name() string { return p.spec.Name }

// Extensions lists handled file extensions.
func (p *Parser) Extensions() []string { return p.spec.Extensions }

func (p *Parser) init() error {
	p.once.Do(func() {
		p.lg = sitter.NewLanguage(p.spec.Grammar())
		var qe *sitter.QueryError
		p.tags, qe = sitter.NewQuery(p.lg, p.spec.Tags)
		if qe != nil {
			p.err = fmt.Errorf("generic %s: tags query: %s", p.spec.Name, qe.Message)
			return
		}
		if strings.TrimSpace(p.spec.Imports) != "" {
			p.imps, qe = sitter.NewQuery(p.lg, p.spec.Imports)
			if qe != nil {
				p.err = fmt.Errorf("generic %s: import query: %s", p.spec.Name, qe.Message)
			}
		}
	})
	return p.err
}

type container struct {
	name       string
	start, end uint
}

type def struct {
	node  *sitter.Node
	end   *sitter.Node // sibling body, when the definition spans it
	name  string
	doc   *sitter.Node
	body  *sitter.Node
	calls lang.CallList
}

type call struct {
	pos        uint
	name, recv string
}

// Parse implements lang.Parser.
func (p *Parser) Parse(_ string, src []byte) (lang.File, error) {
	if err := p.init(); err != nil {
		return lang.File{}, err
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(p.lg); err != nil {
		return lang.File{}, err
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return lang.File{}, fmt.Errorf("generic %s: parse failed", p.spec.Name)
	}
	defer tree.Close()
	root := tree.RootNode()
	text := func(n *sitter.Node) string { return string(src[n.StartByte():n.EndByte()]) }

	var (
		defs  []*def
		conts []container
		calls []call
		seen  = map[uint]*def{}
		heads = map[uint]bool{} // start of every definition name
	)
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	names := p.tags.CaptureNames()
	matches := qc.Matches(p.tags, root, src)
	for m := matches.Next(); m != nil; m = matches.Next() {
		var kind string
		var node, name, doc, recv *sitter.Node
		for i := range m.Captures {
			c := m.Captures[i]
			n := names[c.Index]
			nd := c.Node
			switch {
			case n == "name":
				name = &nd
			case n == "recv":
				recv = &nd
			case n == "doc":
				if doc == nil {
					doc = &nd
				}
			case strings.HasPrefix(n, "definition.") || n == "reference.call":
				kind, node = n, &nd
			}
		}
		if node == nil || name == nil || !m.SatisfiesTextPredicate(p.tags, nil, nil, src) {
			continue
		}
		switch kind {
		case "definition.function", "definition.method":
			if d := seen[node.StartByte()]; d != nil {
				if d.doc == nil {
					d.doc = doc
				}
				continue
			}
			d := &def{node: node, name: text(name), doc: doc, body: node.ChildByFieldName("body")}
			if d.body == nil && p.spec.SiblingBody != "" {
				if nx := node.NextNamedSibling(); nx != nil && nx.Kind() == p.spec.SiblingBody {
					d.body, d.end = nx, nx
				}
			}
			seen[node.StartByte()] = d
			heads[name.StartByte()] = true
			defs = append(defs, d)
		case "definition.class", "definition.interface", "definition.module", "definition.impl":
			conts = append(conts, container{text(name), node.StartByte(), node.EndByte()})
		case "reference.call":
			r := recvOf(src, node, name)
			if recv != nil && recv.StartByte() < name.StartByte() {
				r = normRecv(strings.TrimSpace(string(src[recv.StartByte():name.StartByte()])))
			}
			calls = append(calls, call{name.StartByte(), text(name), r})
		}
	}

	// calls -> innermost function whose body contains them
	sort.Slice(calls, func(i, j int) bool { return calls[i].pos < calls[j].pos })
	for _, c := range calls {
		if heads[c.pos] {
			continue // the name of a definition (e.g. an Elixir `def f(x)` head)
		}
		var best *def
		for _, d := range defs {
			s, e := d.bodyRange()
			if c.pos >= s && c.pos < e && (best == nil || bs(best) < bs(d)) {
				best = d
			}
		}
		if best != nil {
			best.calls.Add(c.name, c.recv)
		}
	}

	var out lang.File
	sort.SliceStable(defs, func(i, j int) bool { return defs[i].node.StartByte() < defs[j].node.StartByte() })
	for _, d := range defs {
		f := lang.Func{
			Name:  d.name,
			Start: int(d.node.StartPosition().Row) + 1,
			End:   int(d.endNode().EndPosition().Row) + 1,
			Sig:   sig(src, d),
			Doc:   doc(src, d),
			Calls: d.calls.Calls(),
		}
		if end := d.endNode().EndPosition(); end.Column == 0 && f.End > f.Start {
			f.End-- // node ends at the start of the next line
		}
		var cs, ce uint
		first := true
		for _, c := range conts {
			if c.start <= d.node.StartByte() && d.endNode().EndByte() <= c.end && (first || c.end-c.start < ce-cs) {
				f.Cls, cs, ce, first = c.name, c.start, c.end, false
			}
		}
		out.Funcs = append(out.Funcs, f)
	}
	if p.imps != nil {
		out.Imports = p.imports(root, src)
	}
	return out, nil
}

func (d *def) endNode() *sitter.Node {
	if d.end != nil {
		return d.end
	}
	return d.node
}

func bs(d *def) uint { s, _ := d.bodyRange(); return s }

func (d *def) bodyRange() (uint, uint) {
	if d.body != nil {
		return d.body.StartByte(), d.body.EndByte()
	}
	return d.node.StartByte(), d.endNode().EndByte()
}

var plainRecv = regexp.MustCompile(`^[A-Za-z_@$][\w@$]*(?:(?:\.|::|->|&\.|\?\.)[A-Za-z_][\w]*)*$`)

// recvOf computes the receiver text of a call: source between the start of the
// name's parent (or of the call node) and the name.
func recvOf(src []byte, call, name *sitter.Node) string {
	start := call.StartByte()
	if p := name.Parent(); p != nil && p.StartByte() >= start {
		start = p.StartByte()
	}
	if start >= name.StartByte() {
		return ""
	}
	return normRecv(strings.TrimSpace(string(src[start:name.StartByte()])))
}

// normRecv turns the source text before a call name into a receiver.
func normRecv(r string) string {
	for _, sep := range []string{"&.", "?.", "::", "->", "."} {
		if strings.HasSuffix(r, sep) {
			r = strings.TrimSpace(strings.TrimSuffix(r, sep))
			break
		}
	}
	if r == "" || r == "new" { // `new Foo()` is a bare call of Foo
		return ""
	}
	if !plainRecv.MatchString(r) {
		return "?"
	}
	if strings.HasPrefix(r, "$this") { // PHP
		r = r[1:]
	}
	r = strings.NewReplacer("::", ".", "->", ".", "&.", ".", "?.", ".").Replace(r)
	if r == "Self" {
		return "self"
	}
	return strings.TrimPrefix(r, "Self.")
}

var ws = regexp.MustCompile(`\s+`)

func clip(s string) string {
	s = strings.TrimSpace(ws.ReplaceAllString(s, " "))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func sig(src []byte, d *def) string {
	s, e := d.node.StartByte(), d.node.EndByte()
	if d.body != nil && d.body.StartByte() > s {
		e = d.body.StartByte()
	} else if i := strings.IndexByte(string(src[s:e]), '\n'); i >= 0 {
		e = s + uint(i)
	}
	return strings.TrimSpace(clip(string(src[s:e])))
}

var commentMark = regexp.MustCompile(`^\s*(?:/\*+!?|\*+/|\*+|//[/!]?|#+|--(?:\s*[|^])?|;+)\s?`)

// xmlTag strips tags of XML doc comments (C# `/// <summary>text</summary>`).
var xmlTag = regexp.MustCompile(`</?[A-Za-z][^<>]*>`)

// doc returns the first non-empty line of the doc comment.
func doc(src []byte, d *def) string {
	var cm []*sitter.Node
	if d.doc != nil {
		cm = []*sitter.Node{d.doc}
	} else {
		cm = commentAbove(d.node)
	}
	for _, c := range cm {
		for _, line := range strings.Split(string(src[c.StartByte():c.EndByte()]), "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(commentMark.ReplaceAllString(line, "")), "*/"))
			if strings.HasPrefix(line, "<") {
				line = strings.TrimSpace(xmlTag.ReplaceAllString(line, ""))
			}
			if line != "" {
				return clip(line)
			}
		}
	}
	return ""
}

// commentAbove returns the contiguous comment block right above n (skipping
// attribute/annotation siblings), in source order.
func commentAbove(n *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	line := n.StartPosition().Row
	for s := n.PrevSibling(); s != nil; s = s.PrevSibling() {
		k := s.Kind()
		switch {
		case strings.Contains(k, "comment"):
			if s.EndPosition().Row+1 < line {
				return out
			}
			out = append([]*sitter.Node{s}, out...)
			line = s.StartPosition().Row
		case strings.Contains(k, "attribute") || strings.Contains(k, "annotation") || strings.Contains(k, "decorator"):
			line = s.StartPosition().Row
		default:
			return out
		}
	}
	// First child on the parent's first line (e.g. ruby: the comment is a
	// sibling of the class body): look above the parent.
	if par := n.Parent(); len(out) == 0 && par != nil && par.StartPosition().Row == n.StartPosition().Row {
		return commentAbove(par)
	}
	return out
}

func (p *Parser) imports(root *sitter.Node, src []byte) []lang.Import {
	var out []lang.Import
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, lang.Import{Path: s})
		}
	}
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	names := p.imps.CaptureNames()
	ms := qc.Matches(p.imps, root, src)
	for m := ms.Next(); m != nil; m = ms.Next() {
		if !m.SatisfiesTextPredicate(p.imps, nil, nil, src) {
			continue
		}
		var path, kind string
		wild := false
		for _, c := range m.Captures {
			t := string(src[c.Node.StartByte():c.Node.EndByte()])
			switch n := names[c.Index]; n {
			case "import", "import.relative", "import.mod":
				path, kind = t, n
			case "wild":
				wild = true
			}
		}
		switch kind {
		case "":
			continue
		case "import.relative":
			if !strings.HasPrefix(path, ".") {
				path = "./" + path
			}
		case "import.mod":
			path = "self::" + path
		}
		if wild {
			path += ".*"
		}
		if kind == "import" && p.spec.ExpandImport != nil {
			for _, e := range p.spec.ExpandImport(path) {
				add(e)
			}
			continue
		}
		add(path)
	}
	return out
}
