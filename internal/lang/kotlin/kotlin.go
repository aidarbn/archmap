// Package kotlin is the Kotlin parser for archmap, built on the tree-sitter
// grammar (tree-sitter-grammars/tree-sitter-kotlin) from internal/ts.
//
// What it emits (contract: package lang):
//
//   - every `fun` declared in a class / object / companion / interface / enum,
//     and top-level; local functions, lambdas and anonymous-object members that
//     live inside a function body are not separate Funcs, their calls belong to
//     the enclosing function;
//   - Cls is the innermost enclosing type; an unnamed `companion object` is
//     transparent (its funs belong to the outer class); for an extension
//     function Cls is the extended type (`fun List<T>?.foo()` -> "List");
//   - Start is the line of the first non-annotation modifier or `fun`; TStart is
//     the first annotation line when annotations sit above it;
//   - Sig is the declaration up to the body (annotations excluded), Doc is the
//     first line of the preceding KDoc (or of a `//` comment);
//   - Decorators are the annotations ("Composable", "Inject", `GET("/x")`),
//     Bases are the supertypes of the enclosing type as written without
//     constructor arguments ("AppCompatActivity", "ViewModel"); the orchestrator
//     can use them for entry points (Activity.onCreate, @Composable screens,
//     ViewModel members);
//   - Calls with Recv: "" for `f()`, "this"/"super" (`this@X` -> "this"),
//     "a.b" for `a?.b!!.f()`, "?" for call results, indexing, parentheses;
//     a recursive bare/`this.` call of the function itself is dropped (as in
//     the reference);
//   - Imports: dotted path without `import`; `import a.b.*` -> "a.b.*"; Alias is
//     the `as` name. The package declaration is not part of lang.File.
//
// Func.Ext is set for extension functions. Entry points (extract.findEntries) are
// derived as follows, kinds "lifecycle"/"ui":
//
//   - Cls != "" and a Base ending in Activity/Fragment/Service/Application:
//     onCreate, onCreateView, onStartCommand; a Base ending in
//     BroadcastReceiver: onReceive;
//   - Decorators containing "Composable" on a top-level func named *Screen or
//     *Route (kind "http", label "SCREEN <name>");
//   - Cls != "" with a Base ending in ViewModel: public funs (Sig without
//     "private"/"protected").
//
// Known grammar limits: `f<T>(x)` with arguments is read as a comparison and
// recovered by a special case; an enum entry with a body makes the grammar
// give up on the rest of the file.
package kotlin

import (
	"regexp"
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/ts"
)

// Parser implements lang.Parser for kotlin.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "kotlin" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".kt"} }

var _ lang.Parser = (*Parser)(nil)

type scope struct {
	cls   string
	bases []string
}

type walker struct {
	t     *ts.Tree
	out   lang.File
	stack []scope
}

// Parse parses one Kotlin file.
func (*Parser) Parse(path string, src []byte) (lang.File, error) {
	tr, err := ts.Parse(ts.Kotlin, blankTrailingCommas(src))
	if err != nil {
		return lang.File{}, err
	}
	defer tr.Close()
	w := &walker{t: tr}
	w.walk(tr.Root())
	return w.out, nil
}

func (w *walker) walk(n *sitter.Node) {
	switch n.Kind() {
	case "import":
		if n.NamedChildCount() > 0 {
			w.addImport(n)
		}
		return
	case "function_declaration":
		w.addFunc(n)
		return
	case "class_declaration", "object_declaration":
		sc := scope{}
		if nm := n.ChildByFieldName("name"); nm != nil {
			sc.cls = w.t.Text(nm)
		}
		for _, c := range ts.NamedChildren(n) {
			if c.Kind() == "delegation_specifiers" {
				sc.bases = w.bases(c)
			}
		}
		w.stack = append(w.stack, sc)
		w.children(n)
		w.stack = w.stack[:len(w.stack)-1]
		return
	}
	w.children(n)
}

func (w *walker) children(n *sitter.Node) {
	for i := uint(0); i < n.ChildCount(); i++ {
		w.walk(n.Child(i))
	}
}

func (w *walker) addImport(n *sitter.Node) {
	var path, alias string
	star, as := false, false
	for _, c := range ts.Children(n) {
		switch c.Kind() {
		case "qualified_identifier", "identifier":
			if as {
				alias = w.t.Text(c)
			} else if path == "" {
				path = w.t.Text(c)
			}
		case "as":
			as = true
		case "*":
			star = true
		}
	}
	if path == "" {
		return
	}
	path = strings.Join(strings.Fields(path), "")
	if star {
		path += ".*"
	}
	w.out.Imports = append(w.out.Imports, lang.Import{Path: path, Alias: alias})
}

