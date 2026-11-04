package autosem

import (
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// KindDeclared marks scenarios declared with `arch:flow` comments.
const KindDeclared = "declared"

// Branch is a fork or outcome at one step of a declared scenario (`arch:on`).
type Branch struct {
	At   string `json:"at"`             // fid of the step
	Cond string `json:"cond"`           // condition, as written
	Flow string `json:"flow,omitempty"` // id of the flow the story continues in
	Text string `json:"text,omitempty"` // outcome text
}

// DeclOrder lists the functions that declare flows in source order (file, line),
// so the first declaration of an id is the one that comes first in the code.
func DeclOrder(f *facts.Facts) []string {
	var out []string
	for fid, fn := range f.Functions {
		if fn.Arch != nil && len(fn.Arch.Flows) > 0 {
			out = append(out, fid)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := f.Functions[out[i]], f.Functions[out[j]]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return out[i] < out[j]
	})
	return out
}

// declaredFlows builds the scenarios announced by `arch:flow` comments, in the
// source order of their entry points (see DeclOrder). The
// first declaration of an id wins; lint reports duplicates.
func declaredFlows(g *graph) []Flow {
	entries := DeclOrder(g.f)
	// members[flow id] = functions marked as its steps
	members := map[string]map[string]string{}
	for fid, fn := range g.f.Functions {
		if fn.Arch == nil {
			continue
		}
		for _, s := range fn.Arch.Steps {
			if members[s.Flow] == nil {
				members[s.Flow] = map[string]string{}
			}
			members[s.Flow][fid] = s.Text
		}
	}
	var out []Flow
	seen := map[string]bool{}
	for _, entry := range entries {
		fn := g.f.Functions[entry]
		for _, d := range fn.Arch.Flows {
			if seen[d.ID] {
				continue
			}
			seen[d.ID] = true
			steps := g.declaredPath(entry, members[d.ID])
			fl := Flow{
				ID: d.ID, Name: d.Name, Kind: KindDeclared, Top: true,
				Actor: d.Actor, Exit: d.Exit, Steps: steps,
				Desc: declaredDesc(fn, d),
			}
			labels := map[string]string{}
			for _, s := range steps {
				if t := members[d.ID][s]; t != "" {
					labels[s] = t
				}
				if a := g.f.Functions[s].Arch; a != nil {
					for _, o := range a.On {
						fl.Branches = append(fl.Branches, Branch{At: s, Cond: o.Cond, Flow: o.Flow, Text: o.Text})
					}
				}
			}
			if len(labels) > 0 {
				fl.Labels = labels
			}
			out = append(out, fl)
		}
	}
	return out
}

func declaredDesc(fn facts.Function, d facts.FlowDecl) string {
	if doc := strings.TrimSpace(fn.Doc); doc != "" && !strings.HasPrefix(doc, "arch:") {
		return doc
	}
	switch {
	case d.Actor != "" && d.Exit != "":
		return d.Actor + ": " + d.Name + " -> " + d.Exit
	case d.Actor != "":
		return d.Actor + ": " + d.Name
	}
	return d.Name
}

// declaredPath lists the entry point and the marked functions reachable from it
// along calls (synchronous first, then queue hand-offs), depth-first in call
// order. Unmarked functions are walked through but not listed.
func (g *graph) declaredPath(entry string, marked map[string]string) []string {
	steps := []string{entry}
	seen := map[string]bool{entry: true}
	var dfs func(f string)
	dfs = func(f string) {
		next := append(append([]string{}, g.callees[f]...), g.async[f]...)
		for _, c := range next {
			if seen[c] {
				continue
			}
			seen[c] = true
			if _, ok := marked[c]; ok {
				steps = append(steps, c)
			}
			dfs(c)
		}
	}
	dfs(entry)
	return steps
}

// Reachable reports the marked functions of flow id that the walk from entry
// reaches (used by lint). The entry point is always included.
func Reachable(f *facts.Facts, entry string) map[string]bool {
	g := newGraph(f, nil)
	seen := map[string]bool{entry: true}
	var dfs func(string)
	dfs = func(x string) {
		for _, c := range append(append([]string{}, g.callees[x]...), g.async[x]...) {
			if !seen[c] {
				seen[c] = true
				dfs(c)
			}
		}
	}
	dfs(entry)
	return seen
}
