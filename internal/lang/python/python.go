// Package python is the python parser for archmap, built on tree-sitter-python.
//
// Contract: see package lang. Use internal/ts for the tree-sitter layer.
//
// Reference behaviour is the Python original (arch.py, parse_python): functions and
// methods of top-level classes (also inside top-level if/try/with), nested defs are
// not separate Funcs and their calls belong to the enclosing Func; Decorators and
// Bases are the source text of the expressions; TStart = first decorator line;
// Sig = def line(s) up to the body; Doc = first docstring line; imports from every
// import statement in the file, including lazy ones inside functions.
//
// Like the original, calls of a Func include those in its decorators, default
// values and annotations. Unlike tree-sitter, ast.parse rejects a file with a
// syntax error entirely; here whatever could be recovered is returned.
package python

import (
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/ts"
)

// Parser implements lang.Parser for python.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "python" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".py"} }

// Parse parses one python file.
func (*Parser) Parse(path string, src []byte) (lang.File, error) {
	tr, err := ts.Parse(ts.Python, src)
	if err != nil {
		return lang.File{}, err
	}
	defer tr.Close()
	p := &state{tr: tr, lines: strings.Split(string(src), "\n")}
	p.walk(tr.Root(), "", nil)
	ts.Walk(tr.Root(), func(n *sitter.Node) bool {
		switch n.Kind() {
		case "import_statement":
			p.importStmt(n)
		case "import_from_statement":
			p.importFrom(n)
		}
		return true
	})
	return lang.File{Funcs: p.funcs, Imports: p.imports}, nil
}

type state struct {
	tr      *ts.Tree
	lines   []string
	funcs   []lang.Func
	imports []lang.Import
}

func (p *state) addImport(s string) {
	if s != "" {
		p.imports = append(p.imports, lang.Import{Path: s})
	}
}

// walk visits the statements of a module/block. Classes are entered only at
// module level, compound statements (if/try/with) only outside classes.
func (p *state) walk(n *sitter.Node, cls string, bases []string) {
	for _, c := range ts.NamedChildren(n) {
		def := c
		var deco *sitter.Node
		if c.Kind() == "decorated_definition" {
			deco = c
			def = c.ChildByFieldName("definition")
			if def == nil {
				continue
			}
		}
		switch def.Kind() {
		case "function_definition":
			p.addFunc(c, def, cls, bases)
		case "class_definition":
			if cls == "" {
				p.walk(p.classBody(def), p.tr.FieldText(def, "name"), p.classBases(def))
			}
		case "if_statement", "try_statement", "with_statement", "match_statement":
			if cls != "" || deco != nil || def.Kind() == "match_statement" {
				continue
			}
			if def.Kind() == "with_statement" && p.isAsync(def) {
				continue // ast.AsyncWith is not walked by the original
			}
			p.walkCompound(def)
		}
	}
}

func (p *state) isAsync(n *sitter.Node) bool {
	c := n.Child(0)
	return c != nil && c.Kind() == "async"
}

func (p *state) classBody(def *sitter.Node) *sitter.Node {
	if b := def.ChildByFieldName("body"); b != nil {
		return b
	}
	return def
}

func (p *state) walkCompound(n *sitter.Node) {
	for _, c := range ts.NamedChildren(n) {
		switch c.Kind() {
		case "block":
			p.walk(c, "", nil)
		case "elif_clause", "else_clause", "except_clause", "except_group_clause", "finally_clause":
			for _, b := range ts.NamedChildren(c) {
				if b.Kind() == "block" {
					p.walk(b, "", nil)
				}
			}
		}
	}
}

func (p *state) classBases(def *sitter.Node) []string {
	sc := def.ChildByFieldName("superclasses")
	var out []string
	if sc == nil {
		return out
	}
	for _, a := range ts.NamedChildren(sc) {
		switch a.Kind() {
		case "keyword_argument", "comment":
			continue
		}
		out = append(out, expr(p.tr.Text(a)))
	}
	return out
}