func (w *walker) bases(n *sitter.Node) []string {
	var out []string
	for _, ds := range ts.NamedChildren(n) {
		var tn *sitter.Node
		for _, c := range ts.NamedChildren(ds) {
			switch c.Kind() {
			case "constructor_invocation":
				for _, cc := range ts.NamedChildren(c) {
					if cc.Kind() == "user_type" {
						tn = cc
						break
					}
				}
			case "user_type":
				tn = c
			case "explicit_delegation":
				for _, cc := range ts.NamedChildren(c) {
					if cc.Kind() == "user_type" {
						tn = cc
						break
					}
				}
			}
			if tn != nil {
				break
			}
		}
		if tn != nil {
			out = append(out, collapse(w.t.Text(tn)))
		}
	}
	return out
}

var (
	spaces   = regexp.MustCompile(`\s+`)
	generics = regexp.MustCompile(`<[^<>]*>`)
)

func collapse(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }

// typeBase reduces a type as written ("Map<K, List<V>>?", "a.b.Foo") to its simple name.
func typeBase(s string) string {
	for {
		r := generics.ReplaceAllString(s, "")
		if r == s {
			break
		}
		s = r
	}
	s = strings.NewReplacer("?", "", "(", "", ")", "", " ", "").Replace(s)
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func clip(s string) string {
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (w *walker) addFunc(n *sitter.Node) {
	t := w.t
	var mods *sitter.Node
	var kids = ts.Children(n)
	for _, c := range kids {
		if c.Kind() == "modifiers" {
			mods = c
		}
	}
	// Name and extension receiver.
	name, recvType := "", ""
	nm := n.ChildByFieldName("name")
	if nm != nil {
		name = t.Text(nm)
	}
	for i, c := range kids {
		if c.Kind() == "function_value_parameters" {
			if name == "" && i > 0 {
				// Grammar quirk: `fun <T> List<T>.ext()` becomes one user_type "List<T>.ext".
				p := kids[i-1]
				if p.Kind() == "identifier" && i > 1 {
					p = kids[i-2]
				}
				if p.Kind() == "user_type" {
					ids := []string{}
					for _, cc := range ts.NamedChildren(p) {
						if cc.Kind() == "identifier" {
							ids = append(ids, t.Text(cc))
						}
					}
					if len(ids) >= 2 {
						name, recvType = ids[len(ids)-1], ids[len(ids)-2]
					}
				}
			}
			break
		}
	}
	if name == "" {
		return
	}
	if recvType == "" {
		for i, c := range kids {
			if c.Kind() == "." && i > 0 {
				recvType = typeBase(t.Text(kids[i-1]))
				break
			}
		}
	}
	cls, bases := "", []string(nil)
	if len(w.stack) > 0 {
		// innermost named type (unnamed companion is transparent)
		for i := len(w.stack) - 1; i >= 0; i-- {
			if w.stack[i].cls != "" {
				cls, bases = w.stack[i].cls, w.stack[i].bases
				break
			}
		}
	}
	if recvType != "" {
		cls, bases = recvType, nil
	}

	// Start: first non-annotation modifier, else `fun`.
	startNode := (*sitter.Node)(nil)
	var decos []string
	if mods != nil {
		for _, m := range ts.NamedChildren(mods) {
			if m.Kind() == "annotation" {
				decos = append(decos, strings.TrimPrefix(collapse(t.Text(m)), "@"))
			} else if startNode == nil {
				startNode = m
			}
		}
	}
	var body *sitter.Node
	for _, c := range kids {
		if c.Kind() == "fun" && startNode == nil {
			startNode = c
		}
		if c.Kind() == "function_body" {
			body = c
		}
	}
	if startNode == nil {
		startNode = n
	}
	f := lang.Func{
		Name: name, Cls: cls, Start: ts.StartLine(startNode), End: ts.EndLine(n),
		Decorators: decos, Bases: bases, Ext: recvType != "",
	}
	if l := ts.StartLine(n); l < f.Start {
		f.TStart = l
	}
	sigEnd := n.EndByte()
	if body != nil {
		sigEnd = body.StartByte()
	}
	sig := collapse(string(t.Src[startNode.StartByte():sigEnd]))
	f.Sig = clip(strings.TrimSpace(strings.TrimRight(sig, "={ ")))
	f.Doc = w.doc(n)

	var cl lang.CallList
	if body != nil {
		w.calls(body, &cl, name)
	}
	f.Calls = cl.Calls()
	w.out.Funcs = append(w.out.Funcs, f)
}

// doc returns the first line of the KDoc (or // comment) right above n.
func (w *walker) doc(n *sitter.Node) string {
	p := n.PrevSibling()
	if p == nil || (p.Kind() != "block_comment" && p.Kind() != "line_comment") ||
		int(n.StartPosition().Row)-int(p.EndPosition().Row) > 1 || !ts.OwnLine(w.t.Src, p) {
		return ""
	}
	if p.Kind() == "line_comment" {
		// first line of the contiguous // block
		first := p
		for q := first.PrevSibling(); q != nil && q.Kind() == "line_comment" && int(first.StartPosition().Row)-int(q.EndPosition().Row) == 1; q = q.PrevSibling() {
			first = q
		}
		return clip(strings.TrimSpace(strings.TrimLeft(w.t.Text(first), "/")))
	}
	txt := w.t.Text(p)
	if !strings.HasPrefix(txt, "/**") {
		return ""
	}
	for _, l := range strings.Split(txt, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(l, "/**"), "*"), "*/"))
		l = strings.TrimSpace(strings.TrimPrefix(l, "*"))
		if l != "" && !strings.HasPrefix(l, "@") {
			return clip(l)
		}
	}
	return ""
}

