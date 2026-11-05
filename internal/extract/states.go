package extract

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// State machines are read from the code with text rules, not guessed: a set of
// named constants/enum members of a "status-like" type is a machine, and a
// function that assigns one of them (x.Status = A, UpdateStatus(..., A), SQL
// SET status = 'a') makes a transition into it. The source state comes from a
// comparison in the same function (== A, case A, WHERE status = 'a') or from
// the name of the call (UpdateStatusIfAccepted). Only what is plainly visible
// is reported.
//
// Transitions declared with `arch:state` are merged in by mergeAnnotatedStates.

// stFile is a source file given to the state pass.
type stFile struct{ Path, Module, Lang, Text string }

// stFunc is a function of a file.
type stFunc struct {
	Fid, File  string
	Start, End int
}

var statusWord = regexp.MustCompile(`(?i)(status|state|stage|phase)`)
var statusTypeRe = regexp.MustCompile(`(?i)(status|state|stage|phase)$`)

// constPrefixRe splits an untyped constant OrderStatusPaid into its type-like
// prefix OrderStatus and the state name Paid.
var constPrefixRe = regexp.MustCompile(`^(\w*?(?:Status|State|Stage|Phase|status|state|stage|phase))([A-Z0-9]\w*)$`)

type machine struct {
	key, module string
	labels      []string
	seen        map[string]bool
	refs        map[string]string // code token -> label
	values      map[string]string // string value -> label
	doc         string            // first paragraph of the type / const block comment
	docs        map[string]string // label -> comment of its constant
}

// note keeps the comment of a state constant (the first one wins).
func (m *machine) note(label, doc string) {
	if doc = strings.TrimSpace(doc); doc == "" || label == "" {
		return
	}
	if m.docs == nil {
		m.docs = map[string]string{}
	}
	if m.docs[label] == "" {
		m.docs[label] = doc
	}
}

// firstPara is the first paragraph of comment lines, joined, cut to 300 runes.
func firstPara(lines []string) string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			if len(out) > 0 {
				break
			}
			continue
		}
		out = append(out, l)
	}
	s := strings.Join(out, " ")
	if r := []rune(s); len(r) > 300 {
		s = strings.TrimSpace(string(r[:299])) + "…"
	}
	return s
}

func (m *machine) add(token, value, label string) {
	if label == "" {
		return
	}
	if !m.seen[label] {
		m.seen[label] = true
		m.labels = append(m.labels, label)
	}
	if token != "" {
		m.refs[token] = label
	}
	if value != "" {
		m.values[value] = label
	}
}

type machineSet struct {
	byKey map[string]*machine // key = module + "\x00" + name
	list  []*machine
}

func (s *machineSet) get(module, name string) *machine {
	k := module + "\x00" + name
	m := s.byKey[k]
	if m == nil {
		m = &machine{key: name, module: module, seen: map[string]bool{}, refs: map[string]string{}, values: map[string]string{}}
		s.byKey[k] = m
		s.list = append(s.list, m)
	}
	return m
}

var (
	goConstBlock = regexp.MustCompile(`^\s*const\s*\(\s*$`)
	goConstOne   = regexp.MustCompile(`^\s*const\s+(\w+)(?:\s+([\w.]+))?\s*=\s*(.+?)\s*$`)
	goConstLine  = regexp.MustCompile(`^\s*(\w+)(?:\s+([\w.]+))?\s*=\s*(.+?)\s*$`)
	goConstBare  = regexp.MustCompile(`^\s*(\w+)\s*$`)
	strLit       = regexp.MustCompile("^(?:\"([^\"]*)\"|`([^`]*)`)")
	pyClassLine  = regexp.MustCompile(`^(\s*)class\s+(\w+)\s*\(([^)]*)\)\s*:`)
	pyMember     = regexp.MustCompile(`^\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+?)\s*$`)
	pyStr        = regexp.MustCompile(`["']([^"']+)["']`)
	tsEnum       = regexp.MustCompile(`(?:export\s+)?(?:declare\s+)?(?:const\s+)?enum\s+(\w+)\s*\{([^}]*)\}`)
	tsEnumMember = regexp.MustCompile(`(\w+)\s*(?:=\s*([^,\n]+))?`)
	tsUnion      = regexp.MustCompile(`type\s+(\w+)\s*=\s*\|?\s*((?:'[^'\n]*'|"[^"\n]*")(?:\s*\|\s*(?:'[^'\n]*'|"[^"\n]*"))+)`)
	tsUnionVal   = regexp.MustCompile(`'([^'\n]*)'|"([^"\n]*)"`)
)

func unq(s string) (string, bool) {
	m := strLit.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	if m[1] != "" {
		return m[1], true
	}
	return m[2], m[2] != ""
}

func collectMachines(files []stFile) []*machine {
	set := &machineSet{byKey: map[string]*machine{}}
	for _, f := range files {
		switch f.Lang {
		case "go":
			goMachines(set, f)
		case "python":
			pyMachines(set, f)
		case "js", "ts", "tsx", "jsx":
			tsMachines(set, f)
		}
	}
	var out []*machine
	for _, m := range set.list {
		if len(m.labels) >= 2 {
			out = append(out, m)
		}
	}
	// the same type name in two modules: tell them apart by module
	cnt := map[string]int{}
	for _, m := range out {
		cnt[m.key]++
	}
	for _, m := range out {
		if cnt[m.key] > 1 {
			m.key = m.key + " (" + m.module + ")"
		}
	}
	return out
}

var goTypeLine = regexp.MustCompile(`^\s*type\s+(\w+)\s+\w`)

