package annot

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestParseAll(t *testing.T) {
	text := `Pays for an order.
arch:flow pay "Pay by Kaspi QR"
arch:actor Планшет
arch:exit "order is paid"
arch:flow pay2 “Second”
arch:step pay "request QR"
arch:ext Kaspi, "iiko"
arch:on paid -> pay-done
arch:on payment expired "cancel order"
arch:on declined -> retry "try again"
arch:state order: validated -> payment_requested`
	n, errs := ParseText(text, 10)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	want := &facts.ArchNote{
		Flows: []facts.FlowDecl{{ID: "pay", Name: "Pay by Kaspi QR", Actor: "Планшет", Exit: "order is paid"}, {ID: "pay2", Name: "Second"}},
		Steps: []facts.StepNote{{Flow: "pay", Text: "request QR"}},
		Ext:   []string{"Kaspi", "iiko"},
		On: []facts.OnNote{{Cond: "paid", Flow: "pay-done"}, {Cond: "payment expired", Text: "cancel order"},
			{Cond: "declined", Flow: "retry", Text: "try again"}},
		State: []facts.StateNote{{Entity: "order", From: "validated", To: "payment_requested"}},
	}
	if !reflect.DeepEqual(n, want) {
		t.Fatalf("got %+v", n)
	}
}

func TestParseErrors(t *testing.T) {
	text := `arch:flow x
arch:flow bad id "N"
arch:step
arch:step f "unterminated
arch:actor Bob
arch:on nothing
arch:state order validated
arch:ext
arch:boom x
plain prose arch:flow not a directive`
	_, errs := ParseText(text, 5)
	var lines []int
	for _, e := range errs {
		lines = append(lines, e.Line)
	}
	if !reflect.DeepEqual(lines, []int{5, 6, 7, 8, 9, 10, 11, 12, 13}) {
		t.Fatalf("lines %v: %v", lines, errs)
	}
}

func TestNoDirective(t *testing.T) {
	if n, errs := ParseText("just a comment", 1); n != nil || errs != nil {
		t.Fatal(n, errs)
	}
}

func TestBlock(t *testing.T) {
	cases := map[string]struct {
		src   string
		start int
		want  []string
	}{
		"go":     {"package a\n\n// Doc.\n// arch:flow a \"A\"\nfunc F() {}\n", 5, []string{"Doc.", `arch:flow a "A"`}},
		"python": {"x = 1\n\n# arch:step a \"s\"\n@decorator\ndef f():\n", 5, []string{`arch:step a "s"`}},
		"jsdoc":  {"/**\n * Hi.\n * arch:ext Kaspi\n */\nfunction f() {}\n", 5, []string{"", "Hi.", "arch:ext Kaspi", ""}},
		"rust":   {"/// arch:ext Pg\n#[inline]\nfn f() {}\n", 3, []string{"arch:ext Pg"}},
		"gap":    {"// arch:ext A\n\nfunc f() {}\n", 3, nil},
		"none":   {"x := 1\nfunc f() {}\n", 2, nil},
	}
	for name, c := range cases {
		var got []string
		for _, l := range Block(strings.Split(c.src, "\n"), c.start) {
			got = append(got, l.Text)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}

func TestBlockLineNumbers(t *testing.T) {
	ls := Block(strings.Split("a\n// one\n// two\nfunc f() {}", "\n"), 4)
	if len(ls) != 2 || ls[0].N != 2 || ls[1].N != 3 {
		t.Fatal(ls)
	}
}

func TestParseJourney(t *testing.T) {
	text := `arch:flow order "Create order"
arch:actor Tablet
arch:journey guest "Guest pays by QR"
arch:actor Guest
arch:exit "order is in the kitchen"
arch:then guest qr > confirm
arch:then guest send
arch:journey quick “Quick”: order > qr>confirm`
	n, errs := ParseText(text, 1)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if n.Flows[0].Actor != "Tablet" {
		t.Errorf("actor before the journey belongs to the flow: %+v", n.Flows)
	}
	want := []facts.JourneyDecl{
		{ID: "guest", Name: "Guest pays by QR", Actor: "Guest", Exit: "order is in the kitchen", Flows: []string{"order"}},
		{ID: "quick", Name: "Quick", Flows: []string{"order", "qr", "confirm"}},
	}
	if !reflect.DeepEqual(n.Journeys, want) {
		t.Fatalf("got %+v", n.Journeys)
	}
	wantThen := []facts.ThenNote{{Journey: "guest", Flows: []string{"qr", "confirm"}}, {Journey: "guest", Flows: []string{"send"}}}
	if !reflect.DeepEqual(n.Then, wantThen) {
		t.Fatalf("then %+v", n.Then)
	}
}

func TestParseJourneyFirstFlow(t *testing.T) {
	// the nearest flow above wins; a flow below is the fallback
	n, errs := ParseText("arch:flow a \"A\"\narch:flow b \"B\"\narch:journey j \"J\"", 1)
	if len(errs) != 0 || n.Journeys[0].Flows[0] != "b" {
		t.Fatalf("%+v %v", n, errs)
	}
	n, errs = ParseText("arch:journey j \"J\"\narch:flow a \"A\"", 1)
	if len(errs) != 0 || n.Journeys[0].Flows[0] != "a" {
		t.Fatalf("%+v %v", n, errs)
	}
}

func TestParseJourneyErrors(t *testing.T) {
	text := `arch:journey
arch:journey j
arch:journey bad:id "N"
arch:journey j "unterminated
arch:journey j "N": a > > b
arch:journey j "N" trailing
arch:journey j "N"
arch:then
arch:then j
arch:then j bad!id`
	n, errs := ParseText(text, 1)
	var lines []int
	for _, e := range errs {
		lines = append(lines, e.Line)
	}
	if !reflect.DeepEqual(lines, []int{1, 2, 3, 4, 5, 6, 8, 9, 10, 7}) { // line 7 is reported last: it waits for a flow
		t.Fatalf("lines %v: %v", lines, errs)
	}
	if n != nil {
		t.Fatalf("nothing valid expected: %+v", n)
	}
}
