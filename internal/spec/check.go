package spec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Finding severities.
const (
	SevError   = "error"
	SevWarning = "warning"
	SevInfo    = "info"
)

// Finding codes.
const (
	CodeSyntax      = "syntax"
	CodeDuplicate   = "duplicate"
	CodeNoFunc      = "no-function"
	CodeAmbiguous   = "ambiguous-function"
	CodeUnreachable = "unreachable-step"
	CodePlanned     = "planned"
	CodeNoFlow      = "no-flow"
	CodeUndeclared  = "undeclared-transition"
	CodeNotImpl     = "not-implemented"
	CodeStateRef    = "unknown-state"
)

// Finding is one result of Check: a place in a spec file or in the code.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Msg      string `json:"msg"`
}

// Resolve finds the function a step reference points to. The reference is a
// fid ("path/file.go::Type.Method"), a path suffix of it ("file.go::Func"), or a
// name: "Type.Method", "Func", "Method". More than one match returns the
// candidates (sorted) and an empty fid; none returns nothing.
func Resolve(f *facts.Facts, ref string) (fid string, cands []string) {
	if _, ok := f.Functions[ref]; ok {
		return ref, nil
	}
	tiers := [3][]string{}
	for id := range f.Functions {
		if strings.Contains(ref, "::") {
			if strings.HasSuffix(id, "/"+ref) {
				tiers[0] = append(tiers[0], id)
			}
			continue
		}
		_, tail, ok := strings.Cut(id, "::")
		if !ok {
			continue
		}
		switch {
		case tail == ref:
			tiers[1] = append(tiers[1], id)
		case strings.HasSuffix(tail, "."+ref):
			tiers[2] = append(tiers[2], id)
		}
	}
	for _, t := range tiers {
		if len(t) == 1 {
			return t[0], nil
		}
		if len(t) > 1 {
			sort.Strings(t)
			return "", t
		}
	}
	return "", nil
}

// FindMachine returns the key of the state machine in facts for an entity:
// the same name (any case), or the name plus a Status/State suffix.
func FindMachine(f *facts.Facts, entity string) string {
	if f == nil {
		return ""
	}
	if _, ok := f.States[entity]; ok {
		return entity
	}
	keys := make([]string, 0, len(f.States))
	for k := range f.States {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	e := strings.ToLower(entity)
	for _, k := range keys {
		if strings.ToLower(k) == e {
			return k
		}
	}
	for _, k := range keys {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, e) && (strings.HasSuffix(lk, "status") || strings.HasSuffix(lk, "state")) {
			return k
		}
	}
	return ""
}

// matches reports whether the code transition c is the spec transition t.
// A code transition with an unknown source state matches by target alone.
func matches(t Transition, c facts.Transition) bool {
	return t.To == c.To && (t.From == "*" || c.From == "" || t.From == c.From)
}

