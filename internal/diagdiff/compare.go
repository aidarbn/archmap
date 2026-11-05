package diagdiff

import (
	"sort"
)

// Diagram statuses.
const (
	Added   = "added"
	Removed = "removed"
	Changed = "changed"
)

// Reasons a diagram disappeared or a step left a flow.
const (
	ReasonEntryDeleted  = "entry-deleted"  // the entry-point function is gone
	ReasonEntryUnmarked = "entry-unmarked" // arch:flow taken off the entry point
	ReasonNotEntry      = "not-entry"      // the function is no longer an entry point
	ReasonSpecRemoved   = "spec-removed"   // the flow was declared in arch/*.arch only, and is not now
	ReasonUnmarked      = "unmarked"       // the declaration is gone (journey) or nothing is left of it
	ReasonFlowsGone     = "flows-gone"     // every flow of a journey is gone

	ReasonDeleted     = "deleted"     // step: the function is gone
	ReasonStepUnmark  = "step-unmark" // step: arch:step (or the spec step) taken off
	ReasonUnreachable = "unreachable" // step: still marked, the entry point no longer reaches it
	ReasonWalk        = "walk"        // step of an automatic flow: the walk no longer goes there
	ReasonFlowGone    = "flow-gone"   // journey: the flow no longer exists
)

// Change codes.
const (
	StepRemoved       = "step-removed"
	StepAdded         = "step-added"
	StepText          = "step-text"
	StepOrder         = "step-order"
	FlowRemoved       = "flow-removed" // from a journey
	FlowAdded         = "flow-added"   // to a journey
	StateAdded        = "state-added"
	StateRemoved      = "state-removed"
	TransitionAdded   = "transition-added"
	TransitionRemoved = "transition-removed"
	TransitionFuncs   = "transition-funcs" // the same transition is made by other functions
)

