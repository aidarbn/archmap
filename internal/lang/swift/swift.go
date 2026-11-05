// Package swift is the swift parser for archmap, built on the tree-sitter grammar
// alex-pinkus/tree-sitter-swift (internal/ts).
//
// What it extracts (reference behaviour is parse_braced of arch.py, made exact by
// the syntax tree):
//
//   - func, init and deinit declared in class/struct/enum/actor/extension/protocol
//     (extension of a protocol too) and free funcs. Cls is the declaring type; for
//     an extension it is the extended type (last component of a dotted name,
//     generics dropped); for nested types the innermost type, as the reference does.
//   - Computed properties, subscripts, observers and bodyless declarations
//     (protocol requirements) are not functions. Local funcs and closures belong to
//     the enclosing function (their calls are part of it).
//   - Start is the line of the first non-attribute token (modifiers or `func`),
//     TStart the line of the first attribute when it is earlier; Sig runs from there
//     to the body brace; Doc is the first line of the preceding /// or /** */ block.
//   - Calls: `f()` (Recv ""), `self.f()`, `obj.f()`, `a.b.f()`, with "?" for any
//     receiver that is not a plain dotted name (call result, subscript, ...).
//   - Decorators are the function's attributes without "@" (`objc`, `MainActor`).
//     To let the orchestrator find entry points, functions of a type that carries
//     @main/@UIApplicationMain/@NSApplicationMain, or conforms to App or one of the
//     application/scene delegate protocols, also get "type:<attr>" and
//     "conforms:<Protocol>" entries.
//   - Imports are module names (first component of the import path), deduplicated.
//
// Swift has no imports between the files of one target; module edges come from calls
// (the orchestrator derives them). Bases is intentionally left empty: the
// orchestrator treats any base ending in "View" as an HTTP view, which would match
// every SwiftUI View.
package swift

import (
	"sort"
	"strings"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/ts"
)

// Parser implements lang.Parser for swift.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "swift" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".swift"} }

var entryAttrs = map[string]bool{"main": true, "UIApplicationMain": true, "NSApplicationMain": true}

var entryProtocols = map[string]bool{
	"App": true, "UIApplicationDelegate": true, "UIWindowSceneDelegate": true,
	"NSApplicationDelegate": true, "WKApplicationDelegate": true, "WKExtensionDelegate": true,
	"UISceneDelegate": true,
}

var infixKinds = map[string]bool{
	"additive_expression": true, "multiplicative_expression": true, "comparison_expression": true,
	"equality_expression": true, "conjunction_expression": true, "disjunction_expression": true,
	"nil_coalescing_expression": true, "range_expression": true, "infix_expression": true,
	"bitwise_operation": true,
}

// typeCtx is the enclosing type of a declaration.
type typeCtx struct {
	name  string
	marks []string // "type:main", "conforms:App"
}

type parser struct {
	t     *ts.Tree
	funcs []lang.Func
}

// Parse parses one swift file; syntax errors yield whatever could be recovered.
func (*Parser) Parse(path string, src []byte) (lang.File, error) {
	t, err := ts.Parse(ts.Swift, src)
	if err != nil {
		return lang.File{}, err
	}
	defer t.Close()
	p := &parser{t: t}
	root := t.Root()
	p.visit(root, nil)
	return lang.File{Funcs: p.funcs, Imports: p.imports(root)}, nil
}

func (p *parser) imports(root *sitter.Node) []lang.Import {
	var out []lang.Import
	seen := map[string]bool{}
	for _, n := range ts.NamedChildren(root) {
		if n.Kind() != "import_declaration" {
			continue
		}
		for _, c := range ts.NamedChildren(n) {
			if c.Kind() != "identifier" {
				continue
			}
			m := p.t.Text(c)
			if i := strings.IndexByte(m, '.'); i >= 0 {
				m = m[:i]
			}
			if m != "" && !seen[m] {
				seen[m] = true
				out = append(out, lang.Import{Path: m})
			}
		}
	}
	return out
}