// Check compares the spec with the code. It returns the Model with the
// enrichment fields filled in (planned steps, resolved functions, transition
// source and drift) and the findings. reach(entry) lists the functions that
// calls from entry reach (nil skips the reachability check). f may be nil.
func Check(s *Set, f *facts.Facts, reach func(entry string) map[string]bool) (Model, []Finding) {
	m := s.Model()
	var out []Finding
	add := func(file string, line int, sev, code, format string, a ...any) {
		out = append(out, Finding{file, line, sev, code, fmt.Sprintf(format, a...)})
	}
	if s == nil {
		return m, nil
	}
	for _, e := range s.Errors {
		add(e.File, e.Line, SevError, CodeSyntax, "%s", e.Msg)
	}
	if f == nil {
		f = &facts.Facts{}
	}
	dupes(s, add)

	codeFlows, codeJourneys := map[string]bool{}, map[string]bool{}
	for _, fn := range f.Functions {
		if fn.Arch == nil {
			continue
		}
		for _, d := range fn.Arch.Flows {
			codeFlows[d.ID] = true
		}
		for _, j := range fn.Arch.Journeys {
			codeJourneys[j.ID] = true
		}
	}
	specFlows := s.FlowIDs()

	// state machines
	for i := range m.States {
		st := &m.States[i]
		key := FindMachine(f, st.Entity)
		st.Code = key
		var code []facts.Transition
		if key != "" {
			code = f.States[key].Transitions
		}
		for k := range st.Transitions {
			t := &st.Transitions[k]
			for _, c := range code {
				if matches(*t, c) {
					t.Fid = c.Fid
					break
				}
			}
			if t.Fid == "" {
				t.Drift = DriftNotImplemented
				add(st.File, t.Line, SevWarning, CodeNotImpl, "states %s: %s -> %s is in the spec, but the code has no such transition (not implemented)", st.Entity, t.From, t.To)
			}
		}
		for _, c := range code {
			declared := false
			for _, t := range st.Transitions {
				if matches(t, c) {
					declared = true
					break
				}
			}
			if declared || c.To == "" {
				continue
			}
			src := c.Source
			if src == "" {
				src = SourceCode
			}
			st.Transitions = append(st.Transitions, Transition{From: c.From, To: c.To, Source: src, Drift: DriftUndeclared, Fid: c.Fid})
			fn := f.Functions[c.Fid]
			from := c.From
			if from == "" {
				from = "?"
			}
			add(fn.File, max(fn.Start, 1), SevError, CodeUndeclared, "states %s: the code moves %s -> %s, which the spec (%s:%d) does not declare", st.Entity, from, c.To, st.File, st.Line)
		}
	}
	machine := map[string]*States{}
	for i := range m.States {
		machine[m.States[i].Entity] = &m.States[i]
	}

	// flows
	for i := range m.Flows {
		fl := &m.Flows[i]
		fl.Code = codeFlows[fl.ID]
		prev, first := "", ""
		for k := range fl.Steps {
			s := &fl.Steps[k]
			if s.Planned {
				fl.Planned++
				add(fl.File, s.Line, SevInfo, CodePlanned, "flow %s: planned step %q (no code yet)", fl.ID, s.Text)
				continue
			}
			fid, cands := Resolve(f, s.Ref)
			switch {
			case fid != "":
				s.Fid = fid
			case len(cands) > 1:
				s.Drift = DriftAmbiguous
				if len(cands) > 5 {
					cands = append(cands[:5:5], "...")
				}
				add(fl.File, s.Line, SevError, CodeAmbiguous, "flow %s: step %q matches several functions: %s", fl.ID, s.Ref, strings.Join(cands, ", "))
				continue
			default:
				s.Drift = DriftMissing
				add(fl.File, s.Line, SevError, CodeNoFunc, "flow %s: step %q: no such function in the code", fl.ID, s.Ref)
				continue
			}
			if prev != "" && prev != fid && reach != nil && !reach(prev)[fid] && !reach(first)[fid] {
				s.Drift = DriftUnreachable
				add(fl.File, s.Line, SevWarning, CodeUnreachable, "flow %s: step %s is not reachable by calls from the previous step %s", fl.ID, s.Ref, f.Functions[prev].Name)
			}
			if first == "" {
				first = fid
			}
			prev = fid
		}
		for _, a := range fl.Alts {
			if a.Flow != "" && !specFlows[a.Flow] && !codeFlows[a.Flow] {
				add(fl.File, a.Line, SevError, CodeNoFlow, "flow %s: alt %s -> %q: no such flow", fl.ID, a.Cond, a.Flow)
			}
		}
		for _, r := range fl.States {
			if st := machine[r.Entity]; st != nil && len(st.Transitions)+len(st.States) > 0 {
				for _, n := range []string{r.From, r.To} {
					if !stateKnown(st, n) {
						add(fl.File, r.Line, SevWarning, CodeStateRef, "flow %s: state %s: %q is not in `states %s`", fl.ID, r.Entity, n, r.Entity)
					}
				}
			}
		}
	}

	// journeys
	for i := range m.Journeys {
		j := &m.Journeys[i]
		j.Code = codeJourneys[j.ID]
		for _, id := range j.Flows {
			if !specFlows[id] && !codeFlows[id] {
				j.Missing = append(j.Missing, id)
				add(j.File, j.Line, SevError, CodeNoFlow, "journey %s: no such flow %q", j.ID, id)
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].File != out[b].File {
			return out[a].File < out[b].File
		}
		return out[a].Line < out[b].Line
	})
	return m, out
}

