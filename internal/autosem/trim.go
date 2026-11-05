package autosem

import (
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Trimming: what the viewer leaves out so that a scenario reads as a story.
//
//   - Flow.Hidden — a technical scenario (health, version, docs, metrics, test /
//     cleanup / bootstrap workers, a one-step scenario without a description).
//     It stays in the data, but lists and counters skip it (see Technical).
//   - Flow.Minor — steps folded into the step before them: error constructors,
//     loggers, metrics, converters, getters and plain calls inside one module
//     (see minorSteps). The viewer plays only the other steps.
//   - Branch.Fold — an outcome that ends the story (no flow to go on in): the
//     viewer folds them into one "+N" mark on the node.
//
// The rules live here and only here; the viewer reads the flags.

// techTokens name technical entry points wherever they appear (function,
// receiver, flow id, route).
var techTokens = map[string]bool{
	"test": true, "tests": true, "cleanup": true, "bootstrap": true, "health": true, "healthz": true,
	"healthcheck": true, "metrics": true, "swagger": true, "openapi": true, "liveness": true,
	"readiness": true, "readyz": true, "livez": true, "pprof": true,
}

// techAlone are technical only as the whole name (Version, Ping, Doc): a
// GetStateVersion is business, a Version handler is not.
var techAlone = map[string]bool{"version": true, "ping": true, "doc": true, "docs": true, "ready": true, "live": true}

// fillerTokens carry no meaning of their own in an entry point name.
var fillerTokens = map[string]bool{
	"get": true, "handle": true, "handler": true, "serve": true, "http": true, "grpc": true, "api": true,
	"server": true, "service": true, "check": true, "work": true, "worker": true, "run": true, "do": true,
	"v1": true, "v2": true, "endpoint": true, "route": true, "task": true, "job": true,
}

var camelSplit = regexp.MustCompile(`[A-Z]+[a-z0-9]*|[a-z0-9]+`)

// tokens splits a name into lower-case words: camelCase, snake_case, paths.
func tokens(s string) []string {
	var out []string
	for _, w := range camelSplit.FindAllString(s, -1) {
		out = append(out, strings.ToLower(w))
	}
	return out
}

// rootRoute matches a catch-all handler of the root path: "/{$}", "/".
var rootRoute = regexp.MustCompile(`(^|\s)/(\{\$\})?$`)

// Technical reports a scenario the reader does not need: health / version /
// docs routes, metrics, test and service workers (cleanup, bootstrap), root
// catch-all handlers, and one-step scenarios without a description. inJourney
// keeps a flow that is part of a through path visible whatever its name.
func Technical(fl Flow, f *facts.Facts, inJourney bool) bool {
	if inJourney || len(fl.Steps) == 0 {
		return false
	}
	fn, ok := f.Functions[fl.Steps[0]]
	if !ok {
		return false
	}
	names := []string{fn.Name, strings.TrimPrefix(fl.ID, "auto:")}
	if fn.Cls != nil {
		names = append(names, *fn.Cls)
	}
	var routes []string
	if fn.Entry != nil {
		routes = fn.Entry.Labels
	}
	for _, n := range append(names, routes...) {
		for _, w := range tokens(n) {
			if techTokens[w] {
				return true
			}
		}
	}
	for _, r := range routes {
		if rootRoute.MatchString(strings.TrimSpace(r)) {
			return true
		}
	}
	// the name is only a technical word and fillers: Version, GetHealth, Ping, HandleDoc
	var core []string
	for _, w := range tokens(fn.Name) {
		if !fillerTokens[w] {
			core = append(core, w)
		}
	}
	if len(core) > 0 {
		alone := true
		for _, w := range core {
			if !techAlone[w] {
				alone = false
				break
			}
		}
		if alone {
			return true
		}
	}
	for _, r := range routes {
		ws := tokens(r)
		var rc []string
		for _, w := range ws {
			if w != "get" && w != "post" && w != "api" && !strings.HasPrefix(w, "v") {
				rc = append(rc, w)
			}
		}
		if len(rc) == 1 && (techAlone[rc[0]] || techTokens[rc[0]]) {
			return true
		}
	}
	// one step and nothing said about it: not a story
	doc := strings.TrimSpace(fn.Doc) != ""
	if len(fl.Steps) == 1 && !doc && len(fl.Labels) == 0 && fl.Exit == "" {
		return true
	}
	// an automatic scenario without a description is shown only among the top ones
	return fl.Kind != KindDeclared && !doc && !fl.Top
}

// minorSteps lists the steps of a flow that fold into the step before them.
// Kept: the entry point, steps marked arch:step (or with arch:state / ext / on),
// a move to another module, external systems, table reads and writes, queue
// hand-offs, state transitions, planned steps. Folded: helpers (error
// constructors, loggers, metrics, converters, mappers, getters — see isHelper)
// and plain calls inside the same module.
func (g *graph) minorSteps(fl Flow) []string {
	marked := map[string]bool{}
	if fl.Kind == KindDeclared {
		for _, s := range fl.Steps {
			marked[s] = true // every step of a declared flow was marked by hand
		}
	}
	for s := range fl.Labels {
		marked[s] = true
	}
	trans := map[string]bool{}
	for _, sm := range g.f.States {
		for _, tr := range sm.Transitions {
			trans[tr.Fid] = true
		}
	}
	extFn := map[string]bool{}
	for _, x := range g.f.Externals {
		for _, fid := range x.Functions {
			extFn[fid] = true
		}
	}
	var out []string
	keptMod := ""
	prev := ""
	for i, s := range fl.Steps {
		fn, ok := g.f.Functions[s]
		keep := i == 0 || !ok || marked[s] || fl.Planned[s]
		if !keep && ok {
			keep = g.meaningful(s, fn, prev, keptMod, trans, extFn)
		}
		if keep {
			if ok {
				keptMod = fn.Module
			}
		} else {
			out = append(out, s)
		}
		prev = s
	}
	return out
}

// meaningful: a step that is not marked still tells something.
func (g *graph) meaningful(s string, fn facts.Function, prev, keptMod string, trans, extFn map[string]bool) bool {
	if fn.Arch != nil && (len(fn.Arch.State) > 0 || len(fn.Arch.Ext) > 0 || len(fn.Arch.On) > 0) {
		return true
	}
	if isHelper(fn) || isGetter(fn) {
		return false
	}
	switch {
	case len(fn.Data) > 0, trans[s], extFn[s], g.isExternal(s), len(fn.AsyncCallees) > 0:
		return true
	case prev != "" && g.isAsync(prev, s):
		return true
	case fn.Module != keptMod:
		return true
	}
	return false
}

var getterName = regexp.MustCompile(`^(Get|get|Is|is|Has|has)[A-Z_]\w*$`)

// isGetter: a short leaf named like an accessor.
func isGetter(fn facts.Function) bool {
	return getterName.MatchString(fn.Name) && len(fn.Callees) == 0 && fn.End-fn.Start <= 4
}

// trimFlows sets Hidden, Minor and Branch.Fold on every flow (see the top of
// this file). Flows of a through path are never hidden.
func (g *graph) trimFlows(flows []Flow, journeys []Journey) {
	inJ := map[string]bool{}
	for _, j := range journeys {
		for _, id := range j.Flows {
			inJ[id] = true
		}
	}
	for i := range flows {
		fl := &flows[i]
		fl.Hidden = Technical(*fl, g.f, inJ[fl.ID])
		fl.Minor = g.minorSteps(*fl)
		for k := range fl.Branches {
			fl.Branches[k].Fold = fl.Branches[k].Flow == ""
		}
	}
}
