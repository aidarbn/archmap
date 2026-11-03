// Package golang is the Go parser for archmap, built on tree-sitter-go.
//
// tree-sitter was chosen over go/parser for uniformity with the other language
// parsers and because it recovers from syntax errors, so a half-edited file still
// yields its intact functions (go/parser stops at the first broken declaration).
//
// What it reports (see package lang for the contract):
//
//   - top-level functions and methods that have a body (declarations without a
//     body are skipped); Cls is the receiver type without "*" and type
//     parameters, Recv the receiver variable name;
//   - Start/End: 1-based inclusive lines of the whole declaration;
//   - Sig: text from "func" up to the body's "{", whitespace-collapsed, <= 200;
//   - Doc: the contiguous "//" comment lines directly above the function (column 0,
//     no //go: directives), joined with a space, <= 200 chars;
//   - Calls: call expressions in the body, closures included, ordered by the
//     position of the called name. Receiver is the plain dotted operand text
//     ("pkg", "s.repo"), "?" for any other operand (call result, index, ...);
//     conversions such as []byte(x) and calls of func literals are not named
//     calls and are skipped;
//   - struct fields and interface methods of named types (lang.File.Types) and
//     the typed parameters of functions (lang.Func.Params): the orchestrator
//     resolves `s.field.M()` and `param.M()` by type;
//   - imports with their optional alias.
package golang

import (
	"sort"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/ts"
)

const maxLen = 200

// Parser implements lang.Parser for go.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "go" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".go"} }

// Parse parses one Go file.
func (*Parser) Parse(_ string, src []byte) (lang.File, error) {
	t, err := ts.Parse(ts.Go, src)
	if err != nil {
		return lang.File{}, err
	}
	defer t.Close()

	var f lang.File
	root := t.Root()
	for _, n := range ts.NamedChildren(root) {
		switch n.Kind() {
		case "import_declaration":
			ts.Walk(n, func(c *sitter.Node) bool {
				if c.Kind() == "import_spec" {
					if imp, ok := importSpec(t, c); ok {
						f.Imports = append(f.Imports, imp)
					}
					return false
				}
				return true
			})
		case "type_declaration":
			for _, s := range ts.NamedChildren(n) {
				if s.Kind() == "type_spec" {
					if d, ok := typeDecl(t, s); ok {
						f.Types = append(f.Types, d)
					}
				}
			}
		case "function_declaration", "method_declaration":
			if fn, ok := function(t, n); ok {
				f.Funcs = append(f.Funcs, fn)
			}
		}
	}
	return f, nil
}

func importSpec(t *ts.Tree, n *sitter.Node) (lang.Import, bool) {
	p := n.ChildByFieldName("path")
	if p == nil {
		return lang.Import{}, false
	}
	path, err := strconv.Unquote(t.Text(p))
	if err != nil {
		path = strings.Trim(t.Text(p), "\"`")
	}
	return lang.Import{Path: path, Alias: t.FieldText(n, "name")}, true
}