func stateKnown(st *States, n string) bool {
	for _, x := range st.States {
		if x == n {
			return true
		}
	}
	for _, x := range st.Final {
		if x == n {
			return true
		}
	}
	for _, t := range st.Transitions {
		if t.Source == SourceSpec && (t.From == n || t.To == n) {
			return true
		}
	}
	return false
}

func dupes(s *Set, add func(file string, line int, sev, code, format string, a ...any)) {
	type at struct {
		file string
		line int
	}
	seen := map[string]at{}
	check := func(kind, id, file string, line int) {
		k := kind + " " + id
		if p, ok := seen[k]; ok {
			add(file, line, SevError, CodeDuplicate, "%s %q is already declared at %s:%d", kind, id, p.file, p.line)
			return
		}
		seen[k] = at{file, line}
	}
	for _, f := range s.Files {
		for _, st := range f.Doc.States {
			check("states", st.Entity, f.Path, st.Line)
		}
		for _, fl := range f.Doc.Flows {
			check("flow", fl.ID, f.Path, fl.Line)
		}
		for _, j := range f.Doc.Journeys {
			check("journey", j.ID, f.Path, j.Line)
		}
	}
}

// MergeStates returns a copy of f whose state machines also hold the
// specification: for every `states` entity the spec transitions that the code
// lacks are added with Source "spec" (Drift "not_implemented"; a `*` source is
// expanded to the known states), code transitions the spec does not declare get
// Drift "undeclared", and declared ones take the trigger and label of the spec.
func MergeStates(f *facts.Facts, s *Set) *facts.Facts {
	if s.Empty() {
		return f
	}
	m := s.Model()
	g := *f
	g.States = make(map[string]*facts.StateMachine, len(f.States))
	for k, v := range f.States {
		g.States[k] = v
	}
	for _, st := range m.States {
		key := FindMachine(f, st.Entity)
		if key == "" {
			key = st.Entity
		}
		sm := &facts.StateMachine{}
		var codeTr []facts.Transition
		if old := f.States[key]; old != nil {
			sm.States = append(sm.States, old.States...)
			sm.Transitions = append(sm.Transitions, old.Transitions...)
			codeTr = old.Transitions
		}
		has := map[string]bool{}
		for _, n := range sm.States {
			has[n] = true
		}
		addState := func(n string) {
			if n != "" && n != "*" && !has[n] {
				has[n] = true
				sm.States = append(sm.States, n)
			}
		}
		final := map[string]bool{}
		for _, n := range st.Final {
			final[n] = true
		}
		for _, n := range st.States {
			addState(n)
		}
		for _, n := range st.Final {
			addState(n)
		}
		for _, t := range st.Transitions {
			addState(t.From)
			addState(t.To)
		}
		for i := range sm.Transitions {
			c := &sm.Transitions[i]
			declared := false
			for _, t := range st.Transitions {
				if matches(t, *c) {
					declared = true
					if c.On == "" {
						c.On = t.On
					}
					if c.Label == "" {
						c.Label = t.Label
					}
				}
			}
			if !declared {
				c.Drift = DriftUndeclared
			}
		}
		for _, t := range st.Transitions {
			implemented := false
			for _, c := range codeTr {
				if matches(t, c) {
					implemented = true
					break
				}
			}
			if implemented {
				continue
			}
			froms := []string{t.From}
			if t.From == "*" {
				froms = nil
				for _, n := range sm.States {
					if n != t.To && !final[n] {
						froms = append(froms, n)
					}
				}
			}
			for _, from := range froms {
				sm.Transitions = append(sm.Transitions, facts.Transition{From: from, To: t.To, Source: SourceSpec, On: t.On, Label: t.Label, Drift: DriftNotImplemented})
			}
		}
		g.States[key] = sm
	}
	return &g
}
