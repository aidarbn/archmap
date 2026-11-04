package extract

import (
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestMergeAnnotatedStates(t *testing.T) {
	out := &facts.Facts{
		Functions: map[string]facts.Function{
			"a.go::Pay": {Arch: &facts.ArchNote{State: []facts.StateNote{
				{Entity: "order", From: "new", To: "paid"},
				{Entity: "invoice", From: "draft", To: "sent"},
			}}},
			"a.go::Plain": {},
		},
		States: map[string]*facts.StateMachine{
			"OrderStatus": {States: []string{"new", "paid"}, Transitions: []facts.Transition{
				{From: "new", To: "paid", Fid: "a.go::Pay", Source: "code"},
			}},
		},
	}
	mergeAnnotatedStates(out)

	os := out.States["OrderStatus"]
	if len(os.Transitions) != 1 || !os.Transitions[0].Confirmed || os.Transitions[0].Source != "code" {
		t.Fatalf("code transition must be confirmed, not duplicated: %+v", os.Transitions)
	}
	if _, ok := out.States["order"]; ok {
		t.Fatal("entity matched OrderStatus, no new machine expected")
	}
	inv := out.States["invoice"]
	if inv == nil || len(inv.Transitions) != 1 || inv.Transitions[0].Source != "annotation" {
		t.Fatalf("invoice: %+v", inv)
	}
	if len(inv.States) != 2 {
		t.Fatalf("states %v", inv.States)
	}

	// no code machine at all: a machine keyed by the entity appears
	out2 := &facts.Facts{Functions: map[string]facts.Function{
		"b.go::F": {Arch: &facts.ArchNote{State: []facts.StateNote{{Entity: "order", From: "a", To: "b"}}}},
	}}
	mergeAnnotatedStates(out2)
	if sm := out2.States["order"]; sm == nil || sm.Transitions[0].Source != "annotation" {
		t.Fatalf("%+v", out2.States)
	}
}
