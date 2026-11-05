package autosem

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Walk tuning. Weights are data so tests and tuning stay in one place.
const (
	MaxSteps = 9 // longest path, entry point included
	// MaxStepsAsync is the longest path of a scenario that must reach a queue
	// hand-off (see asyncPath): the way to the queue takes steps of its own.
	MaxStepsAsync = 12
	// AsyncDepth is how far from the entry point a queue hand-off is looked for.
	AsyncDepth = 6
	// AsyncContinue is how many steps of the handler are always shown.
	AsyncContinue = 2
	MinTop        = 3 // flows shorter than this are never "top"
	MaxTop        = 8
)

// Weights score a candidate callee relative to the current function.
var Weights = struct {
	DeeperLayer   int // callee has calls of its own and sits in a deeper layer (entry -> domain -> adapter)
	DeeperAdapter int // same, into an adapter with calls of its own (a repository read)
	DeeperLeaf    int // same, but the callee is a leaf (an I/O detail)
	StayEntry     int // an entry-layer function calling another entry-layer one (handler glue)
	ToInfra       int // callee is in infra (utility code: barely interesting)
	Shallower     int // callee goes back to a shallower layer
	NonLeaf       int // callee has calls of its own
	Fanout        int // extra for callees with >= 3 calls
	OtherModule   int // callee lives in another module
	Helper        int // callee looks like an error/mapper/log/getter helper
	Trivial       int // callee is a short leaf (a few lines)
	SizePer20     int // per 20 lines of body (max 2)
	Async         int // callee is the handler of a job the current function puts on a queue
}{DeeperLayer: 4, DeeperAdapter: 2, DeeperLeaf: 1, StayEntry: -2, ToInfra: 0, Shallower: -1, NonLeaf: 2, Fanout: 1, OtherModule: 1, Helper: -8, Trivial: -1, SizePer20: 1, Async: 14}

// helperNames are name patterns of functions that rarely carry the story.
var helperNames = []*regexp.Regexp{
	// error constructors
	regexp.MustCompile(`(?i)(error|err)s?$`),
	regexp.MustCompile(`^(New|new|make|Make)\w*(Error|Err)$`),
	regexp.MustCompile(`^(Wrap|Wrapf|Errorf|Is|As|Unwrap)$`),
	// mappers / converters
	regexp.MustCompile(`^(to|from|map|convert|parse|decode|encode|marshal|unmarshal|serialize|deserialize)([A-Z_0-9]|$)`),
	regexp.MustCompile(`(FromSqlc|ToSqlc|FromRow|ToRow|FromDB|ToDB|ToDomain|ToProto|ToPB|ToDTO|ToModel|ToResponse|ToEntity|FromProto|FromPB|FromDomain|FromModel|FromDTO)$`),
	regexp.MustCompile(`^(To|From|Map|Convert)(Domain|Proto|PB|DTO|Model|Response|Entity)`),
	// logging / metrics / tracing
	regexp.MustCompile(`(?i)(logger|metric|telemetry|tracing)`),
	regexp.MustCompile(`^(log|logf|debug|debugf|info|infof|warn|warnf|warning|trace|print|printf|println|observe|record|incr?|inc[A-Z]\w*|metric\w*|span\w*|startSpan|endSpan|track\w*)$`),
	// getters / pointer / conversion helpers
	regexp.MustCompile(`(?i)^(ptr|ptrTo|toPtr|deref|strPtr|stringPtr|boolPtr|int\d*Ptr|atoi|itoa|sprintf|conv\w*|pointer\w*)$`),
	regexp.MustCompile(`Ptr$`),
	regexp.MustCompile(`(FromContext|FromCtx)$`),
	// std-trait style methods and snake_case accessors (new, default, clone, is_dir, as_str...)
	regexp.MustCompile(`^(new|default|clone|fmt|drop|eq|hash|from|into|len|iter|next|deref)$`),
	regexp.MustCompile(`^(is|as|has|to|into|with)_\w+$`),
	// formatting and serialization helpers
	regexp.MustCompile(`^(format|fmt|stringify|normalize|sanitize)[A-Z_0-9]\w*$`),
	regexp.MustCompile(`(ToString|ToQueryParams\w*|QueryString|ToQuery)$`),
	// ubiquitous framework hooks
	regexp.MustCompile(`^use(State|Effect|Memo|Callback|Ref|Context|Reducer|Router|Translation|Theme|Toast|Snackbar|Alert\w*|LocalStorage|SessionStorage|Dispatch|Selector|Navigate|Params|SearchParams|Query\w*)$`),
	// i18n
	regexp.MustCompile(`^(t|tr|_|translate\w*|localize\w*|i18n\w*|gettext)$`),
}

