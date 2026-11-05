package spec

import (
	"sort"
	"strings"
	"unicode"

	"github.com/aidarbn/archmap/internal/facts"
)

// Pages: `page [id] "Name"` with one frame per indented line. The page only
// picks diagrams and puts them side by side; every frame is drawn from the code
// by the viewer, and Check verifies that each reference points to something
// the code (or the spec) has.

// PageCols is the number of columns of the page grid.
const PageCols = 4

// FrameSpan is how many grid columns a frame takes: scenarios (journey, flow)
// and state machines are the wide diagrams and take two, everything else one.
func FrameSpan(kind string) int {
	if kind == FrameJourney || kind == FrameFlow || kind == FrameStates {
		return 2
	}
	return 1
}

// LayoutPage places the frames on the grid: the large ones first (journeys and
// flows, then state machines), then the rest, each group in the order of the
// lines; left to right, a frame that does not fit the row starts the next one.
func LayoutPage(frames []Frame) []Slot {
	rank := func(k string) int {
		switch k {
		case FrameJourney, FrameFlow:
			return 0
		case FrameStates:
			return 1
		}
		return 2
	}
	var order []int
	for r := 0; r <= 2; r++ {
		for i, fr := range frames {
			if rank(fr.Kind) == r {
				order = append(order, i)
			}
		}
	}
	out := make([]Slot, 0, len(order))
	row, col := 0, 0
	for _, i := range order {
		span := min(FrameSpan(frames[i].Kind), PageCols)
		if col > 0 && col+span > PageCols {
			row, col = row+1, 0
		}
		out = append(out, Slot{Frame: i, Row: row, Col: col, Span: span})
		col += span
	}
	return out
}

// FindEndpoint returns the indexes of the endpoints a page reference names:
// the method name ("CancelOrder"), Service.Name ("TabletsService.CancelOrder"),
// the URL path ("/tablet/v2/order/{order_id}/cancel") or "METHOD path".
func FindEndpoint(f *facts.Facts, ref string) []int {
	if f == nil {
		return nil
	}
	var out []int
	for i, e := range f.Endpoints {
		switch {
		case e.Name != "" && e.Name == ref,
			e.Service != "" && e.Name != "" && e.Service+"."+e.Name == ref,
			e.Path != "" && e.Path == ref,
			e.Method != "" && e.Path != "" && strings.EqualFold(e.Method+" "+e.Path, ref):
			out = append(out, i)
		}
	}
	return out
}

// FindModule returns the module a page names: the exact path, else the one
// module whose path ends with "/"+ref ("services/kaspi" for
// "internal/services/kaspi"). Several candidates come back sorted, with "".
func FindModule(f *facts.Facts, ref string) (string, []string) {
	if f == nil {
		return "", nil
	}
	mods := map[string]bool{}
	for m := range f.Modules {
		mods[m] = true
	}
	for _, fn := range f.Functions {
		if fn.Module != "" {
			mods[fn.Module] = true
		}
	}
	if mods[ref] {
		return ref, nil
	}
	var cands []string
	for m := range mods {
		if strings.HasSuffix(m, "/"+ref) {
			cands = append(cands, m)
		}
	}
	sort.Strings(cands)
	if len(cands) == 1 {
		return cands[0], nil
	}
	return "", cands
}

// tableNames: the tables of the schema plus those only seen in data accesses.
func tableNames(f *facts.Facts) map[string]bool {
	out := map[string]bool{}
	if f == nil {
		return out
	}
	if f.Data != nil {
		for n := range f.Data.Tables {
			out[n] = true
		}
	}
	for _, fn := range f.Functions {
		for _, a := range fn.Data {
			if a.Table != "" {
				out[a.Table] = true
			}
		}
	}
	return out
}

