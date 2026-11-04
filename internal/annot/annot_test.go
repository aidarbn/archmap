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