func goMachines(set *machineSet, f stFile) {
	lines := strings.Split(f.Text, "\n")
	inBlock := false
	prevTyp, prevIota := "", false
	// comments: the run of // lines right above the current line, the doc of the
	// open const block, and the doc of each status-like type of the file
	var above, blockDoc []string
	typeDoc := map[string]string{}
	for _, ln := range lines {
		trailing := ""
		if t := strings.TrimSpace(ln); strings.HasPrefix(t, "//") {
			above = append(above, strings.TrimSpace(strings.TrimPrefix(t, "//")))
			continue
		}
		doc := above
		above = nil
		if i := strings.Index(ln, "//"); i >= 0 && !strings.Contains(ln[:i], `"`) {
			trailing = strings.TrimSpace(ln[i+2:])
			ln = ln[:i]
		} else if i := strings.LastIndex(ln, "//"); i >= 0 && strings.Count(ln[:i], `"`)%2 == 0 && strings.Count(ln[:i], "`")%2 == 0 {
			trailing = strings.TrimSpace(ln[i+2:])
			ln = ln[:i]
		}
		if m := goTypeLine.FindStringSubmatch(ln); m != nil && statusTypeRe.MatchString(m[1]) {
			if d := firstPara(doc); d != "" {
				typeDoc[m[1]] = d
				for _, mm := range set.list {
					if mm.module == f.Module && mm.key == m[1] {
						mm.doc = d
					}
				}
			}
		}
		var name, typ, rhs string
		switch {
		case goConstBlock.MatchString(ln):
			inBlock, prevTyp, prevIota = true, "", false
			blockDoc = doc
			continue
		case inBlock && strings.TrimSpace(ln) == ")":
			inBlock = false
			continue
		case inBlock:
			if m := goConstLine.FindStringSubmatch(ln); m != nil {
				name, typ, rhs = m[1], m[2], m[3]
			} else if m := goConstBare.FindStringSubmatch(ln); m != nil && prevTyp != "" && prevIota {
				name, typ, rhs = m[1], prevTyp, "iota"
			} else {
				continue
			}
		default:
			if m := goConstOne.FindStringSubmatch(ln); m != nil {
				name, typ, rhs = m[1], m[2], m[3]
			} else {
				continue
			}
		}
		if inBlock {
			prevTyp, prevIota = typ, strings.Contains(rhs, "iota")
		}
		if i := strings.LastIndex(typ, "."); i >= 0 {
			typ = typ[i+1:]
		}
		val, isStr := unq(rhs)
		memberDoc := trailing
		if memberDoc == "" {
			memberDoc = firstPara(doc)
		}
		var mach *machine
		label := ""
		switch {
		case typ != "" && statusTypeRe.MatchString(typ):
			label = name
			if isStr {
				label = val
			} else if strings.HasPrefix(name, typ) && len(name) > len(typ) {
				label = name[len(typ):]
			}
			v := ""
			if isStr {
				v = val
			}
			mach = set.get(f.Module, typ)
			mach.add(name, v, label)
		case typ == "" && isStr:
			if m := constPrefixRe.FindStringSubmatch(name); m != nil {
				label = val
				mach = set.get(f.Module, m[1])
				mach.add(name, val, val)
			}
		}
		if mach == nil {
			continue
		}
		mach.note(label, memberDoc)
		if mach.doc == "" {
			if d := typeDoc[mach.key]; d != "" {
				mach.doc = d
			} else if inBlock {
				mach.doc = firstPara(blockDoc)
			} else {
				mach.doc = firstPara(doc)
			}
		}
	}
}

var (
	goFinalFunc = regexp.MustCompile(`(?m)^func\s+(?:\([^)]*\)\s*)?(?:Is|is)?\w*(?:Final|Terminal|Finished|Closed)\w*\s*\(`)
	goCaseList  = regexp.MustCompile(`(?s)case\s+([^:]+):\s*return\s+true\b`)
	goIdent     = regexp.MustCompile(`[A-Za-z_][\w.]*`)
)

// goFinals reads the final states the code names itself: in a function called
// IsFinal / IsTerminal… (also a method), the constants of a `case …: return true`.
func goFinals(f stFile, tokens map[string][]*machine) map[*machine][]string {
	if f.Lang != "go" {
		return nil
	}
	var out map[*machine][]string
	for _, loc := range goFinalFunc.FindAllStringIndex(f.Text, -1) {
		rest := f.Text[loc[1]:]
		end := strings.Index(rest, "\n}")
		if end < 0 {
			continue
		}
		for _, cm := range goCaseList.FindAllStringSubmatch(rest[:end], -1) {
			for _, id := range goIdent.FindAllString(cm[1], -1) {
				if i := strings.LastIndex(id, "."); i >= 0 {
					id = id[i+1:]
				}
				ms := tokens[id]
				if len(ms) != 1 {
					continue
				}
				m := ms[0]
				if out == nil {
					out = map[*machine][]string{}
				}
				l := m.refs[id]
				dup := false
				for _, x := range out[m] {
					dup = dup || x == l
				}
				if !dup {
					out[m] = append(out[m], l)
				}
			}
		}
	}
	return out
}

func pyMachines(set *machineSet, f stFile) {
	lines := strings.Split(f.Text, "\n")
	for i := 0; i < len(lines); i++ {
		m := pyClassLine.FindStringSubmatch(lines[i])
		if m == nil || !statusTypeRe.MatchString(m[2]) || !regexp.MustCompile(`(?i)enum|choices`).MatchString(m[3]) {
			continue
		}
		indent := len(m[1])
		mach := set.get(f.Module, m[2])
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				continue
			}
			if len(l)-len(strings.TrimLeft(l, " \t")) <= indent {
				break
			}
			mm := pyMember.FindStringSubmatch(l)
			if mm == nil || strings.HasPrefix(mm[1], "_") || strings.HasPrefix(strings.TrimSpace(mm[2]), "models.") {
				continue
			}
			label, value := mm[1], ""
			if s := pyStr.FindStringSubmatch(mm[2]); s != nil {
				label, value = s[1], s[1]
			}
			mach.add(m[2]+"."+mm[1], value, label)
		}
	}
}

