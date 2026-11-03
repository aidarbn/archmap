// Package js is the JavaScript/TypeScript parser for archmap (.js .jsx .mjs .ts .tsx).
//
// Contract: see package lang. The tree-sitter layer is internal/ts; the dialect
// (javascript, typescript or tsx) is chosen by file extension.
//
// What becomes a lang.Func (top-level statements only, like the reference
// arch.py): function declarations, `const f = (...) => ...` / function
// expressions (also with a type annotation such as `const X: FC<P> = ...`),
// React components wrapped in memo/forwardRef, and methods and class-property
// arrows of top-level classes (Cls = class name; constructors are skipped).
// Handlers nested inside a function are not separate Funcs: their calls belong to
// the enclosing function.
//
// Imports are the module specifiers of import statements, `export ... from`,
// require() and dynamic import() with a string literal. DefaultExport is the
// name behind `export default`.
package js

import (
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/ts"
)

// Parser implements lang.Parser for js.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "js" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".js", ".jsx", ".ts", ".tsx", ".mjs"} }

// Parse parses one file; syntax errors yield whatever could be recovered.
func (*Parser) Parse(path string, src []byte) (lang.File, error) {
	dialect := ts.JavaScript
	switch {
	case strings.HasSuffix(path, ".tsx"):
		dialect = ts.TSX
	case strings.HasSuffix(path, ".ts"):
		dialect = ts.TypeScript
	}
	tree, err := ts.Parse(dialect, src)
	if err != nil {
		return lang.File{}, err
	}
	defer tree.Close()
	p := &state{t: tree}
	root := tree.Root()
	for _, n := range ts.NamedChildren(root) {
		p.topLevel(n)
	}
	p.imports(root)
	return lang.File{Funcs: p.funcs, Imports: p.imps, DefaultExport: p.def}, nil
}

type state struct {
	t     *ts.Tree
	funcs []lang.Func
	imps  []lang.Import
	def   string
	pre   []*sitter.Node // decorators preceding the class member being read
}

func (p *state) topLevel(n *sitter.Node) {
	stmt := n
	if n.Kind() == "export_statement" {
		if d := n.ChildByFieldName("declaration"); d != nil {
			n = d
		} else if v := n.ChildByFieldName("value"); v != nil {
			p.defaultValue(v, stmt)
			return
		} else {
			return
		}
		if isDefault(stmt) {
			p.def = p.t.FieldText(n, "name")
		}
	}
	switch n.Kind() {
	case "function_declaration", "generator_function_declaration":
		p.addFunc(p.t.FieldText(n, "name"), "", stmt, stmt, n)
	case "lexical_declaration", "variable_declaration":
		for _, d := range ts.NamedChildren(n) {
			if d.Kind() != "variable_declarator" {
				continue
			}
			name := d.ChildByFieldName("name")
			if name == nil || name.Kind() != "identifier" {
				continue
			}
			if fn := p.funcValue(d.ChildByFieldName("value")); fn != nil {
				p.addFunc(p.t.Text(name), "", stmt, stmt, fn)
			}
		}
	case "class_declaration", "abstract_class_declaration":
		p.class(stmt, n)
	}
}

// defaultValue handles `export default <expr>` where the expression is not a
// declaration.
func (p *state) defaultValue(v, stmt *sitter.Node) {
	switch v.Kind() {
	case "identifier":
		p.def = p.t.Text(v)
	case "call_expression":
		// export default memo(Foo) / connect(...)(Foo): the wrapped identifier.
		if a := firstArg(v); a != nil && a.Kind() == "identifier" {
			p.def = p.t.Text(a)
		}
	case "class":
		p.def = p.t.FieldText(v, "name")
		if p.def != "" {
			p.class(stmt, v)
		}
	case "function_expression", "function", "generator_function":
		p.def = p.t.FieldText(v, "name")
		if p.def != "" {
			p.addFunc(p.def, "", stmt, stmt, v)
		}
	}
}

func isDefault(stmt *sitter.Node) bool {
	for _, c := range ts.Children(stmt) {
		if c.Kind() == "default" {
			return true
		}
	}
	return false
}

func firstArg(call *sitter.Node) *sitter.Node {
	a := call.ChildByFieldName("arguments")
	if a == nil || a.NamedChildCount() == 0 {
		return nil
	}
	return a.NamedChild(0)
}

// funcValue returns the function node behind a variable value: an arrow function,
// a function expression, or either wrapped in memo/forwardRef (also React.memo).
func (p *state) funcValue(v *sitter.Node) *sitter.Node {
	for v != nil {
		switch v.Kind() {
		case "arrow_function", "function_expression", "function", "generator_function":
			return v
		case "call_expression":
			f := v.ChildByFieldName("function")
			if f == nil {
				return nil
			}
			name := ""
			switch f.Kind() {
			case "identifier":
				name = p.t.Text(f)
			case "member_expression":
				if o := f.ChildByFieldName("object"); o != nil && p.t.Text(o) == "React" {
					name = p.t.FieldText(f, "property")
				}
			}
			if name != "memo" && name != "forwardRef" {
				return nil
			}
			v = firstArg(v)
		case "parenthesized_expression", "as_expression", "satisfies_expression", "non_null_expression":
			if v.NamedChildCount() == 0 {
				return nil
			}
			v = v.NamedChild(0)
		default:
			return nil
		}
	}
	return nil
}