// checkPages resolves the references of every page frame (filling Machines,
// Endpoints, Modules, Missing) and reports the broken ones.
func checkPages(m *Model, f *facts.Facts, flows, journeys map[string]bool, add func(file string, line int, sev, code, format string, a ...any)) {
	specMachine := map[string]bool{}
	for _, st := range m.States {
		specMachine[st.Entity] = true
	}
	tables := tableNames(f)
	for pi := range m.Pages {
		pg := &m.Pages[pi]
		pg.Layout = LayoutPage(pg.Frames)
		for fi := range pg.Frames {
			fr := &pg.Frames[fi]
			miss := func(r, format string, a ...any) {
				fr.Missing = append(fr.Missing, r)
				add(pg.File, fr.Line, SevError, CodePageRef, "page %s: "+format, append([]any{pg.ID}, a...)...)
			}
			switch fr.Kind {
			case FrameJourney:
				for _, r := range fr.Refs {
					if !journeys[r] {
						miss(r, "journey %q: no such journey (arch:journey or a journey in arch/*.arch)", r)
					}
				}
			case FrameFlow:
				for _, r := range fr.Refs {
					if !flows[r] {
						miss(r, "flow %q: no such flow (arch:flow or a flow in arch/*.arch)", r)
					}
				}
			case FrameStates:
				for _, r := range fr.Refs {
					key := FindMachine(f, r)
					if key == "" && specMachine[r] {
						key = r
					}
					fr.Machines = append(fr.Machines, key)
					if key == "" {
						miss(r, "states %q: the code has no state machine of this entity", r)
					}
				}
			case FrameTables:
				for _, r := range fr.Refs {
					if !tables[r] {
						miss(r, "tables %q: no such table in the schema or in data access", r)
					}
				}
			case FrameAPI:
				for _, r := range fr.Refs {
					idx := FindEndpoint(f, r)
					switch {
					case len(idx) == 1:
						fr.Endpoints = append(fr.Endpoints, idx[0])
					case len(idx) > 1:
						fr.Endpoints = append(fr.Endpoints, -1)
						var names []string
						for _, i := range idx {
							e := f.Endpoints[i]
							names = append(names, strings.TrimSpace(e.Service+"."+e.Name+" "+e.Method+" "+e.Path))
						}
						miss(r, "api %q matches several endpoints (write Service.Name or the path): %s", r, strings.Join(names, ", "))
					default:
						fr.Endpoints = append(fr.Endpoints, -1)
						miss(r, "api %q: no such endpoint", r)
					}
				}
			case FrameModule:
				for _, r := range fr.Refs {
					mod, cands := FindModule(f, r)
					fr.Modules = append(fr.Modules, mod)
					switch {
					case mod != "":
					case len(cands) > 1:
						miss(r, "module %q matches several modules: %s", r, strings.Join(cands, ", "))
					default:
						miss(r, "module %q: no such module in the code", r)
					}
				}
			}
		}
	}
}

func isFrameKind(s string) bool {
	for _, k := range FrameKinds {
		if k == s {
			return true
		}
	}
	return false
}

// Slug makes a page id from its name: lower case, letters and digits kept
// (any script), everything else folded into single "-".
func Slug(name string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if dash && sb.Len() > 0 {
				sb.WriteByte('-')
			}
			dash = false
			sb.WriteRune(r)
			continue
		}
		dash = true
	}
	return sb.String()
}

func (p *parser) page(n int, ts []tok) {
	bad := func() { p.errAt(n, ts[0].col, "expected `page \"Name\"` or `page <id> \"Name\"`") }
	pg := &Page{Line: n}
	switch {
	case len(ts) == 2 && ts[1].quoted:
		pg.Name = strings.TrimSpace(ts[1].s)
		pg.ID = Slug(pg.Name)
		if pg.ID == "" {
			p.errAt(n, ts[1].col, "page %q: the name gives an empty id; write `page <id> \"Name\"`", ts[1].s)
			return
		}
	case len(ts) == 2 && !ts[1].quoted:
		if !isIdent(ts[1].s, false) {
			bad()
			return
		}
		pg.ID, pg.Name = ts[1].s, ts[1].s
	case len(ts) == 3 && !ts[1].quoted && ts[2].quoted:
		if !isIdent(ts[1].s, false) {
			p.errAt(n, ts[1].col, "%q is not a valid page id", ts[1].s)
			return
		}
		pg.ID, pg.Name = ts[1].s, strings.TrimSpace(ts[2].s)
		if pg.Name == "" {
			pg.Name = pg.ID
		}
	default:
		bad()
		return
	}
	pg.Comment = p.takeComment()
	p.pg = pg
	p.doc.Pages = append(p.doc.Pages, pg)
}