// helperFiles are file names that hold error mapping, conversion, logging.
var helperFiles = regexp.MustCompile(`(?i)^(errors?|mapper\w*|\w+_mapper|\w*converters?|\w*_convert\w*|logger|log|metrics|i18n)(_test)?\.\w+$`)

// helperClasses are receiver types that are loggers, metrics sinks and the like.
var helperClasses = regexp.MustCompile(`(?i)(logger|logging|metrics?|tracer|span|counter|histogram|gauge|translator|i18n|localizer)`)

// helperModules are module segments holding infrastructure helpers.
var helperModules = regexp.MustCompile(`(?i)(^|/)(log|logger|logging|metrics|telemetry|tracing|i18n|strings|strutil|stringutil|conv|convert|ptr|errors?)(/|$)`)

func isHelper(fn facts.Function) bool {
	if helperFiles.MatchString(path.Base(fn.File)) {
		return true
	}
	for _, re := range helperNames {
		if re.MatchString(fn.Name) {
			return true
		}
	}
	if fn.Cls != nil && helperClasses.MatchString(*fn.Cls) {
		return true
	}
	return helperModules.MatchString(fn.Module)
}

// Flow is a scenario: a path through the call graph from an entry point.
type Flow struct {
	ID    string   `json:"id"` // declared: the arch id; automatic: "auto:" + fid of the entry point. Unique, stable
	Name  string   `json:"name"`
	Desc  string   `json:"desc,omitempty"`
	Kind  string   `json:"kind,omitempty"` // entry-point kind: http | task | main | lifecycle | ui; "declared" for arch:flow
	Top   bool     `json:"top"`            // among the most informative scenarios (<= MaxTop)
	Steps []string `json:"steps"`
	// Declared flows (arch: comments) only:
	Actor    string            `json:"actor,omitempty"`
	Exit     string            `json:"exit,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"` // step fid -> caption
	Branches []Branch          `json:"branches,omitempty"`
	// Effects: step fid -> what the function itself changes ("orders.status ←
	// paid", "INSERT kaspi_payments"); declared and automatic flows alike.
	Effects map[string][]string `json:"effects,omitempty"`
	// Variants (declared flows with arch:on only): the main path and one variant
	// per outcome of a fork, at most MaxVariants; VariantsMore counts the cut ones.
	Variants     []Variant `json:"variants,omitempty"`
	VariantsMore int       `json:"variantsMore,omitempty"`
	// From the .arch specification (see mergeSpec):
	// Spec marks a flow the spec declares (alone or together with arch:flow);
	// Plan is the separate "plan" layer: the steps that have no code yet
	// (`step plan`), never part of Steps — the diagram shows only the code;
	// Unspecified lists steps found in the code that the spec flow does not list;
	// Ext names the external system of a step (step id -> system).
	Spec        bool              `json:"spec,omitempty"`
	Plan        []PlanStep        `json:"plan,omitempty"`
	Unspecified map[string]bool   `json:"unspecified,omitempty"`
	Ext         map[string]string `json:"ext,omitempty"`
	// Trimming (see trim.go): Hidden — a technical scenario that lists skip;
	// Minor — steps folded into the step before them (the viewer plays the rest).
	Hidden bool     `json:"hidden,omitempty"`
	Minor  []string `json:"minor,omitempty"`
}

