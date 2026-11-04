package extract

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// hit is one table access found in a piece of text; off is the byte offset of the statement.
type hit struct {
	table, op string
	fields    []string
	off       int
}

const tblPat = "([\\w.\"`]+)"

var (
	sqlInsertRE = regexp.MustCompile(`(?i)\binsert\s+(?:ignore\s+)?into\s+(?:only\s+)?` + tblPat + `\s*(?:\(([^)]*)\))?`)
	sqlUpdateRE = regexp.MustCompile(`(?i)\bupdate\s+(?:only\s+)?` + tblPat + `(?:\s+(?:as\s+)?\w+)?\s+set\b`)
	sqlDeleteRE = regexp.MustCompile(`(?i)\bdelete\s+from\s+(?:only\s+)?` + tblPat)
	sqlFromRE   = regexp.MustCompile(`(?i)\b(from|join)\s+(?:only\s+)?` + tblPat)
	sqlUpsertRE = regexp.MustCompile(`(?i)\bon\s+conflict\b|\bon\s+duplicate\s+key\b`)
	sqlNextStmt = regexp.MustCompile(`(?i)\b(insert|update|delete|select)\b`)
	sqlSetEnd   = regexp.MustCompile(`(?i)\bwhere\b|\breturning\b|\bfrom\b|;|"|` + "`")
	assignLHS   = regexp.MustCompile(`^\s*"?(\w+)"?\s*=`)
	plainIdent  = regexp.MustCompile(`^\w+$`)
	deleteBefRE = regexp.MustCompile(`(?i)delete\s+$`)
	replaceRE   = regexp.MustCompile(`(?i)\breplace\s+into\b`)
)

// analyzeSQL finds table accesses in SQL-looking text. With a known schema only
// known tables count; without one only writes are accepted.
func analyzeSQL(text string, known map[string]*facts.Table) []hit {
	var hs []hit
	ok := func(t string) bool {
		if len(known) > 0 {
			_, in := known[t]
			return in
		}
		return plainIdent.MatchString(t)
	}
	for _, m := range sqlInsertRE.FindAllStringSubmatchIndex(text, -1) {
		t := normIdent(text[m[2]:m[3]])
		if !ok(t) {
			continue
		}
		op := "insert"
		tail := text[m[1]:min(len(text), m[1]+800)]
		if n := sqlNextStmt.FindStringIndex(tail); n != nil && !strings.EqualFold(tail[n[0]:min(len(tail), n[0]+6)], "select") {
			tail = tail[:n[0]]
		}
		if i := strings.Index(tail, ";"); i >= 0 {
			tail = tail[:i]
		}
		if sqlUpsertRE.MatchString(tail) || replaceRE.MatchString(text[m[0]:m[1]]) {
			op = "upsert"
		}
		var fs []string
		if m[4] >= 0 {
			for _, f := range strings.Split(text[m[4]:m[5]], ",") {
				if f = normIdent(f); f != "" && plainIdent.MatchString(f) {
					fs = append(fs, f)
				}
			}
		}
		hs = append(hs, hit{t, op, fs, m[0]})
	}
	for _, m := range sqlUpdateRE.FindAllStringSubmatchIndex(text, -1) {
		t := normIdent(text[m[2]:m[3]])
		if !ok(t) {
			continue
		}
		rest := text[m[1]:]
		if e := sqlSetEnd.FindStringIndex(rest); e != nil {
			rest = rest[:e[0]]
		}
		var fs []string
		for _, a := range splitTop(rest, ',') {
			if am := assignLHS.FindStringSubmatch(a); am != nil {
				fs = append(fs, strings.ToLower(am[1]))
			}
		}
		hs = append(hs, hit{t, "update", fs, m[0]})
	}
	for _, m := range sqlDeleteRE.FindAllStringSubmatchIndex(text, -1) {
		if t := normIdent(text[m[2]:m[3]]); ok(t) {
			hs = append(hs, hit{t, "delete", nil, m[0]})
		}
	}
	if len(known) > 0 {
		for _, m := range sqlFromRE.FindAllStringSubmatchIndex(text, -1) {
			if strings.EqualFold(text[m[2]:m[3]], "from") && deleteBefRE.MatchString(text[max(0, m[0]-12):m[0]]) {
				continue
			}
			if t := normIdent(text[m[4]:m[5]]); ok(t) {
				hs = append(hs, hit{t, "read", nil, m[0]})
			}
		}
	}
	return hs
}

// accessSet accumulates accesses of one function, merged by (table, op).
type accessSet struct {
	list []facts.Access
	idx  map[string]int
}

