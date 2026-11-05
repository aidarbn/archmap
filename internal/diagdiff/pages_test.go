package diagdiff

import (
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/spec"
)

func pagesSnap(pages ...spec.Page) *Snapshot {
	s := &Snapshot{Flows: map[string]*Flow{}, Journeys: map[string]*Journey{}, Machines: map[string]*Machine{}, Pages: map[string]*Page{}, Funcs: map[string]Func{}}
	s.buildPages(&spec.Model{Pages: pages})
	return s
}

func pg(id, name string, frames ...spec.Frame) spec.Page {
	return spec.Page{ID: id, Name: name, File: "arch/pages/p.arch", Line: 1, Frames: frames}
}

func frame(kind string, refs ...string) spec.Frame {
	if kind == spec.FrameNote {
		return spec.Frame{Kind: kind, Text: refs[0], Line: 2}
	}
	return spec.Frame{Kind: kind, Refs: refs, Line: 2}
}

func TestComparePages(t *testing.T) {
	kaspi := pg("kaspi", "Оплата Kaspi", frame("journey", "guest-pay-kaspi"), frame("tables", "orders", "payments"))
	broken := frame("tables", "orders", "ghosts")
	broken.Missing = []string{"ghosts"}
	cases := []struct {
		name       string
		base, head *Snapshot
		want       []string // "kind id status: changes"
		broken     []string // "diagram ref"
	}{
		{name: "same", base: pagesSnap(kaspi), head: pagesSnap(kaspi)},
		{name: "page appeared", base: pagesSnap(), head: pagesSnap(kaspi),
			want: []string{"page kaspi added: frame-added journey guest-pay-kaspi; frame-added tables orders, payments"}},
		{name: "page gone", base: pagesSnap(kaspi), head: pagesSnap(),
			want: []string{"page kaspi removed(spec-removed)"}},
		{name: "frames and name changed",
			base: pagesSnap(kaspi),
			head: pagesSnap(pg("kaspi", "Kaspi", frame("journey", "guest-pay-kaspi"), frame("tables", "orders"), frame("note", "text"))),
			want: []string{"page kaspi changed: page-renamed Kaspi; frame-removed tables orders, payments; frame-added tables orders; frame-added note \"text\""}},
		{name: "a ref broke in the head", base: pagesSnap(kaspi), head: pagesSnap(pg("kaspi", "Оплата Kaspi", frame("journey", "guest-pay-kaspi"), broken)),
			want:   []string{"page kaspi changed: frame-removed tables orders, payments; frame-added tables orders, ghosts"},
			broken: []string{"kaspi tables ghosts"}},
		{name: "broken already in the base is not new", base: pagesSnap(pg("x", "X", broken)), head: pagesSnap(pg("x", "X", broken))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Compare(c.base, c.head)
			var got []string
			for _, d := range r.Diffs {
				s := d.Kind + " " + d.ID + " " + d.Status
				if d.Reason != "" {
					s += "(" + d.Reason + ")"
				}
				var ch []string
				for _, x := range d.Changes {
					ch = append(ch, x.Code+" "+x.Text)
				}
				if len(ch) > 0 {
					s += ": " + strings.Join(ch, "; ")
				}
				got = append(got, s)
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("diffs:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
			var br []string
			for _, b := range r.Broken {
				if b.Kind != BrokenPageRef {
					t.Errorf("broken kind %s", b.Kind)
				}
				br = append(br, b.Diagram+" "+b.Ref)
			}
			if strings.Join(br, "\n") != strings.Join(c.broken, "\n") {
				t.Errorf("broken %v, want %v", br, c.broken)
			}
		})
	}
}

func TestPagesText(t *testing.T) {
	base := pagesSnap()
	f := frame("api", "Nope")
	f.Missing = []string{"Nope"}
	head := pagesSnap(pg("kaspi", "Оплата Kaspi", frame("journey", "guest-pay-kaspi"), f))
	r := Compare(base, head)
	ru := Text(r, LangRU)
	for _, want := range []string{"Страница «Оплата Kaspi» (kaspi) — новая", "Рамки (2): journey guest-pay-kaspi; api Nope", "страница kaspi показывает api Nope, которого нет в коде (arch/pages/p.arch:2)"} {
		if !strings.Contains(ru, want) {
			t.Errorf("ru text has no %q:\n%s", want, ru)
		}
	}
	md := Markdown(r, LangEN)
	if !strings.Contains(md, "page `kaspi` shows `api Nope`, which the code does not have") {
		t.Errorf("markdown:\n%s", md)
	}
}