type graph struct {
	f       *facts.Facts
	layer   map[string]string // module -> layer
	callees map[string][]string
	async   map[string][]string // function -> handlers of the jobs it queues
}

func newGraph(f *facts.Facts, layer map[string]string) *graph {
	g := &graph{f: f, layer: layer, callees: make(map[string][]string, len(f.Functions)), async: map[string][]string{}}
	for fid, fn := range f.Functions {
		seen := map[string]bool{fid: true}
		for _, c := range fn.AsyncCallees {
			if _, ok := f.Functions[c]; ok && !seen[c] {
				seen[c] = true
				g.async[fid] = append(g.async[fid], c)
			}
		}
		seen = map[string]bool{fid: true}
		for _, c := range fn.Callees {
			if _, ok := f.Functions[c]; ok && !seen[c] {
				seen[c] = true
				g.callees[fid] = append(g.callees[fid], c)
			}
		}
	}
	return g
}

// score rates callee c when standing at cur.
func (g *graph) score(cur, c string) int {
	cf, ff := g.f.Functions[cur], g.f.Functions[c]
	s := 0
	cl, fl := g.layer[cf.Module], g.layer[ff.Module]
	async := g.isAsync(cur, c)
	switch {
	case fl == Test:
		return -100
	case async:
		s += Weights.Async // the story continues in the worker, whatever its layer
	case fl == Infra && cl != Infra:
		s += Weights.ToInfra
	case rank(fl) >= 0 && rank(cl) >= 0 && rank(fl) > rank(cl):
		if fl == Adapter && len(g.callees[c]) > 0 {
			s += Weights.DeeperAdapter
		} else if len(g.callees[c]) > 0 {
			s += Weights.DeeperLayer
		} else {
			s += Weights.DeeperLeaf
		}
	case fl == cl && rank(fl) == 0:
		s += Weights.StayEntry
	case rank(fl) >= 0 && rank(cl) >= 0 && rank(fl) < rank(cl):
		s += Weights.Shallower
	}
	if n := len(g.callees[c]); n > 0 {
		s += Weights.NonLeaf
		if n >= 3 {
			s += Weights.Fanout
		}
	} else if ff.End-ff.Start <= 3 {
		s += Weights.Trivial
	}
	// big bodies carry logic: +SizePer20 per 20 lines, at most 2 steps worth
	if n := (ff.End - ff.Start) / 20; n > 0 {
		if n > 2 {
			n = 2
		}
		s += n * Weights.SizePer20
	}
	if ff.Module != cf.Module {
		s += Weights.OtherModule
	}
	if isHelper(ff) {
		s += Weights.Helper
	}
	return s
}

func (g *graph) isAsync(cur, c string) bool {
	for _, x := range g.async[cur] {
		if x == c {
			return true
		}
	}
	return false
}

// isExternal reports an adapter that talks to an external service: the walk
// ends there.
func (g *graph) isExternal(fid string) bool {
	m := g.f.Functions[fid].Module
	return g.layer[m] == Adapter && externalService(m) != ""
}

// DepthPenalty is subtracted from a candidate's score per level of distance
// from the entry point: sibling steps of a caller (load, validate, save) are
// preferred to diving ever deeper into one branch.
const DepthPenalty = 1

// LeafRepeat is the score lost for every leaf already chosen from the same module.
const LeafRepeat = 1

// value-free best-first expansion state
type cand struct {
	parent, fid string
	score, prio int
	depth, ord  int
	leaf        bool
	mod         string
}

// repoLeaf reports a function of a storage adapter that calls nothing else: a
// plain read or write, the first to go when the scenario has no room.
func (g *graph) repoLeaf(fid string) bool {
	fn := g.f.Functions[fid]
	return g.layer[fn.Module] == Adapter && externalService(fn.Module) == "" && len(g.callees[fid]) == 0 && len(g.async[fid]) == 0
}

