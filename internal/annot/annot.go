// Package annot parses the `arch:` markup that describes scenarios in code
// comments ("tours through the source"). It works on plain comment text and is
// language independent.
//
// Directives, one per comment line above a function or method:
//
//	arch:flow <id> "Name"          scenario that starts at this function
//	arch:actor <who>               who starts it (applies to the last flow)
//	arch:exit "how it ends"        how it ends (applies to the last flow)
//	arch:step <flow-id> "caption"  this function is a step of the flow
//	arch:ext <System>[, <System>]  the function talks to an external system
//	arch:on <cond> -> <flow-id>    branch: the scenario continues in another flow
//	arch:on <cond> "text"          outcome of the step
//	arch:state <entity>: <from> -> <to>  state transition
//	arch:journey <id> "Name"       end-to-end path over several flows; the first
//	                               flow is the nearest arch:flow of the comment;
//	                               arch:actor / arch:exit below it apply to it
//	arch:journey <id> "Name": a > b > c  the same, flows listed in one line
//	arch:then <journey-id> <flow> [> <flow>...]  appends flows to a journey
package annot

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Prefix starts every directive.
const Prefix = "arch:"

// Line is one comment line with its 1-based number in the file.
type Line struct {
	N    int
	Text string
}

// Error is a syntax problem in a directive.
type Error struct {
	Line int // 1-based line in the file (or in the text, for Parse)
	Msg  string
}

func (e Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// IDRe is the shape of a flow id.
var IDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)

var stateRe = regexp.MustCompile(`^([^:]+?)\s*:\s*(.+?)\s*->\s*(.+)$`)

// ParseText parses the raw text of a comment block (lines numbered from first).
func ParseText(text string, first int) (*facts.ArchNote, []Error) {
	var ls []Line
	for i, l := range strings.Split(text, "\n") {
		ls = append(ls, Line{first + i, l})
	}
	return Parse(ls)
}

