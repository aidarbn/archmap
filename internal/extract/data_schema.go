package extract

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// buildSchema reads tables and fields from SQL migrations and ORM models.
func (c *apiCtx) buildSchema() *facts.DataModel {
	sb := &schemaBuilder{tables: map[string]*facts.Table{}}
	c.modelTbl = map[string]string{}
	c.prismaAcc = map[string]string{}
	c.ormRepo = map[string]string{}
	sb.sqlMigrations(c)
	c.djangoModels(sb)
	c.prismaModels(sb)
	c.typeormEntities(sb)
	return &facts.DataModel{Tables: sb.tables}
}

type schemaBuilder struct {
	tables map[string]*facts.Table
}

func (s *schemaBuilder) ensure(name, source string) *facts.Table {
	t := s.tables[name]
	if t == nil {
		t = &facts.Table{Source: source, Fields: []facts.Field{}}
		s.tables[name] = t
	}
	return t
}

func (s *schemaBuilder) addField(t *facts.Table, name, typ string) {
	for i, f := range t.Fields {
		if f.Name == name {
			t.Fields[i].Type = typ
			return
		}
	}
	t.Fields = append(t.Fields, facts.Field{Name: name, Type: typ})
}

func (s *schemaBuilder) addRef(t *facts.Table, field, table, to string) {
	for _, r := range t.Refs {
		if r.Field == field && r.Table == table {
			return
		}
	}
	t.Refs = append(t.Refs, facts.TableRef{Field: field, Table: table, To: to})
}

// ---- SQL ----

var (
	sqlLineComment  = regexp.MustCompile(`--[^\n]*`)
	sqlBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlDollarTag    = regexp.MustCompile(`\$\w*\$`)
	sqlString       = regexp.MustCompile(`'(?:[^']|'')*'`)
	goosDown        = regexp.MustCompile(`(?im)^\s*--\s*\+(?:goose|migrate)\s+down`)
	sqlCreateTable  = regexp.MustCompile(`(?is)^\s*create\s+(?:(?:global\s+|local\s+)?(?:temp(?:orary)?|unlogged)\s+)?table\s+(?:if\s+not\s+exists\s+)?([\w."` + "`" + `]+)\s*\(`)
	sqlAlterTable   = regexp.MustCompile(`(?is)^\s*alter\s+table\s+(?:only\s+)?(?:if\s+exists\s+)?([\w."` + "`" + `]+)\s+(.*)$`)
	sqlDropTable    = regexp.MustCompile(`(?is)^\s*drop\s+table\s+(?:if\s+exists\s+)?(.*?)(?:\s+(?:cascade|restrict))?\s*$`)
	sqlRenameTable  = regexp.MustCompile(`(?is)^\s*alter\s+table\s+(?:if\s+exists\s+)?([\w."` + "`" + `]+)\s+rename\s+to\s+([\w."` + "`" + `]+)\s*$`)
	sqlRefInline    = regexp.MustCompile(`(?is)\breferences\s+([\w."` + "`" + `]+)\s*(?:\(\s*([\w"` + "`" + `]+)\s*\))?`)
	sqlFK           = regexp.MustCompile(`(?is)foreign\s+key\s*\(([^)]*)\)\s*references\s+([\w."` + "`" + `]+)\s*(?:\(([^)]*)\))?`)
	sqlColStop      = regexp.MustCompile(`(?i)\s+(not\s+null|null|default|primary|references|unique|check|generated|constraint|collate|identity|on\s+(?:update|delete)|comment)\b.*$`)
	sqlConstraintKW = map[string]bool{"constraint": true, "primary": true, "unique": true, "foreign": true, "check": true, "exclude": true, "like": true, "index": true, "key": true, "fulltext": true}
)

func normIdent(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return strings.ToLower(strings.Trim(s, "\"`[] "))
}

func cleanSQL(s string) string {
	s = sqlBlockComment.ReplaceAllString(s, " ")
	s = sqlLineComment.ReplaceAllString(s, " ")
	s = stripDollar(s)
	s = sqlString.ReplaceAllStringFunc(s, func(m string) string {
		if len(m) <= 40 && !strings.ContainsAny(m, ",;()") {
			return m
		}
		return "''"
	})
	return s
}