// asyncPath finds the way from an entry point to the nearest queue hand-off in
// its call closure (AsyncDepth levels): the entry, the functions down to the one
// that puts the job on the queue, and the job's handler. nil if there is none.
// Among equally short ways the better-scored steps win; helpers, tests, infra
// code and the insides of external clients are not walked, as in reach.
func (g *graph) asyncPath(entry string) []string {
	dist := map[string]int{entry: 0}
	parent := map[string]string{}
	viaQueue := map[string]bool{} // the step was reached over a queue hand-off
	level := []string{entry}
	for d := 0; d < AsyncDepth && len(level) > 0; d++ {
		var next []string
		for _, cur := range level {
			if g.isExternal(cur) && cur != entry {
				continue
			}
			visit := func(c string, queue bool) {
				fn := g.f.Functions[c]
				if l := g.layer[fn.Module]; l == Test || l == Infra || isHelper(fn) {
					return
				}
				switch dc, seen := dist[c]; {
				case !seen:
					dist[c] = d + 1
					parent[c], viaQueue[c] = cur, queue
					next = append(next, c)
				case dc == d+1 && !viaQueue[c] && g.score(cur, c) > g.score(parent[c], c):
					parent[c], viaQueue[c] = cur, queue
				}
			}
			for _, c := range g.callees[cur] {
				visit(c, false)
			}
			for _, c := range g.async[cur] {
				visit(c, true)
			}
		}
		for _, c := range next { // the shortest way to a hand-off
			if viaQueue[c] {
				return g.chainTo(parent, entry, c)
			}
		}
		level = next
	}
	return nil
}

func (g *graph) chainTo(parent map[string]string, entry, to string) []string {
	var rev []string
	for x := to; ; x = parent[x] {
		rev = append(rev, x)
		if x == entry {
			break
		}
	}
	out := make([]string, len(rev))
	for i, x := range rev {
		out[len(rev)-1-i] = x
	}
	return out
}