// visit looks for declarations below n. Function bodies and properties are not
// entered: nested funcs/types there belong to the enclosing function.
func (p *parser) visit(n *sitter.Node, cur *typeCtx) {
	for _, c := range ts.NamedChildren(n) {
		switch c.Kind() {
		case "class_declaration", "protocol_declaration":
			p.typeDecl(c, cur)
		case "function_declaration", "init_declaration", "deinit_declaration":
			p.funcDecl(c, cur)
		case "property_declaration", "subscript_declaration", "comment", "multiline_comment":
		default:
			p.visit(c, cur) // #if blocks, ERROR recovery, bodies
		}
	}
}

func (p *parser) typeDecl(n *sitter.Node, outer *typeCtx) {
	ctx := &typeCtx{name: typeName(p.t, n.ChildByFieldName("name"))}
	for _, c := range ts.NamedChildren(n) {
		switch c.Kind() {
		case "modifiers":
			for _, a := range ts.NamedChildren(c) {
				if a.Kind() == "attribute" {
					if name := attrName(p.t, a); entryAttrs[name] {
						ctx.marks = append(ctx.marks, "type:"+name)
					}
				}
			}
		case "inheritance_specifier":
			if in := c.ChildByFieldName("inherits_from"); in != nil {
				if name := typeName(p.t, in); entryProtocols[name] {
					ctx.marks = append(ctx.marks, "conforms:"+name)
				}
			}
		}
	}
	if body := n.ChildByFieldName("body"); body != nil {
		p.visit(body, ctx)
	}
}

// typeName is the last identifier of a (possibly dotted, generic) type.
func typeName(t *ts.Tree, n *sitter.Node) string {
	if n == nil {
		return ""
	}
	last := ""
	ts.Walk(n, func(c *sitter.Node) bool {
		switch c.Kind() {
		case "type_arguments":
			return false
		case "type_identifier":
			last = t.Text(c)
		}
		return true
	})
	if last == "" {
		return t.Text(n)
	}
	return last
}

func attrName(t *ts.Tree, a *sitter.Node) string {
	for _, c := range ts.NamedChildren(a) {
		if c.Kind() == "user_type" {
			return typeName(t, c)
		}
	}
	return strings.TrimPrefix(t.Text(a), "@")
}

func (p *parser) funcDecl(n *sitter.Node, cur *typeCtx) {
	body := n.ChildByFieldName("body")
	if body == nil {
		return // protocol requirement or broken declaration
	}
	var name string
	switch n.Kind() {
	case "init_declaration":
		name = "init"
	case "deinit_declaration":
		name = "deinit"
	default:
		name = p.t.FieldText(n, "name")
	}
	if name == "" {
		return
	}
	// First non-attribute token: modifiers other than attributes, else the keyword.
	var first *sitter.Node
	var decos []string
	for _, c := range ts.Children(n) {
		if c.Kind() == "modifiers" {
			for _, m := range ts.Children(c) {
				if !m.IsNamed() {
					continue
				}
				if m.Kind() == "attribute" {
					decos = append(decos, strings.TrimPrefix(p.t.Text(m), "@"))
				} else if first == nil {
					first = m
				}
			}
			continue
		}
		if first == nil {
			first = c
		}
		break
	}
	if first == nil {
		first = n
	}
	start := ts.StartLine(first)
	f := lang.Func{
		Name:  name,
		Start: start,
		End:   ts.EndLine(n),
		Sig:   truncate(collapse(string(p.t.Src[first.StartByte():body.StartByte()])), 200),
		Doc:   p.doc(n),
	}
	if s := ts.StartLine(n); s < start {
		f.TStart = s
	}
	if cur != nil {
		f.Cls = cur.name
		decos = append(decos, cur.marks...)
	}
	f.Decorators = decos
	var calls lang.CallList
	for _, c := range p.collectCalls(body) {
		calls.Add(c.Name, c.Recv)
	}
	f.Calls = calls.Calls()
	p.funcs = append(p.funcs, f)
}

type posCall struct {
	lang.Call
	pos uint
}