func function(t *ts.Tree, n *sitter.Node) (lang.Func, bool) {
	body := n.ChildByFieldName("body")
	name := n.ChildByFieldName("name")
	if body == nil || name == nil {
		return lang.Func{}, false
	}
	fn := lang.Func{
		Name:  t.Text(name),
		Start: ts.StartLine(n),
		End:   ts.EndLine(n),
		Sig:   clip(collapse(string(t.Src[n.StartByte():body.StartByte()]))),
		Doc:   clip(doc(t, n)),
	}
	fn.Params = params(t, n.ChildByFieldName("parameters"))
	if r := n.ChildByFieldName("receiver"); r != nil {
		fn.Cls, fn.Recv = receiver(t, r)
	}

	type pos struct {
		at   uint
		call lang.Call
	}
	var found []pos
	seenTypes := map[string]bool{}
	ts.Walk(body, func(c *sitter.Node) bool {
		var name, recv string
		var at uint
		var ok bool
		switch c.Kind() {
		case "composite_literal":
			if tn := literalType(t, c.ChildByFieldName("type")); tn != "" && !seenTypes[tn] {
				seenTypes[tn] = true
				fn.Types = append(fn.Types, tn)
			}
		case "call_expression":
			name, recv, at, ok = callee(t, c.ChildByFieldName("function"))
		case "type_conversion_expression":
			// The grammar cannot tell f[T](x) from a conversion; with type
			// arguments it is almost always an explicit generic call.
			name, recv, at, ok = genericCallee(t, c.ChildByFieldName("type"))
		}
		if ok {
			found = append(found, pos{at, lang.Call{Name: name, Recv: recv}})
		}
		return true
	})
	sort.SliceStable(found, func(i, j int) bool { return found[i].at < found[j].at })
	var cl lang.CallList
	for _, p := range found {
		cl.Add(p.call.Name, p.call.Recv)
	}
	fn.Calls = cl.Calls()
	return fn, true
}

// literalType is the last name segment of a composite literal's type
// ("jobs.XArgs{}" -> "XArgs"); "" for slices, maps and anonymous structs.
func literalType(t *ts.Tree, n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "type_identifier":
		return t.Text(n)
	case "qualified_type":
		return t.FieldText(n, "name")
	case "generic_type":
		return literalType(t, n.ChildByFieldName("type"))
	}
	return ""
}

// receiver returns the receiver type (no "*", no type parameters) and variable.
func receiver(t *ts.Tree, r *sitter.Node) (cls, recv string) {
	for _, p := range ts.NamedChildren(r) {
		if p.Kind() != "parameter_declaration" {
			continue
		}
		recv = t.FieldText(p, "name")
		typ := p.ChildByFieldName("type")
		for typ != nil {
			switch typ.Kind() {
			case "pointer_type", "parenthesized_type":
				typ = typ.NamedChild(0)
				continue
			case "generic_type":
				typ = typ.ChildByFieldName("type")
				continue
			}
			break
		}
		return t.Text(typ), recv
	}
	return "", ""
}

// callee extracts name, receiver and name position of a call's function node.
func callee(t *ts.Tree, fn *sitter.Node) (name, recv string, at uint, ok bool) {
	for fn != nil && fn.Kind() == "parenthesized_expression" && fn.NamedChildCount() == 1 {
		fn = fn.NamedChild(0)
	}
	if fn == nil {
		return
	}
	switch fn.Kind() {
	case "identifier":
		return t.Text(fn), "", fn.StartByte(), true
	case "selector_expression":
		f := fn.ChildByFieldName("field")
		if f == nil {
			return
		}
		return t.Text(f), operand(t, fn.ChildByFieldName("operand")), f.StartByte(), true
	}
	return
}

// genericCallee handles f[T] / pkg.F[T] parsed as a generic type.
func genericCallee(t *ts.Tree, typ *sitter.Node) (name, recv string, at uint, ok bool) {
	if typ == nil || typ.Kind() != "generic_type" {
		return
	}
	n := typ.ChildByFieldName("type")
	switch {
	case n == nil:
	case n.Kind() == "type_identifier":
		return t.Text(n), "", n.StartByte(), true
	case n.Kind() == "qualified_type":
		nm := n.ChildByFieldName("name")
		return t.FieldText(n, "name"), t.FieldText(n, "package"), nm.StartByte(), true
	}
	return
}

// operand is the dotted source text of a plain a.b.c chain, "?" otherwise.
func operand(t *ts.Tree, n *sitter.Node) string {
	if n == nil {
		return "?"
	}
	switch n.Kind() {
	case "identifier":
		return t.Text(n)
	case "selector_expression":
		in := operand(t, n.ChildByFieldName("operand"))
		if in == "?" {
			return "?"
		}
		return in + "." + t.FieldText(n, "field")
	}
	return "?"
}

