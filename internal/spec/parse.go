package spec

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type tok struct {
	s      string
	quoted bool
	col    int // 1-based rune column
}

type perr struct {
	col int
	msg string
}

func (e *perr) Error() string { return e.msg }

// tokenize splits a line into words and "quoted strings"; `#` outside quotes
// starts a comment.
func tokenize(line string) ([]tok, *perr) {
	var out []tok
	rs := []rune(line)
	i := 0
	for i < len(rs) {
		c := rs[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			return out, nil
		case c == '"':
			start := i
			i++
			var sb strings.Builder
			closed := false
			for i < len(rs) {
				if rs[i] == '\\' && i+1 < len(rs) {
					switch rs[i+1] {
					case 'n':
						sb.WriteRune('\n')
					case '"', '\\':
						sb.WriteRune(rs[i+1])
					default:
						return out, &perr{i + 1, fmt.Sprintf("unknown escape \\%c (use \\\" \\\\ \\n)", rs[i+1])}
					}
					i += 2
					continue
				}
				if rs[i] == '"' {
					closed = true
					i++
					break
				}
				sb.WriteRune(rs[i])
				i++
			}
			if !closed {
				return out, &perr{start + 1, "unterminated string"}
			}
			if i < len(rs) && rs[i] != ' ' && rs[i] != '\t' && rs[i] != ':' && rs[i] != '#' {
				return out, &perr{i + 1, "a space is expected after a closing quote"}
			}
			out = append(out, tok{sb.String(), true, start + 1})
		default:
			start := i
			for i < len(rs) && rs[i] != ' ' && rs[i] != '\t' && rs[i] != '\r' && rs[i] != '#' {
				if rs[i] == '"' {
					return out, &perr{i + 1, "a quote in the middle of a word"}
				}
				i++
			}
			out = append(out, tok{string(rs[start:i]), false, start + 1})
		}
	}
	return out, nil
}

// splitArrow turns `a->b` words into `a`, `->`, `b`.
func splitArrow(ts []tok) []tok {
	var out []tok
	for _, t := range ts {
		if t.quoted || t.s == "->" || !strings.Contains(t.s, "->") {
			out = append(out, t)
			continue
		}
		col := t.col
		parts := strings.Split(t.s, "->")
		for i, p := range parts {
			if p != "" {
				out = append(out, tok{p, false, col})
			}
			col += utf8.RuneCountInString(p)
			if i < len(parts)-1 {
				out = append(out, tok{"->", false, col})
				col += 2
			}
		}
	}
	return out
}

func isIdent(s string, star bool) bool {
	if s == "" {
		return false
	}
	if star && s == "*" {
		return true
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return s[0] != '-' && s[0] != '.'
}

func isRef(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '.', r == ':', r == '/', r == '$', r == '*', r == '<', r == '>', r == '[', r == ']':
		default:
			return false
		}
	}
	return true
}

type parser struct {
	file string
	errs []Error
	doc  *Doc

	st *States
	fl *Flow
	jn *Journey
}

func (p *parser) errAt(line, col int, format string, a ...any) {
	p.errs = append(p.errs, Error{File: p.file, Line: line, Col: col, Msg: fmt.Sprintf(format, a...)})
}

// Parse reads one .arch file. It keeps going after an error so that all of
// them are reported; the returned Doc holds what could be read.
func Parse(file, text string) (*Doc, []Error) {
	p := &parser{file: file, doc: &Doc{}}
	for i, raw := range strings.Split(text, "\n") {
		n := i + 1
		raw = strings.TrimRight(raw, "\r")
		ts, e := tokenize(raw)
		if e != nil {
			p.errAt(n, e.col, "%s", e.msg)
			continue
		}
		if len(ts) == 0 {
			continue
		}
		indented := raw[0] == ' ' || raw[0] == '\t'
		if !indented {
			p.header(n, ts)
			continue
		}
		p.body(n, ts)
	}
	return p.doc, p.errs
}

func (p *parser) header(n int, ts []tok) {
	p.st, p.fl, p.jn = nil, nil, nil
	kw := ts[0]
	switch kw.s {
	case "states":
		if len(ts) != 2 || ts[1].quoted || !isIdent(ts[1].s, false) {
			p.errAt(n, kw.col, "expected `states <Entity>`")
			return
		}
		p.st = &States{Entity: ts[1].s, Line: n}
		p.doc.States = append(p.doc.States, p.st)
	case "flow":
		if len(ts) < 2 || len(ts) > 3 || ts[1].quoted || !isIdent(ts[1].s, false) || (len(ts) == 3 && !ts[2].quoted) {
			p.errAt(n, kw.col, "expected `flow <id> [\"name\"]`")
			return
		}
		p.fl = &Flow{ID: ts[1].s, Line: n}
		if len(ts) == 3 {
			p.fl.Name = ts[2].s
		}
		p.doc.Flows = append(p.doc.Flows, p.fl)
	case "journey":
		p.journey(n, ts)
	default:
		if kw.s == "step" || kw.s == "actor" || kw.s == "exit" || kw.s == "alt" || kw.s == "final" || kw.s == "state" {
			p.errAt(n, kw.col, "`%s` must be indented inside states/flow", kw.s)
			return
		}
		p.errAt(n, kw.col, "unknown block %q (expected states, flow or journey)", kw.s)
	}
}

