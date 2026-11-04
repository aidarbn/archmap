package autosem

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestVariants(t *testing.T) {
	b := newJB()
	pay := decl("pay")
	pay.Flows[0].Exit = "order paid"
	b.add("Pay", "http", pay, []string{"Request", "Poll"}, nil)
	b.add("Request", "", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "pay"}},
		On: []facts.OnNote{{Cond: "kaspi down", Text: "show error"}}}, nil, nil)
	b.add("Poll", "", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "pay"}},
		On: []facts.OnNote{{Cond: "paid", Flow: "kassa"}, {Cond: "expired", Flow: "cancel", Text: "cancel the order"}}}, nil, nil)
	b.add("Kassa", "http", decl("kassa"), nil, nil)
	b.add("Cancel", "http", decl("cancel"), nil, nil)
	m := Build(b.f)
	fl := flowByID(m, "pay")
	steps := fl.Steps
	if len(steps) != 3 {
		t.Fatalf("steps %v", steps)
	}
	want := []Variant{
		{ID: "pay#0", Name: "main path", Main: true, Choices: []Choice{}, Steps: steps, End: VariantEnd{Kind: "exit", Text: "order paid"}},
		{ID: "pay#1", Name: "kaspi down → show error", Choices: []Choice{{jid("Request"), "kaspi down"}}, Steps: steps[:2], End: VariantEnd{Kind: "text", Text: "show error"}},
		{ID: "pay#2", Name: "paid → Flow kassa", Choices: []Choice{{jid("Poll"), "paid"}}, Steps: steps, End: VariantEnd{Kind: "flow", Flow: "kassa"}},
		{ID: "pay#3", Name: "expired → cancel the order", Choices: []Choice{{jid("Poll"), "expired"}}, Steps: steps, End: VariantEnd{Kind: "flow", Flow: "cancel", Text: "cancel the order"}},
	}
	if !reflect.DeepEqual(fl.Variants, want) {
		t.Fatalf("got %+v", fl.Variants)
	}
	if fl.VariantsMore != 0 {
		t.Fatal("more")
	}
	if v := flowByID(m, "kassa").Variants; v != nil {
		t.Fatalf("a flow without forks has no variants: %+v", v)
	}
	if v := flowByID(m, "auto:"+jid("Poll")); v != nil {
		t.Fatal("not a flow")
	}
}

// Many outcomes: same ends fold into one "any" variant, the rest is cut at MaxVariants.
func TestVariantsLimit(t *testing.T) {
	b := newJB()
	var on []facts.OnNote
	for i := 0; i < 8; i++ { // eight ways to the same end
		on = append(on, facts.OnNote{Cond: fmt.Sprintf("e%d", i), Text: "fail"})
	}
	for i := 0; i < 20; i++ {
		on = append(on, facts.OnNote{Cond: fmt.Sprintf("c%d", i), Flow: fmt.Sprintf("t%d", i)})
	}
	a := decl("a")
	a.On = on
	b.add("A", "http", a, nil, nil)
	m := Build(b.f)
	fl := flowByID(m, "a")
	if len(fl.Variants) != MaxVariants {
		t.Fatalf("%d variants", len(fl.Variants))
	}
	// main + folded "fail" + 20 flow ends = 22, cut to 12
	if fl.VariantsMore != 10 {
		t.Fatalf("more %d", fl.VariantsMore)
	}
	f1 := fl.Variants[1]
	if !f1.Any || len(f1.Choices) != 8 || f1.End.Text != "fail" || f1.Name != "e0 | e1 | e2 | e3 | e4 | e5 | e6 | e7 → fail" {
		t.Fatalf("%+v", f1)
	}
	if fl.Variants[11].ID != "a#11" {
		t.Fatal(fl.Variants[11].ID)
	}
}

func TestVariantsUnderLimitNotFolded(t *testing.T) {
	b := newJB()
	a := decl("a")
	a.On = []facts.OnNote{{Cond: "x", Text: "same"}, {Cond: "y", Text: "same"}}
	b.add("A", "http", a, nil, nil)
	fl := flowByID(Build(b.f), "a")
	if len(fl.Variants) != 3 || fl.Variants[1].Any {
		t.Fatalf("%+v", fl.Variants)
	}
}