// Ref is a function as the report shows it.
type Ref struct {
	Fid  string `json:"fid"`
	Name string `json:"name"`
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Change is one difference inside a diagram.
type Change struct {
	Code   string   `json:"code"`
	Text   string   `json:"text,omitempty"`   // step caption (new one for step-text)
	Old    string   `json:"old,omitempty"`    // old caption (step-text)
	Func   *Ref     `json:"func,omitempty"`   // step function
	Reason string   `json:"reason,omitempty"` // step-removed, flow-removed
	Flow   string   `json:"flow,omitempty"`   // flow id (journeys)
	State  string   `json:"state,omitempty"`  // state-added / removed
	From   string   `json:"from,omitempty"`   // transitions
	To     string   `json:"to,omitempty"`
	Funcs  []Ref    `json:"funcs,omitempty"`  // transition functions (new ones for transition-funcs)
	Before []Ref    `json:"before,omitempty"` // transition-funcs: the old functions
	Order  []string `json:"order,omitempty"`  // step-order: the new order (display names)
}

// Diff is the difference of one diagram.
type Diff struct {
	Kind    string   `json:"kind"` // flow | journey | states
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`           // added | removed | changed
	Reason  string   `json:"reason,omitempty"` // removed: why
	Entry   *Ref     `json:"entry,omitempty"`  // removed flow: its entry point in the base
	Changes []Change `json:"changes,omitempty"`
}

// Report is the outcome of Compare.
type Report struct {
	Base  string `json:"base,omitempty"`
	Head  string `json:"head,omitempty"`
	Diffs []Diff `json:"diffs"`
	// Broken lists the broken diagrams of the head that the base did not have;
	// OldBroken counts those that were broken in the base already.
	Broken    []Broken `json:"broken"`
	OldBroken int      `json:"old_broken"`
}

// Count returns how many diagrams have the status.
func (r Report) Count(status string) int {
	n := 0
	for _, d := range r.Diffs {
		if d.Status == status {
			n++
		}
	}
	return n
}

// Empty reports that no diagram changed and nothing new is broken.
func (r Report) Empty() bool { return len(r.Diffs) == 0 && len(r.Broken) == 0 }

// Compare finds what the head changes in the diagrams of the base.
func Compare(base, head *Snapshot) Report {
	r := Report{Diffs: []Diff{}, Broken: []Broken{}}
	r.Diffs = append(r.Diffs, compareFlows(base, head)...)
	r.Diffs = append(r.Diffs, compareJourneys(base, head)...)
	r.Diffs = append(r.Diffs, compareMachines(base, head)...)
	r.Diffs = append(r.Diffs, comparePages(base, head)...)
	old := map[string]bool{}
	for _, b := range base.Broken {
		old[b.key()] = true
	}
	for _, b := range head.Broken {
		if old[b.key()] {
			r.OldBroken++
		} else {
			r.Broken = append(r.Broken, b)
		}
	}
	return r
}

func (s *Snapshot) ref(fid string) *Ref {
	x, ok := s.Funcs[fid]
	if !ok {
		return &Ref{Fid: fid, Name: fid}
	}
	return &Ref{Fid: fid, Name: x.Name, File: x.File, Line: x.Line}
}

// refIn is the function from the snapshot that has it (head first).
func refIn(fid string, a, b *Snapshot) *Ref {
	if _, ok := a.Funcs[fid]; ok {
		return a.ref(fid)
	}
	return b.ref(fid)
}

// order lists the ids of both snapshots: the head order, then the ones only in
// the base, then (snapshots made by hand, without an order) the rest sorted.
func order[T any](head, base []string, hm, bm map[string]T) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range [][]string{head, base} {
		for _, id := range l {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	var rest []string
	for _, m := range []map[string]T{hm, bm} {
		for id := range m {
			if !seen[id] {
				seen[id] = true
				rest = append(rest, id)
			}
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func compareFlows(base, head *Snapshot) []Diff {
	// entry points of the declared flows of the head: an automatic flow of the
	// base that became declared is not "gone"
	declEntry := map[string]bool{}
	for _, fl := range head.Flows {
		if fl.Declared {
			declEntry[fl.Entry()] = true
		}
	}
	var out []Diff
	for _, id := range order(head.flowOrder, base.flowOrder, head.Flows, base.Flows) {
		b, h := base.Flows[id], head.Flows[id]
		switch {
		case b != nil && h != nil:
			if !b.Shown() && !h.Shown() {
				continue
			}
			if ch := flowChanges(b, h, base, head); len(ch) > 0 {
				out = append(out, Diff{Kind: KindFlow, ID: id, Name: h.Name, Status: Changed, Changes: ch})
			}
		case b != nil:
			if !b.Shown() {
				continue
			}
			e := b.Entry()
			if !b.Declared && declEntry[e] {
				continue
			}
			d := Diff{Kind: KindFlow, ID: id, Name: b.Name, Status: Removed}
			if e != "" {
				d.Entry = base.ref(e)
			}
			hf, ok := head.Funcs[e]
			switch {
			case e != "" && !ok:
				d.Reason = ReasonEntryDeleted
			case b.Declared && e != "" && base.Funcs[e].Flows[id] && !hf.Flows[id]:
				d.Reason = ReasonEntryUnmarked
			case b.Spec:
				d.Reason = ReasonSpecRemoved
			case !b.Declared && !hf.Entry:
				d.Reason = ReasonNotEntry
			case !b.Declared:
				continue // still an entry point: only left the top, not a change of the code
			default:
				d.Reason = ReasonUnmarked
			}
			out = append(out, d)
		case h != nil:
			if !h.Shown() {
				continue
			}
			d := Diff{Kind: KindFlow, ID: id, Name: h.Name, Status: Added}
			for _, s := range h.Steps {
				d.Changes = append(d.Changes, Change{Code: StepAdded, Text: s.Text, Func: head.ref(s.Fid)})
			}
			out = append(out, d)
		}
	}
	return out
}

func flowChanges(b, h *Flow, base, head *Snapshot) []Change {
	var ch []Change
	inH := map[string]Step{}
	for _, s := range h.Steps {
		inH[s.Fid] = s
	}
	inB := map[string]Step{}
	for _, s := range b.Steps {
		inB[s.Fid] = s
	}
	for _, s := range b.Steps {
		if _, ok := inH[s.Fid]; ok {
			continue
		}
		c := Change{Code: StepRemoved, Text: s.Text, Func: base.ref(s.Fid)}
		hf, ok := head.Funcs[s.Fid]
		switch {
		case !ok:
			c.Reason = ReasonDeleted
		case !h.Declared:
			c.Reason = ReasonWalk
		case !hf.Steps[h.ID] && base.Funcs[s.Fid].Steps[b.ID]:
			c.Reason = ReasonStepUnmark
		case hf.Steps[h.ID]:
			c.Reason = ReasonUnreachable
		default:
			c.Reason = ReasonWalk
		}
		ch = append(ch, c)
	}
	for _, s := range h.Steps {
		if _, ok := inB[s.Fid]; !ok {
			ch = append(ch, Change{Code: StepAdded, Text: s.Text, Func: head.ref(s.Fid)})
		}
	}
	for _, s := range h.Steps {
		if o, ok := inB[s.Fid]; ok && o.Text != s.Text {
			ch = append(ch, Change{Code: StepText, Text: s.Text, Old: o.Text, Func: head.ref(s.Fid)})
		}
	}
	// the relative order of the steps both have
	var ob, oh []string
	for _, s := range b.Steps {
		if _, ok := inH[s.Fid]; ok {
			ob = append(ob, s.Fid)
		}
	}
	for _, s := range h.Steps {
		if _, ok := inB[s.Fid]; ok {
			oh = append(oh, s.Fid)
		}
	}
	if !equal(ob, oh) {
		c := Change{Code: StepOrder}
		for _, s := range h.Steps {
			c.Order = append(c.Order, stepName(s, head))
		}
		ch = append(ch, c)
	}
	return ch
}

func stepName(s Step, sn *Snapshot) string {
	if s.Text != "" {
		return s.Text
	}
	return sn.ref(s.Fid).Name
}

func compareJourneys(base, head *Snapshot) []Diff {
	var out []Diff
	brokenJ := map[string]bool{}
	for _, b := range head.Broken {
		if b.Kind == BrokenJourneyFlow {
			brokenJ[b.Diagram] = true
		}
	}
	for _, id := range order(head.journeyOrder, base.journeyOrder, head.Journeys, base.Journeys) {
		b, h := base.Journeys[id], head.Journeys[id]
		switch {
		case b != nil && h != nil:
			if !b.Shown() && !h.Shown() {
				continue
			}
			var ch []Change
			hs, bs := set(h.Flows), set(b.Flows)
			for _, f := range b.Flows {
				if !hs[f] {
					c := Change{Code: FlowRemoved, Flow: f, Reason: ReasonUnmarked}
					if _, ok := head.Flows[f]; !ok {
						c.Reason = ReasonFlowGone
					}
					ch = append(ch, c)
				}
			}
			for _, f := range h.Flows {
				if !bs[f] {
					ch = append(ch, Change{Code: FlowAdded, Flow: f})
				}
			}
			if len(ch) > 0 {
				out = append(out, Diff{Kind: KindJourney, ID: id, Name: h.Name, Status: Changed, Changes: ch})
			}
		case b != nil:
			if !b.Declared {
				continue // automatic journeys come and go with the flows
			}
			d := Diff{Kind: KindJourney, ID: id, Name: b.Name, Status: Removed, Reason: ReasonUnmarked}
			if brokenJ[id] {
				d.Reason = ReasonFlowsGone
			}
			for _, f := range b.Flows {
				if _, ok := head.Flows[f]; !ok {
					d.Changes = append(d.Changes, Change{Code: FlowRemoved, Flow: f, Reason: ReasonFlowGone})
				}
			}
			out = append(out, d)
		case h != nil:
			if !h.Declared {
				continue
			}
			d := Diff{Kind: KindJourney, ID: id, Name: h.Name, Status: Added}
			for _, f := range h.Flows {
				d.Changes = append(d.Changes, Change{Code: FlowAdded, Flow: f})
			}
			out = append(out, d)
		}
	}
	return out
}

func compareMachines(base, head *Snapshot) []Diff {
	keys := map[string]bool{}
	for k := range base.Machines {
		keys[k] = true
	}
	for k := range head.Machines {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	refs := func(fids []string) []Ref {
		var out []Ref
		for _, f := range fids {
			out = append(out, *refIn(f, head, base))
		}
		return out
	}
	var out []Diff
	for _, k := range sorted {
		b, h := base.Machines[k], head.Machines[k]
		if b == nil {
			b = &Machine{Entity: k}
		}
		if h == nil {
			h = &Machine{Entity: k}
		}
		var ch []Change
		bs, hs := set(b.States), set(h.States)
		for _, s := range h.States {
			if !bs[s] {
				ch = append(ch, Change{Code: StateAdded, State: s})
			}
		}
		for _, s := range b.States {
			if !hs[s] {
				ch = append(ch, Change{Code: StateRemoved, State: s})
			}
		}
		bt := map[[2]string]Transition{}
		for _, t := range b.Transitions {
			bt[[2]string{t.From, t.To}] = t
		}
		ht := map[[2]string]Transition{}
		for _, t := range h.Transitions {
			ht[[2]string{t.From, t.To}] = t
		}
		for _, t := range h.Transitions {
			o, ok := bt[[2]string{t.From, t.To}]
			switch {
			case !ok:
				ch = append(ch, Change{Code: TransitionAdded, From: t.From, To: t.To, Funcs: refs(t.Fids)})
			case !equal(o.Fids, t.Fids):
				ch = append(ch, Change{Code: TransitionFuncs, From: t.From, To: t.To, Funcs: refs(t.Fids), Before: refs(o.Fids)})
			}
		}
		for _, t := range b.Transitions {
			if _, ok := ht[[2]string{t.From, t.To}]; !ok {
				ch = append(ch, Change{Code: TransitionRemoved, From: t.From, To: t.To, Funcs: refs(t.Fids)})
			}
		}
		if len(ch) == 0 {
			continue
		}
		st := Changed
		switch {
		case base.Machines[k] == nil:
			st = Added
		case head.Machines[k] == nil:
			st = Removed
		}
		out = append(out, Diff{Kind: KindStates, ID: k, Name: k, Status: st, Changes: ch})
	}
	return out
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