func splitTop(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func matchParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (c *apiCtx) isSQLQueryFile(src string) bool {
	return regexp.MustCompile(`(?m)^\s*--\s*name:\s*\w+\s+:\w+`).MatchString(src)
}

func (s *schemaBuilder) sqlMigrations(c *apiCtx) {
	for _, p := range c.paths {
		if path.Ext(p) != ".sql" {
			continue
		}
		b := path.Base(p)
		if strings.HasSuffix(b, ".down.sql") || b == "down.sql" || strings.Contains(p, "/testdata/") || strings.Contains(p, "/seed") || strings.Contains(p, "/fixtures/") {
			continue
		}
		src := c.read(p)
		if c.isSQLQueryFile(src) {
			continue
		}
		if m := goosDown.FindStringIndex(src); m != nil {
			src = src[:m[0]]
		}
		source := "migration"
		if !strings.Contains(p, "migrat") && (b == "schema.sql" || strings.Contains(p, "sqlc")) {
			source = "sqlc"
		}
		for _, st := range strings.Split(cleanSQL(src), ";") {
			s.statement(st, source)
		}
	}
}

func (s *schemaBuilder) statement(st, source string) {
	st = strings.TrimSpace(st)
	if st == "" {
		return
	}
	if m := sqlCreateTable.FindStringSubmatchIndex(st); m != nil {
		name := normIdent(st[m[2]:m[3]])
		open := m[1] - 1
		end := matchParen(st, open)
		if end < 0 {
			return
		}
		t := s.ensure(name, source)
		t.Fields, t.Refs = []facts.Field{}, nil
		for _, item := range splitTop(st[open+1:end], ',') {
			s.columnOrConstraint(t, strings.TrimSpace(item))
		}
		return
	}
	if m := sqlRenameTable.FindStringSubmatch(st); m != nil {
		from, to := normIdent(m[1]), normIdent(m[2])
		if t := s.tables[from]; t != nil {
			s.tables[to] = t
			delete(s.tables, from)
		}
		return
	}
	if m := sqlAlterTable.FindStringSubmatch(st); m != nil {
		t := s.tables[normIdent(m[1])]
		if t == nil {
			return
		}
		for _, act := range splitTop(m[2], ',') {
			s.alterAction(t, strings.TrimSpace(act))
		}
		return
	}
	if m := sqlDropTable.FindStringSubmatch(st); m != nil && regexp.MustCompile(`(?i)^\s*drop\s+table`).MatchString(st) {
		for _, n := range strings.Split(m[1], ",") {
			delete(s.tables, normIdent(n))
		}
	}
}

func (s *schemaBuilder) columnOrConstraint(t *facts.Table, item string) {
	if item == "" {
		return
	}
	fields := strings.Fields(item)
	first := strings.ToLower(strings.Trim(fields[0], "\"`"))
	if sqlConstraintKW[first] {
		for _, m := range sqlFK.FindAllStringSubmatch(item, -1) {
			cols := strings.Split(m[1], ",")
			tos := strings.Split(m[3], ",")
			for i, col := range cols {
				to := ""
				if i < len(tos) {
					to = normIdent(tos[i])
				}
				s.addRef(t, normIdent(col), normIdent(m[2]), to)
			}
		}
		return
	}
	if len(fields) < 2 {
		return
	}
	name := normIdent(fields[0])
	rest := strings.TrimSpace(item[len(fields[0]):])
	typ := strings.TrimSpace(sqlColStop.ReplaceAllString(rest, ""))
	s.addField(t, name, strings.ToLower(typ))
	if m := sqlRefInline.FindStringSubmatch(rest); m != nil {
		s.addRef(t, name, normIdent(m[1]), normIdent(m[2]))
	}
}

func (s *schemaBuilder) alterAction(t *facts.Table, act string) {
	low := strings.ToLower(act)
	switch {
	case strings.HasPrefix(low, "add "):
		body := strings.TrimSpace(act[4:])
		bl := strings.ToLower(body)
		switch {
		case strings.HasPrefix(bl, "column "):
			body = strings.TrimSpace(body[7:])
		case strings.HasPrefix(bl, "constraint ") || strings.HasPrefix(bl, "foreign ") || strings.HasPrefix(bl, "primary ") || strings.HasPrefix(bl, "unique") || strings.HasPrefix(bl, "check"):
			s.columnOrConstraint(t, "constraint "+body) // a table constraint, never a column
			return
		}
		body = regexp.MustCompile(`(?i)^if\s+not\s+exists\s+`).ReplaceAllString(body, "")
		s.columnOrConstraint(t, body)
	case strings.HasPrefix(low, "drop column"):
		n := normIdent(regexp.MustCompile(`(?i)^drop\s+column\s+(?:if\s+exists\s+)?(\S+)`).ReplaceAllString(act, "$1"))
		for i, f := range t.Fields {
			if f.Name == n {
				t.Fields = append(t.Fields[:i], t.Fields[i+1:]...)
				break
			}
		}
		var refs []facts.TableRef
		for _, r := range t.Refs {
			if r.Field != n {
				refs = append(refs, r)
			}
		}
		t.Refs = refs
	case strings.HasPrefix(low, "rename column"):
		if m := regexp.MustCompile(`(?i)^rename\s+column\s+(\S+)\s+to\s+(\S+)`).FindStringSubmatch(act); m != nil {
			from, to := normIdent(m[1]), normIdent(m[2])
			for i := range t.Fields {
				if t.Fields[i].Name == from {
					t.Fields[i].Name = to
				}
			}
			for i := range t.Refs {
				if t.Refs[i].Field == from {
					t.Refs[i].Field = to
				}
			}
		}
	case strings.HasPrefix(low, "alter column"):
		if m := regexp.MustCompile(`(?is)^alter\s+column\s+(\S+)\s+(?:set\s+data\s+)?type\s+(.*?)(?:\s+using\s+.*)?$`).FindStringSubmatch(act); m != nil {
			s.addField(t, normIdent(m[1]), strings.ToLower(strings.TrimSpace(m[2])))
		}
	}
}

// ---- Django models ----

var (
	djFieldRE   = regexp.MustCompile(`(?m)^[ \t]{4}(\w+)\s*(?::[^=\n]+)?=\s*(?:\w+\.)*(\w*(?:Field|Key))\(`)
	djMetaTable = regexp.MustCompile(`db_table\s*=\s*['"](\w+)['"]`)
	djAbstract  = regexp.MustCompile(`abstract\s*=\s*True`)
)

type djModel struct {
	cl       *pyClass
	table    string
	abstract bool
	isModel  bool
}

func (c *apiCtx) djangoModels(sb *schemaBuilder) {
	idx := c.pyClassIndex()
	models := map[*pyClass]*djModel{}
	var all []*pyClass
	for _, cs := range idx {
		all = append(all, cs...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].file != all[j].file {
			return all[i].file < all[j].file
		}
		return all[i].start < all[j].start
	})
	isModel := func(cl *pyClass) bool {
		_, ext := c.pyChain(cl)
		for _, e := range ext {
			if e == "Model" || e == "AbstractUser" || e == "AbstractBaseUser" {
				return true
			}
		}
		return false
	}
	for _, cl := range all {
		if strings.Contains(cl.file, "/migrations/") || isTestPath(cl.file) || !isModel(cl) {
			continue
		}
		m := &djModel{cl: cl, isModel: true, abstract: djAbstract.MatchString(cl.text)}
		app := path.Base(path.Dir(cl.file))
		if app == "models" {
			app = path.Base(path.Dir(path.Dir(cl.file)))
		}
		m.table = app + "_" + strings.ToLower(cl.name)
		if mm := djMetaTable.FindStringSubmatch(cl.text); mm != nil {
			m.table = mm[1]
		}
		models[cl] = m
		if prev, dup := c.modelTbl[cl.name]; dup && prev != m.table {
			c.modelTbl[cl.name] = ""
		} else if !m.abstract {
			c.modelTbl[cl.name] = m.table
		}
	}
	resolve := func(name string, near *pyClass) string {
		name = strings.Trim(name, "'\" ")
		if name == "self" {
			return models[near].table
		}
		if strings.Contains(name, ".") {
			name = name[strings.LastIndex(name, ".")+1:]
		}
		if t := c.modelTbl[name]; t != "" {
			return t
		}
		return ""
	}
	for _, cl := range all {
		m := models[cl]
		if m == nil || m.abstract {
			continue
		}
		t := sb.ensure(m.table, "orm")
		hasPK := false
		var classes []*pyClass
		chain, _ := c.pyChain(cl)
		for i := len(chain) - 1; i >= 0; i-- { // ancestors first
			if x := models[chain[i]]; x != nil && (chain[i] == cl || x.abstract) {
				classes = append(classes, chain[i])
			}
		}
		var m2m [][2]string
		for _, x := range classes {
			for _, fi := range djFieldRE.FindAllStringSubmatchIndex(x.text, -1) {
				args := x.text[fi[1]:]
				if end := matchParen(x.text, fi[1]-1); end > 0 {
					args = x.text[fi[1]:end]
				}
				name, kind := x.text[fi[2]:fi[3]], x.text[fi[4]:fi[5]]
				if strings.Contains(args, "primary_key=True") {
					hasPK = true
				}
				switch kind {
				case "ForeignKey", "OneToOneField":
					col := name + "_id"
					if dc := regexp.MustCompile(`db_column\s*=\s*['"](\w+)['"]`).FindStringSubmatch(args); dc != nil {
						col = dc[1]
					}
					sb.addField(t, col, kind)
					first := strings.TrimSpace(strings.SplitN(args, ",", 2)[0])
					if tt := resolve(first, cl); tt != "" {
						sb.addRef(t, col, tt, "id")
					}
				case "ManyToManyField":
					first := strings.TrimSpace(strings.SplitN(args, ",", 2)[0])
					if tt := resolve(first, cl); tt != "" {
						m2m = append(m2m, [2]string{name, tt})
					}
				default:
					sb.addField(t, name, kind)
				}
			}
		}
		if !hasPK {
			t.Fields = append([]facts.Field{{Name: "id", Type: "AutoField"}}, t.Fields...)
		}
		for _, mm := range m2m {
			jt := sb.ensure(m.table+"_"+mm[0], "orm")
			sb.addField(jt, "id", "AutoField")
			sb.addField(jt, strings.ToLower(cl.name)+"_id", "ForeignKey")
			sb.addField(jt, mm[0]+"_id", "ForeignKey")
			sb.addRef(jt, strings.ToLower(cl.name)+"_id", m.table, "id")
			sb.addRef(jt, mm[0]+"_id", mm[1], "id")
		}
	}
}