func (p *state) class(stmt, n *sitter.Node) {
	name := p.t.FieldText(n, "name")
	body := n.ChildByFieldName("body")
	if name == "" || body == nil {
		return
	}
	defer func() { p.pre = nil }()
	prevDeco := false
	for _, m := range ts.NamedChildren(body) {
		if m.Kind() == "decorator" {
			// tree-sitter-typescript puts class member decorators in the class body.
			p.pre = append(p.pre, m)
			prevDeco = true
			continue
		}
		if !prevDeco {
			p.pre = nil
		}
		prevDeco = false
		switch m.Kind() {
		case "method_definition":
			mn := m.ChildByFieldName("name")
			if mn == nil {
				continue
			}
			nm := p.t.Text(mn)
			if nm == "constructor" || mn.Kind() == "computed_property_name" {
				continue
			}
			p.addFunc(strings.TrimPrefix(nm, "#"), name, m, m, m)
		case "public_field_definition", "field_definition":
			fnm := m.ChildByFieldName("name")
			if fnm == nil {
				fnm = m.ChildByFieldName("property")
			}
			v := m.ChildByFieldName("value")
			if fnm == nil || v == nil || fnm.Kind() == "computed_property_name" {
				continue
			}
			if fn := p.funcValue(v); fn != nil {
				p.addFunc(strings.TrimPrefix(p.t.Text(fnm), "#"), name, m, m, fn)
			}
		}
	}
}

// addFunc records a function. decl is the node whose text starts the signature
// (decorators are excluded from Start and reported as TStart); stmt is the
// outermost node (used for the doc comment); fn holds the body and its end line.
func (p *state) addFunc(name, cls string, stmt, decl, fn *sitter.Node) {
	body := fn.ChildByFieldName("body")
	if name == "" || body == nil {
		return
	}
	var decos []string
	var decoNodes []*sitter.Node
	decoNodes = append(decoNodes, p.pre...)
	first := decl.StartByte()
	for _, c := range ts.Children(decl) {
		if c.Kind() == "decorator" {
			decoNodes = append(decoNodes, c)
			continue
		}
		first = c.StartByte()
		break
	}
	startLine := ts.StartLine(decl)
	tstart := 0
	if len(decoNodes) > 0 {
		tstart = ts.StartLine(decoNodes[0])
		startLine = lineOf(p.t.Src, first)
		stmt = decoNodes[0] // doc comments sit above the first decorator
		for _, d := range decoNodes {
			decos = append(decos, strings.TrimPrefix(strings.TrimSpace(p.t.Text(d)), "@"))
		}
	}
	sig := collapse(string(p.t.Src[first:body.StartByte()]))
	sig = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sig), "{"))
	sig = strings.TrimSpace(strings.TrimSuffix(sig, "=>"))
	if len(sig) > 200 {
		sig = sig[:200]
	}
	end := ts.EndLine(fn)
	if end < startLine {
		end = startLine
	}
	f := lang.Func{
		Name: name, Cls: cls, Start: startLine, End: end, TStart: tstart,
		Sig: sig, Doc: p.doc(stmt), Decorators: decos,
	}
	var cl lang.CallList
	for _, c := range p.calls(body) {
		if c.Name == name && (c.Recv == "" || c.Recv == "this" || c.Recv == "self") {
			continue
		}
		cl.Add(c.Name, c.Recv)
	}
	f.Calls = cl.Calls()
	f.Queues = p.queues(body)
	p.funcs = append(p.funcs, f)
}