// refs splits `a, b c` (commas and spaces) into words; quoted strings are
// single refs when quoted is allowed (api: "POST /orders").
func (p *parser) refs(n int, ts []tok, quoted bool, valid func(string) bool, what string) ([]string, bool) {
	var out []string
	for _, t := range ts {
		if t.quoted {
			s := strings.TrimSpace(t.s)
			if !quoted || s == "" {
				p.errAt(n, t.col, "unexpected string; %s are written without quotes", what)
				return nil, false
			}
			out = append(out, s)
			continue
		}
		for _, w := range strings.FieldsFunc(t.s, func(r rune) bool { return r == ',' }) {
			if !valid(w) {
				p.errAt(n, t.col, "%q is not a valid name in %s", w, what)
				return nil, false
			}
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		p.errAt(n, ts[0].col, "`%s` needs at least one name", ts[0].s)
		return nil, false
	}
	return out, true
}

func (p *parser) pageLine(n int, ts []tok) {
	kw := ts[0].s
	if ts[0].quoted || !isFrameKind(kw) {
		p.errAt(n, ts[0].col, "unknown line %q in page (expected journey, flow, states, tables, api, module or note)", ts[0].s)
		return
	}
	fr := Frame{Kind: kw, Line: n}
	switch kw {
	case FrameNote:
		v, ok := p.text(n, ts)
		if !ok {
			return
		}
		fr.Text = v
	case FrameJourney, FrameFlow:
		rs, ok := p.refsAfter(n, ts, false, func(s string) bool { return isIdent(s, false) }, kw+" ids")
		if !ok {
			return
		}
		if len(rs) != 1 {
			p.errAt(n, ts[1].col, "one %s per line: `%s <id>`", kw, kw)
			return
		}
		fr.Refs = rs
	case FrameStates:
		rs, ok := p.refsAfter(n, ts, false, func(s string) bool { return isIdent(s, false) }, "entity names")
		if !ok {
			return
		}
		fr.Refs = rs
	case FrameTables:
		rs, ok := p.refsAfter(n, ts, false, func(s string) bool { return isIdent(s, false) }, "table names")
		if !ok {
			return
		}
		fr.Refs = rs
	case FrameAPI:
		rs, ok := p.refsAfter(n, ts, true, isEndpointRef, "endpoint names")
		if !ok {
			return
		}
		fr.Refs = rs
	case FrameModule:
		rs, ok := p.refsAfter(n, ts, false, isModule, "module paths")
		if !ok {
			return
		}
		fr.Refs = rs
	}
	p.pg.Frames = append(p.pg.Frames, fr)
}

func (p *parser) refsAfter(n int, ts []tok, quoted bool, valid func(string) bool, what string) ([]string, bool) {
	if len(ts) < 2 {
		p.errAt(n, ts[0].col, "`%s` needs at least one name", ts[0].s)
		return nil, false
	}
	return p.refs(n, ts[1:], quoted, valid, what)
}

// isEndpointRef: Name, Service.Name or a URL path ("/orders/{id}").
func isEndpointRef(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t\"#") && !strings.Contains(s, "->")
}

// pageHeader is the `page` line of a page, without the newline: the shortest
// form that parses back to the same id and name.
func pageHeader(id, name string) string {
	switch {
	case name == "" || name == id:
		return "page " + id
	case Slug(name) == id:
		return "page " + quote(name)
	default:
		return "page " + id + " " + quote(name)
	}
}

// frameValue is what follows the keyword on a frame line.
func frameValue(fr Frame) string {
	if fr.Kind == FrameNote {
		return quote(fr.Text)
	}
	parts := make([]string, len(fr.Refs))
	for k, r := range fr.Refs {
		if strings.ContainsAny(r, " \t,#\"") {
			r = quote(r)
		}
		parts[k] = r
	}
	return strings.Join(parts, ", ")
}

// framesWidth is the width of the longest keyword of the frames: the values
// stand in one column, one space after it.
func framesWidth(frames []Frame) int {
	w := 0
	for _, fr := range frames {
		w = max(w, width(fr.Kind))
	}
	return w
}

func formatPage(pg *Page) string {
	var sb strings.Builder
	sb.WriteString(pageHeader(pg.ID, pg.Name) + "\n")
	w := framesWidth(pg.Frames)
	for _, fr := range pg.Frames {
		sb.WriteString("  " + fr.Kind + strings.Repeat(" ", w-width(fr.Kind)+1) + frameValue(fr) + "\n")
	}
	return sb.String()
}