func (p *parser) journey(n int, ts []tok) {
	bad := func() { p.errAt(n, ts[0].col, "expected `journey <id> [\"name\"]: flow > flow > ...`") }
	if len(ts) < 3 || ts[1].quoted {
		bad()
		return
	}
	id := ts[1].s
	rest := ts[2:]
	colon := false
	if strings.HasSuffix(id, ":") {
		id = strings.TrimSuffix(id, ":")
		colon = true
	}
	name := ""
	if !colon && rest[0].quoted {
		name = rest[0].s
		rest = rest[1:]
	}
	var sb strings.Builder
	if colon {
		sb.WriteString(":")
	}
	for _, t := range rest {
		if t.quoted {
			bad()
			return
		}
		sb.WriteString(t.s)
	}
	chain, ok := strings.CutPrefix(sb.String(), ":")
	if !isIdent(id, false) || !ok {
		bad()
		return
	}
	var flows []string
	for _, f := range strings.Split(chain, ">") {
		if !isIdent(f, false) {
			p.errAt(n, ts[1].col, "journey %q: %q is not a flow id", id, f)
			return
		}
		flows = append(flows, f)
	}
	p.jn = &Journey{ID: id, Name: name, Flows: flows, Line: n}
	p.doc.Journeys = append(p.doc.Journeys, p.jn)
}

func (p *parser) body(n int, ts []tok) {
	switch {
	case p.st != nil:
		p.statesLine(n, ts)
	case p.fl != nil:
		p.flowLine(n, ts)
	case p.jn != nil:
		p.errAt(n, ts[0].col, "a journey has no body; write it on one line")
	default:
		p.errAt(n, ts[0].col, "indented line outside of states/flow")
	}
}