func tsMachines(set *machineSet, f stFile) {
	for _, m := range tsEnum.FindAllStringSubmatch(f.Text, -1) {
		if !statusTypeRe.MatchString(m[1]) {
			continue
		}
		mach := set.get(f.Module, m[1])
		for _, line := range strings.Split(m[2], ",") {
			mm := tsEnumMember.FindStringSubmatch(strings.TrimSpace(line))
			if mm == nil || mm[1] == "" {
				continue
			}
			label, value := mm[1], ""
			if v, ok := unqAny(mm[2]); ok {
				label, value = v, v
			}
			mach.add(m[1]+"."+mm[1], value, label)
		}
	}
	for _, m := range tsUnion.FindAllStringSubmatch(f.Text, -1) {
		if !statusTypeRe.MatchString(m[1]) {
			continue
		}
		mach := set.get(f.Module, m[1])
		for _, v := range tsUnionVal.FindAllStringSubmatch(m[2], -1) {
			s := v[1] + v[2]
			mach.add("", s, s)
		}
	}
}

func unqAny(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1], true
	}
	return "", false
}

// stmt is one statement of a function body: raw keeps string contents, masked
// replaces them with spaces so that code can be matched without string noise.
type stmt struct {
	raw, masked string
	lits        []lit // string literals in the statement
}

type lit struct {
	start, end int // byte range of the contents inside raw
	val        string
}

// statements splits a function body into statements: comments removed, a
// statement continues while parentheses are open or a string is unfinished.
func statements(body, language string) []stmt {
	var out []stmt
	var raw, masked strings.Builder
	var lits []lit
	depth := 0
	inStr := byte(0) // quote char: ' " ` or 'T' for python triple double, 't' triple single
	litStart := 0
	flush := func() {
		if strings.TrimSpace(raw.String()) != "" {
			out = append(out, stmt{raw.String(), masked.String(), lits})
		}
		raw.Reset()
		masked.Reset()
		lits = nil
	}
	py := language == "python"
	n := len(body)
	for i := 0; i < n; i++ {
		c := body[i]
		if inStr != 0 {
			end := false
			switch inStr {
			case 'T':
				end = strings.HasPrefix(body[i:], `"""`)
			case 't':
				end = strings.HasPrefix(body[i:], `'''`)
			case '`':
				end = c == '`'
			default:
				if c == '\\' && i+1 < n {
					raw.WriteByte(c)
					masked.WriteByte(' ')
					i++
					raw.WriteByte(body[i])
					masked.WriteByte(' ')
					continue
				}
				end = c == inStr || c == '\n'
			}
			if end {
				s := raw.String()
				lits = append(lits, lit{litStart, len(s), s[litStart:]})
				if inStr == 'T' || inStr == 't' {
					raw.WriteString(body[i : i+3])
					masked.WriteString("   ")
					i += 2
				} else if c == '\n' {
					raw.WriteByte(c)
					masked.WriteByte(c)
					inStr = 0
					if depth <= 0 {
						flush()
					}
					continue
				} else {
					raw.WriteByte(c)
					masked.WriteByte(c)
				}
				inStr = 0
				continue
			}
			raw.WriteByte(c)
			if c == '\n' {
				masked.WriteByte(c)
			} else {
				masked.WriteByte(' ')
			}
			continue
		}
		switch {
		case !py && c == '/' && i+1 < n && body[i+1] == '/':
			for i < n && body[i] != '\n' {
				i++
			}
			i--
			continue
		case py && c == '#':
			for i < n && body[i] != '\n' {
				i++
			}
			i--
			continue
		case !py && c == '/' && i+1 < n && body[i+1] == '*':
			j := strings.Index(body[i+2:], "*/")
			if j < 0 {
				i = n
			} else {
				i += j + 3
			}
			continue
		case py && (strings.HasPrefix(body[i:], `"""`) || strings.HasPrefix(body[i:], `'''`)):
			inStr = 'T'
			if body[i] == '\'' {
				inStr = 't'
			}
			raw.WriteString(body[i : i+3])
			masked.WriteString("   ")
			i += 2
			litStart = raw.Len()
			continue
		case c == '"' || c == '\'' || (c == '`' && !py):
			inStr = c
			raw.WriteByte(c)
			masked.WriteByte(c)
			litStart = raw.Len()
			continue
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == '\n':
			raw.WriteByte(' ')
			masked.WriteByte(' ')
			if depth <= 0 {
				flush()
				depth = 0
			}
			continue
		case c == ';' && depth <= 0:
			flush()
			continue
		}
		raw.WriteByte(c)
		masked.WriteByte(c)
	}
	flush()
	return out
}

var (
	chainRe     = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\s*\.\s*[A-Za-z_][A-Za-z0-9_]*)*`)
	caseBefore  = regexp.MustCompile(`\bcase\b\s*(?:[\w.]+\s*,\s*)*$`)
	inBefore    = regexp.MustCompile(`\bin\s*[(\[{]\s*(?:[\w.]+\s*,\s*)*$`)
	lhsRe       = regexp.MustCompile(`([\w.]+)\s*$`)
	calleeRe    = regexp.MustCompile(`([A-Za-z_][\w.]*)\s*$`)
	transCallee = regexp.MustCompile(`^(?:[Uu]pdate|[Ss]et|[Cc]hange|[Mm]ark|[Tt]ransition|[Mm]ove|[Aa]pply)(?:$|[A-Z_0-9])`)
	ifSuffix    = regexp.MustCompile(`(?:If|_if_)([A-Za-z0-9_]+)$`)
	sqlSet      = regexp.MustCompile(`(?is)\bset\b.*?\b(?:\w+\.)?(\w*(?:status|state|stage|phase)\w*)\s*=\s*'([^']+)'`)
	sqlWhere    = regexp.MustCompile(`(?is)\bwhere\b.*`)
	sqlFrom     = regexp.MustCompile(`(?i)\b\w*(?:status|state|stage|phase)\w*\s*(?:=\s*'([^']+)'|in\s*\(([^)]*)\))`)
	sqlVal      = regexp.MustCompile(`'([^']+)'`)
)

// unclosedCall returns the callee name of the innermost call still open at
// position pos of masked, "" if the position is not inside call arguments.
func unclosedCall(masked string, pos int) string {
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch masked[i] {
		case ')', ']':
			depth++
		case '(', '[':
			if depth > 0 {
				depth--
				continue
			}
			if masked[i] == '[' {
				return ""
			}
			if m := calleeRe.FindStringSubmatch(masked[:i]); m != nil {
				name := m[1]
				if j := strings.LastIndex(name, "."); j >= 0 {
					name = name[j+1:]
				}
				return name
			}
			return ""
		}
	}
	return ""
}

