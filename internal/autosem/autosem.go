// Package autosem builds the semantic layer (the arch.model.json format) from
// facts alone, with plain heuristics and no model: module layers and roles from
// paths and contents, scenarios from walks over the call graph.
//
// Flow JSON: {name, desc, kind, top, steps[fid]}. kind is the entry-point kind
// (http | task | main | lifecycle | ui); top marks up to MaxTop of the most
// informative scenarios (deep, spanning several modules and layers, at least
// one or two per kind); top flows come first, then the rest by the same score.
package autosem

import (
	"encoding/json"
	"sort"

	"github.com/aidarbn/archmap/internal/facts"
)

// ModuleInfo describes a module in the semantic layer.
type ModuleInfo struct {
	Role  string `json:"role"`
	Desc  string `json:"desc"`
	Layer string `json:"layer"`
}

// Model is the arch.model.json format.
type Model struct {
	Title   string                `json:"title"`
	Summary string                `json:"summary"`
	Modules map[string]ModuleInfo `json:"modules"`
	Flows   []Flow                `json:"flows"`
}

// Build derives the model from facts.
func Build(f *facts.Facts) Model {
	m := Model{Title: f.Repo, Modules: map[string]ModuleInfo{}, Flows: []Flow{}}
	if f.Project != nil {
		if f.Project.Name != "" {
			m.Title = f.Project.Name
		}
		m.Summary = f.Project.Description
	}
	stats := collectStats(f)
	layer := map[string]string{}
	for path, mod := range f.Modules {
		l := classify(path, stats[path])
		layer[path] = l
		m.Modules[path] = ModuleInfo{Role: roleOf(path, l), Desc: mod.Doc, Layer: l}
	}
	for p := range stats { // modules that only exist through functions
		if _, ok := layer[p]; !ok {
			layer[p] = classify(p, stats[p])
			m.Modules[p] = ModuleInfo{Role: roleOf(p, layer[p]), Layer: layer[p]}
		}
	}
	m.Flows = buildFlows(newGraph(f, layer))
	return m
}

// JSON returns the model for the viewer.
func JSON(f *facts.Facts) ([]byte, error) { return json.Marshal(Build(f)) }

// SortedModules lists module paths of a model (for stable output in tools).
func (m Model) SortedModules() []string {
	ks := make([]string, 0, len(m.Modules))
	for k := range m.Modules {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
