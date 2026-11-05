// Package diagdiff compares the "diagrams" of two revisions of a repository —
// scenarios (arch:flow and automatic ones), journeys (arch:journey) and state
// machines (arch:state, .arch `states`) — for a pull request comment
// (`archmap diff`).
//
// A Snapshot is what one revision shows, built from facts, the spec and the
// autosem model (no extraction of its own); Compare turns two snapshots into a
// Report; Text, Markdown and JSON print it.
package diagdiff

import (
	"sort"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lint"
	"github.com/aidarbn/archmap/internal/spec"
)

// Diagram kinds.
const (
	KindFlow    = "flow"
	KindJourney = "journey"
	KindStates  = "states"
)

// Step is a step of a flow.
type Step struct {
	Fid  string `json:"fid"`
	Text string `json:"text,omitempty"` // caption (arch:step / spec), empty for automatic flows
}

// Transition is a state change; Fids are the functions that make it.
type Transition struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Fids []string `json:"fids,omitempty"`
}

// Flow is a scenario as one revision shows it.
type Flow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Declared bool   `json:"declared"` // arch:flow or spec
	Spec     bool   `json:"spec,omitempty"`
	// Top and Described decide whether an automatic flow is compared (like the
	// viewer shows it): among the top ones, or its entry point has a doc comment.
	Top       bool   `json:"top,omitempty"`
	Described bool   `json:"described,omitempty"`
	Steps     []Step `json:"steps"`
}

// Shown reports whether the flow takes part in the comparison.
func (f *Flow) Shown() bool { return f != nil && (f.Declared || f.Top || f.Described) }

// Entry is the entry point (first step), "" without steps.
func (f *Flow) Entry() string {
	if len(f.Steps) == 0 {
		return ""
	}
	return f.Steps[0].Fid
}

// Journey is a path over flows.
type Journey struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Declared bool     `json:"declared"`
	Top      bool     `json:"top,omitempty"`
	Flows    []string `json:"flows"`
}

// Shown reports whether the journey takes part in the comparison.
func (j *Journey) Shown() bool { return j != nil && (j.Declared || j.Top) }

// Machine is a state machine of one entity.
type Machine struct {
	Entity      string       `json:"entity"`
	States      []string     `json:"states"`
	Transitions []Transition `json:"transitions"`
}

// Func is what the comparison needs to know about a function.
type Func struct {
	Name  string `json:"name"` // Cls.name or name
	File  string `json:"file"`
	Line  int    `json:"line"`
	Entry bool   `json:"entry,omitempty"` // an entry point (http, task, ...)
	// Flows: ids of arch:flow on it; Steps: flows it is a step of (arch:step or a spec step).
	Flows map[string]bool `json:"-"`
	Steps map[string]bool `json:"-"`
}

// Broken kinds.
const (
	BrokenJourneyFlow = "journey-flow" // a journey names a flow that does not exist
	BrokenOnFlow      = "on-flow"      // arch:on / alt -> a flow that does not exist
	BrokenNoEntry     = "no-entry"     // arch:step of a flow without arch:flow (no entry point)
	BrokenNoFunction  = "no-function"  // a spec step that names no function
	BrokenUnreachable = "unreachable"  // a step the entry point of its flow does not reach
)

// Broken is a diagram that does not hold together in a revision.
type Broken struct {
	Kind    string `json:"kind"`
	Diagram string `json:"diagram"`        // journey or flow id
	Ref     string `json:"ref"`            // the missing flow id, step ref or fid
	Func    string `json:"func,omitempty"` // display name of the function where it is
	File    string `json:"file"`
	Line    int    `json:"line"`
}

func (b Broken) key() string { return b.Kind + "\x00" + b.Diagram + "\x00" + b.Ref }

// Snapshot is the set of diagrams of one revision.
type Snapshot struct {
	Flows    map[string]*Flow    `json:"flows"`
	Journeys map[string]*Journey `json:"journeys"`
	Machines map[string]*Machine `json:"machines"`
	Funcs    map[string]Func     `json:"-"`
	Broken   []Broken            `json:"broken"`
	// order of first appearance, for stable output
	flowOrder, journeyOrder []string
}