func (a *accessSet) add(table, op string, fields []string, line int) {
	if a.idx == nil {
		a.idx = map[string]int{}
	}
	k := table + "|" + op
	if i, ok := a.idx[k]; ok {
		for _, f := range fields {
			if !contains(a.list[i].Fields, f) {
				a.list[i].Fields = append(a.list[i].Fields, f)
			}
		}
		return
	}
	a.idx[k] = len(a.list)
	a.list = append(a.list, facts.Access{Table: table, Op: op, Fields: append([]string(nil), fields...), Line: line})
}

var queryNameRE = regexp.MustCompile(`(?m)^\s*--\s*name:\s*(\w+)\s+:(\w+)`)

// loadSQLC reads sqlc query files (-- name: X :one) and returns the Go package names sqlc generates.
func (c *apiCtx) loadSQLC() (pkgs []string) {
	c.sqlcQ = map[string][]facts.Access{}
	for _, p := range c.paths {
		if path.Ext(p) == ".sql" {
			src := c.read(p)
			ms := queryNameRE.FindAllStringSubmatchIndex(src, -1)
			for i, m := range ms {
				end := len(src)
				if i+1 < len(ms) {
					end = ms[i+1][0]
				}
				name := src[m[2]:m[3]]
				var as accessSet
				for _, h := range analyzeSQL(cleanSQL(src[m[1]:end]), c.schema.Tables) {
					as.add(h.table, h.op, h.fields, 0)
				}
				if len(as.list) > 0 {
					c.sqlcQ[name] = as.list
				}
			}
		}
		if b := path.Base(p); b == "sqlc.yaml" || b == "sqlc.yml" || b == "sqlc.json" {
			for _, m := range regexp.MustCompile(`package"?:\s*"?(\w+)`).FindAllStringSubmatch(c.read(p), -1) {
				pkgs = append(pkgs, m[1])
			}
		}
	}
	for n := range c.sqlcQ {
		c.sqlcNames = append(c.sqlcNames, n)
	}
	sort.Strings(c.sqlcNames)
	return pkgs
}

// ---- go-jet ----

var (
	jetTableRE = regexp.MustCompile(`var\s+(\w+)\s*=\s*new\w+Table\(\s*"[^"]*"\s*,\s*"(\w+)"`)
	jetColRE   = regexp.MustCompile(`(\w+)Column\s*:?=\s*\w+\.\w*Column\w*\("(\w+)"\)`)
)

type jetInfo struct {
	tables map[string]string            // Go name -> table
	cols   map[string]map[string]string // table -> Go field -> column
}

func (c *apiCtx) loadJet() *jetInfo {
	ji := &jetInfo{tables: map[string]string{}, cols: map[string]map[string]string{}}
	for _, p := range c.paths {
		if path.Ext(p) != ".go" || !strings.Contains(p, "/table/") {
			continue
		}
		src := c.read(p)
		if !strings.Contains(src, "go-jet") {
			continue
		}
		m := jetTableRE.FindStringSubmatch(src)
		if m == nil {
			continue
		}
		ji.tables[m[1]] = m[2]
		cm := map[string]string{}
		for _, x := range jetColRE.FindAllStringSubmatch(src, -1) {
			cm[x[1]] = x[2]
		}
		ji.cols[m[2]] = cm
	}
	return ji
}

var (
	jetAliasRE = regexp.MustCompile(`(\w+)\s*:?=\s*(?:\w+\.)?table\.([A-Z]\w*)\b`)
	jetStmtRE  = regexp.MustCompile(`(?:\b(?:\w+\.)?table\.([A-Z]\w*)|\b([a-z_]\w*))\.(INSERT|UPDATE|DELETE|INNER_JOIN|LEFT_JOIN|RIGHT_JOIN|FULL_JOIN|CROSS_JOIN)\(`)
	jetFromRE  = regexp.MustCompile(`\.FROM\(\s*(?:(?:\w+\.)?table\.([A-Z]\w*)|([a-z_]\w*))\s*[,)]`)
	jetJoinRE  = regexp.MustCompile(`\.(?:INNER|LEFT|RIGHT|FULL|CROSS)_JOIN\(\s*(?:(?:\w+\.)?table\.([A-Z]\w*)|([a-z_]\w*))\s*[,)]`)
	jetConfl   = regexp.MustCompile(`\.ON_CONFLICT\(`)
	jetColUse  = regexp.MustCompile(`\.([A-Z]\w*)`)
)

func sub(s string, a, b int) string {
	if a < 0 {
		return ""
	}
	return s[a:b]
}

