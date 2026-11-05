// Package lint checks the `arch:` scenario markup of a repository: syntax,
// references between flows and reachability of steps along the call graph.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/annot"
	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/spec"
)

// Problem is one finding.
type Problem struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

func (p Problem) String() string { return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Msg) }

// Result is the outcome of a lint run.
type Result struct {
	Problems  []Problem `json:"problems"`
	Warnings  []Problem `json:"warnings"`  // do not fail the run, except with --strict (journey gaps, spec not implemented)
	Info      []Problem `json:"info"`      // planned steps of the spec: not a problem, counted
	Annotated int       `json:"annotated"` // functions with arch: markup
	Flows     int       `json:"flows"`     // distinct declared flow ids
	// The .arch specification (arch/*.arch):
	Spec      bool `json:"spec"`       // spec files declare something
	SpecFlows int  `json:"spec_flows"` // flows declared in the spec
	Pages     int  `json:"pages"`      // pages declared in the spec (arch/pages/*.arch and others)
	Planned   int  `json:"planned"`    // planned steps (no code yet)
	// ClientLinks counts the journey links with no link in the code (via
	// "client": the client calls the next flow); listed in Info.
	ClientLinks int `json:"client_links"`
	// Docs is the description coverage (information only, set by the command).
	Docs *DocCoverage `json:"docs,omitempty"`
}

// Run checks the markup of the functions in f and the specification in
// <root>/arch/*.arch against the code; root is the repository root the file
// paths of f are relative to.
func Run(root string, f *facts.Facts) Result {
	set, err := spec.Load(root)
	if err != nil {
		set = &spec.Set{}
		res := RunWith(root, f, set)
		res.Problems = append(res.Problems, Problem{File: spec.Dir, Msg: "reading the spec: " + err.Error()})
		return res
	}
	return RunWith(root, f, set)
}

