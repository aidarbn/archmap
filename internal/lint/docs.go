package lint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Count is "n of total have a description".
type Count struct {
	Have  int `json:"have"`
	Total int `json:"total"`
}

func (c Count) String() string { return fmt.Sprintf("%d/%d", c.Have, c.Total) }

// Missing is one place without a description.
type Missing struct {
	Kind string `json:"kind"` // module | function | endpoint | actor | external
	Name string `json:"name"`
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

func (m Missing) String() string {
	loc := m.File
	if m.Line > 0 {
		loc = fmt.Sprintf("%s:%d", m.File, m.Line)
	}
	if loc == "" {
		loc = "-"
	}
	return fmt.Sprintf("%s: no description (%s %s)", loc, m.Kind, m.Name)
}

// DocCoverage tells how much of the system carries a description written in
// the code. It is information, never a problem.
type DocCoverage struct {
	Modules   Count     `json:"modules"`   // modules with functions
	Entries   Count     `json:"entries"`   // functions that are entry points
	Endpoints Count     `json:"endpoints"` // API endpoints and tasks
	Actors    Count     `json:"actors"`    // who uses the system (as on the map)
	Externals Count     `json:"externals"` // outside systems
	Missing   []Missing `json:"missing,omitempty"`
}

// Line is the one-line summary.
func (d DocCoverage) Line() string {
	return fmt.Sprintf("descriptions: modules %s, entry points %s, endpoints %s, actors %s, externals %s",
		d.Modules, d.Entries, d.Endpoints, d.Actors, d.Externals)
}

// Docs counts the descriptions of f; Missing lists what has none, in the
// order modules, actors, endpoints, entry points, externals.
func Docs(f *facts.Facts) DocCoverage {
	var d DocCoverage
	var miss []Missing
	withFns := map[string]bool{}
	for _, fn := range f.Functions {
		withFns[fn.Module] = true
	}
	mods := make([]string, 0, len(withFns))
	for m := range withFns {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		d.Modules.Total++
		if f.Modules[m].Doc != "" {
			d.Modules.Have++
			continue
		}
		file := m
		if fs := f.Modules[m].Files; len(fs) > 0 {
			file = fs[0]
		}
		miss = append(miss, Missing{Kind: "module", Name: m, File: file})
	}

	for _, a := range Actors(f) {
		d.Actors.Total++
		if a.Desc != "" {
			d.Actors.Have++
			continue
		}
		miss = append(miss, Missing{Kind: "actor", Name: a.Name, File: a.File, Line: a.Line})
	}

	for _, e := range f.Endpoints {
		d.Endpoints.Total++
		if e.Desc != "" {
			d.Endpoints.Have++
			continue
		}
		m := Missing{Kind: "endpoint", Name: epName(e)}
		if e.Source == "proto" && f.Services[e.Service].File != "" {
			m.File, m.Line = f.Services[e.Service].File, e.Line
		} else if fn, ok := f.Functions[e.Fid]; ok {
			m.File, m.Line = fn.File, fn.Start
		}
		miss = append(miss, m)
	}

	fids := make([]string, 0, len(f.Functions))
	for fid, fn := range f.Functions {
		if fn.Entry != nil {
			fids = append(fids, fid)
		}
	}
	sort.Strings(fids)
	for _, fid := range fids {
		fn := f.Functions[fid]
		d.Entries.Total++
		if strings.TrimSpace(fn.Doc) != "" {
			d.Entries.Have++
			continue
		}
		name := fn.Name
		if fn.Cls != nil {
			name = *fn.Cls + "." + name
		}
		miss = append(miss, Missing{Kind: "function", Name: name, File: fn.File, Line: fn.Start})
	}

	for _, x := range f.Externals {
		d.Externals.Total++
		if x.Desc != "" {
			d.Externals.Have++
			continue
		}
		m := Missing{Kind: "external", Name: x.Name}
		if len(x.Modules) > 0 {
			m.File = x.Modules[0]
		}
		miss = append(miss, m)
	}
	d.Missing = miss
	return d
}

func epName(e facts.Endpoint) string {
	switch {
	case e.Method != "" && e.Path != "":
		return e.Method + " " + e.Path
	case e.Service != "" && e.Name != "":
		return e.Service + "." + e.Name
	case e.Path != "":
		return e.Path
	}
	return e.Name
}

// Actor is who uses the system, grouped the way the map groups them.
type Actor struct {
	Name string
	Desc string
	File string // where to describe it: the arch:actor markup or the service
	Line int
}

// Actors groups the endpoints into actors like the viewer does: by the actor
// declared on the flow of the handler (`arch:actor`), else by the role of the
// endpoint; a declared actor of a flow that does not start at an endpoint
// (cron, queue) is an actor too. The description is the `arch:actor "…"` text,
// else the description of the service most of its endpoints belong to.
func Actors(f *facts.Facts) []Actor {
	declared := map[string]string{} // fid -> actor name
	fids := make([]string, 0, len(f.Functions))
	for fid := range f.Functions {
		fids = append(fids, fid)
	}
	sort.Strings(fids)
	for _, fid := range fids {
		if a := f.Functions[fid].Arch; a != nil {
			for _, fl := range a.Flows {
				if fl.Actor != "" {
					declared[fid] = strings.TrimSpace(fl.Actor)
					break
				}
			}
		}
	}
	docs := map[string]facts.ActorDoc{}
	for _, a := range f.Actors {
		docs[strings.ToLower(a.Name)] = a
	}
	type group struct {
		name, fid string
		svcs      map[string]int
	}
	groups := map[string]*group{}
	var order []string
	get := func(key, name string) *group {
		g := groups[key]
		if g == nil {
			g = &group{name: name, svcs: map[string]int{}}
			groups[key] = g
			order = append(order, key)
		}
		return g
	}
	inGroup := map[string]bool{}
	for _, e := range f.Endpoints {
		if e.Kind == "task" || e.Kind == "cli" {
			continue
		}
		var g *group
		if n, ok := declared[e.Fid]; ok && e.Fid != "" {
			g = get("n:"+strings.ToLower(n), n)
		} else {
			r := role(e.Auth)
			g = get("r:"+r, r)
		}
		if g.fid == "" {
			g.fid = e.Fid
		}
		if e.Service != "" {
			g.svcs[e.Service]++
		}
		inGroup[e.Fid] = true
	}
	for _, fid := range fids {
		if n, ok := declared[fid]; ok && !inGroup[fid] {
			g := get("n:"+strings.ToLower(n), n)
			if g.fid == "" {
				g.fid = fid
			}
		}
	}
	var out []Actor
	for _, k := range order {
		g := groups[k]
		a := Actor{Name: g.name}
		if doc, ok := docs[strings.ToLower(g.name)]; ok {
			a.Desc, a.File, a.Line = doc.Desc, doc.File, doc.Line
		} else {
			top, n := "", 0
			for s, c := range g.svcs {
				if c > n || c == n && s < top {
					top, n = s, c
				}
			}
			if s, ok := f.Services[top]; ok {
				a.Desc, a.File, a.Line = s.Desc, s.File, s.Line
			}
			if a.File == "" {
				if fn, ok := f.Functions[g.fid]; ok {
					a.File, a.Line = fn.File, fn.Start
				}
			}
		}
		out = append(out, a)
	}
	return out
}

func role(auth string) string {
	a := strings.TrimSpace(auth)
	if a == "" || a == "public" || a == "none" {
		return "public"
	}
	a = strings.TrimPrefix(a, "role:")
	if i := strings.IndexAny(a, ", |\t"); i >= 0 {
		a = a[:i]
	}
	if a == "" {
		return "public"
	}
	return a
}
