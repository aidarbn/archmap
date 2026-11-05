package autosem

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

// jfacts builds a repo from entry points: add(name, kind, arch, callees, async).
type jb struct {
	f *facts.Facts
}

func newJB() *jb {
	return &jb{&facts.Facts{Repo: "demo", Modules: map[string]facts.Module{"internal/x": {}}, Functions: map[string]facts.Function{}}}
}

func jid(n string) string { return "internal/x/f.go::" + n }

func (b *jb) add(name, kind string, arch *facts.ArchNote, callees, async []string) {
	fn := facts.Function{Name: name, Start: len(b.f.Functions)*10 + 1, End: len(b.f.Functions)*10 + 9, Module: "internal/x", File: "internal/x/f.go", Lang: "go", Arch: arch}
	for _, c := range callees {
		fn.Callees = append(fn.Callees, jid(c))
	}
	for _, c := range async {
		fn.AsyncCallees = append(fn.AsyncCallees, jid(c))
	}
	if kind != "" {
		fn.Entry = &facts.Entry{Kind: kind, Labels: []string{name}}
	}
	b.f.Functions[jid(name)] = fn
}

func journeyByID(m Model, id string) *Journey {
	for i := range m.Journeys {
		if m.Journeys[i].ID == id {
			return &m.Journeys[i]
		}
	}
	return nil
}

func flowByID(m Model, id string) *Flow {
	for i := range m.Flows {
		if m.Flows[i].ID == id {
			return &m.Flows[i]
		}
	}
	return nil
}

func decl(id string) *facts.ArchNote {
	return &facts.ArchNote{Flows: []facts.FlowDecl{{ID: id, Name: "Flow " + id, Actor: "Tablet"}}}
}

func TestFlowIDsStable(t *testing.T) {
	b := newJB()
	b.add("Pay", "http", decl("pay"), nil, nil)
	b.add("Auto", "http", nil, nil, nil)
	m := Build(b.f)
	if flowByID(m, "pay") == nil || flowByID(m, "auto:"+jid("Auto")) == nil {
		t.Fatalf("ids: %+v", m.Flows)
	}
	if !reflect.DeepEqual(Build(b.f).Flows[0].ID, m.Flows[0].ID) {
		t.Fatal("unstable")
	}
}

// Declared journey: first flow of the block, then arch:then in source order,
// links from arch:on / queue where the code has them, "declared" otherwise.
func TestDeclaredJourney(t *testing.T) {
	b := newJB()
	a := decl("order")
	a.Journeys = []facts.JourneyDecl{{ID: "guest", Name: "Guest pays", Actor: "Guest", Flows: []string{"order"}}}
	b.add("Order", "http", a, []string{"Check"}, nil)
	b.add("Check", "", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "order"}},
		On: []facts.OnNote{{Cond: "need pay", Flow: "qr"}}}, nil, nil)
	q := decl("qr")
	q.Then = []facts.ThenNote{{Journey: "guest", Flows: []string{"qr"}}} // declared again: ignored as a repeat
	b.add("QR", "http", q, nil, []string{"Verify"})
	b.add("Verify", "task", decl("verify"), nil, nil)
	c := decl("confirm")
	c.Then = []facts.ThenNote{{Journey: "guest", Flows: []string{"verify", "confirm"}}}
	b.add("Confirm", "http", c, nil, nil)
	m := Build(b.f)
	j := journeyByID(m, "guest")
	if j == nil || !j.Declared || !j.Top || j.Actor != "Guest" {
		t.Fatalf("%+v", m.Journeys)
	}
	if want := []string{"order", "qr", "verify", "confirm"}; !reflect.DeepEqual(j.Flows, want) {
		t.Fatalf("flows %v", j.Flows)
	}
	want := []Link{{"order", "qr", ViaOn, "need pay"}, {"qr", "verify", ViaQueue, ""}, {"verify", "confirm", ViaClient, ""}}
	if !reflect.DeepEqual(j.Links, want) {
		t.Fatalf("links %+v", j.Links)
	}
	if m.Journeys[0].ID != "guest" {
		t.Fatal("declared journeys come first")
	}
}

func TestJourneyUnknownFlowSkipped(t *testing.T) {
	b := newJB()
	a := decl("a")
	a.Journeys = []facts.JourneyDecl{{ID: "j", Name: "J", Flows: []string{"a", "ghost"}}}
	b.add("A", "http", a, nil, nil)
	j := journeyByID(Build(b.f), "j")
	if j == nil || !reflect.DeepEqual(j.Flows, []string{"a"}) || len(j.Links) != 0 {
		t.Fatalf("%+v", j)
	}
}

