package extract

import (
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Descriptions of tables and columns written in the code.
//
// SQL (migrations, schema.sql): `COMMENT ON TABLE t IS '…'`, `COMMENT ON COLUMN
// t.c IS '…'` (IS NULL clears), a `-- …` comment at the end of a column's line
// (or of an `ALTER TABLE … ADD COLUMN …;` line), `-- …` lines right above a
// column or above `ALTER TABLE … ADD COLUMN` (no blank line in between), and
// `-- …` lines right above `CREATE TABLE` for the table. Statements are read in
// file order, so the last description wins; the Down part of goose/migrate
// files is ignored as before. Directives (`-- +goose …`, `-- name: …`) and
// separator lines are not descriptions.
//
// The comments are kept through cleanSQL (which strips comments and long
// strings) as identifier-like tokens: `__amdocL_<hex>__` for comment lines above
// code, `__amdocT_<hex>__` for a comment after code on the same line,
// `__amstr_<hex>__` for the string literal of COMMENT ON.
//
// Go: the doc / line comment of a struct field mapped to a column fills a
// column that has no description yet (and the struct doc a table without one).
// A struct maps to a table by its name (Order, OrderRow, OrderModel → orders /
// order) or, when its fields carry db / gorm column tags, by the columns: at
// least two tagged fields, most of them columns of one table only.
//
// Django: help_text (else verbose_name) of a model field; the model docstring
// for the table.

var (
	sqlDocTok      = regexp.MustCompile(`__amdoc([LT])_([0-9a-f]*)__`)
	sqlStrTok      = regexp.MustCompile(`__amstr_([0-9a-f]*)__`)
	sqlCommentOnRE = regexp.MustCompile(`(?is)(comment\s+on\s+(?:table|column)\s+[\w."` + "`" + `]+\s+is\s+)[eE]?'((?:[^']|'')*)'`)
	sqlCommentStmt = regexp.MustCompile(`(?is)^comment\s+on\s+(table|column)\s+([\w."` + "`" + `]+)\s+is\s+(__amstr_[0-9a-f]*__|null)`)
	sqlDirective   = regexp.MustCompile(`^(?:\+(?:goose|migrate)\b|name:\s)`)
	sqlSeparator   = regexp.MustCompile(`^[-=*#~_\s]*$`)
	spaceRun       = regexp.MustCompile(`\s+`)
)

func docTok(kind byte, text string) string {
	return " __amdoc" + string(kind) + "_" + hex.EncodeToString([]byte(text)) + "__ "
}

func unhex(s string) string {
	b, err := hex.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// normDesc trims a description and folds its whitespace.
func normDesc(s string) string {
	return strings.TrimSpace(spaceRun.ReplaceAllString(s, " "))
}

// normDescLines keeps the line breaks of a multi-line description (lists in COMMENT ON),
// trimming each line and dropping the empty ones.
func normDescLines(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = normDesc(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// lineComment returns the index of `--` outside single-quoted strings, or -1.
func lineComment(line string) int {
	in := false
	for i := 0; i+1 < len(line); i++ {
		switch {
		case line[i] == '\'':
			in = !in
		case !in && line[i] == '-' && line[i+1] == '-':
			return i
		}
	}
	return -1
}

// encodeSQLDocs turns the comments that describe tables and columns into tokens
// that survive cleanSQL (see the package comment above).
func encodeSQLDocs(src string) string {
	src = sqlCommentOnRE.ReplaceAllStringFunc(src, func(m string) string {
		p := sqlCommentOnRE.FindStringSubmatch(m)
		return p[1] + "__amstr_" + hex.EncodeToString([]byte(strings.ReplaceAll(p[2], "''", "'"))) + "__"
	})
	lines := strings.Split(src, "\n")
	var pending []string
	for i, line := range lines {
		ci := lineComment(line)
		code, text := line, ""
		if ci >= 0 {
			code, text = line[:ci], strings.TrimSpace(strings.TrimLeft(line[ci:], "-"))
		}
		if strings.TrimSpace(code) == "" {
			switch {
			case ci < 0: // a blank line: the comment above describes nothing
				pending = nil
			case sqlDirective.MatchString(text) || sqlSeparator.MatchString(text):
			default:
				pending = append(pending, text)
			}
			lines[i] = ""
			continue
		}
		var b strings.Builder
		if len(pending) > 0 {
			b.WriteString(docTok('L', strings.Join(pending, " ")))
			pending = nil
		}
		b.WriteString(code)
		if text != "" && !sqlDirective.MatchString(text) && !sqlSeparator.MatchString(text) {
			b.WriteString(docTok('T', text))
		}
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

type sqlDoc struct {
	trailing bool // a comment after code on the same line
	text     string
}

// splitDocs takes the doc tokens out of s: the ones before any code (lead) and
// the rest (inner), and returns s without tokens.
func splitDocs(s string) (plain string, lead, inner []sqlDoc) {
	ms := sqlDocTok.FindAllStringSubmatchIndex(s, -1)
	if ms == nil {
		return s, nil, nil
	}
	var b strings.Builder
	prev := 0
	for _, m := range ms {
		gap := s[prev:m[0]]
		b.WriteString(gap)
		d := sqlDoc{trailing: s[m[2]:m[3]] == "T", text: normDesc(unhex(s[m[4]:m[5]]))}
		if strings.TrimSpace(b.String()) == "" {
			lead = append(lead, d)
		} else {
			inner = append(inner, d)
		}
		prev = m[1]
		b.WriteByte(' ')
	}
	b.WriteString(s[prev:])
	return b.String(), lead, inner
}

func joinDocs(ds []sqlDoc, trailing bool) string {
	var parts []string
	for _, d := range ds {
		if d.trailing == trailing && d.text != "" {
			parts = append(parts, d.text)
		}
	}
	return strings.Join(parts, " ")
}

func fieldIndex(t *facts.Table, name string) int {
	for i, f := range t.Fields {
		if f.Name == name {
			return i
		}
	}
	return -1
}

// describe sets the description of a column (the last one written wins).
func (s *schemaBuilder) describe(t *facts.Table, col, desc string) {
	if t == nil || desc == "" {
		return
	}
	if i := fieldIndex(t, col); i >= 0 {
		t.Fields[i].Desc = desc
	}
}

// describeLast gives trailing comments that ended up after a comma or a
// semicolon to the column defined just before them.
func (s *schemaBuilder) describeLast(ds []sqlDoc) {
	if s.last.t != nil {
		s.describe(s.last.t, s.last.col, joinDocs(ds, true))
	}
}

// commentOn applies COMMENT ON TABLE / COLUMN.
func (s *schemaBuilder) commentOn(m []string) {
	kind, target, val := strings.ToLower(m[1]), m[2], ""
	if sm := sqlStrTok.FindStringSubmatch(m[3]); sm != nil {
		val = normDescLines(unhex(sm[1]))
	}
	if kind == "table" {
		if t := s.tables[normIdent(target)]; t != nil {
			t.Desc = val
		}
		return
	}
	i := strings.LastIndex(target, ".")
	if i < 0 {
		return
	}
	t, col := s.tables[normIdent(target[:i])], normIdent(target[i+1:])
	if t == nil {
		return
	}
	if j := fieldIndex(t, col); j >= 0 {
		t.Fields[j].Desc = val
	}
}

// ---- Go structs ----

var goTagHint = regexp.MustCompile("(?:db|gorm|json|bun|pg):\"")

type goStructDoc struct {
	name   string
	doc    string
	fields []goFieldDoc
	tagged bool // at least one db/gorm/bun/pg column tag
}

type goFieldDoc struct {
	col    string // column name: the tag, else snake_case of the field name
	tagged bool
	doc    string
}

// goStructDocs reads the structs with field comments from the Go files.
func (c *apiCtx) goStructDocs() []goStructDoc {
	var out []goStructDoc
	fset := token.NewFileSet()
	for _, p := range c.paths {
		if path.Ext(p) != ".go" || isTestPath(p) || strings.HasSuffix(p, ".pb.go") {
			continue
		}
		src := c.read(p)
		if !strings.Contains(src, "struct {") || !goTagHint.MatchString(src) || !strings.Contains(src, "//") {
			continue
		}
		f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts, ok := sp.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				sd := goStructDoc{name: ts.Name.Name, doc: cgText(ts.Doc)}
				if sd.doc == "" && len(gd.Specs) == 1 {
					sd.doc = cgText(gd.Doc)
				}
				sd.doc = trimGoDoc(sd.doc, ts.Name.Name)
				for _, fl := range st.Fields.List {
					col, tagged, skip := goColumn(fl)
					if skip {
						continue
					}
					doc := cgText(fl.Doc)
					if doc == "" {
						doc = cgText(fl.Comment)
					}
					for _, n := range fl.Names {
						c := col
						if c == "" {
							c = goSnake(n.Name)
						}
						sd.fields = append(sd.fields, goFieldDoc{col: c, tagged: tagged, doc: trimGoDoc(doc, n.Name)})
						sd.tagged = sd.tagged || tagged
					}
				}
				out = append(out, sd)
			}
		}
	}
	return out
}

func cgText(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	return normDesc(g.Text())
}

// trimGoDoc drops the Go habit of starting a doc with the name ("ID is the id" stays as is,
// "Order — a customer order" becomes "a customer order").
func trimGoDoc(doc, name string) string {
	for _, sep := range []string{" — ", " – ", " - ", ": "} {
		if strings.HasPrefix(doc, name+sep) {
			return strings.TrimSpace(doc[len(name+sep):])
		}
	}
	return doc
}

// goColumn reads the column name of a struct field from its tags; skip for `db:"-"`.
func goColumn(fl *ast.Field) (col string, tagged, skip bool) {
	if fl.Tag == nil {
		return "", false, false
	}
	tag := reflect.StructTag(strings.Trim(fl.Tag.Value, "`"))
	first := func(v string) string { return strings.TrimSpace(strings.SplitN(v, ",", 2)[0]) }
	for _, k := range []string{"db", "bun", "pg"} {
		if v, ok := tag.Lookup(k); ok {
			if n := first(v); n == "-" {
				return "", false, true
			} else if n != "" {
				return strings.ToLower(n), true, false
			}
		}
	}
	if v, ok := tag.Lookup("gorm"); ok {
		for _, part := range strings.Split(v, ";") {
			if strings.HasPrefix(part, "column:") {
				return strings.ToLower(strings.TrimPrefix(part, "column:")), true, false
			}
		}
		if v == "-" {
			return "", false, true
		}
	}
	if v, ok := tag.Lookup("json"); ok {
		if n := first(v); n != "" && n != "-" {
			return strings.ToLower(n), false, false
		}
	}
	return "", false, false
}

// tableNameCandidates: the table names a struct name may stand for.
func tableNameCandidates(name string) []string {
	base := name
	for _, suf := range []string{"Row", "Model", "Record", "Entity", "DB", "Db"} {
		if strings.HasSuffix(base, suf) && len(base) > len(suf) {
			base = strings.TrimSuffix(base, suf)
			break
		}
	}
	sn := goSnake(base)
	out := []string{sn, sn + "s", sn + "es"}
	if strings.HasSuffix(sn, "y") {
		out = append(out, strings.TrimSuffix(sn, "y")+"ies")
	}
	return out
}

// goDocs fills the descriptions still empty from the comments of the Go structs mapped to tables.
func (c *apiCtx) goDocs(sb *schemaBuilder) {
	if len(sb.tables) == 0 {
		return
	}
	structs := c.goStructDocs()
	sort.SliceStable(structs, func(i, j int) bool { return structs[i].name < structs[j].name })
	for _, sd := range structs {
		hasDoc := sd.doc != ""
		for _, f := range sd.fields {
			hasDoc = hasDoc || f.doc != ""
		}
		if !hasDoc {
			continue
		}
		t := sb.tableOfStruct(sd)
		if t == nil {
			continue
		}
		if t.Desc == "" {
			t.Desc = sd.doc
		}
		for _, f := range sd.fields {
			if i := fieldIndex(t, f.col); i >= 0 && t.Fields[i].Desc == "" && f.doc != "" {
				t.Fields[i].Desc = f.doc
			}
		}
	}
}

// tableOfStruct: the table a struct maps to, or nil.
func (s *schemaBuilder) tableOfStruct(sd goStructDoc) *facts.Table {
	covered := func(t *facts.Table, taggedOnly bool) (hit, n int) {
		for _, f := range sd.fields {
			if taggedOnly && !f.tagged {
				continue
			}
			n++
			if fieldIndex(t, f.col) >= 0 {
				hit++
			}
		}
		return
	}
	for _, name := range tableNameCandidates(sd.name) {
		if t := s.tables[name]; t != nil {
			if hit, _ := covered(t, false); hit >= 1 {
				return t
			}
		}
	}
	if !sd.tagged {
		return nil
	}
	var best *facts.Table
	bestHit, ties := 0, 0
	names := make([]string, 0, len(s.tables))
	for n := range s.tables {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t := s.tables[n]
		hit, all := covered(t, true)
		if hit < 2 || hit*4 < all*3 {
			continue
		}
		switch {
		case hit > bestHit:
			best, bestHit, ties = t, hit, 0
		case hit == bestHit:
			ties++
		}
	}
	if ties > 0 {
		return nil
	}
	return best
}

// ---- Django ----

var (
	djHelpRE    = regexp.MustCompile(`(?s)\bhelp_text\s*=\s*(?:_|gettext_lazy|gettext)?\(?\s*(?:[rRuU])?(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")`)
	djVerboseRE = regexp.MustCompile(`(?s)\bverbose_name\s*=\s*(?:_|gettext_lazy|gettext)?\(?\s*(?:[rRuU])?(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")`)
	pyDocRE     = regexp.MustCompile(`(?s)^class[^\n]*:\s*\n\s*(?:[rRuU])?("""|''')(.*?)("""|''')`)
)

// djFieldDesc: help_text of a Django field's arguments, else verbose_name.
func djFieldDesc(args string) string {
	for _, re := range []*regexp.Regexp{djHelpRE, djVerboseRE} {
		if m := re.FindStringSubmatch(args); m != nil {
			return normDesc(m[1] + m[2])
		}
	}
	return ""
}

// pyClassDoc: the docstring of a class.
func pyClassDoc(text string) string {
	if m := pyDocRE.FindStringSubmatch(text); m != nil {
		return normDesc(m[2])
	}
	return ""
}

// goSnake: snake_case of a Go identifier with acronyms kept whole (UserID → user_id, HTTPCode → http_code).
func goSnake(s string) string {
	rs := []rune(s)
	var b strings.Builder
	isUp := func(r rune) bool { return r >= 'A' && r <= 'Z' }
	for i, r := range rs {
		if i > 0 && isUp(r) && (!isUp(rs[i-1]) || (i+1 < len(rs) && !isUp(rs[i+1]) && rs[i+1] != '_')) && rs[i-1] != '_' {
			b.WriteByte('_')
		}
		b.WriteString(strings.ToLower(string(r)))
	}
	return b.String()
}