// collectCalls returns every call below n ordered by position of the callee name.
func (p *parser) collectCalls(n *sitter.Node) []lang.Call {
	var found []posCall
	ts.Walk(n, func(c *sitter.Node) bool {
		if c.Kind() == "call_expression" {
			if pc, ok := p.call(c); ok {
				found = append(found, pc)
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

func (p *parser) call(c *sitter.Node) (posCall, bool) {
	callee := c.NamedChild(0)
	if callee == nil {
		return posCall{}, false
	}
	// `x[0]` is a call_expression with a call_suffix starting with "[": a subscript.
	if c.NamedChildCount() > 1 {
		if suf := c.NamedChild(1); suf.Kind() == "call_suffix" && strings.HasPrefix(p.t.Text(suf), "[") {
			return posCall{}, false
		}
	}
	// The grammar parses `!f(x)` / `-a.f(x)` as a call of a prefix expression; an
	// implicit member call `.f(x)` has the same shape and is not a named call.
	// Likewise `a + b.f()` comes out as a call of the infix expression `a + b.f`:
	// the call belongs to the rightmost operand.
	for callee.NamedChildCount() > 0 && ((callee.Kind() == "prefix_expression" && !strings.HasPrefix(p.t.Text(callee), ".")) || infixKinds[callee.Kind()]) {
		callee = callee.NamedChild(callee.NamedChildCount() - 1)
	}
	switch callee.Kind() {
	case "simple_identifier":
		return posCall{lang.Call{Name: p.t.Text(callee)}, callee.StartByte()}, true
	case "navigation_expression":
		suf := callee.ChildByFieldName("suffix")
		if suf == nil {
			return posCall{}, false
		}
		id := suf.ChildByFieldName("suffix")
		if id == nil || id.Kind() != "simple_identifier" {
			return posCall{}, false // .0 tuple index etc.
		}
		recv, ok := p.dotted(callee.ChildByFieldName("target"))
		if !ok {
			recv = "?"
		}
		return posCall{lang.Call{Name: p.t.Text(id), Recv: recv}, id.StartByte()}, true
	}
	return posCall{}, false
}

// dotted renders a plain dotted name (identifiers, self, super, optional/forced
// unwraps); ok is false for anything else.
func (p *parser) dotted(n *sitter.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Kind() {
	case "simple_identifier", "self_expression", "super_expression":
		return p.t.Text(n), true
	case "postfix_expression":
		return p.dotted(n.ChildByFieldName("target"))
	case "navigation_expression":
		l, ok := p.dotted(n.ChildByFieldName("target"))
		suf := n.ChildByFieldName("suffix")
		if !ok || suf == nil {
			return "", false
		}
		id := suf.ChildByFieldName("suffix")
		if id == nil || id.Kind() != "simple_identifier" {
			return "", false
		}
		return l + "." + p.t.Text(id), true
	case "tuple_expression":
		// `(x)` parenthesised
		if n.NamedChildCount() == 1 {
			return p.dotted(n.NamedChild(0))
		}
	}
	return "", false
}

// doc returns the first line of the /// or /** */ block directly above n.
func (p *parser) doc(n *sitter.Node) string {
	var chain []*sitter.Node
	line := ts.StartLine(n)
	for s := n.PrevSibling(); s != nil && (s.Kind() == "comment" || s.Kind() == "multiline_comment"); s = s.PrevSibling() {
		if ts.EndLine(s) < line-1 || !ts.OwnLine(p.t.Src, s) {
			break
		}
		chain = append([]*sitter.Node{s}, chain...)
		line = ts.StartLine(s)
	}
	for _, c := range chain {
		txt := p.t.Text(c)
		switch {
		case strings.HasPrefix(txt, "///"):
			if l := strings.TrimSpace(strings.TrimPrefix(txt, "///")); l != "" && !lang.IsDirectiveLine(l) {
				return truncate(l, 200)
			}
		case strings.HasPrefix(txt, "/**"):
			for _, l := range strings.Split(txt, "\n") {
				l = strings.TrimSpace(l)
				l = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(l, "/**"), "*"), "*/"))
				if l != "" && !lang.IsDirectiveLine(l) {
					return truncate(l, 200)
				}
			}
		}
	}
	return ""
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

var _ lang.Parser = (*Parser)(nil)