func (c *apiCtx) jetAccess(body string, ji *jetInfo, emit func(table, op string, fields []string, off int)) {
	if len(ji.tables) == 0 || !strings.Contains(body, "table.") {
		return
	}
	alias := map[string]string{}
	for _, m := range jetAliasRE.FindAllStringSubmatch(body, -1) {
		if t, ok := ji.tables[m[2]]; ok {
			alias[m[1]] = t
		}
	}
	resolve := func(goName, v string) string {
		if goName != "" {
			return ji.tables[goName]
		}
		return alias[v]
	}
	for _, m := range jetStmtRE.FindAllStringSubmatchIndex(body, -1) {
		t := resolve(sub(body, m[2], m[3]), sub(body, m[4], m[5]))
		if t == "" {
			continue
		}
		verb := body[m[6]:m[7]]
		open := m[1] - 1
		var fields []string
		if end := matchParen(body, open); end > open {
			for _, fm := range jetColUse.FindAllStringSubmatch(body[open:end], -1) {
				if col, ok := ji.cols[t][fm[1]]; ok {
					fields = append(fields, col)
				}
			}
		}
		switch verb {
		case "INSERT":
			op := "insert"
			tail := body[m[1]:min(len(body), m[1]+700)]
			if i := strings.Index(tail, ".Sql()"); i >= 0 {
				tail = tail[:i]
			}
			if jetConfl.MatchString(tail) {
				op = "upsert"
			}
			emit(t, op, fields, m[0])
		case "UPDATE":
			emit(t, "update", fields, m[0])
		case "DELETE":
			emit(t, "delete", nil, m[0])
		default:
			emit(t, "read", nil, m[0])
		}
	}
	for _, rx := range []*regexp.Regexp{jetFromRE, jetJoinRE} {
		for _, m := range rx.FindAllStringSubmatchIndex(body, -1) {
			if t := resolve(sub(body, m[2], m[3]), sub(body, m[4], m[5])); t != "" {
				emit(t, "read", nil, m[0])
			}
		}
	}
}

// ---- Django ORM ----

