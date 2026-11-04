package autosem

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func declFacts() *facts.Facts {
	f := &facts.Facts{Repo: "demo", Modules: map[string]facts.Module{}, Functions: map[string]facts.Function{}}
	add := func(name string, arch *facts.ArchNote, entry bool, callees, async []string) {
		fn := facts.Function{Name: name, Start: 1, End: 30, Module: "internal/x", File: "internal/x/f.go", Lang: "go",
			Callees: callees, AsyncCallees: async, Arch: arch}
		if entry {
			fn.Entry = &facts.Entry{Kind: "http", Labels: []string{"POST /pay"}}
		}
		f.Functions["internal/x/f.go::"+name] = fn
		f.Modules["internal/x"] = facts.Module{}
	}
	id := func(n string) string { return "internal/x/f.go::" + n }
	add("Pay", &facts.ArchNote{
		Flows: []facts.FlowDecl{{ID: "pay", Name: "Pay by QR", Actor: "Tablet", Exit: "order paid"}},
		Steps: []facts.StepNote{{Flow: "pay", Text: "start"}},
	}, true, []string{id("glue")}, nil)
	add("glue", nil, false, []string{id("requestQR"), id("save")}, []string{id("Poll")})
	add("requestQR", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "pay", Text: "ask Kaspi"}}, Ext: []string{"Kaspi"}}, false, nil, nil)
	add("save", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "pay", Text: "store"}}}, false, nil, nil)
	add("Poll", &facts.ArchNote{
		Steps: []facts.StepNote{{Flow: "pay", Text: "poll"}},
		On:    []facts.OnNote{{Cond: "paid", Flow: "done"}, {Cond: "expired", Text: "cancel"}},
	}, false, nil, nil)
	add("orphan", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "pay"}}}, false, nil, nil)
	// another entry point without markup: stays an automatic flow
	add("Other", nil, true, []string{id("save")}, nil)
	return f
}

// Declared scenarios are ranked like automatic ones: at most MaxTop are "top",
// and every actor keeps one.
func TestDeclaredTopCapped(t *testing.T) {
	f := &facts.Facts{Repo: "demo", Modules: map[string]facts.Module{"internal/x": {}}, Functions: map[string]facts.Function{}}
	id := func(n string) string { return "internal/x/f.go::" + n }
	add := func(name string, arch *facts.ArchNote, entry bool, callees []string) {
		fn := facts.Function{Name: name, Start: 1, End: 9, Module: "internal/x", File: "internal/x/f.go", Lang: "go", Callees: callees, Arch: arch}
		if entry {
			fn.Entry = &facts.Entry{Kind: "http"}
		}
		f.Functions[id(name)] = fn
	}
	actors := []string{"Tablet", "Cabinet", "Admin"}
	for i := 0; i < 15; i++ {
		fl, step := fmt.Sprintf("f%d", i), fmt.Sprintf("step%d", i)
		add(fmt.Sprintf("Entry%d", i), &facts.ArchNote{
			Flows: []facts.FlowDecl{{ID: fl, Name: "Flow " + fl, Actor: actors[i%3]}},
			Steps: []facts.StepNote{{Flow: fl}},
		}, true, []string{id(step)})
		add(step, &facts.ArchNote{Steps: []facts.StepNote{{Flow: fl}}}, false, nil)
	}
	m := Build(f)
	top, byActor := 0, map[string]bool{}
	for _, fl := range m.Flows {
		if fl.Top {
			top++
			byActor[fl.Actor] = true
		}
	}
	if top == 0 || top > MaxTop {
		t.Fatalf("%d top flows of %d, want 1..%d", top, len(m.Flows), MaxTop)
	}
	for _, a := range actors {
		if !byActor[a] {
			t.Errorf("actor %s has no top flow", a)
		}
	}
}

func TestDeclaredFlowsFirst(t *testing.T) {
	m := Build(declFacts())
	if len(m.Flows) < 1 {
		t.Fatal("no flows")
	}
	d := m.Flows[0]
	id := func(n string) string { return "internal/x/f.go::" + n }
	if d.ID != "pay" || d.Kind != KindDeclared || !d.Top || d.Name != "Pay by QR" || d.Actor != "Tablet" || d.Exit != "order paid" {
		t.Fatalf("%+v", d)
	}
	if want := []string{id("Pay"), id("requestQR"), id("save"), id("Poll")}; !reflect.DeepEqual(d.Steps, want) {
		t.Fatalf("steps %v", d.Steps)
	}
	if d.Labels[id("requestQR")] != "ask Kaspi" || d.Labels[id("Pay")] != "start" {
		t.Fatalf("labels %v", d.Labels)
	}
	want := []Branch{{At: id("Poll"), Cond: "paid", Flow: "done"}, {At: id("Poll"), Cond: "expired", Text: "cancel"}}
	if !reflect.DeepEqual(d.Branches, want) {
		t.Fatalf("branches %+v", d.Branches)
	}
	// the declared entry point has no duplicate automatic flow
	n := 0
	for _, fl := range m.Flows {
		if fl.Steps[0] == id("Pay") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d flows from Pay", n)
	}
}
