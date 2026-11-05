package render

import (
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// The parser of top-level declarations used by TestAssembledScriptCompiles (render_test.go).

func TestTopDecls(t *testing.T) {
	src := "const a = 1, b = f(x, y), { c, d: e, ...g } = h;\nlet [k, , l = 2] = m;\n" +
		"function n() { const inner = 1; }\nasync function o() {}\nclass P { q() { let r; } }\n" +
		"const s = `x ${ { t: 1 }.t } y`, u = /[{(]'/.test(v) ? 1 : 2;\n" +
		"// const w = 1;\n/* let x2 = 2; */\nif (z) { var hidden = 1; }\nconst re = a / 2, last = '}';\n"
	ds, err := topDecls(src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range ds {
		got = append(got, d.name)
	}
	sort.Strings(got)
	want := []string{"P", "a", "b", "c", "e", "g", "k", "l", "last", "n", "o", "re", "s", "u"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

type jsDecl struct {
	name    string
	lexical bool // const / let / class: a second declaration in the same scope is an error
}

type jsTok struct {
	kind byte // 'i' identifier, 'p' punctuation, 's' string / template / regex / number
	text string
	line bool // first token of its line
}

// jsTokens splits JavaScript into identifiers, punctuation and literals; comments are dropped, strings, templates
// (with their ${} parts) and regular expressions become one literal token each.
func jsTokens(src string) ([]jsTok, error) {
	var out []jsTok
	r := []rune(src)
	n := len(r)
	nl := true
	// a '/' starts a regex unless the previous token ends an expression
	regexOK := func() bool {
		if len(out) == 0 {
			return true
		}
		p := out[len(out)-1]
		switch p.kind {
		case 's':
			return false
		case 'i':
			switch p.text {
			case "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "throw", "case", "do", "else", "yield", "await":
				return true
			}
			return false
		}
		return !strings.Contains(")]}", p.text)
	}
	var tmpl func(i int) (int, error)
	// skip a string from its opening quote at i; returns the index after it
	str := func(i int) (int, error) {
		q := r[i]
		for i++; i < n; i++ {
			switch r[i] {
			case '\\':
				i++
			case q:
				return i + 1, nil
			case '\n':
				return 0, errAt(r, i, "unterminated string")
			}
		}
		return 0, errAt(r, i, "unterminated string")
	}
	// skip the code inside ${ … } starting at i (after "${"); returns the index after the closing brace
	code := func(i int) (int, error) {
		d := 0
		for i < n {
			c := r[i]
			switch {
			case c == '\'' || c == '"':
				k, err := str(i)
				if err != nil {
					return 0, err
				}
				i = k
				continue
			case c == '`':
				k, err := tmpl(i)
				if err != nil {
					return 0, err
				}
				i = k
				continue
			case c == '{':
				d++
			case c == '}':
				if d == 0 {
					return i + 1, nil
				}
				d--
			}
			i++
		}
		return 0, errAt(r, i, "unterminated template expression")
	}
	tmpl = func(i int) (int, error) {
		for i++; i < n; i++ {
			switch {
			case r[i] == '\\':
				i++
			case r[i] == '`':
				return i + 1, nil
			case r[i] == '$' && i+1 < n && r[i+1] == '{':
				k, err := code(i + 2)
				if err != nil {
					return 0, err
				}
				i = k - 1
			}
		}
		return 0, errAt(r, i, "unterminated template")
	}
	push := func(kind byte, text string) { out = append(out, jsTok{kind, text, nl}); nl = false }
	for i := 0; i < n; {
		c := r[i]
		switch {
		case c == '\n':
			nl = true
			i++
		case unicode.IsSpace(c):
			i++
		case c == '/' && i+1 < n && r[i+1] == '/':
			for i < n && r[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && r[i+1] == '*':
			k := i + 2
			for ; k+1 < n && !(r[k] == '*' && r[k+1] == '/'); k++ {
				if r[k] == '\n' {
					nl = true
				}
			}
			if k+1 >= n {
				return nil, errAt(r, i, "unterminated comment")
			}
			i = k + 2
		case c == '\'' || c == '"':
			k, err := str(i)
			if err != nil {
				return nil, err
			}
			push('s', string(r[i:k]))
			i = k
		case c == '`':
			k, err := tmpl(i)
			if err != nil {
				return nil, err
			}
			push('s', "`…`")
			i = k
		case c == '/' && regexOK():
			k, cls := i+1, false
			for ; k < n; k++ {
				if r[k] == '\\' {
					k++
				} else if r[k] == '[' {
					cls = true
				} else if r[k] == ']' {
					cls = false
				} else if r[k] == '/' && !cls {
					break
				} else if r[k] == '\n' {
					return nil, errAt(r, k, "unterminated regex")
				}
			}
			k++
			for k < n && (unicode.IsLetter(r[k])) {
				k++
			}
			push('s', "/re/")
			i = k
		case c == '_' || c == '$' || unicode.IsLetter(c):
			k := i
			for k < n && (r[k] == '_' || r[k] == '$' || unicode.IsLetter(r[k]) || unicode.IsDigit(r[k])) {
				k++
			}
			push('i', string(r[i:k]))
			i = k
		case unicode.IsDigit(c) || (c == '.' && i+1 < n && unicode.IsDigit(r[i+1])):
			k := i
			for k < n && (unicode.IsDigit(r[k]) || unicode.IsLetter(r[k]) || r[k] == '.' || r[k] == '_') {
				k++
			}
			push('s', string(r[i:k]))
			i = k
		case c == '.' && i+2 < n && r[i+1] == '.' && r[i+2] == '.':
			push('p', "...")
			i += 3
		default:
			push('p', string(c))
			i++
		}
	}
	return out, nil
}

type jsErr string

func (e jsErr) Error() string { return string(e) }

func errAt(r []rune, i int, msg string) error {
	if i > len(r) {
		i = len(r)
	}
	return jsErr(msg + " at line " + strconv.Itoa(strings.Count(string(r[:i]), "\n")+1))
}

// topDecls: the names declared at the top level of a script — function / async function / class / const / let /
// var (every declarator, destructuring patterns included).
func topDecls(src string) ([]jsDecl, error) {
	toks, err := jsTokens(src)
	if err != nil {
		return nil, err
	}
	var out []jsDecl
	depth := 0
	isOpen := func(s string) bool { return s == "{" || s == "(" || s == "[" }
	isClose := func(s string) bool { return s == "}" || s == ")" || s == "]" }
	// a statement starts after ; or } at the top level, or at the start of a line
	stmtStart := func(k int) bool {
		if k == 0 {
			return true
		}
		p := toks[k-1]
		return (p.kind == 'p' && (p.text == ";" || p.text == "}")) || toks[k].line
	}
	// names of a binding pattern starting at k; returns them and the index after the pattern
	var pattern func(k int) ([]string, int)
	pattern = func(k int) ([]string, int) {
		if k >= len(toks) {
			return nil, k
		}
		tk := toks[k]
		if tk.kind == 'i' {
			return []string{tk.text}, k + 1
		}
		if tk.text != "{" && tk.text != "[" {
			return nil, k + 1
		}
		obj, closer := tk.text == "{", map[string]string{"{": "}", "[": "]"}[tk.text]
		var names []string
		k++
		for k < len(toks) && toks[k].text != closer {
			switch {
			case toks[k].text == ",":
				k++
			case toks[k].text == "...":
				ns, nk := pattern(k + 1)
				names, k = append(names, ns...), nk
			case obj && toks[k].kind == 'i' && k+1 < len(toks) && toks[k+1].text == ":":
				ns, nk := pattern(k + 2)
				names, k = append(names, ns...), nk
			default:
				ns, nk := pattern(k)
				names, k = append(names, ns...), nk
			}
			// a default value: skip to the next , or the closer at this level
			if k < len(toks) && toks[k].text == "=" {
				d := 0
				for k++; k < len(toks); k++ {
					s := toks[k].text
					if toks[k].kind == 'p' && isOpen(s) {
						d++
					} else if toks[k].kind == 'p' && isClose(s) {
						if d == 0 {
							break
						}
						d--
					} else if d == 0 && s == "," {
						break
					}
				}
			}
		}
		return names, k + 1
	}
	for k := 0; k < len(toks); k++ {
		tk := toks[k]
		if tk.kind == 'p' {
			if isOpen(tk.text) {
				depth++
			} else if isClose(tk.text) {
				depth--
				if depth < 0 {
					return nil, jsErr("unbalanced brackets")
				}
			}
			continue
		}
		if depth != 0 || tk.kind != 'i' || !stmtStart(k) {
			continue
		}
		kw := tk.text
		if kw == "async" && k+1 < len(toks) && toks[k+1].text == "function" {
			k++
			kw = "function"
		}
		switch kw {
		case "function", "class":
			m := k + 1
			if m < len(toks) && toks[m].text == "*" {
				m++
			}
			if m < len(toks) && toks[m].kind == 'i' {
				out = append(out, jsDecl{toks[m].text, kw == "class"})
			}
		case "const", "let", "var":
			m := k + 1
			for m < len(toks) {
				ns, nm := pattern(m)
				for _, x := range ns {
					out = append(out, jsDecl{x, kw != "var"})
				}
				m = nm
				// the initializer: up to a , or ; at this level (or the end of the statement on a new line)
				d := 0
				for ; m < len(toks); m++ {
					s := toks[m]
					if s.kind == 'p' && isOpen(s.text) {
						d++
					} else if s.kind == 'p' && isClose(s.text) {
						d--
					} else if d == 0 && s.kind == 'p' && (s.text == "," || s.text == ";") {
						break
					}
				}
				if m >= len(toks) || toks[m].text == ";" {
					break
				}
				m++ // past the comma: the next declarator
			}
			k = m - 1
		}
	}
	if depth != 0 {
		return nil, jsErr("unbalanced brackets at the end")
	}
	return out, nil
}