func lineOf(src []byte, off uint) int {
	return strings.Count(string(src[:off]), "\n") + 1
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

type posCall struct {
	pos uint
	lang.Call
}

// calls lists the calls inside body ordered by the position of the call's name.
func (p *state) calls(body *sitter.Node) []lang.Call {
	var found []posCall
	ts.Walk(body, func(n *sitter.Node) bool {
		var fnNode *sitter.Node
		switch n.Kind() {
		case "call_expression":
			fnNode = n.ChildByFieldName("function")
		case "new_expression":
			fnNode = n.ChildByFieldName("constructor")
		default:
			return true
		}
		if fnNode == nil {
			return true
		}
		switch fnNode.Kind() {
		case "identifier":
			found = append(found, posCall{fnNode.StartByte(), lang.Call{Name: p.t.Text(fnNode)}})
		case "member_expression":
			prop := fnNode.ChildByFieldName("property")
			if prop == nil || (prop.Kind() != "property_identifier" && prop.Kind() != "private_property_identifier") {
				break
			}
			found = append(found, posCall{prop.StartByte(), lang.Call{Name: strings.TrimPrefix(p.t.Text(prop), "#"), Recv: p.recv(fnNode.ChildByFieldName("object"))}})
		case "non_null_expression":
			if fnNode.NamedChildCount() > 0 && fnNode.NamedChild(0).Kind() == "identifier" {
				c := fnNode.NamedChild(0)
				found = append(found, posCall{c.StartByte(), lang.Call{Name: p.t.Text(c)}})
			}
		}
		return true
	})
	sort.SliceStable(found, func(i, j int) bool { return found[i].pos < found[j].pos })
	out := make([]lang.Call, len(found))
	for i, f := range found {
		out[i] = f.Call
	}
	return out
}

// recv renders the receiver of a method call: a plain dotted name or "?".
func (p *state) recv(n *sitter.Node) string {
	s, ok := p.dotted(n)
	if !ok || s == "" {
		return "?"
	}
	return s
}

func (p *state) dotted(n *sitter.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Kind() {
	case "identifier", "this", "super":
		return p.t.Text(n), true
	case "non_null_expression":
		if n.NamedChildCount() > 0 {
			return p.dotted(n.NamedChild(0))
		}
	case "member_expression":
		o, ok := p.dotted(n.ChildByFieldName("object"))
		pr := n.ChildByFieldName("property")
		if !ok || pr == nil || (pr.Kind() != "property_identifier" && pr.Kind() != "private_property_identifier") {
			return "", false
		}
		return o + "." + p.t.Text(pr), true
	}
	return "", false
}

// doc returns the first line of the comment block directly above n.
func (p *state) doc(n *sitter.Node) string {
	row := n.StartPosition().Row
	var block []*sitter.Node
	for s := n.PrevSibling(); s != nil && s.Kind() == "comment"; s = s.PrevSibling() {
		if s.EndPosition().Row+1 < row || !ts.OwnLine(p.t.Src, s) {
			break
		}
		block = append([]*sitter.Node{s}, block...)
		row = s.StartPosition().Row
	}
	for _, c := range block {
		for _, ln := range strings.Split(p.t.Text(c), "\n") {
			ln = strings.TrimSpace(ln)
			ln = strings.TrimPrefix(ln, "/**")
			ln = strings.TrimPrefix(ln, "/*")
			ln = strings.TrimPrefix(ln, "//")
			ln = strings.TrimSuffix(ln, "*/")
			ln = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), "*"))
			if ln != "" {
				if len(ln) > 200 {
					ln = ln[:200]
				}
				return ln
			}
		}
	}
	return ""
}

// imports collects module specifiers of the whole file.
func (p *state) imports(root *sitter.Node) {
	seen := map[string]bool{}
	add := func(s *sitter.Node) {
		if s == nil || s.Kind() != "string" {
			return
		}
		v := strings.Trim(p.t.Text(s), "\"'`")
		if v != "" && !seen[v] {
			seen[v] = true
			p.imps = append(p.imps, lang.Import{Path: v})
		}
	}
	ts.Walk(root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "import_statement", "export_statement":
			add(n.ChildByFieldName("source"))
		case "import_require_clause":
			for _, c := range ts.NamedChildren(n) {
				add(c)
			}
		case "call_expression":
			f := n.ChildByFieldName("function")
			if f != nil && (f.Kind() == "import" || (f.Kind() == "identifier" && p.t.Text(f) == "require")) {
				add(firstArg(n))
			}
		}
		return true
	})
}

var _ lang.Parser = (*Parser)(nil)

// queues finds BullMQ-style queue hints: `x.add('name', ...)` gives
// "add:name", `new Worker('name', handler)` gives "worker:name[:handler]".
func (p *state) queues(body *sitter.Node) []string {
	var out []string
	str := func(n *sitter.Node) (string, bool) {
		if n == nil || (n.Kind() != "string" && n.Kind() != "template_string") {
			return "", false
		}
		s := strings.Trim(p.t.Text(n), "'\"`")
		if s == "" || strings.ContainsAny(s, "${ ") {
			return "", false
		}
		return s, true
	}
	ts.Walk(body, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "call_expression":
			fnNode, args := n.ChildByFieldName("function"), n.ChildByFieldName("arguments")
			if fnNode == nil || args == nil || fnNode.Kind() != "member_expression" || args.NamedChildCount() == 0 {
				return true
			}
			if prop := fnNode.ChildByFieldName("property"); prop != nil && p.t.Text(prop) == "add" {
				if s, ok := str(args.NamedChild(0)); ok {
					out = append(out, "add:"+s)
				}
			}
		case "new_expression":
			c, args := n.ChildByFieldName("constructor"), n.ChildByFieldName("arguments")
			if c == nil || args == nil || p.t.Text(c) != "Worker" || args.NamedChildCount() == 0 {
				return true
			}
			if s, ok := str(args.NamedChild(0)); ok {
				h := ""
				if args.NamedChildCount() > 1 && args.NamedChild(1).Kind() == "identifier" {
					h = ":" + p.t.Text(args.NamedChild(1))
				}
				out = append(out, "worker:"+s+h)
			}
		}
		return true
	})
	return out
}