// walk builds the scenario of an entry point: best-first over the call graph.
// Every candidate callee of every chosen function competes by score (see
// Weights) minus DepthPenalty per level; the best is taken, its own callees
// become candidates. Branches end at leaves and external-API adapters; ties go
// to the call that comes first in the body. At most MaxSteps functions are
// chosen; they are returned in depth-first order of the chosen call tree
// (callers first, callees in call order), which reads as a path.
func (g *graph) walk(entry string) []string {
	chosen := map[string]bool{entry: true}
	kids := map[string][]string{}
	order := map[string]int{entry: 0}
	var frontier []cand
	ord := 0
	chain := g.asyncPath(entry) // entry ... queuing function, handler; nil without a hand-off
	maxSteps := MaxSteps
	if chain != nil {
		maxSteps = MaxStepsAsync
	}
	leafSeen := map[string]int{} // adapter calls already chosen per module: diminishing returns
	eff := func(c cand) int {
		if c.leaf {
			return c.prio - 10*LeafRepeat*leafSeen[c.mod]
		}
		return c.prio
	}
	expand := func(parent string, depth int) {
		if g.isExternal(parent) {
			return
		}
		next := append(append([]string{}, g.callees[parent]...), g.async[parent]...)
		for _, c := range next {
			if chosen[c] {
				continue
			}
			s := g.score(parent, c)
			if s < 0 {
				continue
			}
			if chain != nil && g.repoLeaf(c) {
				continue // a scenario that must reach a queue has no room for plain reads
			}
			ord++
			frontier = append(frontier, cand{parent: parent, fid: c, score: s, prio: s*10 - depth*DepthPenalty*10, depth: depth, ord: ord, leaf: g.layer[g.f.Functions[c].Module] == Adapter, mod: g.f.Functions[c].Module})
		}
	}
	expand(entry, 1)
	n := 1
	take := func(bi int) {
		c := frontier[bi]
		frontier = append(frontier[:bi], frontier[bi+1:]...)
		chosen[c.fid] = true
		n++
		if c.leaf {
			leafSeen[c.mod]++
		}
		order[c.fid] = n
		kids[c.parent] = append(kids[c.parent], c.fid)
		expand(c.fid, c.depth+1)
	}
	if chain != nil {
		// the way to the queue is part of the scenario whatever it scores ...
		for i := 1; i < len(chain); i++ {
			bi := -1
			for j, c := range frontier {
				if c.fid == chain[i] && c.parent == chain[i-1] {
					bi = j
					break
				}
			}
			if bi < 0 { // below the score threshold: add the step by hand
				frontier = append(frontier, cand{parent: chain[i-1], fid: chain[i], depth: i})
				bi = len(frontier) - 1
			}
			depth := frontier[bi].depth
			if i == len(chain)-1 {
				frontier[bi].depth = depth - 1 // the handler is a new story: its steps compete from near the top
			}
			take(bi)
		}
		// ... and so is the start of the handler's story
		handler := chain[len(chain)-1]
		for k := 0; k < AsyncContinue && n < maxSteps; k++ {
			bi := -1
			for j, c := range frontier {
				if c.parent == handler && !chosen[c.fid] && (bi < 0 || eff(c) > eff(frontier[bi])) {
					bi = j
				}
			}
			if bi < 0 {
				break
			}
			take(bi)
		}
	}
	for n < maxSteps && len(frontier) > 0 {
		bi := -1
		for i, c := range frontier {
			if chosen[c.fid] {
				continue
			}
			if bi < 0 || eff(c) > eff(frontier[bi]) || (eff(c) == eff(frontier[bi]) && c.ord < frontier[bi].ord) {
				bi = i
			}
		}
		if bi < 0 {
			break
		}
		take(bi)
	}
	// depth-first order, callees in call order of the caller's body
	var steps []string
	var dfs func(f string)
	dfs = func(f string) {
		steps = append(steps, f)
		sort.SliceStable(kids[f], func(i, j int) bool { return g.callIndex(f, kids[f][i]) < g.callIndex(f, kids[f][j]) })
		for _, k := range kids[f] {
			dfs(k)
		}
	}
	dfs(entry)
	return steps
}

func (g *graph) callIndex(parent, c string) int {
	for i, x := range g.callees[parent] {
		if x == c {
			return i
		}
	}
	for i, x := range g.async[parent] { // queued jobs run after the synchronous calls
		if x == c {
			return len(g.callees[parent]) + i
		}
	}
	return -1
}

func flowName(fn facts.Function) string {
	label := ""
	if fn.Entry != nil && len(fn.Entry.Labels) > 0 {
		label = fn.Entry.Labels[0]
	}
	if label == "" {
		label = fn.Name
		if fn.Cls != nil {
			label = *fn.Cls + "." + fn.Name
		}
	}
	if fn.Entry != nil && fn.Entry.Kind == "task" {
		return "task " + label
	}
	return label
}

