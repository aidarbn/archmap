package autosem

import (
	"fmt"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/spec"
)

// PlannedID is the id a planned spec step gets in Flow.Steps (there is no
// function to put there): "plan:<flow id>:<position in the spec flow>".
func PlannedID(flow string, pos int) string { return fmt.Sprintf("plan:%s:%d", flow, pos) }

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

// mergeSpec puts the spec into the code-derived model:
//   - a spec flow with the id of a code flow (arch:flow) takes over the name,
//     actor, exit, captions and the order of steps; code steps the spec does not
//     list follow, marked in Unspecified;
//   - a spec-only flow is a declared flow whose planned steps (no code yet) have
//     ids from PlannedID and are marked in Planned;
//   - spec alts become Branches; spec journeys replace or extend the code ones.
func mergeSpec(m *Model, sm spec.Model) {
	for _, sf := range sm.Flows {
		var steps []string
		labels := map[string]string{}
		planned := map[string]bool{}
		planMod := map[string]string{}
		ext := map[string]string{}
		seen := map[string]bool{}
		last := ""
		for k, st := range sf.Steps {
			id := ""
			switch {
			case st.Planned:
				id = PlannedID(sf.ID, k)
				planned[id] = true
				if st.Module != "" {
					planMod[id] = st.Module
				}
			case st.Fid != "" && !seen[st.Fid]:
				id = st.Fid
				last = id
			default:
				continue // unresolved: lint reports it
			}
			seen[id] = true
			steps = append(steps, id)
			if st.Text != "" {
				labels[id] = st.Text
			}
			if st.Ext != "" {
				ext[id] = st.Ext
			}
		}
		idx := -1
		for i := range m.Flows {
			if m.Flows[i].ID == sf.ID {
				idx = i
				break
			}
		}
		var fl Flow
		if idx >= 0 {
			fl = m.Flows[idx]
			if fl.Kind != KindDeclared {
				continue // an automatic flow never has a declared id
			}
			var extra []string
			for _, s := range fl.Steps {
				if !seen[s] {
					extra = append(extra, s)
				}
			}
			if len(extra) > 0 {
				fl.Unspecified = map[string]bool{}
				for _, s := range extra {
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
			steps = append(steps, extra...)
			if sf.Name != "" {
				fl.Name = sf.Name
			}
			fl.Variants, fl.VariantsMore = nil, 0
		} else {
			fl = Flow{ID: sf.ID, Name: sf.Name, Kind: KindDeclared, Top: true}
			if fl.Name == "" {
				fl.Name = sf.ID
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
		if fl.Steps == nil {
			fl.Steps = []string{}
		}
		fl.Labels = nilIfEmpty(labels)
		if len(planned) > 0 {
			fl.Planned = planned
		}
		fl.PlanMod = nilIfEmpty(planMod)
		fl.Ext = nilIfEmpty(ext)
		for _, a := range sf.Alts {
			dup := false
			for _, b := range fl.Branches {
				dup = dup || (b.Cond == a.Cond && b.Flow == a.Flow)
			}
			if !dup {
				fl.Branches = append(fl.Branches, Branch{At: last, Cond: a.Cond, Flow: a.Flow, Text: a.Text})
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
			if !ok {
				l = Link{From: ids[k], To: ids[k+1], Via: ViaDeclared}
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

func nilIfEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}