type funcEvidence struct {
	to     []mlabel
	from   map[*machine][]string
	ifFrom map[*machine]string
	// via: the status was passed to a setter (a function that writes its
	// status parameter) — label -> fid of the setter; insert: the status is the
	// value of a new row (INSERT … VALUES), not a change of an existing one.
	via    map[mlabel]string
	noFrom map[mlabel]bool // the source is unknown whatever the comparisons say (mappedEvidence)
	insert map[mlabel]bool
}

func (e *funcEvidence) addVia(m *machine, l, fid string) {
	e.addTo(m, l)
	if e.via == nil {
		e.via = map[mlabel]string{}
	}
	if _, ok := e.via[mlabel{m, l}]; !ok {
		e.via[mlabel{m, l}] = fid
	}
}

func (e *funcEvidence) addNew(m *machine, l string) {
	e.addTo(m, l)
	if e.insert == nil {
		e.insert = map[mlabel]bool{}
	}
	e.insert[mlabel{m, l}] = true
}

// setters are the functions that write a status they get as a parameter
// (x.Status = status, Params{Status: string(status)}, or by handing it on to
// another setter): name -> fid, "" when two functions share the name.
type setters map[string]string

var (
	goSigRe      = regexp.MustCompile(`(?s)^\s*func\s*(?:\([^)]*\)\s*)?(\w+)\s*(?:\[[^\]]*\])?\s*\(([^)]*)\)`)
	statusParam  = regexp.MustCompile(`(?i)^(?:new|next|target|to)?_?(?:status|state|stage|phase)$`)
	readCallRe   = regexp.MustCompile(`^(?:List|Get|Find|Count|Select|Search|Filter|Query|Has|Exists|Is|Check|Fetch|Load|Read|Lookup|Where)`)
	fieldKeyRe   = regexp.MustCompile(`(\w+)\s*:\s*(?:[\w.]+\(\s*)?$`)
	braceTypeRe  = regexp.MustCompile(`([A-Za-z_][\w.]*)\s*$`)
	sqlInsert    = regexp.MustCompile(`(?is)\binsert\s+into\s+[\w.]+\s*\(([^)]*)\)\s*values\s*\(`)
	sqlStatusCol = regexp.MustCompile(`(?i)^(?:\w+\.)?\w*(?:status|state|stage|phase)\w*$`)
)

// goParams returns the name and the status-like parameters of a Go function
// whose text starts at its signature.
func goParams(text string, ms []*machine) (string, []string) {
	m := goSigRe.FindStringSubmatch(text)
	if m == nil {
		return "", nil
	}
	var names, out []string
	for _, part := range strings.Split(m[2], ",") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		names = append(names, f[0])
		if len(f) < 2 { // `a, b T`: the type comes later
			continue
		}
		typ := strings.TrimLeft(f[len(f)-1], "*[]")
		if i := strings.LastIndex(typ, "."); i >= 0 {
			typ = typ[i+1:]
		}
		isM := false
		for _, mm := range ms {
			isM = isM || mm.key == typ
		}
		for _, n := range names {
			if statusParam.MatchString(n) || isM {
				out = append(out, n)
			}
		}
		names = nil
	}
	return m[1], out
}

// findSetters: functions with a status parameter that they write into a
// status field, or hand on to another setter (two rounds of helpers).
func findSetters(bodies map[string]string, ms []*machine) setters {
	type cand struct {
		name, fid, body string
		params          []string
	}
	var cs []cand
	for fid, body := range bodies {
		name, ps := goParams(body, ms)
		if name != "" && len(ps) > 0 {
			cs = append(cs, cand{name, fid, body, ps})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].fid < cs[j].fid })
	out := setters{}
	add := func(c cand) {
		if f, ok := out[c.name]; ok && f != c.fid {
			out[c.name] = ""
			return
		}
		out[c.name] = c.fid
	}
	isSet := func(c cand) bool {
		for _, p := range c.params {
			q := regexp.QuoteMeta(p)
			if regexp.MustCompile(`\b\w*(?:Status|State|Stage|Phase|status|state)\w*\s*(?::|=)\s*[&*]?(?:[\w.]+\(\s*)?[*]?`+q+`\b`).MatchString(c.body) &&
				!regexp.MustCompile(`[=!]=\s*[*]?`+q+`\b`).MatchString(c.body) {
				return true
			}
		}
		return false
	}
	for _, c := range cs {
		if isSet(c) {
			add(c)
		}
	}
	for round := 0; round < 2; round++ {
		for _, c := range cs {
			if _, ok := out[c.name]; ok {
				continue
			}
			for _, p := range c.params {
				for _, cm := range regexp.MustCompile(`\b(\w+)\s*\(([^()]*\b`+regexp.QuoteMeta(p)+`\b[^()]*)\)`).FindAllStringSubmatch(c.body, -1) {
					if _, ok := out[cm[1]]; ok && cm[1] != c.name {
						add(c)
					}
				}
			}
		}
	}
	return out
}

// outerCalls lists the calls open at pos from the inside out (at most n): `a.Set(ctx, string(X` → [string, Set].
func outerCalls(masked string, pos, n int) []string {
	var out []string
	depth := 0
	for i := pos - 1; i >= 0 && len(out) < n; i-- {
		switch masked[i] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth > 0 {
				depth--
				continue
			}
			if masked[i] != '(' {
				return out
			}
			m := calleeRe.FindStringSubmatch(masked[:i])
			if m == nil {
				return out
			}
			name := m[1]
			if j := strings.LastIndex(name, "."); j >= 0 {
				name = name[j+1:]
			}
			out = append(out, name)
		}
	}
	return out
}

var defName = regexp.MustCompile(`^\s*(?:func\s*(?:\([^)]*\)\s*)?|(?:async\s+)?def\s+|(?:async\s+)?function\s+)(\w+)`)

var sqlComment = regexp.MustCompile(`--[^\n]*`)

var createRe = regexp.MustCompile(`^(?:Create|Insert|New|Add|Register|Build|Make)`)