// Parse parses cleaned comment lines (no comment markers). Lines that do not
// start with "arch:" are prose and ignored. It returns nil when there is no
// directive.
func Parse(lines []Line) (*facts.ArchNote, []Error) {
	var (
		n    facts.ArchNote
		errs []Error
		seen bool
		// what arch:actor / arch:exit apply to: the last flow or journey
		last = ""
		// block journeys that still need their first flow: index in n.Journeys,
		// line, and how many flows were declared above them
		pend []pendingJourney
	)
	for _, l := range lines {
		t := strings.TrimSpace(l.Text)
		if !strings.HasPrefix(t, Prefix) {
			continue
		}
		seen = true
		body := t[len(Prefix):]
		verb, rest := body, ""
		if i := strings.IndexAny(body, " \t"); i >= 0 {
			verb, rest = body[:i], strings.TrimSpace(body[i+1:])
		}
		bad := func(format string, a ...any) { errs = append(errs, Error{l.N, fmt.Sprintf(format, a...)}) }
		switch verb {
		case "flow":
			tk, err := tokens(rest)
			switch {
			case err != nil:
				bad("arch:flow: %v", err)
			case len(tk) != 2:
				bad(`arch:flow: want <id> "Name"`)
			case !IDRe.MatchString(tk[0]):
				bad("arch:flow: bad id %q", tk[0])
			default:
				n.Flows = append(n.Flows, facts.FlowDecl{ID: tk[0], Name: tk[1]})
				last = "flow"
			}
		case "journey":
			j, err := parseJourney(rest)
			if err != nil {
				bad("arch:journey: %v", err)
				continue
			}
			if len(j.Flows) == 0 {
				pend = append(pend, pendingJourney{len(n.Journeys), l.N, len(n.Flows)})
			}
			n.Journeys = append(n.Journeys, j)
			last = "journey"
		case "then":
			tk := strings.Fields(strings.ReplaceAll(rest, ">", " "))
			ok := len(tk) >= 2 && IDRe.MatchString(tk[0])
			for i := 1; ok && i < len(tk); i++ {
				ok = IDRe.MatchString(tk[i])
			}
			if !ok {
				bad("arch:then: want <journey-id> <flow-id> [> <flow-id>...]")
				continue
			}
			n.Then = append(n.Then, facts.ThenNote{Journey: tk[0], Flows: tk[1:]})
		case "actor", "exit":
			v, err := unquote(rest)
			switch {
			case err != nil:
				bad("arch:%s: %v", verb, err)
			case v == "":
				bad("arch:%s: empty value", verb)
			case last == "":
				bad("arch:%s: no arch:flow or arch:journey above it", verb)
			case last == "journey" && verb == "actor":
				n.Journeys[len(n.Journeys)-1].Actor = v
			case last == "journey":
				n.Journeys[len(n.Journeys)-1].Exit = v
			case verb == "actor":
				n.Flows[len(n.Flows)-1].Actor = v
			default:
				n.Flows[len(n.Flows)-1].Exit = v
			}
		case "step":
			tk, err := tokens(rest)
			switch {
			case err != nil:
				bad("arch:step: %v", err)
			case len(tk) < 1 || len(tk) > 2:
				bad(`arch:step: want <flow-id> "caption"`)
			case !IDRe.MatchString(tk[0]):
				bad("arch:step: bad flow id %q", tk[0])
			default:
				s := facts.StepNote{Flow: tk[0]}
				if len(tk) == 2 {
					s.Text = tk[1]
				}
				n.Steps = append(n.Steps, s)
			}
		case "ext":
			got := 0
			for _, p := range strings.Split(rest, ",") {
				v, err := unquote(strings.TrimSpace(p))
				if err != nil {
					bad("arch:ext: %v", err)
					continue
				}
				if v != "" {
					n.Ext = append(n.Ext, v)
					got++
				}
			}
			if got == 0 {
				bad("arch:ext: want a system name")
			}
		case "on":
			o, err := parseOn(rest)
			if err != nil {
				bad("arch:on: %v", err)
				continue
			}
			n.On = append(n.On, o)
		case "state":
			m := stateRe.FindStringSubmatch(rest)
			if m == nil {
				bad("arch:state: want <entity>: <from> -> <to>")
				continue
			}
			from, e1 := unquote(m[2])
			to, e2 := unquote(m[3])
			if e1 != nil || e2 != nil || from == "" || to == "" {
				bad("arch:state: bad states")
				continue
			}
			n.State = append(n.State, facts.StateNote{Entity: m[1], From: from, To: to})
		default:
			bad("unknown directive arch:%s", verb)
		}
	}
	// the first flow of a block journey: the nearest arch:flow above it, else
	// the first one below it
	var drop []int
	for _, p := range pend {
		switch {
		case p.flowsAbove > 0:
			n.Journeys[p.idx].Flows = []string{n.Flows[p.flowsAbove-1].ID}
		case len(n.Flows) > 0:
			n.Journeys[p.idx].Flows = []string{n.Flows[0].ID}
		default:
			errs = append(errs, Error{p.line, `arch:journey: no arch:flow in this comment (put it on the entry point of the first flow, or use the list form: arch:journey <id> "Name": a > b)`})
			drop = append(drop, p.idx)
		}
	}
	for i := len(drop) - 1; i >= 0; i-- {
		n.Journeys = append(n.Journeys[:drop[i]], n.Journeys[drop[i]+1:]...)
	}
	if !seen || n.Empty() {
		return nil, errs
	}
	return &n, errs
}

type pendingJourney struct{ idx, line, flowsAbove int }