// RunWith is Run with an already loaded spec.
func RunWith(root string, f *facts.Facts, set *spec.Set) Result {
	var res Result
	specFlows := set.FlowIDs()
	add := func(file string, line int, format string, a ...any) {
		res.Problems = append(res.Problems, Problem{file, line, fmt.Sprintf(format, a...)})
	}
	fids := make([]string, 0, len(f.Functions))
	for fid := range f.Functions {
		fids = append(fids, fid)
	}
	sort.Strings(fids)

	src := map[string][]string{}
	lines := func(file string) []string {
		if l, ok := src[file]; ok {
			return l
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		var l []string
		if err == nil {
			l = strings.Split(string(b), "\n")
		}
		src[file] = l
		return l
	}
	sites := map[string][]annot.Line{}
	for _, fid := range fids {
		fn := f.Functions[fid]
		bl := annot.Block(lines(fn.File), fn.Start)
		sites[fid] = bl
		if _, errs := annot.Parse(bl); len(errs) > 0 {
			for _, e := range errs {
				add(fn.File, e.Line, "%s", e.Msg)
			}
		}
	}
	// where a directive sits: the first comment line holding all of the parts
	at := func(fid string, parts ...string) int {
		for _, l := range sites[fid] {
			ok := strings.Contains(l.Text, annot.Prefix)
			for _, p := range parts {
				ok = ok && strings.Contains(l.Text, p)
			}
			if ok {
				return l.N
			}
		}
		return f.Functions[fid].Start
	}

	// declared flows
	declaredAt := map[string]string{} // flow id -> entry fid
	for _, fid := range fids {
		if !f.Functions[fid].Arch.Empty() {
			res.Annotated++
		}
	}
	for _, fid := range autosem.DeclOrder(f) {
		fn := f.Functions[fid]
		for _, d := range fn.Arch.Flows {
			if prev, dup := declaredAt[d.ID]; dup {
				add(fn.File, at(fid, "arch:flow", " "+d.ID), "flow %q is already declared at %s", d.ID, prev)
				continue
			}
			declaredAt[d.ID] = fmt.Sprintf("%s:%d", fn.File, at(fid, "arch:flow", " "+d.ID))
			res.Flows++
		}
	}
	entryOf := map[string]string{}
	for _, fid := range autosem.DeclOrder(f) {
		for _, d := range f.Functions[fid].Arch.Flows {
			if _, ok := entryOf[d.ID]; !ok {
				entryOf[d.ID] = fid
			}
		}
	}
	reach := map[string]map[string]bool{}
	for _, fid := range fids {
		fn := f.Functions[fid]
		a := fn.Arch
		if a == nil {
			continue
		}
		for _, s := range a.Steps {
			// reachability from the entry point: UnreachableSteps below
			if _, ok := entryOf[s.Flow]; !ok && !specFlows[s.Flow] {
				add(fn.File, at(fid, "arch:step", " "+s.Flow), "arch:step refers to flow %q, which has no arch:flow (no entry point)", s.Flow)
			}
		}
		for _, o := range a.On {
			if o.Flow != "" {
				if _, ok := entryOf[o.Flow]; !ok && !specFlows[o.Flow] {
					add(fn.File, at(fid, "arch:on", o.Cond), "arch:on -> %q: no such flow", o.Flow)
				}
			}
		}
		for _, e := range a.Ext {
			if strings.ContainsAny(e, ":>\"") {
				add(fn.File, at(fid, "arch:ext"), "arch:ext: %q does not look like a system name", e)
			}
		}
		for _, s := range a.State {
			if s.From == s.To {
				add(fn.File, at(fid, "arch:state", s.Entity), "arch:state %s: %q -> %q changes nothing", s.Entity, s.From, s.To)
			}
		}
	}
	for _, u := range UnreachableSteps(f) {
		add(u.File, at(u.Fid, "arch:step", " "+u.Flow), "%s", u.Msg(f))
	}
	// a warning (an error with --strict): the extractor does not yet see a
	// status passed as an argument or written by SQL text, so on real projects
	// most findings are gaps of the extractor, not of the markup
	for _, u := range UnbackedStates(f) {
		res.Warnings = append(res.Warnings, Problem{u.File, at(u.Fid, "arch:state", u.Entity), u.Msg(f)})
	}
	// journeys: ids, references, gaps
	type site struct {
		file string
		line int
	}
	journeyAt := map[string]site{}
	for _, fid := range autosem.NoteOrder(f) {
		fn := f.Functions[fid]
		for _, j := range fn.Arch.Journeys {
			ln := at(fid, "arch:journey", " "+j.ID)
			if prev, dup := journeyAt[j.ID]; dup {
				add(fn.File, ln, "journey %q is already declared at %s:%d", j.ID, prev.file, prev.line)
				continue
			}
			journeyAt[j.ID] = site{fn.File, ln}
			for _, id := range j.Flows {
				if _, ok := entryOf[id]; !ok && !specFlows[id] {
					add(fn.File, ln, "journey %q: no such flow %q", j.ID, id)
				}
			}
		}
	}
	for _, fid := range autosem.NoteOrder(f) {
		fn := f.Functions[fid]
		for _, t := range fn.Arch.Then {
			ln := at(fid, "arch:then", " "+t.Journey)
			if _, ok := journeyAt[t.Journey]; !ok {
				add(fn.File, ln, "arch:then: no such journey %q", t.Journey)
				continue
			}
			for _, id := range t.Flows {
				if _, ok := entryOf[id]; !ok && !specFlows[id] {
					add(fn.File, ln, "arch:then %s: no such flow %q", t.Journey, id)
				}
			}
		}
	}
	for _, j := range autosem.Build(f).Journeys {
		s, ok := journeyAt[j.ID]
		if !ok || !j.Declared {
			continue
		}
		for _, l := range j.Links {
			if l.Via == autosem.ViaClient {
				// knowledge from outside the code, not a fault: information
				res.Info = append(res.Info, Problem{s.file, s.line, fmt.Sprintf("journey %q: %q -> %q is a client link (no arch:on or queue in the code: the client calls the next flow itself)", j.ID, l.From, l.To)})
				res.ClientLinks++
			}
		}
	}
	// the specification: syntax, references to code, drift
	if !set.Empty() || len(set.Errors) > 0 {
		reachFn := func(entry string) map[string]bool {
			if reach[entry] == nil {
				reach[entry] = autosem.Reachable(f, entry)
			}
			return reach[entry]
		}
		m, findings := spec.Check(set, f, reachFn)
		res.Spec = true
		res.SpecFlows = len(m.Flows)
		res.Pages = len(m.Pages)
		for _, x := range findings {
			p := Problem{File: x.File, Line: x.Line, Msg: x.Msg}
			switch x.Severity {
			case spec.SevError:
				res.Problems = append(res.Problems, p)
			case spec.SevWarning:
				res.Warnings = append(res.Warnings, p)
			default:
				res.Info = append(res.Info, p)
				if x.Code == spec.CodePlanned {
					res.Planned++
				}
			}
		}
	}
	byPos := func(ps []Problem) {
		sort.SliceStable(ps, func(i, j int) bool {
			a, b := ps[i], ps[j]
			if a.File != b.File {
				return a.File < b.File
			}
			return a.Line < b.Line
		})
	}
	byPos(res.Problems)
	byPos(res.Warnings)
	byPos(res.Info)
	if res.Info == nil {
		res.Info = []Problem{}
	}
	if res.Problems == nil {
		res.Problems = []Problem{}
	}
	if res.Warnings == nil {
		res.Warnings = []Problem{}
	}
	return res
}