var convCall = regexp.MustCompile(`^(?:string|int|int32|int64|uint8|str|String|Number)$`)

// setterCall: the status at pos is an argument of a setter (through a conversion) → fid of the setter.
func setterCall(masked string, pos int, set setters) (string, bool) {
	for _, c := range outerCalls(masked, pos, 2) {
		if fid, ok := set[c]; ok && !readCallRe.MatchString(c) {
			return fid, true
		}
		if !convCall.MatchString(c) {
			return "", false
		}
	}
	return "", false
}

// braceType is the type of the composite literal open at pos (`Params{ … `), "" when none in reach.
func braceType(masked string, pos int) string {
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch masked[i] {
		case '}':
			depth++
		case '{':
			if depth > 0 {
				depth--
				continue
			}
			if m := braceTypeRe.FindStringSubmatch(masked[:i]); m != nil {
				n := m[1]
				if j := strings.LastIndex(n, "."); j >= 0 {
					n = n[j+1:]
				}
				return n
			}
			return ""
		}
	}
	return ""
}

// sqlInsertEvidence: INSERT INTO t (…, status, …) VALUES (…, 'new', …) creates a row in that status.
func sqlInsertEvidence(ev *funcEvidence, sql string, values map[string][]*machine) {
	sql = sqlComment.ReplaceAllString(sql, "")
	loc := sqlInsert.FindStringSubmatchIndex(sql)
	if loc == nil {
		return
	}
	cols := strings.Split(sql[loc[2]:loc[3]], ",")
	var vals []string
	depth, start := 0, loc[1]
	for i := loc[1]; i < len(sql); i++ {
		c := sql[i]
		if c == '(' {
			depth++
		} else if c == ')' && depth > 0 {
			depth--
		} else if (c == ',' && depth == 0) || (c == ')' && depth == 0) {
			vals = append(vals, strings.TrimSpace(sql[start:i]))
			start = i + 1
			if c == ')' {
				break
			}
		}
	}
	for i, col := range cols {
		if i >= len(vals) || !sqlStatusCol.MatchString(strings.TrimSpace(col)) {
			continue
		}
		v := vals[i]
		if len(v) < 2 || v[0] != '\'' || v[len(v)-1] != '\'' {
			continue
		}
		v = v[1 : len(v)-1]
		mach := pickMachine(sql, []string{v}, values)
		if mach == nil {
			continue
		}
		l := mach.values[v]
		ev.addTo(mach, l)
		if ev.insert == nil {
			ev.insert = map[mlabel]bool{}
		}
		ev.insert[mlabel{mach, l}] = true
	}
}

type mlabel struct {
	m     *machine
	label string
}

func (e *funcEvidence) addFrom(m *machine, l string) {
	if e.from == nil {
		e.from = map[*machine][]string{}
	}
	for _, x := range e.from[m] {
		if x == l {
			return
		}
	}
	e.from[m] = append(e.from[m], l)
}

func (e *funcEvidence) addTo(m *machine, l string) {
	for _, x := range e.to {
		if x.m == m && x.label == l {
			return
		}
	}
	e.to = append(e.to, mlabel{m, l})
}

// ctxKind says how a reference to a state is used at masked[start:end].
// "from": compared; "to": assigned or passed to an update; "": other.
func ctxKind(masked string, start, end int, language string) (kind, callee string) {
	before := strings.TrimRight(masked[:start], " \t")
	after := strings.TrimLeft(masked[end:], " \t")
	for _, op := range []string{"===", "!==", "==", "!="} {
		if strings.HasSuffix(before, op) || strings.HasPrefix(after, op) {
			return "from", ""
		}
	}
	if caseBefore.MatchString(before) || inBefore.MatchString(before) {
		return "from", ""
	}
	if strings.HasSuffix(before, "=") {
		var prev byte
		if len(before) >= 2 {
			prev = before[len(before)-2]
		}
		if prev == '<' || prev == '>' || prev == '=' || prev == '!' {
			return "", ""
		}
		lhsText := strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(before, "="), ":"), " \t")
		lm := lhsRe.FindStringSubmatch(lhsText)
		if lm == nil || !statusWord.MatchString(lm[1][strings.LastIndex(lm[1], ".")+1:]) {
			return "", ""
		}
		if prev == ':' || strings.Contains(lm[1], ".") {
			return "to", ""
		}
		switch strings.ToLower(unclosedCall(masked, start)) { // keyword argument or plain variable
		case "filter", "get", "exclude":
			return "from", ""
		}
		return "to", ""
	}
	// a field of a composite literal: Params{Status: string(StatusPaid)} — a write unless the type reads (ListFilter…)
	if language == "go" || language == "js" || language == "ts" || language == "tsx" || language == "jsx" {
		if fm := fieldKeyRe.FindStringSubmatch(before); fm != nil && statusWord.MatchString(fm[1]) && !caseBefore.MatchString(before) {
			if t := braceType(masked, start); t != "" && !readCallRe.MatchString(t) && !regexp.MustCompile(`(?i)filter|query|where|search`).MatchString(t) {
				// a new object (&Order{…}, CreateOrderParams{…}, NewX(…, Params{…})) is born in that status
				bi := strings.LastIndex(masked[:start], t+"{")
				outer := ""
				if bi >= 0 {
					if oc := outerCalls(masked, bi, 1); len(oc) > 0 {
						outer = oc[0]
					}
				}
				if createRe.MatchString(t) || createRe.MatchString(outer) || (outer == "" && !regexp.MustCompile(`Params$|Args$|Input$|Update`).MatchString(t)) {
					return "new", ""
				}
				return "to", ""
			}
		}
	}
	if c := unclosedCall(masked, start); c != "" {
		prefix := strings.TrimRight(before, " \t,(")
		if prefix != "" && transCallee.MatchString(c) && !strings.HasSuffix(prefix, ":") {
			return "to", c
		}
	}
	return "", ""
}

func normalize(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(s))
}