func (p *state) addFunc(outer, fn *sitter.Node, cls string, bases []string) {
	f := lang.Func{
		Name:  p.tr.FieldText(fn, "name"),
		Cls:   cls,
		Start: ts.StartLine(fn),
		End:   codeEnd(fn),
		Bases: bases,
	}
	body := fn.ChildByFieldName("body")
	first := firstStmt(body)
	f.Sig = p.signature(fn, first, f.Name)
	f.Doc = p.docstring(first)
	if outer != fn {
		if s := ts.StartLine(outer); s < f.Start {
			f.TStart = s
		}
		for _, d := range ts.NamedChildren(outer) {
			if d.Kind() == "decorator" {
				if e := d.NamedChild(0); e != nil {
					f.Decorators = append(f.Decorators, expr(p.tr.Text(e)))
				}
			}
		}
	}
	f.Calls = p.calls(outer)
	p.funcs = append(p.funcs, f)
}

// codeEnd is the last line of the last statement of n. tree-sitter attaches
// trailing comments to the enclosing block, ast does not; this matches ast.
func codeEnd(n *sitter.Node) int {
	switch n.Kind() {
	case "function_definition", "class_definition", "decorated_definition", "block",
		"if_statement", "elif_clause", "else_clause", "try_statement", "except_clause",
		"except_group_clause", "finally_clause", "with_statement", "for_statement",
		"while_statement", "match_statement", "case_clause":
		for i := int(n.NamedChildCount()) - 1; i >= 0; i-- {
			if c := n.NamedChild(uint(i)); c.Kind() != "comment" {
				return codeEnd(c)
			}
		}
	}
	return ts.EndLine(n)
}

func firstStmt(body *sitter.Node) *sitter.Node {
	if body == nil {
		return nil
	}
	for _, c := range ts.NamedChildren(body) {
		if c.Kind() != "comment" {
			return c
		}
	}
	return nil
}

// signature mirrors the original: the lines from the def line up to the line
// before the first body statement, joined and whitespace-collapsed.
func (p *state) signature(fn, first *sitter.Node, name string) string {
	a := int(fn.StartPosition().Row)
	b := a
	if first != nil {
		b = int(first.StartPosition().Row)
	}
	var s string
	if b > a {
		parts := make([]string, 0, b-a)
		for i := a; i < b && i < len(p.lines); i++ {
			parts = append(parts, strings.TrimSpace(p.lines[i]))
		}
		s = strings.Join(parts, " ")
	} else if first != nil {
		// one-liner `def f(): return 1`: cut the body off
		s = string(p.tr.Src[fn.StartByte():first.StartByte()])
	} else {
		s = string(p.tr.Src[fn.StartByte():fn.EndByte()])
	}
	s = strings.TrimSpace(strings.TrimRight(collapse(s), ":"))
	s = strings.TrimSpace(strings.TrimRight(s, ":"))
	if s == "" {
		return name
	}
	return truncate(s, 200)
}

func (p *state) docstring(first *sitter.Node) string {
	if first == nil || first.Kind() != "expression_statement" {
		return ""
	}
	e := first.NamedChild(0)
	if e == nil {
		return ""
	}
	var parts []*sitter.Node
	switch e.Kind() {
	case "string":
		parts = []*sitter.Node{e}
	case "concatenated_string":
		parts = ts.NamedChildren(e)
	default:
		return ""
	}
	var val strings.Builder
	for _, s := range parts {
		if s.Kind() != "string" {
			continue
		}
		v, ok := p.stringValue(s)
		if !ok {
			return ""
		}
		val.WriteString(v)
	}
	for _, ln := range strings.Split(val.String(), "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return truncate(t, 200)
		}
	}
	return ""
}