// Build makes the snapshot of a revision from its facts and spec (set may be nil).
func Build(f *facts.Facts, set *spec.Set) *Snapshot {
	m := autosem.BuildWith(f, set)
	sf := spec.MergeStates(f, set)
	s := &Snapshot{Flows: map[string]*Flow{}, Journeys: map[string]*Journey{}, Machines: map[string]*Machine{}, Funcs: map[string]Func{}}

	for fid, fn := range f.Functions {
		x := Func{Name: funcName(fn), File: fn.File, Line: fn.Start, Entry: fn.Entry != nil, Flows: map[string]bool{}, Steps: map[string]bool{}}
		if fn.Arch != nil {
			for _, d := range fn.Arch.Flows {
				x.Flows[d.ID] = true
			}
			for _, st := range fn.Arch.Steps {
				x.Steps[st.Flow] = true
			}
		}
		s.Funcs[fid] = x
	}
	if m.Spec != nil {
		for _, fl := range m.Spec.Flows {
			for _, st := range fl.Steps {
				if x, ok := s.Funcs[st.Fid]; ok && st.Fid != "" {
					x.Steps[fl.ID] = true
				}
			}
		}
	}

	for _, fl := range m.Flows {
		if _, dup := s.Flows[fl.ID]; dup {
			continue
		}
		d := &Flow{ID: fl.ID, Name: fl.Name, Declared: fl.Kind == autosem.KindDeclared, Spec: fl.Spec, Top: fl.Top, Steps: []Step{}}
		for _, st := range fl.Steps {
			d.Steps = append(d.Steps, Step{Fid: st, Text: fl.Labels[st]})
		}
		if e := d.Entry(); e != "" && !d.Declared {
			d.Described = f.Functions[e].Doc != ""
		}
		s.Flows[fl.ID] = d
		s.flowOrder = append(s.flowOrder, fl.ID)
	}
	for _, j := range m.Journeys {
		if _, dup := s.Journeys[j.ID]; dup {
			continue
		}
		s.Journeys[j.ID] = &Journey{ID: j.ID, Name: j.Name, Declared: j.Declared, Top: j.Top, Flows: append([]string{}, j.Flows...)}
		s.journeyOrder = append(s.journeyOrder, j.ID)
	}
	for key, sm := range sf.States {
		mc := &Machine{Entity: key, States: append([]string{}, sm.States...)}
		idx := map[[2]string]int{}
		for _, t := range sm.Transitions {
			k := [2]string{t.From, t.To}
			i, ok := idx[k]
			if !ok {
				i = len(mc.Transitions)
				idx[k] = i
				mc.Transitions = append(mc.Transitions, Transition{From: t.From, To: t.To})
			}
			if t.Fid != "" && !contains(mc.Transitions[i].Fids, t.Fid) {
				mc.Transitions[i].Fids = append(mc.Transitions[i].Fids, t.Fid)
			}
		}
		for i := range mc.Transitions {
			sort.Strings(mc.Transitions[i].Fids)
		}
		s.Machines[key] = mc
	}
	s.Broken = broken(f, m, s)
	return s
}

// broken finds the diagrams that do not hold together (see the Broken kinds).
func broken(f *facts.Facts, m autosem.Model, s *Snapshot) []Broken {
	var out []Broken
	seen := map[string]bool{}
	add := func(b Broken) {
		if !seen[b.key()] {
			seen[b.key()] = true
			out = append(out, b)
		}
	}
	at := func(fid string) (string, string, int) {
		x := s.Funcs[fid]
		return x.Name, x.File, x.Line
	}
	has := func(id string) bool { _, ok := s.Flows[id]; return ok }
	for _, fid := range autosem.NoteOrder(f) {
		a := f.Functions[fid].Arch
		name, file, line := at(fid)
		for _, j := range a.Journeys {
			for _, id := range j.Flows {
				if !has(id) {
					add(Broken{BrokenJourneyFlow, j.ID, id, name, file, line})
				}
			}
		}
		for _, t := range a.Then {
			for _, id := range t.Flows {
				if !has(id) {
					add(Broken{BrokenJourneyFlow, t.Journey, id, name, file, line})
				}
			}
		}
		for _, st := range a.Steps {
			if !has(st.Flow) {
				add(Broken{BrokenNoEntry, st.Flow, fid, name, file, line})
			}
		}
		for _, o := range a.On {
			if o.Flow != "" && !has(o.Flow) {
				add(Broken{BrokenOnFlow, o.Flow, o.Flow, name, file, line})
			}
		}
	}
	for _, u := range lint.UnreachableSteps(f) {
		name, file, line := at(u.Fid)
		add(Broken{BrokenUnreachable, u.Flow, u.Fid, name, file, line})
	}
	if m.Spec != nil {
		for _, fl := range m.Spec.Flows {
			for _, st := range fl.Steps {
				switch st.Drift {
				case spec.DriftMissing:
					add(Broken{BrokenNoFunction, fl.ID, st.Ref, "", fl.File, st.Line})
				case spec.DriftUnreachable:
					name, _, _ := at(st.Fid)
					add(Broken{BrokenUnreachable, fl.ID, st.Fid, name, fl.File, st.Line})
				}
			}
			for _, a := range fl.Alts {
				if a.Flow != "" && !has(a.Flow) {
					add(Broken{BrokenOnFlow, a.Flow, a.Flow, "", fl.File, a.Line})
				}
			}
		}
		for _, j := range m.Spec.Journeys {
			for _, id := range j.Missing {
				add(Broken{BrokenJourneyFlow, j.ID, id, "", j.File, j.Line})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.key() < b.key()
	})
	return out
}

func funcName(fn facts.Function) string {
	if fn.Cls != nil && *fn.Cls != "" {
		return *fn.Cls + "." + fn.Name
	}
	return fn.Name
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