// analyzeFunc collects state evidence from the statements of one function.
func analyzeFunc(sts []stmt, ms []*machine, tokens map[string][]*machine, values map[string][]*machine, consts, queries map[string]string, language string, set setters, ret map[string][]mlabel) funcEvidence {
	var ev funcEvidence
	usedSQL := map[string]bool{}
	// the function's own name (a handler named like the sqlc query) is not a use of the query
	self := ""
	if len(sts) > 0 {
		if m := defName.FindStringSubmatch(sts[0].masked); m != nil {
			self = m[1]
		}
	}
	for si, st := range sts {
		for _, loc := range chainRe.FindAllStringIndex(st.masked, -1) {
			chain := strings.Join(strings.Fields(strings.ReplaceAll(st.masked[loc[0]:loc[1]], " ", "")), "")
			segs := strings.Split(chain, ".")
			for len(segs) > 1 && (segs[len(segs)-1] == "value" || segs[len(segs)-1] == "name") && language != "go" {
				segs = segs[:len(segs)-1]
			}
			if name := segs[len(segs)-1]; !usedSQL[name] && !(si == 0 && name == self) {
				if q := consts[name] + queries[name]; q != "" {
					usedSQL[name] = true
					sqlEvidence(&ev, q, values)
					sqlInsertEvidence(&ev, q, values)
				}
			}
			var cands []*machine
			var tok string
			if language == "go" {
				tok = segs[len(segs)-1]
			} else if len(segs) >= 2 {
				tok = segs[len(segs)-2] + "." + segs[len(segs)-1]
			}
			cands = tokens[tok]
			if len(cands) != 1 {
				continue
			}
			m := cands[0]
			label := m.refs[tok]
			kind, callee := ctxKind(st.masked, loc[0], loc[1], language)
			switch kind {
			case "from":
				ev.addFrom(m, label)
			case "to":
				if fid, ok := set[callee]; ok && callee != "" {
					ev.addVia(m, label, fid)
				} else {
					ev.addTo(m, label)
				}
				if mm := ifSuffix.FindStringSubmatch(callee); mm != nil {
					for _, l := range m.labels {
						if normalize(l) == normalize(mm[1]) {
							if ev.ifFrom == nil {
								ev.ifFrom = map[*machine]string{}
							}
							ev.ifFrom[m] = l
						}
					}
				}
			case "new":
				ev.addNew(m, label)
			case "":
				if fid, ok := setterCall(st.masked, loc[0], set); ok {
					ev.addVia(m, label, fid)
				}
			}
		}
		for _, l := range st.lits {
			ms2 := values[l.val]
			if len(ms2) == 1 && len(l.val) < 40 {
				m := ms2[0]
				label := m.values[l.val]
				start := l.start - 1
				if start < 0 {
					continue
				}
				kind, callee := ctxKind(st.masked, start, l.end+1, language)
				// a bare literal counts only next to a status-like name
				ok := false
				switch kind {
				case "to":
					ok = callee != "" && statusWord.MatchString(callee) || callee == "" && true
				case "from":
					ok = statusNear(st.masked, start, l.end+1)
				case "new":
					ev.addNew(m, label)
				case "":
					if fid, isSet := setterCall(st.masked, start, set); isSet {
						ev.addVia(m, label, fid)
					}
				}
				if !ok {
					continue
				}
				if fid, isSet := set[callee]; kind == "to" && isSet && callee != "" {
					ev.addVia(m, label, fid)
				} else if kind == "to" {
					ev.addTo(m, label)
				} else {
					ev.addFrom(m, label)
				}
			}
			if len(l.val) > 12 {
				sqlEvidence(&ev, l.val, values)
				sqlInsertEvidence(&ev, l.val, values)
			}
		}
	}
	mappedEvidence(&ev, sts, set, ret)
	return ev
}

var goAssignCall = regexp.MustCompile(`\b(\w+)\s*:?=\s*(?:[\w.]+\.)?(\w+)\s*\(`)

// mappedEvidence: v := MapXToStatus(…) and then Setter(…, v): every status the mapper returns can be
// written. The source is not known (the comparisons around are on the new value), so From stays empty.
func mappedEvidence(ev *funcEvidence, sts []stmt, set setters, ret map[string][]mlabel) {
	if len(ret) == 0 || len(set) == 0 {
		return
	}
	vars := map[string][]mlabel{}
	for _, st := range sts {
		for _, m := range goAssignCall.FindAllStringSubmatch(st.masked, -1) {
			if ls, ok := ret[m[2]]; ok {
				vars[m[1]] = ls
			}
		}
	}
	for v, ls := range vars {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(v) + `\b`)
		for _, st := range sts {
			for _, loc := range re.FindAllStringIndex(st.masked, -1) {
				fid, ok := setterCall(st.masked, loc[0], set)
				if !ok {
					continue
				}
				for _, l := range ls {
					ev.addVia(l.m, l.label, fid)
					if ev.noFrom == nil {
						ev.noFrom = map[mlabel]bool{}
					}
					ev.noFrom[l] = true
				}
			}
		}
	}
}

var goReturnTok = regexp.MustCompile(`\breturn\s+(?:\w+\.)?(\w+)\s*(?:\n|$|;|,\s*nil)`)

// statusReturners: functions that return the constants of one machine (a mapper) → those states.
func statusReturners(bodies map[string]string, tokens map[string][]*machine) map[string][]mlabel {
	out := map[string][]mlabel{}
	amb := map[string]bool{}
	for _, body := range bodies {
		name, _ := goParams(body, nil)
		if name == "" {
			if m := goSigRe.FindStringSubmatch(body); m != nil {
				name = m[1]
			}
		}
		if name == "" {
			continue
		}
		var m *machine
		var ls []mlabel
		ok := true
		for _, r := range goReturnTok.FindAllStringSubmatch(body, -1) {
			ms := tokens[r[1]]
			if len(ms) != 1 {
				continue
			}
			if m != nil && ms[0] != m {
				ok = false
				break
			}
			m = ms[0]
			l := mlabel{m, m.refs[r[1]]}
			dup := false
			for _, x := range ls {
				dup = dup || x == l
			}
			if !dup {
				ls = append(ls, l)
			}
		}
		if !ok || m == nil || len(ls) < 2 {
			continue
		}
		if _, seen := out[name]; seen {
			amb[name] = true
		}
		out[name] = ls
	}
	for n := range amb {
		delete(out, n)
	}
	return out
}