// ---- Prisma ----

var (
	prismaModelRE = regexp.MustCompile(`(?m)^model\s+(\w+)\s*\{([^}]*)\}`)
	prismaFieldRE = regexp.MustCompile(`^(\w+)\s+(\w+)(\[\])?(\?)?(.*)$`)
	prismaRelRE   = regexp.MustCompile(`@relation\([^)]*fields:\s*\[([^\]]*)\][^)]*references:\s*\[([^\]]*)\]`)
	prismaMapRE   = regexp.MustCompile(`@map\("([^"]+)"\)`)
	prismaScalars = map[string]bool{"String": true, "Int": true, "BigInt": true, "Float": true, "Decimal": true, "Boolean": true, "DateTime": true, "Json": true, "Bytes": true}
)

func (c *apiCtx) prismaModels(sb *schemaBuilder) {
	for _, p := range c.paths {
		if path.Ext(p) != ".prisma" {
			continue
		}
		src := c.read(p)
		names := map[string]string{} // model -> table
		for _, m := range prismaModelRE.FindAllStringSubmatch(src, -1) {
			tbl := m[1]
			if mm := regexp.MustCompile(`@@map\("([^"]+)"\)`).FindStringSubmatch(m[2]); mm != nil {
				tbl = mm[1]
			}
			names[m[1]] = tbl
		}
		for _, m := range prismaModelRE.FindAllStringSubmatch(src, -1) {
			t := sb.ensure(names[m[1]], "orm")
			t.Fields, t.Refs = []facts.Field{}, nil
			c.prismaAcc[strings.ToLower(m[1][:1])+m[1][1:]] = names[m[1]]
			cols := map[string]string{}
			for _, line := range strings.Split(m[2], "\n") {
				line = strings.TrimSpace(line)
				fm := prismaFieldRE.FindStringSubmatch(line)
				if fm == nil || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "@@") {
					continue
				}
				name, typ := fm[1], fm[2]
				col := name
				if mm := prismaMapRE.FindStringSubmatch(fm[5]); mm != nil {
					col = mm[1]
				}
				cols[name] = col
				if _, isModel := names[typ]; isModel {
					if rm := prismaRelRE.FindStringSubmatch(fm[5]); rm != nil {
						from, to := strings.Fields(strings.ReplaceAll(rm[1], ",", " ")), strings.Fields(strings.ReplaceAll(rm[2], ",", " "))
						for i := range from {
							tt := ""
							if i < len(to) {
								tt = to[i]
							}
							sb.addRef(t, from[i], names[typ], tt)
						}
					}
					continue
				}
				_ = prismaScalars
				sb.addField(t, col, typ+fm[3]+fm[4])
			}
			for i, r := range t.Refs {
				if cc, ok := cols[r.Field]; ok {
					t.Refs[i].Field = cc
				}
			}
		}
	}
}