// parseJourney parses `<id> "Name"` (block form: the first flow is the nearest
// arch:flow) or `<id> "Name": a > b > c` (list form).
func parseJourney(rest string) (facts.JourneyDecl, error) {
	want := fmt.Errorf(`want <id> "Name" or <id> "Name": <flow> > <flow>`)
	i := strings.IndexAny(rest, " \t")
	if i < 0 {
		return facts.JourneyDecl{}, want
	}
	id, tail := rest[:i], strings.TrimSpace(rest[i+1:])
	if !IDRe.MatchString(id) {
		return facts.JourneyDecl{}, fmt.Errorf("bad id %q", id)
	}
	if tail == "" || (tail[0] != '"' && !strings.HasPrefix(tail, "“")) {
		return facts.JourneyDecl{}, want
	}
	end := closer([]rune(tail)[0])
	rs := []rune(tail)
	j := 1
	for j < len(rs) && rs[j] != end && (end != '”' || rs[j] != '"') {
		j++
	}
	if j >= len(rs) {
		return facts.JourneyDecl{}, fmt.Errorf("unterminated quote")
	}
	name := string(rs[1:j])
	after := strings.TrimSpace(string(rs[j+1:]))
	d := facts.JourneyDecl{ID: id, Name: name}
	if name == "" {
		return d, want
	}
	if after == "" {
		return d, nil
	}
	if after[0] != ':' {
		return d, want
	}
	for _, f := range strings.Split(after[1:], ">") {
		f = strings.TrimSpace(f)
		if !IDRe.MatchString(f) {
			return d, fmt.Errorf("bad flow id %q in the list", f)
		}
		d.Flows = append(d.Flows, f)
	}
	return d, nil
}

func parseOn(rest string) (facts.OnNote, error) {
	if i := strings.Index(rest, "->"); i >= 0 {
		cond, err := unquote(strings.TrimSpace(rest[:i]))
		if err != nil || cond == "" {
			return facts.OnNote{}, fmt.Errorf("want <condition> -> <flow-id>")
		}
		tk, err := tokens(strings.TrimSpace(rest[i+2:]))
		if err != nil {
			return facts.OnNote{}, err
		}
		if len(tk) < 1 || len(tk) > 2 || !IDRe.MatchString(tk[0]) {
			return facts.OnNote{}, fmt.Errorf("want a flow id after ->")
		}
		o := facts.OnNote{Cond: cond, Flow: tk[0]}
		if len(tk) == 2 {
			o.Text = tk[1]
		}
		return o, nil
	}
	if i := strings.IndexAny(rest, `"“`); i > 0 {
		cond := strings.TrimSpace(rest[:i])
		tk, err := tokens(rest[i:])
		if err != nil {
			return facts.OnNote{}, err
		}
		if cond != "" && len(tk) == 1 && tk[0] != "" {
			return facts.OnNote{Cond: cond, Text: tk[0]}, nil
		}
	}
	return facts.OnNote{}, fmt.Errorf(`want <condition> -> <flow-id> or <condition> "text"`)
}

func closer(r rune) rune {
	if r == '“' {
		return '”'
	}
	return '"'
}

// tokens splits s on spaces; "double quoted" parts (also “ ”) are one token
// without the quotes.
func tokens(s string) ([]string, error) {
	var out []string
	rs := []rune(strings.TrimSpace(s))
	for i := 0; i < len(rs); {
		switch rs[i] {
		case ' ', '\t':
			i++
		case '"', '“':
			end := closer(rs[i])
			j := i + 1
			for j < len(rs) && rs[j] != end && (end != '”' || rs[j] != '"') {
				j++
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated quote")
			}
			out = append(out, string(rs[i+1:j]))
			i = j + 1
		default:
			j := i
			for j < len(rs) && rs[j] != ' ' && rs[j] != '\t' {
				j++
			}
			out = append(out, string(rs[i:j]))
			i = j
		}
	}
	return out, nil
}

// unquote trims s and removes one pair of surrounding quotes.
func unquote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if s[0] == '"' || strings.HasPrefix(s, "“") {
		tk, err := tokens(s)
		if err != nil {
			return "", err
		}
		if len(tk) != 1 {
			return "", fmt.Errorf("text after the closing quote")
		}
		return tk[0], nil
	}
	return s, nil
}