type rawCall struct {
	pos        uint
	name, recv string
}

func (w *walker) calls(body *sitter.Node, cl *lang.CallList, self string) {
	var raw []rawCall
	ts.Walk(body, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "call_expression":
			if n.NamedChildCount() == 0 {
				return true
			}
			callee := n.NamedChild(0)
			switch callee.Kind() {
			case "identifier":
				raw = append(raw, rawCall{callee.StartByte(), w.t.Text(callee), ""})
			case "navigation_expression":
				if last := callee.NamedChild(callee.NamedChildCount() - 1); last != nil && last.Kind() == "identifier" && callee.NamedChildCount() >= 2 {
					raw = append(raw, rawCall{last.StartByte(), w.t.Text(last), w.recv(callee.NamedChild(0))})
				}
			}
		case "binary_expression":
			// `f<T>(x)` is parsed as `(f < T) > (x)`.
			if r, ok := w.genericCall(n); ok {
				raw = append(raw, r)
			}
		}
		return true
	})
	sort.SliceStable(raw, func(i, j int) bool { return raw[i].pos < raw[j].pos })
	for _, r := range raw {
		if r.name == self && (r.recv == "" || r.recv == "this") {
			continue
		}
		cl.Add(r.name, r.recv)
	}
}

func (w *walker) genericCall(n *sitter.Node) (rawCall, bool) {
	op, l, r := n.ChildByFieldName("operator"), n.ChildByFieldName("left"), n.ChildByFieldName("right")
	if op == nil || l == nil || r == nil || w.t.Text(op) != ">" || r.Kind() != "parenthesized_expression" || l.Kind() != "binary_expression" {
		return rawCall{}, false
	}
	lop, ll := l.ChildByFieldName("operator"), l.ChildByFieldName("left")
	if lop == nil || ll == nil || w.t.Text(lop) != "<" {
		return rawCall{}, false
	}
	switch ll.Kind() {
	case "identifier":
		return rawCall{ll.StartByte(), w.t.Text(ll), ""}, true
	case "navigation_expression":
		if c := ll.NamedChildCount(); c >= 2 && ll.NamedChild(c-1).Kind() == "identifier" {
			last := ll.NamedChild(c - 1)
			return rawCall{last.StartByte(), w.t.Text(last), w.recv(ll.NamedChild(0))}, true
		}
	}
	return rawCall{}, false
}

var trailingComma = regexp.MustCompile(`,(\s*[)\]])`)

// blankTrailingCommas replaces the comma of `, )` / `, ]` with a space: the
// grammar does not accept a trailing comma in call arguments and loses the
// rest of the file. Offsets and line numbers are preserved.
func blankTrailingCommas(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	for _, m := range trailingComma.FindAllSubmatchIndex(src, -1) {
		out[m[0]] = ' '
	}
	return out
}

// recv renders the receiver expression: a plain dotted name or "?".
func (w *walker) recv(n *sitter.Node) string {
	switch n.Kind() {
	case "identifier":
		return w.t.Text(n)
	case "this_expression":
		return "this"
	case "super_expression":
		return "super"
	case "unary_expression":
		if op := n.ChildByFieldName("operator"); op != nil && w.t.Text(op) == "!!" {
			if a := n.ChildByFieldName("argument"); a != nil {
				return w.recv(a)
			}
		}
	case "navigation_expression":
		if c := n.NamedChildCount(); c >= 2 && n.NamedChild(c-1).Kind() == "identifier" {
			if r := w.recv(n.NamedChild(0)); r != "?" {
				return r + "." + w.t.Text(n.NamedChild(c-1))
			}
		}
	}
	return "?"
}