// buildFlows makes one flow per entry point, ordered with the "top" ones first.
func buildFlows(g *graph) []Flow {
	declared := declaredFlows(g)
	declTop := g.markDeclaredTop(declared)
	isDeclared := map[string]bool{}
	for _, fl := range declared {
		isDeclared[fl.Steps[0]] = true
	}
	var fids []string
	for fid, fn := range g.f.Functions {
		if fn.Entry != nil && g.layer[fn.Module] != Test && !isDeclared[fid] {
			fids = append(fids, fid)
		}
	}
	sort.Strings(fids)

	type cand struct {
		flow  Flow
		score int
		mods  int
	}
	cands := make([]*cand, 0, len(fids))
	for _, fid := range fids {
		fn := g.f.Functions[fid]
		steps := g.walk(fid)
		var mods []string
		seen := map[string]bool{}
		for _, s := range steps {
			if m := g.f.Functions[s].Module; !seen[m] {
				seen[m] = true
				mods = append(mods, m)
			}
		}
		desc := strings.TrimSpace(fn.Doc)
		if desc == "" {
			desc = fmt.Sprintf("%s: path through %s", fn.Entry.Kind, strings.Join(mods, ", "))
		}
		layers := map[string]bool{}
		for m := range seen {
			layers[g.layer[m]] = true
		}
		c := &cand{
			flow:  Flow{ID: "auto:" + fid, Name: flowName(fn), Desc: desc, Kind: fn.Entry.Kind, Steps: steps},
			mods:  len(mods),
			score: g.flowScore(fn.Entry.Kind, g.reach(fid), len(steps), len(layers)),
		}
		cands = append(cands, c)
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

	// pick the top: best of every kind, then the second of every kind, then by score;
	// skip scenarios that mostly repeat an already chosen one.
	chosen := map[*cand]bool{}
	var kinds []string
	byKind := map[string][]*cand{}
	for _, c := range cands {
		if len(c.flow.Steps) < MinTop {
			continue
		}
		if _, ok := byKind[c.flow.Kind]; !ok {
			kinds = append(kinds, c.flow.Kind)
		}
		byKind[c.flow.Kind] = append(byKind[c.flow.Kind], c)
	}
	// main() wires everything together and reaches most of the code: it keeps
	// no guaranteed seat when the project has real business entry points.
	if len(byKind["http"])+len(byKind["task"])+len(byKind["ui"]) > 0 {
		for i, k := range kinds {
			if k == "main" {
				kinds = append(kinds[:i], kinds[i+1:]...)
				break
			}
		}
	}
	sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]][0].score > byKind[kinds[j]][0].score })
	var top []*cand
	overlaps := func(c *cand) bool {
		in := map[string]bool{}
		for _, s := range c.flow.Steps {
			in[s] = true
		}
		for _, t := range top {
			same := 0
			for _, s := range t.flow.Steps {
				if in[s] {
					same++
				}
			}
			if same*10 >= len(c.flow.Steps)*7 {
				return true
			}
		}
		return false
	}
	take := func(c *cand) {
		if len(top)+declTop < MaxTop && !chosen[c] && !overlaps(c) {
			chosen[c] = true
			top = append(top, c)
		}
	}
	for round := 0; round < 2; round++ {
		for _, k := range kinds {
			n := 0
			for _, c := range byKind[k] {
				if chosen[c] {
					continue
				}
				before := len(top)
				take(c)
				if len(top) > before {
					n++
				}
				if n == 1 {
					break
				}
			}
		}
	}
	for _, c := range cands {
		if len(c.flow.Steps) >= MinTop {
			take(c)
		}
	}
	sort.SliceStable(top, func(i, j int) bool { return top[i].score > top[j].score })

	flows := append(make([]Flow, 0, len(declared)+len(cands)), declared...)
	for _, c := range top {
		c.flow.Top = true
		flows = append(flows, c.flow)
	}
	for _, c := range cands {
		if !chosen[c] {
			flows = append(flows, c.flow)
		}
	}
	for i := range flows {
		flows[i].Effects = g.effects(flows[i].Steps)
	}
	return flows
}

// effects lists, per step, what the function itself changes: declared state
// transitions and writes to tables (not the closure over its callees).
func (g *graph) effects(steps []string) map[string][]string {
	var out map[string][]string
	for _, fid := range steps {
		fn, ok := g.f.Functions[fid]
		if !ok {
			continue
		}
		var list []string
		if fn.Arch != nil {
			for _, s := range fn.Arch.State {
				list = append(list, s.Entity+" ← "+s.To)
			}
		}
		for _, a := range fn.Data {
			if a.Op == "read" {
				continue
			}
			e := strings.ToUpper(a.Op) + " " + a.Table
			if a.Op == "update" && len(a.Fields) > 0 {
				e += " (" + strings.Join(a.Fields, ", ") + ")"
			}
			list = append(list, e)
		}
		if len(list) > 0 {
			if out == nil {
				out = map[string][]string{}
			}
			out[fid] = list
		}
	}
	return out
}
