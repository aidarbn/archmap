package autosem

import (
	"fmt"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/spec"
)

// PlannedID is the id of a planned spec step (there is no function behind it):
// "plan:<flow id>:<position in the spec flow>".
func PlannedID(flow string, pos int) string { return fmt.Sprintf("plan:%s:%d", flow, pos) }

// PlanStep is a step the spec plans and the code does not have yet
// (`step plan "…" [in module]`). It lives in the separate plan layer of a flow
// (Flow.Plan), never among Steps: a diagram shows what the code does.
type PlanStep struct {
	ID     string `json:"id"` // PlannedID
	Text   string `json:"text"`
	Module string `json:"module,omitempty"` // the module the step is meant for
	// After is the code step (fid) the planned one follows in the spec; "" — before the first.
	After string `json:"after,omitempty"`
}

// BuildWith derives the model from facts and merges the `.arch` specification
// into it (see mergeSpec). A nil or empty set gives the same result as Build.
func BuildWith(f *facts.Facts, set *spec.Set) Model {
	m := buildModel(f, set)
	return m
}

// JSONWith is JSON for BuildWith.
func JSONWith(f *facts.Facts, set *spec.Set) ([]byte, error) {
	return marshalModel(BuildWith(f, set))
}

// mergeSpec puts the spec into the code-derived model. The spec, like the
// arch: markup, only names, describes and adds: which functions are steps and
// in what order they go is decided by the call graph.
//   - a spec flow with the id of a code flow (arch:flow) takes over the name,
//     actor, exit and captions; its steps join the marked ones and all are
//     ordered by the walk from the entry point (declaredPath); code steps the
//     spec does not list are marked in Unspecified;
//   - a spec-only flow walks from its root step (the one no other step of it
//     reaches); steps the walk does not reach go last (lint reports them);
//   - planned steps go to Flow.Plan, not to Steps;
//   - spec alts become Branches; spec journeys replace or extend the code ones,
//     with the links the code shows between their flows (else via "client").
func (g *graph) mergeSpec(m *Model, sm spec.Model) {
	for _, sf := range sm.Flows {
		var specFids []string
		var plan []PlanStep
		labels := map[string]string{}
		ext := map[string]string{}
		seen := map[string]bool{}
		last := ""
		for k, st := range sf.Steps {
			switch {
			case st.Planned:
				plan = append(plan, PlanStep{ID: PlannedID(sf.ID, k), Text: st.Text, Module: st.Module, After: last})
				continue
			case st.Fid != "" && !seen[st.Fid]:
				seen[st.Fid] = true
				specFids = append(specFids, st.Fid)
				last = st.Fid
			default:
				continue // unresolved: lint reports it
			}
			if st.Text != "" {
				labels[st.Fid] = st.Text
			}
			if st.Ext != "" {
				ext[st.Fid] = st.Ext
			}
		}
		idx := -1
		for i := range m.Flows {
			if m.Flows[i].ID == sf.ID {
				idx = i
				break
			}
		}
		marked := map[string]string{}
		for _, s := range specFids {
			marked[s] = ""
		}
		var fl Flow
		entry := ""
		if idx >= 0 {
			fl = m.Flows[idx]
			if fl.Kind != KindDeclared {
				continue // an automatic flow never has a declared id
			}
			if len(fl.Steps) > 0 {
				entry = fl.Steps[0]
			}
			for _, s := range fl.Steps {
				marked[s] = ""
				if !seen[s] {
					if fl.Unspecified == nil {
						fl.Unspecified = map[string]bool{}
					}
					fl.Unspecified[s] = true
				}
			}
			merged := map[string]string{}
			for k, v := range fl.Labels {
				merged[k] = v
			}
			for k, v := range labels {
				merged[k] = v
			}
			labels = merged
			if sf.Name != "" {
				fl.Name = sf.Name
			}
			fl.Variants, fl.VariantsMore = nil, 0
		} else {
			fl = Flow{ID: sf.ID, Name: sf.Name, Kind: KindDeclared, Top: true}
			if fl.Name == "" {
				fl.Name = sf.ID
			}
			entry = g.specRoot(specFids)
		}
		steps := []string{}
		if entry != "" {
			steps = g.declaredPath(entry, marked)
			in := map[string]bool{}
			for _, s := range steps {
				in[s] = true
			}
			for _, s := range specFids {
				if !in[s] {
					steps = append(steps, s) // not reachable: lint reports it
				}
			}
		}
		fl.Spec = true
		if sf.Actor != "" {
			fl.Actor = sf.Actor
		}
		if sf.Exit != "" {
			fl.Exit = sf.Exit
		}
		fl.Steps = steps
		fl.Labels = nilIfEmpty(labels)
		fl.Plan = plan
		fl.Ext = nilIfEmpty(ext)
		end := ""
		if len(steps) > 0 {
			end = steps[len(steps)-1]
		}
		for _, a := range sf.Alts {
			dup := false
			for _, b := range fl.Branches {
				dup = dup || (b.Cond == a.Cond && b.Flow == a.Flow)
			}
			if !dup {
				fl.Branches = append(fl.Branches, Branch{At: end, Cond: a.Cond, Flow: a.Flow, Text: a.Text})
			}
		}
		if idx >= 0 {
			m.Flows[idx] = fl
			continue
		}
		at := 0
		for i, x := range m.Flows {
			if x.Kind == KindDeclared {
				at = i + 1
			}
		}
		m.Flows = append(m.Flows[:at], append([]Flow{fl}, m.Flows[at:]...)...)
	}

	byID := map[string]Flow{}
	for _, fl := range m.Flows {
		byID[fl.ID] = fl
	}
	fg := g.flowGraph(m.Flows)
	for _, sj := range sm.Journeys {
		var ids []string
		have := map[string]bool{}
		for _, id := range sj.Flows {
			if _, ok := byID[id]; ok && !have[id] {
				have[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		idx := -1
		for i := range m.Journeys {
			if m.Journeys[i].ID == sj.ID {
				idx = i
			}
		}
		j := Journey{ID: sj.ID, Name: sj.Name, Declared: true, Top: true, Flows: ids, Links: []Link{}}
		old := map[[2]string]Link{}
		if idx >= 0 {
			j.Actor, j.Exit = m.Journeys[idx].Actor, m.Journeys[idx].Exit
			for _, l := range m.Journeys[idx].Links {
				old[[2]string{l.From, l.To}] = l
			}
			if j.Name == "" {
				j.Name = m.Journeys[idx].Name
			}
		}
		if j.Name == "" {
			j.Name = j.ID
		}
		if first := byID[ids[0]]; j.Actor == "" {
			j.Actor = first.Actor
		}
		if last := byID[ids[len(ids)-1]]; j.Exit == "" {
			j.Exit = last.Exit
		}
		for k := 0; k+1 < len(ids); k++ {
			l, ok := old[[2]string{ids[k], ids[k+1]}]
			if !ok || l.Via == ViaClient {
				// the link the code shows between the two flows, else a client call
				l = Link{From: ids[k], To: ids[k+1], Via: ViaClient}
				for _, e := range fg.out[fg.byID[ids[k]]] {
					if e.to == fg.byID[ids[k+1]] {
						l.Via, l.Cond = e.via, e.cond
						break
					}
				}
			}
			j.Links = append(j.Links, l)
		}
		j.Desc = journeyDesc(j)
		if idx >= 0 {
			m.Journeys[idx] = j
			continue
		}
		at := 0
		for i, x := range m.Journeys {
			if x.Declared {
				at = i + 1
			}
		}
		m.Journeys = append(m.Journeys[:at], append([]Journey{j}, m.Journeys[at:]...)...)
	}
}

// specRoot picks the entry point of a spec-only flow among its resolved steps:
// the first one that no other step reaches by calls ("" without steps).
func (g *graph) specRoot(fids []string) string {
	for _, c := range fids {
		reached := false
		for _, o := range fids {
			if o != c && g.reachable(o)[c] {
				reached = true
				break
			}
		}
		if !reached {
			return c
		}
	}
	if len(fids) > 0 {
		return fids[0]
	}
	return ""
}

func nilIfEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}