// statusNear reports whether a status-like name is the other side of the comparison.
func statusNear(masked string, start, end int) bool {
	before := strings.TrimRight(masked[:start], " \t=!")
	if m := lhsRe.FindStringSubmatch(before); m != nil && statusWord.MatchString(m[1]) {
		return true
	}
	after := strings.TrimLeft(masked[end:], " \t=!")
	if m := chainRe.FindString(after); m != "" && statusWord.MatchString(m) {
		return true
	}
	return false
}

var sqlTable = regexp.MustCompile(`(?i)\bupdate\s+(?:only\s+)?(?:\w+\.)?(\w+)|\binsert\s+into\s+(?:\w+\.)?(\w+)`)

// tableNames: the machine key names the table — orders → OrderStatus, kaspi_certificates → KaspiCertStatus
// (every word of the table, singular, or its first four letters, is in the key).
func tableNames(table, key string) bool {
	k := strings.ToLower(key)
	for _, w := range strings.Split(table, "_") {
		w = strings.TrimSuffix(w, "s")
		if w == "" {
			continue
		}
		if !strings.Contains(k, w) && (len(w) < 4 || !strings.Contains(k, w[:4])) {
			return false
		}
	}
	return true
}

// pickMachine chooses the machine an SQL statement works on: the one that has
// all the quoted states of the statement, and if several do, the one named
// after the table (orders -> OrderStatus).
func pickMachine(sql string, vals []string, values map[string][]*machine) *machine {
	var cands []*machine
	for _, m := range values[vals[0]] {
		ok := true
		for _, v := range vals[1:] {
			if _, has := m.values[v]; !has {
				ok = false
			}
		}
		if ok {
			cands = append(cands, m)
		}
	}
	if len(cands) > 1 {
		if t := sqlTable.FindStringSubmatch(sql); t != nil {
			tbl := strings.ToLower(t[1] + t[2])
			var named []*machine
			for _, m := range cands {
				if tableNames(tbl, m.key) {
					named = append(named, m)
				}
			}
			cands = named
		}
	}
	if len(cands) == 1 {
		return cands[0]
	}
	return nil
}

func sqlEvidence(ev *funcEvidence, sql string, values map[string][]*machine) {
	sql = sqlComment.ReplaceAllString(sql, "") // "-- WHERE на status …" in a comment must not cut the SET off
	w := sqlWhere.FindString(sql)
	m := sqlSet.FindStringSubmatch(strings.TrimSuffix(sql, w)) // SET lives before WHERE
	if m == nil {
		return
	}
	var from []string
	if w != "" {
		for _, fm := range sqlFrom.FindAllStringSubmatch(w, -1) {
			if fm[2] == "" {
				from = append(from, fm[1])
				continue
			}
			for _, v := range sqlVal.FindAllStringSubmatch(fm[2], -1) {
				from = append(from, v[1])
			}
		}
	}
	mach := pickMachine(sql, append([]string{m[2]}, from...), values)
	if mach == nil { // a WHERE on another column's status must not hide the SET
		if mach = pickMachine(sql, []string{m[2]}, values); mach == nil {
			return
		}
	}
	ev.addTo(mach, mach.values[m[2]])
	for _, v := range from {
		if l, ok := mach.values[v]; ok {
			ev.addFrom(mach, l)
		}
	}
}

// maxFromPerTransition: more distinct source states than this are reported
// without a From instead of as a fan of transitions.
const maxFromPerTransition = 3

// detectStates finds state machines and their transitions.
func detectStates(files []stFile, funcs []stFunc, queries map[string]string) map[string]*facts.StateMachine {
	ms := collectMachines(files)
	if len(ms) == 0 {
		return nil
	}
	tokens := map[string][]*machine{}
	values := map[string][]*machine{}
	for _, m := range ms {
		for t := range m.refs {
			tokens[t] = append(tokens[t], m)
		}
		for v := range m.values {
			values[v] = append(values[v], m)
		}
	}
	// Go tokens are bare names; the same name in two machines is ambiguous and dropped
	byFile := map[string]stFile{}
	for _, f := range files {
		byFile[f.Path] = f
	}
	lineCache := map[string][]string{}
	constCache := map[string]map[string]string{}
	type tkey struct {
		m            *machine
		from, to, id string
	}
	seen := map[tkey]bool{}
	res := map[*machine]*facts.StateMachine{}
	bodyOf := func(fn stFunc) (stFile, string, bool) {
		f, ok := byFile[fn.File]
		if !ok {
			return f, "", false
		}
		lines, ok := lineCache[fn.File]
		if !ok {
			lines = strings.Split(f.Text, "\n")
			lineCache[fn.File] = lines
		}
		if fn.Start < 1 || fn.End > len(lines) || fn.Start > fn.End {
			return f, "", false
		}
		return f, strings.Join(lines[fn.Start-1:fn.End], "\n"), true
	}
	goBodies := map[string]string{}
	for _, fn := range funcs {
		if f, body, ok := bodyOf(fn); ok && f.Lang == "go" {
			goBodies[fn.Fid] = body
		}
	}
	set := findSetters(goBodies, ms)
	ret := statusReturners(goBodies, tokens)
	for _, fn := range funcs {
		f, body, ok := bodyOf(fn)
		if !ok {
			continue
		}
		consts, done := constCache[fn.File]
		if !done {
			consts = sqlConsts(f)
			constCache[fn.File] = consts
		}
		ev := analyzeFunc(statements(body, f.Lang), ms, tokens, values, consts, queries, f.Lang, set, ret)
		for _, t := range ev.to {
			if ev.insert[t] { // a new row: created in this status, there is no source
				k := tkey{t.m, "", t.label, fn.Fid}
				if !seen[k] {
					seen[k] = true
					sm := res[t.m]
					if sm == nil {
						sm = &facts.StateMachine{States: append([]string(nil), t.m.labels...)}
						res[t.m] = sm
					}
					sm.Transitions = append(sm.Transitions, facts.Transition{To: t.label, Fid: fn.Fid, Source: "code", Insert: true})
				}
				continue
			}
			froms := []string{}
			if ev.noFrom[t] {
				froms = []string{""}
			} else if l := ev.ifFrom[t.m]; l != "" {
				froms = []string{l}
			} else {
				for _, l := range ev.from[t.m] {
					if l != t.label {
						froms = append(froms, l)
					}
				}
			}
			if len(froms) == 0 || len(froms) > maxFromPerTransition {
				froms = []string{""}
			}
			for _, from := range froms {
				k := tkey{t.m, from, t.label, fn.Fid}
				if seen[k] {
					continue
				}
				seen[k] = true
				sm := res[t.m]
				if sm == nil {
					sm = &facts.StateMachine{States: append([]string(nil), t.m.labels...)}
					res[t.m] = sm
				}
				sm.Transitions = append(sm.Transitions, facts.Transition{From: from, To: t.label, Fid: fn.Fid, Source: "code", Via: ev.via[t]})
			}
		}
	}
	finals := map[*machine][]string{}
	for _, f := range files {
		for m, l := range goFinals(f, tokens) {
			finals[m] = append(finals[m], l...)
		}
	}
	out := map[string]*facts.StateMachine{}
	for m, sm := range res {
		sm.Doc = m.doc
		if len(m.docs) > 0 {
			sm.Docs = m.docs
		}
		sm.Final = finals[m]
		sort.SliceStable(sm.Transitions, func(i, j int) bool {
			a, b := sm.Transitions[i], sm.Transitions[j]
			if a.Fid != b.Fid {
				return a.Fid < b.Fid
			}
			if a.From != b.From {
				return a.From < b.From
			}
			return a.To < b.To
		})
		out[m.key] = sm
	}
	return out
}