// doc joins the contiguous column-0 "//" comment lines right above n.
func doc(t *ts.Tree, n *sitter.Node) string {
	var lines []string
	row := n.StartPosition().Row
	for c := n.PrevSibling(); c != nil && c.Kind() == "comment"; c = c.PrevSibling() {
		txt := t.Text(c)
		if c.EndPosition().Row+1 != row || c.StartPosition().Column != 0 || !strings.HasPrefix(txt, "//") {
			break
		}
		row = c.StartPosition().Row
		if !strings.HasPrefix(txt, "//go:") {
			lines = append([]string{strings.TrimSpace(txt[2:])}, lines...)
		}
	}
	return strings.Join(lines, " ")
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string) string {
	if r := []rune(s); len(r) > maxLen {
		return string(r[:maxLen])
	}
	return s
}

var _ lang.Parser = (*Parser)(nil)

// namedType reduces a type node to a plain named type: (pointer to) T, pkg.T,
// T[...]. ok is false for slices, maps, funcs, channels and other shapes.
func namedType(t *ts.Tree, n *sitter.Node) (typ, pkg string, ok bool) {
	for n != nil {
		switch n.Kind() {
		case "pointer_type", "parenthesized_type":
			n = n.NamedChild(0)
			continue
		case "generic_type":
			n = n.ChildByFieldName("type")
			continue
		case "type_identifier":
			return t.Text(n), "", true
		case "qualified_type":
			return t.FieldText(n, "name"), t.FieldText(n, "package"), true
		}
		return "", "", false
	}
	return "", "", false
}

// typeDecl reads a `type X struct{...}` / `type X interface{...}` spec.
func typeDecl(t *ts.Tree, n *sitter.Node) (lang.TypeDecl, bool) {
	name := t.FieldText(n, "name")
	typ := n.ChildByFieldName("type")
	if name == "" || typ == nil {
		return lang.TypeDecl{}, false
	}
	d := lang.TypeDecl{Name: name, Kind: "other"}
	switch typ.Kind() {
	case "struct_type":
		d.Kind = "struct"
		ts.Walk(typ, func(c *sitter.Node) bool {
			if c.Kind() != "field_declaration" {
				return true
			}
			ft, pkg, ok := namedType(t, c.ChildByFieldName("type"))
			var names []string
			for _, x := range ts.NamedChildren(c) {
				if x.Kind() == "field_identifier" {
					names = append(names, t.Text(x))
				}
			}
			if ok {
				if len(names) == 0 {
					d.Fields = append(d.Fields, lang.Field{Type: ft, Pkg: pkg})
				}
				for _, nm := range names {
					d.Fields = append(d.Fields, lang.Field{Name: nm, Type: ft, Pkg: pkg})
				}
			}
			return false // nested anonymous structs are not followed
		})
	case "interface_type":
		d.Kind = "interface"
		for _, e := range ts.NamedChildren(typ) {
			switch e.Kind() {
			case "method_elem":
				if m := t.FieldText(e, "name"); m != "" {
					d.Methods = append(d.Methods, m)
				}
			case "type_elem":
				if e.NamedChildCount() == 1 {
					if et, pkg, ok := namedType(t, e.NamedChild(0)); ok {
						d.Embeds = append(d.Embeds, lang.Field{Type: et, Pkg: pkg})
					}
				}
			}
		}
	}
	return d, true
}

// params lists the named, plainly typed parameters of a function.
func params(t *ts.Tree, list *sitter.Node) []lang.Field {
	if list == nil {
		return nil
	}
	var out []lang.Field
	for _, p := range ts.NamedChildren(list) {
		if p.Kind() != "parameter_declaration" {
			continue
		}
		pt, pkg, ok := namedType(t, p.ChildByFieldName("type"))
		if !ok {
			continue
		}
		for _, x := range ts.NamedChildren(p) {
			if x.Kind() == "identifier" {
				out = append(out, lang.Field{Name: t.Text(x), Type: pt, Pkg: pkg})
			}
		}
	}
	return out
}