// ---- TypeORM ----

var (
	tormEntityRE = regexp.MustCompile(`(?s)@Entity\(([^)]*)\)\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(\w+)[^{]*\{(.*?)\n\}`)
	tormColRE    = regexp.MustCompile(`@(Column|PrimaryColumn|PrimaryGeneratedColumn|CreateDateColumn|UpdateDateColumn|DeleteDateColumn|VersionColumn)\(([^@;]*?)\)\s*(?:@\w+\([^)]*\)\s*)*(?:public\s+|private\s+|protected\s+|readonly\s+)*(\w+)[?!]?\s*[:=]\s*([^;=\n]*)`)
	tormRelRE    = regexp.MustCompile(`@(?:ManyToOne|OneToOne)\(\s*\(\)\s*=>\s*(\w+)[^@;]*?\)\s*(?:@JoinColumn\(([^)]*)\)\s*)?(?:public\s+|private\s+|readonly\s+)*(\w+)[?!]?\s*:`)
	tormRepoRE   = regexp.MustCompile(`(\w+)\s*:\s*Repository<(\w+)>`)
)

func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteString(strings.ToLower(string(r)))
	}
	return b.String()
}

func (c *apiCtx) typeormEntities(sb *schemaBuilder) {
	ent := map[string]string{}
	type pending struct{ cls, tbl, body string }
	var all []pending
	for _, p := range c.paths {
		if !jsExts[path.Ext(p)] || isTestPath(p) {
			continue
		}
		src := c.read(p)
		if !strings.Contains(src, "@Entity(") {
			continue
		}
		for _, m := range tormEntityRE.FindAllStringSubmatch(src, -1) {
			tbl := snakeCase(m[2])
			if s := firstStrRE.FindStringSubmatch(m[1]); s != nil {
				tbl = s[1]
			} else if s := regexp.MustCompile(`name:\s*['"](\w+)['"]`).FindStringSubmatch(m[1]); s != nil {
				tbl = s[1]
			}
			ent[m[2]] = tbl
			all = append(all, pending{m[2], tbl, m[3]})
		}
	}
	for cls, tbl := range ent {
		c.modelTbl[cls] = tbl
	}
	for _, e := range all {
		t := sb.ensure(e.tbl, "orm")
		for _, m := range tormColRE.FindAllStringSubmatch(e.body, -1) {
			name := m[3]
			if nm := regexp.MustCompile(`name:\s*['"](\w+)['"]`).FindStringSubmatch(m[2]); nm != nil {
				name = nm[1]
			}
			sb.addField(t, name, strings.TrimSpace(m[4]))
		}
		for _, m := range tormRelRE.FindAllStringSubmatch(e.body, -1) {
			col := m[3] + "Id"
			if nm := regexp.MustCompile(`name:\s*['"](\w+)['"]`).FindStringSubmatch(m[2]); nm != nil {
				col = nm[1]
			}
			sb.addField(t, col, "relation")
			if tt, ok := ent[m[1]]; ok {
				sb.addRef(t, col, tt, "id")
			}
		}
	}
	for _, p := range c.paths {
		if !jsExts[path.Ext(p)] || isTestPath(p) {
			continue
		}
		src := c.read(p)
		if !strings.Contains(src, "Repository<") {
			continue
		}
		for _, m := range tormRepoRE.FindAllStringSubmatch(src, -1) {
			if _, ok := ent[m[2]]; ok {
				c.ormRepo[p+"::"+m[1]] = m[2]
			}
		}
	}
}

// stripDollar removes $tag$ ... $tag$ bodies (function bodies, DO blocks).
func stripDollar(s string) string {
	var b strings.Builder
	for {
		loc := sqlDollarTag.FindStringIndex(s)
		if loc == nil {
			b.WriteString(s)
			return b.String()
		}
		tag := s[loc[0]:loc[1]]
		end := strings.Index(s[loc[1]:], tag)
		b.WriteString(s[:loc[0]])
		b.WriteString(" ")
		if end < 0 {
			return b.String()
		}
		s = s[loc[1]+end+len(tag):]
	}
}