var goSQLConst = regexp.MustCompile("(?s)\\b(\\w+)\\s*=\\s*`([^`]*)`")

// sqlConsts returns the raw SQL strings that a Go file keeps in constants (the
// shape sqlc generates), by constant name; a function that uses the constant
// gets the SQL as its own.
func sqlConsts(f stFile) map[string]string {
	if f.Lang != "go" || !strings.Contains(strings.ToLower(f.Text), "set ") {
		return nil
	}
	out := map[string]string{}
	for _, m := range goSQLConst.FindAllStringSubmatch(f.Text, -1) {
		if sqlSet.MatchString(m[2]) {
			out[m[1]] = m[2]
		}
	}
	return out
}

var sqlcName = regexp.MustCompile(`(?m)^--\s*name:\s*(\w+)`)

// sqlQueries reads named SQL queries (sqlc style, `-- name: Foo :one`) from the
// .sql files of the repository, by query name. The generated Go code that
// runs them is usually left out of the map, so a transition is attributed to
// the hand-written function that calls the query.
func sqlQueries(root string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "migrations", "migrate", ".archmap":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".sql") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 1<<20 {
			return nil
		}
		text := string(b)
		idx := sqlcName.FindAllStringSubmatchIndex(text, -1)
		for i, m := range idx {
			end := len(text)
			if i+1 < len(idx) {
				end = idx[i+1][0]
			}
			out[text[m[2]:m[3]]] = text[m[1]:end]
		}
		return nil
	})
	return out
}

// mergeAnnotatedStates adds the transitions declared with `arch:state`
// (Function.Arch.State) to out.States with Source "annotation". The machine is
// the one whose key equals the entity (case-insensitive, or the entity plus a
// Status/State suffix); with none, a machine keyed by the entity is created.
// A code transition with the same Fid, From and To is not duplicated: it is
// kept and marked Confirmed together with the annotation one.
func mergeAnnotatedStates(out *facts.Facts) {
	fids := make([]string, 0, len(out.Functions))
	for fid, fn := range out.Functions {
		if fn.Arch != nil && len(fn.Arch.State) > 0 {
			fids = append(fids, fid)
		}
	}
	if len(fids) == 0 {
		return
	}
	sort.Strings(fids)
	if out.States == nil {
		out.States = map[string]*facts.StateMachine{}
	}
	keys := make([]string, 0, len(out.States))
	for k := range out.States {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	find := func(entity string) string {
		e := strings.ToLower(entity)
		for _, k := range keys {
			if strings.ToLower(k) == e {
				return k
			}
		}
		// Order → OrderStatus before OrderMetaStatus: the entity name plus the suffix alone first
		for _, suf := range []string{"status", "state", "stage", "phase"} {
			for _, k := range keys {
				if strings.ToLower(k) == e+suf {
					return k
				}
			}
		}
		for _, k := range keys {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, e) && statusTypeRe.MatchString(lk) && len(lk) > len(e) {
				return k
			}
		}
		return entity
	}
	for _, fid := range fids {
		for _, st := range out.Functions[fid].Arch.State {
			key := find(st.Entity)
			sm := out.States[key]
			if sm == nil {
				sm = &facts.StateMachine{}
				out.States[key] = sm
				keys = append(keys, key)
				sort.Strings(keys)
			}
			has := map[string]bool{}
			for _, s := range sm.States {
				has[s] = true
			}
			for _, s := range []string{st.From, st.To} {
				if s != "" && !has[s] {
					has[s] = true
					sm.States = append(sm.States, s)
				}
			}
			dup := false
			for i := range sm.Transitions {
				t := &sm.Transitions[i]
				if t.Fid == fid && t.From == st.From && t.To == st.To {
					t.Confirmed = true
					dup = true
				}
			}
			if !dup {
				sm.Transitions = append(sm.Transitions, facts.Transition{From: st.From, To: st.To, Fid: fid, Source: "annotation"})
			}
		}
	}
	for _, sm := range out.States {
		sort.SliceStable(sm.Transitions, func(i, j int) bool {
			a, b := sm.Transitions[i], sm.Transitions[j]
			if a.Fid != b.Fid {
				return a.Fid < b.Fid
			}
			if a.From != b.From {
				return a.From < b.From
			}
			return a.To < b.To
		})
	}
}
