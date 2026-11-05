package lint

import (
	"fmt"
	"sort"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
)

// Unreachable is an `arch:step <flow>` marker on a function that the call graph
// does not reach from the entry point of the flow: the diagram would show a
// step the code never takes from there.
type Unreachable struct {
	Flow  string `json:"flow"`
	Fid   string `json:"fid"`   // the marked function
	Entry string `json:"entry"` // the entry point of the flow (arch:flow)
	File  string `json:"file"`  // where the marked function is
	Line  int    `json:"line"`
}

// Msg is the lint message of the finding.
func (u Unreachable) Msg(f *facts.Facts) string {
	fn, en := f.Functions[u.Fid], f.Functions[u.Entry]
	return fmt.Sprintf("step of flow %q is not reachable by calls from its entry point %s: %s -> %s (%s:%d)",
		u.Flow, en.Name, u.Flow, fn.Name, u.File, u.Line)
}

// UnreachableSteps lists the `arch:step` markers whose function is not
// reachable from the entry point of their flow over the same edges autosem
// walks to build scenarios: direct calls, calls through fields and interfaces
// (resolved by the extractor into Callees) and queue hand-offs (AsyncCallees),
// so a worker step of the same flow counts as reachable. A step that only a
// branch (`arch:on -> other`) leads to belongs to the other flow and is checked
// against that flow's entry point. Markers of flows without an arch:flow (spec
// flows, typos) are not listed here. Sorted by file and line.
//
// A call made through a function value, reflection or code generation that the
// extractor does not resolve is invisible here and shows up as a false finding.
func UnreachableSteps(f *facts.Facts) []Unreachable {
	entryOf := map[string]string{}
	for _, fid := range autosem.DeclOrder(f) {
		for _, d := range f.Functions[fid].Arch.Flows {
			if _, ok := entryOf[d.ID]; !ok {
				entryOf[d.ID] = fid
			}
		}
	}
	reach := map[string]map[string]bool{}
	var out []Unreachable
	for fid, fn := range f.Functions {
		if fn.Arch == nil {
			continue
		}
		for _, s := range fn.Arch.Steps {
			entry, ok := entryOf[s.Flow]
			if !ok || entry == fid {
				continue
			}
			if reach[entry] == nil {
				reach[entry] = autosem.Reachable(f, entry)
			}
			if !reach[entry][fid] {
				out = append(out, Unreachable{Flow: s.Flow, Fid: fid, Entry: entry, File: fn.File, Line: fn.Start})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Flow < b.Flow
	})
	return out
}

// UnbackedState is an `arch:state` transition the code does not make: neither
// the annotated function nor anything it calls (synchronously or through a
// queue) writes the target state of that entity, as far as the extractor sees.
type UnbackedState struct {
	Entity string `json:"entity"` // as written in the markup
	From   string `json:"from"`
	To     string `json:"to"`
	Fid    string `json:"fid"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// Msg is the lint message of the finding.
func (u UnbackedState) Msg(f *facts.Facts) string {
	return fmt.Sprintf("arch:state %s: %s -> %s is not in the code: %s and the functions it calls never set %q (%s:%d)",
		u.Entity, u.From, u.To, f.Functions[u.Fid].Name, u.To, u.File, u.Line)
}

// UnbackedStates lists the `arch:state` annotations with no code transition to
// the same state of the same machine made by the annotated function or by a
// function it reaches. The source state is not compared: the extractor often
// cannot see it. Sorted by file and line.
func UnbackedStates(f *facts.Facts) []UnbackedState {
	// annotation transitions sit in the machines next to the code ones (see
	// extract.mergeAnnotatedStates); find the machine of each annotation
	machineOf := map[string]map[string]string{} // fid -> "from\x00to" -> machine key
	for key, sm := range f.States {
		for _, t := range sm.Transitions {
			if t.Source != "annotation" && !t.Confirmed {
				continue
			}
			if machineOf[t.Fid] == nil {
				machineOf[t.Fid] = map[string]string{}
			}
			machineOf[t.Fid][t.From+"\x00"+t.To] = key
		}
	}
	reach := map[string]map[string]bool{}
	var out []UnbackedState
	for fid, fn := range f.Functions {
		if fn.Arch == nil {
			continue
		}
		for _, st := range fn.Arch.State {
			key, ok := machineOf[fid][st.From+"\x00"+st.To]
			backed := false
			if sm := f.States[key]; ok && sm != nil {
				if reach[fid] == nil {
					reach[fid] = autosem.Reachable(f, fid)
				}
				for _, t := range sm.Transitions {
					if t.Source == "code" && t.To == st.To && reach[fid][t.Fid] {
						backed = true
						break
					}
				}
			}
			if !backed {
				out = append(out, UnbackedState{Entity: st.Entity, From: st.From, To: st.To, Fid: fid, File: fn.File, Line: fn.Start})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.To < b.To
	})
	return out
}