var (
	djChainRE    = regexp.MustCompile(`^\s*\.(\w+)\s*\(`)
	djVarNewRE   = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*(\w+)\(`)
	djVarGetRE   = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*(?:await\s+)?(\w+)\.objects\.(?:get|create|get_or_create|first|last)\(`)
	djVarFilter  = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*(\w+)\.objects\b.*\.(?:first|last|get)\(`)
	djGetOr404RE = regexp.MustCompile(`(?m)(?:^\s*(\w+)\s*=\s*)?\b(?:get_object_or_404|get_list_or_404)\(\s*(\w+)\b`)
	djKwRE       = regexp.MustCompile(`^\s*(\w+)\s*=[^=]`)
	djWriteMeths = map[string]string{"create": "insert", "bulk_create": "insert", "update": "update", "bulk_update": "update",
		"delete": "delete", "get_or_create": "upsert", "update_or_create": "upsert"}
	djQuoted      = regexp.MustCompile(`['"](\w+)['"]`)
	djQuotedKey   = regexp.MustCompile(`['"](\w+)['"]\s*:`)
	djBareKw      = regexp.MustCompile(`\b(\w+)\s*=`)
	djUpdateField = regexp.MustCompile(`update_fields\s*=\s*[\[(]([^\])]*)`)
)

func (c *apiCtx) djangoAccess(body string, emit func(table, op string, fields []string, off int)) {
	if !strings.Contains(body, "objects") && !strings.Contains(body, ".save(") && !strings.Contains(body, "_or_404") && !strings.Contains(body, "_default_manager") {
		return
	}
	if c.djObjRE == nil {
		var names []string
		for n, t := range c.modelTbl {
			if t != "" {
				names = append(names, regexp.QuoteMeta(n))
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			c.djObjRE = regexp.MustCompile(`\bNEVERMATCH\b`)
		} else {
			c.djObjRE = regexp.MustCompile(`\b(` + strings.Join(names, "|") + `)\.(?:objects|_default_manager)\b`)
		}
	}
	for _, m := range c.djObjRE.FindAllStringSubmatchIndex(body, -1) {
		model := body[m[2]:m[3]]
		rest := body[m[1]:]
		op, fields := "read", []string(nil)
		pos := 0
		for {
			cm := djChainRE.FindStringSubmatchIndex(rest[pos:])
			if cm == nil {
				break
			}
			name := rest[pos+cm[2] : pos+cm[3]]
			open := pos + cm[1] - 1
			end := matchParen(rest, open)
			if end < 0 {
				break
			}
			if w, ok := djWriteMeths[name]; ok {
				op = w
				fields = append(fields, djFields(name, rest[open+1:end])...)
			}
			pos = end + 1
		}
		emit(c.modelTbl[model], op, fields, m[0])
	}
	// instances: obj = Model(...); obj.save() / obj.delete()
	vars := map[string][2]string{} // var -> (model, "new"|"db")
	valid := func(n string) bool { return c.modelTbl[n] != "" }
	for _, m := range djVarNewRE.FindAllStringSubmatch(body, -1) {
		if valid(m[2]) {
			vars[m[1]] = [2]string{m[2], "new"}
		}
	}
	for _, rx := range []*regexp.Regexp{djVarGetRE, djVarFilter} {
		for _, m := range rx.FindAllStringSubmatch(body, -1) {
			if valid(m[2]) {
				vars[m[1]] = [2]string{m[2], "db"}
			}
		}
	}
	for _, m := range djGetOr404RE.FindAllStringSubmatchIndex(body, -1) {
		model := body[m[4]:m[5]]
		if !valid(model) {
			continue
		}
		emit(c.modelTbl[model], "read", nil, m[0])
		if m[2] >= 0 {
			vars[body[m[2]:m[3]]] = [2]string{model, "db"}
		}
	}
	for v, mt := range vars {
		rx := regexp.MustCompile(`\b` + regexp.QuoteMeta(v) + `\.(save|delete)\(([^)]*)\)`)
		for _, m := range rx.FindAllStringSubmatchIndex(body, -1) {
			if body[m[2]:m[3]] == "delete" {
				emit(c.modelTbl[mt[0]], "delete", nil, m[0])
				continue
			}
			var fields []string
			if uf := djUpdateField.FindStringSubmatch(body[m[4]:m[5]]); uf != nil {
				for _, f := range djQuoted.FindAllStringSubmatch(uf[1], -1) {
					fields = append(fields, f[1])
				}
			}
			op := "update"
			if mt[1] == "new" {
				op = "insert"
			}
			emit(c.modelTbl[mt[0]], op, fields, m[0])
		}
	}
}

func djFields(method, args string) []string {
	var out []string
	switch method {
	case "bulk_update":
		if i := strings.Index(args, "["); i >= 0 {
			for _, f := range djQuoted.FindAllStringSubmatch(args[i:], -1) {
				out = append(out, f[1])
			}
		}
		return out
	case "update_or_create", "get_or_create":
		if i := strings.Index(args, "defaults"); i >= 0 {
			for _, f := range djQuotedKey.FindAllStringSubmatch(args[i:], -1) {
				out = append(out, f[1])
			}
			for _, f := range djBareKw.FindAllStringSubmatch(args[i:], -1) {
				if f[1] != "defaults" {
					out = append(out, f[1])
				}
			}
		}
		return out
	case "bulk_create", "delete":
		return nil
	}
	for _, a := range splitTop(args, ',') {
		if m := djKwRE.FindStringSubmatch(a); m != nil {
			switch m[1] {
			case "using", "batch_size", "ignore_conflicts":
			default:
				out = append(out, m[1])
			}
		}
	}
	return out
}

// ---- Prisma / TypeORM ----

var (
	prismaCallRE = regexp.MustCompile(`\b(?:prisma|db|tx|client|this\.prisma|this\.db|this\.client)\.(\w+)\.(find\w+|count|aggregate|groupBy|create|createMany|createManyAndReturn|update|updateMany|upsert|delete|deleteMany)\(`)
	tormCallRE   = regexp.MustCompile(`\bthis\.(\w+)\.(save|insert|update|upsert|delete|remove|softDelete|softRemove|find\w*|count\w*|exist\w*)\(`)
	tormGetRepo  = regexp.MustCompile(`getRepository\(\s*(\w+)\s*\)\.(save|insert|update|upsert|delete|remove|softDelete|find\w*|count\w*)\(`)
	prismaKeyRE  = regexp.MustCompile(`^\s*(\w+)\s*(?::|$)`)
)

func ormOp(m string) string {
	switch m {
	case "create", "createMany", "createManyAndReturn", "insert":
		return "insert"
	case "update", "updateMany", "softDelete", "softRemove":
		return "update"
	case "upsert", "save":
		return "upsert"
	case "delete", "deleteMany", "remove":
		return "delete"
	}
	return "read"
}

func (c *apiCtx) jsORMAccess(file, body string, emit func(table, op string, fields []string, off int)) {
	if len(c.prismaAcc) > 0 {
		for _, m := range prismaCallRE.FindAllStringSubmatchIndex(body, -1) {
			t, ok := c.prismaAcc[body[m[2]:m[3]]]
			if !ok {
				continue
			}
			op := ormOp(body[m[4]:m[5]])
			var fields []string
			if op != "read" && op != "delete" {
				open := m[1] - 1
				if end := matchParen(body, open); end > open {
					fields = prismaDataKeys(body[open:end])
				}
			}
			emit(t, op, fields, m[0])
		}
	}
	for _, m := range tormCallRE.FindAllStringSubmatchIndex(body, -1) {
		if ent := c.ormRepo[file+"::"+body[m[2]:m[3]]]; ent != "" {
			emit(c.modelTbl[ent], ormOp(body[m[4]:m[5]]), nil, m[0])
		}
	}
	for _, m := range tormGetRepo.FindAllStringSubmatchIndex(body, -1) {
		if t := c.modelTbl[body[m[2]:m[3]]]; t != "" {
			emit(t, ormOp(body[m[4]:m[5]]), nil, m[0])
		}
	}
}

// prismaDataKeys returns the top-level keys of `data: { ... }` in a Prisma call.
func prismaDataKeys(args string) []string {
	i := strings.Index(args, "data")
	if i < 0 {
		return nil
	}
	j := strings.Index(args[i:], "{")
	if j < 0 {
		return nil
	}
	start := i + j
	end := matchBrace(args, start)
	var keys []string
	for _, part := range splitTopBraces(args[start+1 : end]) {
		if m := prismaKeyRE.FindStringSubmatch(part); m != nil {
			keys = append(keys, m[1])
		}
	}
	return keys
}

func splitTopBraces(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// ---- driver ----

// buildAccess finds, for every function, which tables it reads and writes.
func (c *apiCtx) buildAccess() map[string][]facts.Access {
	known := c.schema.Tables
	sqlcPkgs := c.loadSQLC()
	var sqlcCall *regexp.Regexp
	if len(c.sqlcNames) > 0 {
		quoted := make([]string, len(c.sqlcNames))
		for i, n := range c.sqlcNames {
			quoted[i] = regexp.QuoteMeta(n)
		}
		sqlcCall = regexp.MustCompile(`([\w.]*)\.(` + strings.Join(quoted, "|") + `)\(`)
	}
	ji := c.loadJet()
	out := map[string][]facts.Access{}
	for _, fid := range c.order {
		f := c.fns[fid]
		body, start := c.body(fid)
		if body == "" {
			continue
		}
		var as accessSet
		lineOf := func(off int) int { return start + strings.Count(body[:min(off, len(body))], "\n") }
		lineText := func(off int) string {
			off = min(off, len(body))
			s := strings.LastIndex(body[:off], "\n") + 1
			e := strings.Index(body[s:], "\n")
			if e < 0 {
				return body[s:]
			}
			return body[s : s+e]
		}
		emit := func(table, op string, fields []string, off int) {
			if isCodeComment(lineText(off)) {
				return
			}
			if len(known) > 0 {
				if _, ok := known[table]; !ok {
					return
				}
			}
			as.add(table, op, fields, lineOf(off))
		}
		for _, h := range analyzeSQL(body, known) {
			emit(h.table, h.op, h.fields, h.off)
		}
		if sqlcCall != nil && f.lang == "go" && importsAny(c.read(f.file), sqlcPkgs) {
			for _, m := range sqlcCall.FindAllStringSubmatchIndex(body, -1) {
				if recv := body[m[2]:m[3]]; recv != "" && recv == f.Recv {
					continue
				}
				if isCodeComment(lineText(m[0])) {
					continue
				}
				for _, a := range c.sqlcQ[body[m[4]:m[5]]] {
					as.add(a.Table, a.Op, a.Fields, lineOf(m[0]))
				}
			}
		}
		if f.lang == "go" {
			c.jetAccess(body, ji, emit)
		}
		if f.lang == "python" {
			c.djangoAccess(body, emit)
		}
		if jsExts[path.Ext(f.file)] {
			c.jsORMAccess(f.file, body, emit)
		}
		if len(as.list) > 0 {
			sort.SliceStable(as.list, func(i, j int) bool { return as.list[i].Line < as.list[j].Line })
			out[fid] = as.list
		}
	}
	return out
}

func importsAny(src string, pkgs []string) bool {
	for _, p := range pkgs {
		if strings.Contains(src, p+".") || strings.Contains(src, "/"+p+"\"") {
			return true
		}
	}
	return false
}
