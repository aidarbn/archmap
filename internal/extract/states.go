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

func goMachines(set *machineSet, f stFile) {
	lines := strings.Split(f.Text, "\n")
	inBlock := false
	prevTyp, prevIota := "", false
	for _, ln := range lines {
		if i := strings.Index(ln, "//"); i >= 0 && !strings.Contains(ln[:i], `"`) {
			ln = ln[:i]
		}
		var name, typ, rhs string
		switch {
		case goConstBlock.MatchString(ln):
			inBlock, prevTyp, prevIota = true, "", false
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
		switch {
		case typ != "" && statusTypeRe.MatchString(typ):
			label := name
			if isStr {
				label = val
			} else if strings.HasPrefix(name, typ) && len(name) > len(typ) {
				label = name[len(typ):]
			}
			v := ""
			if isStr {
				v = val
			}
			set.get(f.Module, typ).add(name, v, label)
		case typ == "" && isStr:
			if m := constPrefixRe.FindStringSubmatch(name); m != nil {
				set.get(f.Module, m[1]).add(name, val, val)
			}
		}
	}
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
func analyzeFunc(sts []stmt, ms []*machine, tokens map[string][]*machine, values map[string][]*machine, consts, queries map[string]string, language string) funcEvidence {
	var ev funcEvidence
	usedSQL := map[string]bool{}
	for _, st := range sts {
		for _, loc := range chainRe.FindAllStringIndex(st.masked, -1) {
			chain := strings.Join(strings.Fields(strings.ReplaceAll(st.masked[loc[0]:loc[1]], " ", "")), "")
			segs := strings.Split(chain, ".")
			for len(segs) > 1 && (segs[len(segs)-1] == "value" || segs[len(segs)-1] == "name") && language != "go" {
				segs = segs[:len(segs)-1]
			}
			if name := segs[len(segs)-1]; !usedSQL[name] {
				if q := consts[name] + queries[name]; q != "" {
					usedSQL[name] = true
					sqlEvidence(&ev, q, values)
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
				ev.addTo(m, label)
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
				}
				if !ok {
					continue
				}
				if kind == "to" {
					ev.addTo(m, label)
				} else {
					ev.addFrom(m, label)
				}
			}
			if len(l.val) > 12 {
				sqlEvidence(&ev, l.val, values)
			}
		}
	}
	return ev
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

var sqlTable = regexp.MustCompile(`(?i)\bupdate\s+(?:only\s+)?(?:\w+\.)?(\w+)`)

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
			tbl := strings.TrimSuffix(strings.ToLower(t[1]), "s") // orders -> order
			var named []*machine
			for _, m := range cands {
				if strings.Contains(strings.ToLower(m.key), tbl) {
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
	for _, fn := range funcs {
		f, ok := byFile[fn.File]
		if !ok {
			continue
		}
		lines, ok := lineCache[fn.File]
		if !ok {
			lines = strings.Split(f.Text, "\n")
			lineCache[fn.File] = lines
		}
		if fn.Start < 1 || fn.End > len(lines) || fn.Start > fn.End {
			continue
		}
		body := strings.Join(lines[fn.Start-1:fn.End], "\n")
		consts, done := constCache[fn.File]
		if !done {
			consts = sqlConsts(f)
			constCache[fn.File] = consts
		}
		ev := analyzeFunc(statements(body, f.Lang), ms, tokens, values, consts, queries, f.Lang)
		for _, t := range ev.to {
			froms := []string{}
			if l := ev.ifFrom[t.m]; l != "" {
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
				sm.Transitions = append(sm.Transitions, facts.Transition{From: from, To: t.label, Fid: fn.Fid, Source: "code"})
			}
		}
	}
	out := map[string]*facts.StateMachine{}
	for m, sm := range res {
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
