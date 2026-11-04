package autosem

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Journey tuning.
const (
	// MaxJourneyFlows caps the flows of one automatic journey.
	MaxJourneyFlows = 16
)

// Link kinds of a journey.
const (
	ViaOn       = "on"       // arch:on <cond> -> flow
	ViaQueue    = "queue"    // a step puts a job on a queue whose handler is the entry point of the next flow
	ViaDeclared = "declared" // the order was written by hand (arch:then / list form) and the code shows no link
)

// Link is an edge of a journey between two of its flows (flow ids).
type Link struct {
	From string `json:"from"`
	To   string `json:"to"`
	Via  string `json:"via"`
	Cond string `json:"cond,omitempty"`
}

// Journey is a path through several flows: what a client does from the first
// call to the last, including what the backend starts on its own. Declared
// journeys (`arch:journey`) keep the order written by hand; automatic ones are
// walks over the links between flows, with branching where paths share a prefix.
type Journey struct {
	ID       string   `json:"id"` // declared: the arch id; automatic: "j:" + id of the first flow
	Name     string   `json:"name"`
	Desc     string   `json:"desc,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Exit     string   `json:"exit,omitempty"`
	Declared bool     `json:"declared"`
	Top      bool     `json:"top"`
	Flows    []string `json:"flows"` // flow ids, first = where it starts; unique
	Links    []Link   `json:"links"`
}

// machineActor matches actors that are not a person: the journey of such a
// flow is not a client journey.
var machineActor = regexp.MustCompile(`(?i)cron|schedul|queue|worker|timer|system|очеред|воркер|планировщик|таймер|систем|расписан`)

type flowEdge struct {
	to        int
	via, cond string
}

// flowGraph is the graph of flows: nodes are model flows, edges are
// `arch:on -> flow` and queue hand-offs to another flow's entry point.
type flowGraph struct {
	flows []Flow
	byID  map[string]int
	out   [][]flowEdge
}

func (g *graph) flowGraph(flows []Flow) *flowGraph {
	fg := &flowGraph{flows: flows, byID: map[string]int{}, out: make([][]flowEdge, len(flows))}
	byEntry := map[string]int{}
	for i, fl := range flows {
		if _, ok := fg.byID[fl.ID]; !ok && fl.ID != "" {
			fg.byID[fl.ID] = i
		}
		if len(fl.Steps) > 0 {
			if _, ok := byEntry[fl.Steps[0]]; !ok {
				byEntry[fl.Steps[0]] = i
			}
		}
	}
	for i, fl := range flows {
		seen := map[flowEdge]bool{}
		add := func(e flowEdge) {
			if e.to != i && !seen[e] {
				seen[e] = true
				fg.out[i] = append(fg.out[i], e)
			}
		}
		for _, s := range fl.Steps {
			for _, b := range fl.Branches {
				if b.At == s && b.Flow != "" {
					if j, ok := fg.byID[b.Flow]; ok {
						add(flowEdge{j, ViaOn, b.Cond})
					}
				}
			}
		}
		if len(fl.Steps) > 0 {
			for _, c := range g.queued(fl.Steps[0]) {
				if j, ok := byEntry[c]; ok {
					add(flowEdge{j, ViaQueue, ""})
				}
			}
		}
	}
	return fg
}

// queued lists the jobs' handlers put on a queue by the functions the entry
// point calls synchronously (AsyncDepth levels; helpers, tests and infra code
// are not walked), in breadth-first order. The queuing function is usually a
// plain scheduler adapter that the scenario does not list as a step.
func (g *graph) queued(entry string) []string {
	var out []string
	seen := map[string]bool{entry: true}
	level := []string{entry}
	for d := 0; d <= AsyncDepth && len(level) > 0; d++ {
		var next []string
		for _, cur := range level {
			out = append(out, g.async[cur]...)
			for _, c := range g.callees[cur] {
				fn := g.f.Functions[c]
				if seen[c] || g.layer[fn.Module] == Test || g.layer[fn.Module] == Infra || isHelper(fn) {
					continue
				}
				seen[c] = true
				next = append(next, c)
			}
		}
		level = next
	}
	return out
}

// human reports whether a person (not a timer or the backend itself) starts the flow.
func human(fl Flow, f *facts.Facts) bool {
	if fl.Actor != "" {
		return !machineActor.MatchString(fl.Actor)
	}
	if len(fl.Steps) > 0 {
		if e := f.Functions[fl.Steps[0]].Entry; e != nil {
			return e.Kind == "http" || e.Kind == "ui"
		}
	}
	return false
}

// NoteOrder lists the functions that carry any `arch:` markup in source order (file, line).
func NoteOrder(f *facts.Facts) []string {
	var out []string
	for fid, fn := range f.Functions {
		if !fn.Arch.Empty() {
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

// declaredSpecs returns the journeys declared with `arch:journey`/`arch:then`
// as written: id -> flow ids in order (unresolved), first declaration wins.
func declaredSpecs(f *facts.Facts) ([]facts.JourneyDecl, map[string][]string) {
	var decls []facts.JourneyDecl
	idx := map[string]int{}
	for _, fid := range NoteOrder(f) {
		for _, j := range f.Functions[fid].Arch.Journeys {
			if _, dup := idx[j.ID]; dup {
				continue
			}
			idx[j.ID] = len(decls)
			decls = append(decls, j)
		}
	}
	then := map[string][]string{}
	for _, fid := range NoteOrder(f) {
		for _, t := range f.Functions[fid].Arch.Then {
			if _, ok := idx[t.Journey]; ok {
				then[t.Journey] = append(then[t.Journey], t.Flows...)
			}
		}
	}
	return decls, then
}

// buildJourneys assembles the journeys: declared ones first, then automatic ones.
func (g *graph) buildJourneys(flows []Flow) []Journey {
	fg := g.flowGraph(flows)
	out := []Journey{}
	decls, then := declaredSpecs(g.f)
	covered := []map[string]bool{}
	for _, d := range decls {
		ids := append(append([]string{}, d.Flows...), then[d.ID]...)
		j := Journey{ID: d.ID, Name: d.Name, Actor: d.Actor, Exit: d.Exit, Declared: true, Flows: []string{}, Links: []Link{}}
		have := map[string]bool{}
		var seq []int
		for _, id := range ids {
			i, ok := fg.byID[id]
			if !ok {
				continue // lint reports it
			}
			seq = append(seq, i)
			if !have[id] {
				have[id] = true
				j.Flows = append(j.Flows, id)
			}
		}
		if len(seq) == 0 {
			continue
		}
		first, last := flows[seq[0]], flows[seq[len(seq)-1]]
		if j.Actor == "" {
			j.Actor = first.Actor
		}
		if j.Exit == "" {
			j.Exit = last.Exit
		}
		seenLink := map[Link]bool{}
		for k := 0; k+1 < len(seq); k++ {
			a, b := seq[k], seq[k+1]
			n := 0
			for _, e := range fg.out[a] {
				if e.to == b {
					l := Link{flows[a].ID, flows[b].ID, e.via, e.cond}
					if !seenLink[l] {
						seenLink[l] = true
						j.Links = append(j.Links, l)
					}
					n++
				}
			}
			if n == 0 {
				l := Link{From: flows[a].ID, To: flows[b].ID, Via: ViaDeclared}
				if !seenLink[l] {
					seenLink[l] = true
					j.Links = append(j.Links, l)
				}
			}
		}
		j.Desc = journeyDesc(j)
		out = append(out, j)
		covered = append(covered, have)
	}
	nDecl := len(out)

	// automatic: from every flow a person starts, the walk over the links
	type auto struct {
		j   Journey
		set map[string]bool
	}
	var autos []auto
	for s, fl := range flows {
		if !human(fl, g.f) || len(fg.out[s]) == 0 {
			continue
		}
		j := Journey{ID: "j:" + fl.ID, Declared: false, Actor: fl.Actor, Flows: []string{}, Links: []Link{}}
		inJ := map[int]bool{}
		onPath := map[int]bool{}
		seenLink := map[Link]bool{}
		var dfs func(i int)
		dfs = func(i int) {
			inJ[i] = true
			j.Flows = append(j.Flows, flows[i].ID)
			onPath[i] = true
			for _, e := range fg.out[i] {
				if onPath[e.to] {
					continue // no cycles
				}
				if !inJ[e.to] && len(j.Flows) >= MaxJourneyFlows {
					continue
				}
				l := Link{flows[i].ID, flows[e.to].ID, e.via, e.cond}
				if !seenLink[l] {
					seenLink[l] = true
					j.Links = append(j.Links, l)
				}
				if !inJ[e.to] {
					dfs(e.to)
				}
			}
			onPath[i] = false
		}
		dfs(s)
		if len(j.Flows) < 2 {
			continue
		}
		set := map[string]bool{}
		for _, id := range j.Flows {
			set[id] = true
		}
		lastFl := flows[fg.byID[j.Flows[len(j.Flows)-1]]]
		j.Exit = lastFl.Exit
		if isChain(j) {
			j.Name = fl.Name + " → " + lastFl.Name
		} else {
			j.Name = fmt.Sprintf("%s → … (%d flows)", fl.Name, len(j.Flows))
		}
		j.Desc = journeyDesc(j)
		autos = append(autos, auto{j, set})
	}
	sort.SliceStable(autos, func(a, b int) bool { return len(autos[a].j.Flows) > len(autos[b].j.Flows) })
	kept := append([]map[string]bool{}, covered...)
	var keep []Journey
	for _, a := range autos {
		sub := false
		for _, k := range kept {
			sub = sub || subset(a.set, k)
		}
		if sub {
			continue // a part of a longer journey
		}
		kept = append(kept, a.set)
		keep = append(keep, a.j)
	}
	// declared are all "top" up to MaxTop, automatic ones fill the rest
	for i := 0; i < nDecl; i++ {
		out[i].Top = i < MaxTop
	}
	for i := range keep {
		keep[i].Top = nDecl+i < MaxTop
	}
	return append(out, keep...)
}

func subset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// isChain reports a journey that is a single line of flows.
func isChain(j Journey) bool {
	out := map[string]int{}
	for _, l := range j.Links {
		out[l.From]++
		if out[l.From] > 1 {
			return false
		}
	}
	return len(j.Links) == len(j.Flows)-1
}

func journeyDesc(j Journey) string {
	switch {
	case j.Actor != "" && j.Exit != "":
		return j.Actor + ": " + j.Name + " -> " + j.Exit
	case j.Actor != "":
		return j.Actor + ": " + j.Name
	}
	return strings.TrimSpace(j.Name)
}
