// Package autosem builds the semantic layer (module layers and roles,
// scenarios) from facts alone, with plain heuristics and no hand-written model:
// layers and roles from paths and contents, scenarios from walks over the call
// graph, including hand-offs through job queues (async_callees).
//
// Scenarios declared with `arch:` comments (package annot) come first, with
// kind "declared", top=true and the extra fields id, actor, exit, labels (step
// fid -> caption) and branches ({at, cond, flow?, text?}); automatic scenarios
// follow, without duplicates of the declared entry points.
//
// Flow JSON: {name, desc, kind, top, steps[fid]}. kind is the entry-point kind
// (http | task | main | lifecycle | ui); top marks up to MaxTop of the most
// informative scenarios, ranked by the reach of the entry point (see rank.go:
// modules, external services and queues in its call closure; plain CRUD is
// demoted), at least one or two per kind; top flows come first, then the rest
// by the same score.
package autosem

import (
	"encoding/json"
	"sort"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/spec"
)

// ModuleInfo describes a module in the semantic layer.
type ModuleInfo struct {
	Role  string `json:"role"`
	Desc  string `json:"desc"`
	Layer string `json:"layer"`
}

// Model is the semantic layer handed to the viewer.
type Model struct {
	Title   string                `json:"title"`
	Summary string                `json:"summary"`
	Modules map[string]ModuleInfo `json:"modules"`
	Flows   []Flow                `json:"flows"`
	// Journeys are paths over several flows (see Journey): declared first.
	Journeys []Journey `json:"journeys"`
	// Spec is the `.arch` specification checked against the code (nil without
	// spec files): planned steps, transition source and drift. See internal/spec.
	Spec *spec.Model `json:"spec,omitempty"`
}

// Build derives the model from facts.
func Build(f *facts.Facts) Model { return buildModel(f, nil) }

func marshalModel(m Model) ([]byte, error) { return json.Marshal(m) }

func buildModel(f *facts.Facts, set *spec.Set) Model {
	m := Model{Title: f.Repo, Modules: map[string]ModuleInfo{}, Flows: []Flow{}, Journeys: []Journey{}}
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
	g := newGraph(f, layer)
	m.Flows = buildFlows(g)
	buildVariants(m.Flows)
	m.Journeys = g.buildJourneys(m.Flows)
	if !set.Empty() || (set != nil && len(set.Errors) > 0) {
		reach := map[string]map[string]bool{}
		sm, _ := spec.Check(set, f, func(entry string) map[string]bool {
			if reach[entry] == nil {
				reach[entry] = g.reachable(entry)
			}
			return reach[entry]
		})
		g.mergeSpec(&m, sm)
		m.Spec = &sm
	}
	g.trimFlows(m.Flows, m.Journeys)
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
