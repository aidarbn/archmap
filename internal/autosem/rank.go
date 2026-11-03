package autosem

// Ranking of scenarios ("top" ones). A scenario is as informative as the part
// of the system its entry point reaches: how many different domain and adapter
// modules and functions lie in its call closure, whether external services
// (clients that are not a repository) and job queues are touched. A plain CRUD
// path (entry -> service -> repository) is demoted. Nothing here depends on the
// names of a particular project.

// Rank tuning; data, so that tests and tuning live in one place.
var Rank = struct {
	Depth      int // call-closure depth from the entry point
	PerModule  int // per distinct domain/external-adapter module in the closure
	PerStore   int // per distinct storage adapter (repository, queue)
	FuncsDiv   int // closure functions / FuncsDiv ...
	FuncsMax   int // ... at most this much
	PerExt     int // per distinct external-service adapter (max ExtMax)
	ExtMax     int
	PerAsync   int // per queue hand-off in the closure (max AsyncMax)
	AsyncMax   int
	PerLayer   int // per distinct layer on the walked path
	PerStep    int // per step of the walked path
	CRUD       int // penalty of a bare entry -> service -> repository path
	MainFactor int // percent of the score kept by "main" entry points (wiring code)
}{Depth: 6, PerModule: 4, PerStore: 1, FuncsDiv: 2, FuncsMax: 20, PerExt: 10, ExtMax: 3, PerAsync: 6, AsyncMax: 3, PerLayer: 2, PerStep: 1, CRUD: 25, MainFactor: 60}

// reach describes the call closure of one entry point.
type reach struct {
	mods  map[string]bool // domain/adapter/other modules other than the entry's own
	ext   map[string]bool // external-service adapters
	funcs int
	async int
}

// reach walks callees and queue hand-offs breadth-first to Rank.Depth.
// Helpers (error mappers, loggers...), tests and infra code are neither counted
// nor entered: they are everywhere and say nothing about the scenario.
func (g *graph) reach(entry string) reach {
	r := reach{mods: map[string]bool{}, ext: map[string]bool{}}
	own := g.f.Functions[entry].Module
	dist := map[string]int{entry: 0}
	queue := []string{entry}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if dist[cur] >= Rank.Depth {
			continue
		}
		visit := func(c string, async bool) {
			if _, ok := dist[c]; ok {
				return
			}
			fn := g.f.Functions[c]
			if l := g.layer[fn.Module]; l == Test || l == Infra || isHelper(fn) {
				return
			}
			dist[c] = dist[cur] + 1
			if async {
				r.async++
			}
			r.funcs++
			if m := fn.Module; m != own {
				switch g.layer[m] {
				case Domain, Adapter, Other:
					r.mods[m] = true
				}
				if g.layer[m] == Adapter && externalService(m) != "" {
					r.ext[m] = true
				}
			}
			queue = append(queue, c)
		}
		if g.isExternal(cur) && cur != entry {
			continue // an external client is the end of the story
		}
		for _, c := range g.callees[cur] {
			visit(c, false)
		}
		for _, c := range g.async[cur] {
			visit(c, true)
		}
	}
	return r
}

// crud reports a bare entry -> service -> repository path: at most one domain
// module and plain storage adapters, no external services, no queues.
func (g *graph) crud(r reach) bool {
	if len(r.ext) > 0 || r.async > 0 {
		return false
	}
	domain := 0
	for m := range r.mods {
		if g.layer[m] != Adapter {
			domain++
		}
	}
	return domain <= 1 && len(r.mods) <= 2
}

// flowScore rates a scenario: reach of its entry point plus a little for the walked path.
func (g *graph) flowScore(kind string, r reach, steps, layers int) int {
	stores := 0 // storage adapters count for less: every handler reads something
	for m := range r.mods {
		if g.layer[m] == Adapter && !r.ext[m] {
			stores++
		}
	}
	s := Rank.PerModule*(len(r.mods)-stores) + Rank.PerStore*stores +
		min(r.funcs/Rank.FuncsDiv, Rank.FuncsMax) +
		Rank.PerExt*min(len(r.ext), Rank.ExtMax) +
		Rank.PerAsync*min(r.async, Rank.AsyncMax) +
		Rank.PerLayer*layers + Rank.PerStep*steps
	if g.crud(r) {
		s -= Rank.CRUD
	}
	if kind == "main" {
		s = s * Rank.MainFactor / 100
	}
	return s
}