// Automatic journeys: from a flow a person starts, over on-links and queues;
// paths with a shared prefix are one journey with a branch; no cycles; parts of
// longer journeys and cron-started chains are dropped.
func TestAutoJourneys(t *testing.T) {
	b := newJB()
	// pay -(on paid)-> confirm -(queue)-> send ; pay -(on failed)-> cancel ; confirm -(on again)-> pay (cycle)
	pay := decl("pay")
	b.add("Pay", "http", pay, []string{"Poll"}, nil)
	b.add("Poll", "", &facts.ArchNote{Steps: []facts.StepNote{{Flow: "pay"}},
		On: []facts.OnNote{{Cond: "paid", Flow: "confirm"}, {Cond: "failed", Flow: "cancel"}}}, nil, nil)
	cf := decl("confirm")
	cf.Flows[0].Actor = "" // entry kind decides
	b.add("Confirm", "http", &facts.ArchNote{Flows: cf.Flows, On: []facts.OnNote{{Cond: "again", Flow: "pay"}}}, nil, []string{"Send"})
	b.add("Send", "task", nil, nil, nil)
	b.add("Cancel", "task", decl("cancel"), nil, nil)
	// a cron chain: not a client journey
	b.add("Nightly", "task", &facts.ArchNote{Flows: []facts.FlowDecl{{ID: "nightly", Name: "Nightly", Actor: "Cron"}}}, nil, []string{"Send"})
	// a short chain contained in the long one
	b.add("Resend", "http", nil, nil, []string{"Send"})
	m := Build(b.f)

	sendID := "auto:" + jid("Send")
	j := journeyByID(m, "j:pay")
	if j == nil || j.Declared {
		t.Fatalf("%+v", m.Journeys)
	}
	if want := []string{"pay", "confirm", sendID, "cancel"}; !reflect.DeepEqual(j.Flows, want) {
		t.Fatalf("flows %v", j.Flows)
	}
	for _, l := range j.Links {
		if l.To == "pay" {
			t.Fatalf("cycle link %+v", l)
		}
	}
	if len(j.Links) != 3 {
		t.Fatalf("links %+v", j.Links)
	}
	if r := journeyByID(m, "j:auto:"+jid("Resend")); r == nil || len(r.Flows) != 2 || r.Links[0].Via != ViaQueue {
		t.Errorf("Resend -> Send: %+v", r)
	}
	// Confirm is started by a person too, but its journey is made of the same flows
	if journeyByID(m, "j:confirm") != nil {
		t.Error("a journey inside a longer one must be dropped")
	}
	for _, x := range m.Journeys {
		if x.ID == "j:nightly" {
			t.Errorf("cron journey: %+v", x)
		}
	}
	if !j.Top {
		t.Error("top")
	}
}

func TestAutoJourneyNotInsideDeclared(t *testing.T) {
	b := newJB()
	a := decl("a")
	a.Journeys = []facts.JourneyDecl{{ID: "jd", Name: "JD", Flows: []string{"a", "b"}}}
	b.add("A", "http", &facts.ArchNote{Flows: a.Flows, Journeys: a.Journeys, On: []facts.OnNote{{Cond: "x", Flow: "b"}}}, nil, nil)
	b.add("B", "http", decl("b"), nil, nil)
	m := Build(b.f)
	if len(m.Journeys) != 1 || m.Journeys[0].ID != "jd" {
		t.Fatalf("%+v", m.Journeys)
	}
}

func TestJourneyFlowCap(t *testing.T) {
	b := newJB()
	n := MaxJourneyFlows + 5
	for i := 0; i < n; i++ {
		var on []facts.OnNote
		if i+1 < n {
			on = []facts.OnNote{{Cond: "next", Flow: fmt.Sprintf("f%d", i+1)}}
		}
		b.add(fmt.Sprintf("F%d", i), "http", &facts.ArchNote{Flows: decl(fmt.Sprintf("f%d", i)).Flows, On: on}, nil, nil)
	}
	m := Build(b.f)
	j := journeyByID(m, "j:f0")
	if j == nil || len(j.Flows) != MaxJourneyFlows {
		t.Fatalf("%+v", j)
	}
}