func (p *parser) names(n int, ts []tok) []string {
	var out []string
	for _, t := range ts {
		if t.quoted {
			p.errAt(n, t.col, "expected state names")
			return nil
		}
		for _, w := range strings.FieldsFunc(t.s, func(r rune) bool { return r == ',' }) {
			if !isIdent(w, false) {
				p.errAt(n, t.col, "%q is not a valid state name", w)
				return nil
			}
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		p.errAt(n, ts[0].col, "expected state names")
	}
	return out
}

func (p *parser) statesLine(n int, ts []tok) {
	st := p.st
	switch ts[0].s {
	case "final":
		if ns := p.names(n, ts[1:]); ns != nil {
			st.Final = append(st.Final, ns...)
		}
		return
	case "state":
		if ns := p.names(n, ts[1:]); ns != nil {
			st.States = append(st.States, ns...)
		}
		return
	}
	ts = splitArrow(ts)
	if len(ts) < 3 || ts[1].s != "->" || ts[0].quoted || ts[2].quoted {
		p.errAt(n, ts[0].col, "expected `from -> to [on trigger] [\"label\"]`, `state a, b` or `final a, b`")
		return
	}
	t := Transition{From: ts[0].s, To: ts[2].s, Line: n}
	if !isIdent(t.From, true) {
		p.errAt(n, ts[0].col, "%q is not a valid state name", t.From)
		return
	}
	if !isIdent(t.To, false) {
		p.errAt(n, ts[2].col, "%q is not a valid state name", t.To)
		return
	}
	rest := ts[3:]
	if len(rest) > 0 && !rest[0].quoted && rest[0].s == "on" {
		if len(rest) < 2 || rest[1].quoted || !isIdent(rest[1].s, false) {
			p.errAt(n, rest[0].col, "expected a trigger after `on`")
			return
		}
		t.On = rest[1].s
		rest = rest[2:]
	}
	if len(rest) > 0 {
		if !rest[0].quoted || len(rest) > 1 {
			p.errAt(n, rest[0].col, "unexpected %q; a label must be in quotes", rest[0].s)
			return
		}
		t.Label = rest[0].s
	}
	st.Transitions = append(st.Transitions, t)
}

// text reads `keyword text...` or `keyword "text"`.
func (p *parser) text(n int, ts []tok) (string, bool) {
	rest := ts[1:]
	if len(rest) == 0 {
		p.errAt(n, ts[0].col, "`%s` needs a value", ts[0].s)
		return "", false
	}
	if rest[0].quoted {
		if len(rest) > 1 {
			p.errAt(n, rest[1].col, "unexpected text after the string")
			return "", false
		}
		return rest[0].s, true
	}
	parts := make([]string, len(rest))
	for i, t := range rest {
		if t.quoted {
			p.errAt(n, t.col, "mix of words and a quoted string")
			return "", false
		}
		parts[i] = t.s
	}
	return strings.Join(parts, " "), true
}

func (p *parser) flowLine(n int, ts []tok) {
	fl := p.fl
	switch ts[0].s {
	case "actor":
		if v, ok := p.text(n, ts); ok {
			fl.Actor = v
		}
	case "exit":
		if v, ok := p.text(n, ts); ok {
			fl.Exit = v
		}
	case "step":
		p.step(n, ts)
	case "alt":
		p.alt(n, ts)
	case "state":
		p.flowState(n, ts)
	default:
		p.errAt(n, ts[0].col, "unknown line %q in flow (expected actor, step, state, alt, exit)", ts[0].s)
	}
}

func (p *parser) step(n int, ts []tok) {
	if len(ts) < 2 {
		p.errAt(n, ts[0].col, "expected `step <Type.Method|Func|path::fid> [ext System] [\"caption\"]` or `step plan \"caption\"`")
		return
	}
	s := Step{Line: n}
	rest := ts[2:]
	if ts[1].quoted {
		p.errAt(n, ts[1].col, "a step needs a function reference or `plan` before the caption")
		return
	}
	if ts[1].s == "plan" {
		s.Planned = true
	} else {
		if !isRef(ts[1].s) {
			p.errAt(n, ts[1].col, "%q is not a function reference", ts[1].s)
			return
		}
		s.Ref = ts[1].s
	}
	if len(rest) > 0 && !rest[0].quoted && rest[0].s == "ext" {
		if s.Planned {
			p.errAt(n, rest[0].col, "a planned step has no ext")
			return
		}
		if len(rest) < 2 || rest[1].quoted || !isIdent(rest[1].s, false) {
			p.errAt(n, rest[0].col, "expected a system name after `ext`")
			return
		}
		s.Ext = rest[1].s
		rest = rest[2:]
	}
	if len(rest) > 0 {
		if !rest[0].quoted || len(rest) > 1 {
			p.errAt(n, rest[0].col, "unexpected %q; a caption must be in quotes", rest[0].s)
			return
		}
		s.Text = rest[0].s
	}
	if s.Planned && s.Text == "" {
		p.errAt(n, ts[1].col, "a planned step needs a caption: step plan \"...\"")
		return
	}
	p.fl.Steps = append(p.fl.Steps, s)
}

func (p *parser) alt(n int, ts []tok) {
	ts = splitArrow(ts)
	rest := ts[1:]
	var cond []string
	for len(rest) > 0 && !rest[0].quoted && rest[0].s != "->" {
		cond = append(cond, rest[0].s)
		rest = rest[1:]
	}
	if len(cond) == 0 {
		p.errAt(n, ts[0].col, "expected `alt <condition> [-> flow] [\"text\"]`")
		return
	}
	a := Alt{Cond: strings.Join(cond, " "), Line: n}
	if len(rest) > 0 && rest[0].s == "->" && !rest[0].quoted {
		if len(rest) < 2 || rest[1].quoted || !isIdent(rest[1].s, false) {
			p.errAt(n, rest[0].col, "expected a flow id after `->`")
			return
		}
		a.Flow = rest[1].s
		rest = rest[2:]
	}
	if len(rest) > 0 {
		if !rest[0].quoted || len(rest) > 1 {
			p.errAt(n, rest[0].col, "unexpected %q; the text must be in quotes", rest[0].s)
			return
		}
		a.Text = rest[0].s
	}
	p.fl.Alts = append(p.fl.Alts, a)
}

func (p *parser) flowState(n int, ts []tok) {
	var sb strings.Builder
	for i, t := range ts[1:] {
		if t.quoted {
			p.errAt(n, t.col, "unexpected string")
			return
		}
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(t.s)
	}
	bad := func() { p.errAt(n, ts[0].col, "expected `state <Entity>: from -> to`") }
	ent, tr, ok := strings.Cut(sb.String(), ":")
	if !ok {
		bad()
		return
	}
	ent = strings.TrimSpace(ent)
	from, to, ok := strings.Cut(tr, "->")
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if !ok || !isIdent(ent, false) || !isIdent(from, false) || !isIdent(to, false) {
		bad()
		return
	}
	p.fl.States = append(p.fl.States, StateRef{Entity: ent, From: from, To: to, Line: n})
}