// stringValue returns the (approximate) value of a plain str literal; ok is
// false for f-strings and bytes.
func (p *state) stringValue(s *sitter.Node) (string, bool) {
	t := p.tr.Text(s)
	i := strings.IndexAny(t, `"'`)
	if i < 0 {
		return "", false
	}
	prefix := strings.ToLower(t[:i])
	if strings.ContainsAny(prefix, "fb") {
		return "", false
	}
	q := 1
	if strings.HasPrefix(t[i:], `"""`) || strings.HasPrefix(t[i:], `'''`) {
		q = 3
	}
	if len(t) < i+2*q {
		return "", false
	}
	raw := t[i+q : len(t)-q]
	if strings.Contains(prefix, "r") {
		return raw, true
	}
	return unescape(raw), true
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '\n':
		case '\\', '\'', '"':
			b.WriteByte(s[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

type callPos struct {
	row, col uint
	c        lang.Call
}

// calls collects every call under n (nested defs, lambdas, decorators and
// defaults included), ordered by position of the called name.
func (p *state) calls(n *sitter.Node) []lang.Call {
	var items []callPos
	ts.Walk(n, func(x *sitter.Node) bool {
		if x.Kind() != "call" {
			return true
		}
		f := x.ChildByFieldName("function")
		if f == nil {
			return true
		}
		switch f.Kind() {
		case "identifier":
			pos := f.StartPosition()
			items = append(items, callPos{pos.Row, pos.Column, lang.Call{Name: p.tr.Text(f)}})
		case "attribute":
			a := f.ChildByFieldName("attribute")
			if a == nil {
				return true
			}
			pos := a.StartPosition()
			items = append(items, callPos{pos.Row, pos.Column, lang.Call{Name: p.tr.Text(a), Recv: p.dotted(f.ChildByFieldName("object"))}})
		}
		return true
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].row != items[j].row {
			return items[i].row < items[j].row
		}
		return items[i].col < items[j].col
	})
	var cl lang.CallList
	for _, it := range items {
		cl.Add(it.c.Name, it.c.Recv)
	}
	return cl.Calls()
}

// dotted renders a receiver: a plain dotted name, "super" for `super()`, else "?".
func (p *state) dotted(n *sitter.Node) string {
	if n == nil {
		return "?"
	}
	switch n.Kind() {
	case "identifier":
		return p.tr.Text(n)
	case "attribute":
		d := p.dotted(n.ChildByFieldName("object"))
		if d == "" || d == "?" {
			return "?"
		}
		return d + "." + p.tr.FieldText(n, "attribute")
	case "call":
		if f := n.ChildByFieldName("function"); f != nil && f.Kind() == "identifier" && p.tr.Text(f) == "super" {
			return "super"
		}
	}
	return "?"
}

func (p *state) importStmt(n *sitter.Node) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		switch c.Kind() {
		case "dotted_name":
			p.addImport(p.tr.Text(c))
		case "aliased_import":
			p.addImport(p.tr.FieldText(c, "name"))
		}
	}
}

func (p *state) importFrom(n *sitter.Node) {
	modNode := n.ChildByFieldName("module_name")
	mod := ""
	hasModule := false
	if modNode != nil {
		mod = p.tr.Text(modNode)
		hasModule = modNode.Kind() == "dotted_name" || strings.Trim(mod, ".") != ""
	}
	p.addImport(mod)
	for i := uint(0); i < n.ChildCount(); i++ {
		if n.FieldNameForChild(uint32(i)) != "name" {
			continue
		}
		c := n.Child(i)
		name := p.tr.Text(c)
		if c.Kind() == "aliased_import" {
			name = p.tr.FieldText(c, "name")
		}
		if hasModule {
			p.addImport(mod + "." + name)
		} else {
			p.addImport(mod + name)
		}
	}
}

var bracketSpace = strings.NewReplacer("( ", "(", "[ ", "[", "{ ", "{", " )", ")", " ]", "]", " }", "}")

// expr renders an expression the way ast.unparse spaces multi-line brackets.
func expr(s string) string {
	return strings.ReplaceAll(bracketSpace.Replace(collapse(s)), ", )", ",)")
}

// collapse joins whitespace runs into single spaces.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

var _ lang.Parser = (*Parser)(nil)
