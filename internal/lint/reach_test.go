package lint

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

// fn builds a function of file a.go with calls, queue hand-offs and markup.
func fn(name string, line int, calls, async []string, arch *facts.ArchNote) facts.Function {
	return facts.Function{Name: name, File: "a.go", Module: ".", Lang: "go", Start: line, End: line + 1,
		Callees: calls, AsyncCallees: async, Arch: arch}
}

func flowNote(id string) *facts.ArchNote {
	return &facts.ArchNote{Flows: []facts.FlowDecl{{ID: id, Name: id}}}
}

func stepNote(flows ...string) *facts.ArchNote {
	n := &facts.ArchNote{}
	for _, f := range flows {
		n.Steps = append(n.Steps, facts.StepNote{Flow: f})
	}
	return n
}

func TestUnreachableSteps(t *testing.T) {
	const (
		entry = "a.go::Entry"
		mid   = "a.go::mid"
		step  = "a.go::step"
		work  = "a.go::worker"
		other = "a.go::Other"
	)
	tests := []struct {
		name  string
		funcs map[string]facts.Function
		want  []string // "flow:fid"
	}{
		{"direct call", map[string]facts.Function{
			entry: fn("Entry", 1, []string{step}, nil, flowNote("pay")),
			step:  fn("step", 10, nil, nil, stepNote("pay")),
		}, nil},
		{"through an unmarked function", map[string]facts.Function{
			entry: fn("Entry", 1, []string{mid}, nil, flowNote("pay")),
			mid:   fn("mid", 5, []string{step}, nil, nil),
			step:  fn("step", 10, nil, nil, stepNote("pay")),
		}, nil},
		{"worker behind a queue", map[string]facts.Function{
			entry: fn("Entry", 1, []string{mid}, nil, flowNote("pay")),
			mid:   fn("mid", 5, nil, []string{work}, nil),
			work:  fn("worker", 20, []string{step}, nil, stepNote("pay")),
			step:  fn("step", 10, nil, nil, stepNote("pay")),
		}, nil},
		{"call removed", map[string]facts.Function{
			entry: fn("Entry", 1, nil, nil, flowNote("pay")),
			step:  fn("step", 10, nil, nil, stepNote("pay")),
		}, []string{"pay:" + step}},
		{"step of a branch flow is checked against that flow", map[string]facts.Function{
			entry: fn("Entry", 1, nil, nil, &facts.ArchNote{Flows: []facts.FlowDecl{{ID: "pay"}}, On: []facts.OnNote{{Cond: "x", Flow: "refund"}}}),
			other: fn("Other", 30, []string{step}, nil, flowNote("refund")),
			step:  fn("step", 10, nil, nil, stepNote("refund")),
		}, nil},
		{"one marker of two is unreachable", map[string]facts.Function{
			entry: fn("Entry", 1, []string{step}, nil, flowNote("pay")),
			other: fn("Other", 30, nil, nil, flowNote("refund")),
			step:  fn("step", 10, nil, nil, stepNote("pay", "refund")),
		}, []string{"refund:" + step}},
		{"flow without arch:flow is not this check", map[string]facts.Function{
			step: fn("step", 10, nil, nil, stepNote("ghost")),
		}, nil},
		{"call to an unknown function does not count", map[string]facts.Function{
			entry: fn("Entry", 1, []string{"x.go::gone"}, nil, flowNote("pay")),
			step:  fn("step", 10, nil, nil, stepNote("pay")),
		}, []string{"pay:" + step}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &facts.Facts{Functions: tc.funcs}
			var got []string
			for _, u := range UnreachableSteps(f) {
				got = append(got, u.Flow+":"+u.Fid)
				if u.Msg(f) == "" || u.Line != tc.funcs[u.Fid].Start {
					t.Errorf("bad finding %+v", u)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUnbackedStates(t *testing.T) {
	const (
		svc  = "a.go::Pay"
		repo = "a.go::save"
		far  = "a.go::Other"
	)
	note := &facts.ArchNote{State: []facts.StateNote{{Entity: "Order", From: "new", To: "paid"}}}
	ann := facts.Transition{From: "new", To: "paid", Fid: svc, Source: "annotation"}
	tests := []struct {
		name  string
		funcs map[string]facts.Function
		trans []facts.Transition
		want  int
	}{
		{"the function itself sets it", map[string]facts.Function{svc: fn("Pay", 1, nil, nil, note)},
			[]facts.Transition{{From: "new", To: "paid", Fid: svc, Source: "code", Confirmed: true}}, 0},
		{"a callee sets it, source state unseen", map[string]facts.Function{svc: fn("Pay", 1, []string{repo}, nil, note), repo: fn("save", 9, nil, nil, nil)},
			[]facts.Transition{ann, {To: "paid", Fid: repo, Source: "code"}}, 0},
		{"a worker behind a queue sets it", map[string]facts.Function{svc: fn("Pay", 1, nil, []string{repo}, note), repo: fn("save", 9, nil, nil, nil)},
			[]facts.Transition{ann, {To: "paid", Fid: repo, Source: "code"}}, 0},
		{"only an unrelated function sets it", map[string]facts.Function{svc: fn("Pay", 1, nil, nil, note), far: fn("Other", 20, nil, nil, nil)},
			[]facts.Transition{ann, {To: "paid", Fid: far, Source: "code"}}, 1},
		{"the code sets another state", map[string]facts.Function{svc: fn("Pay", 1, []string{repo}, nil, note), repo: fn("save", 9, nil, nil, nil)},
			[]facts.Transition{ann, {To: "failed", Fid: repo, Source: "code"}}, 1},
		{"no code machine at all", map[string]facts.Function{svc: fn("Pay", 1, nil, nil, note)},
			[]facts.Transition{ann}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &facts.Facts{Functions: tc.funcs, States: map[string]*facts.StateMachine{"Order": {States: []string{"new", "paid"}, Transitions: tc.trans}}}
			got := UnbackedStates(f)
			if len(got) != tc.want {
				t.Fatalf("got %+v, want %d", got, tc.want)
			}
			for _, u := range got {
				if u.Fid != svc || u.To != "paid" || u.Msg(f) == "" {
					t.Errorf("bad finding %+v", u)
				}
			}
		})
	}
}
