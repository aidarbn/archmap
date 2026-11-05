package diagdiff

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// snap is a hand-made snapshot: funcs are "fid" or "fid:flow1,flow2" (steps it
// is marked with); entry points carry "!" ("Pay!:pay" — entry, arch:flow pay).
type snap struct {
	funcs    []string
	flows    []*Flow
	journeys []*Journey
	machines []*Machine
	broken   []Broken
}

func (s snap) build() *Snapshot {
	out := &Snapshot{Flows: map[string]*Flow{}, Journeys: map[string]*Journey{}, Machines: map[string]*Machine{}, Funcs: map[string]Func{}}
	for _, spec := range s.funcs {
		fid, marks, _ := strings.Cut(spec, ":")
		entry := strings.HasSuffix(fid, "!")
		fid = strings.TrimSuffix(fid, "!")
		x := Func{Name: fid, File: "a.go", Line: 10, Entry: entry, Flows: map[string]bool{}, Steps: map[string]bool{}}
		for _, m := range strings.Split(marks, ",") {
			if m == "" {
				continue
			}
			if entry {
				x.Flows[m] = true
			} else {
				x.Steps[m] = true
			}
		}
		out.Funcs[fid] = x
	}
	for _, f := range s.flows {
		out.Flows[f.ID] = f
	}
	for _, j := range s.journeys {
		out.Journeys[j.ID] = j
	}
	for _, m := range s.machines {
		out.Machines[m.Entity] = m
	}
	out.Broken = s.broken
	return out
}

func steps(xs ...string) []Step {
	var out []Step
	for _, x := range xs {
		fid, text, _ := strings.Cut(x, "=")
		out = append(out, Step{Fid: fid, Text: text})
	}
	return out
}

func declared(id string, st ...string) *Flow {
	return &Flow{ID: id, Name: id, Declared: true, Steps: steps(st...)}
}

// line is a compact form of a diff: "kind id status[ reason]: change; change".
func line(d Diff) string {
	s := d.Kind + " " + d.ID + " " + d.Status
	if d.Reason != "" {
		s += " " + d.Reason
	}
	var cs []string
	for _, c := range d.Changes {
		x := c.Code
		switch {
		case c.Func != nil:
			x += " " + c.Func.Fid
		case c.Flow != "":
			x += " " + c.Flow
		case c.State != "":
			x += " " + c.State
		case c.To != "":
			x += " " + c.From + ">" + c.To
		}
		if c.Reason != "" {
			x += " (" + c.Reason + ")"
		}
		if c.Code == StepText {
			x += " " + c.Old + "->" + c.Text
		}
		cs = append(cs, x)
	}
	if len(cs) > 0 {
		s += ": " + strings.Join(cs, "; ")
	}
	return s
}

func TestCompare(t *testing.T) {
	payBase := snap{
		funcs: []string{"Pay!:pay", "qr:pay", "save:pay", "poll:pay"},
		flows: []*Flow{declared("pay", "Pay=start", "qr=ask Kaspi", "save=store", "poll=poll")},
	}
	tests := []struct {
		name       string
		base, head snap
		want       []string
		broken     int
	}{
		{"nothing changed", payBase, payBase, nil, 0},
		{"step function deleted",
			payBase,
			snap{funcs: []string{"Pay!:pay", "qr:pay", "poll:pay"}, flows: []*Flow{declared("pay", "Pay=start", "qr=ask Kaspi", "poll=poll")}},
			[]string{"flow pay changed: step-removed save (deleted)"}, 0},
		{"arch:step taken off",
			payBase,
			snap{funcs: []string{"Pay!:pay", "qr:pay", "save", "poll:pay"}, flows: []*Flow{declared("pay", "Pay=start", "qr=ask Kaspi", "poll=poll")}},
			[]string{"flow pay changed: step-removed save (step-unmark)"}, 0},
		{"marked but no longer reachable",
			payBase,
			snap{funcs: []string{"Pay!:pay", "qr:pay", "save:pay", "poll:pay"}, flows: []*Flow{declared("pay", "Pay=start", "qr=ask Kaspi", "poll=poll")}},
			[]string{"flow pay changed: step-removed save (unreachable)"}, 0},
		{"step added, caption changed, order changed",
			payBase,
			snap{funcs: []string{"Pay!:pay", "qr:pay", "save:pay", "poll:pay", "log:pay"},
				flows: []*Flow{declared("pay", "Pay=start", "save=store it", "qr=ask Kaspi", "poll=poll", "log=log")}},
			[]string{"flow pay changed: step-added log; step-text save store->store it; step-order"}, 0},
		{"entry point deleted",
			payBase,
			snap{funcs: []string{"qr:pay", "save:pay", "poll:pay"}},
			[]string{"flow pay removed entry-deleted"}, 0},
		{"arch:flow taken off the entry point",
			payBase,
			snap{funcs: []string{"Pay!", "qr:pay", "save:pay", "poll:pay"}},
			[]string{"flow pay removed entry-unmarked"}, 0},
		{"new declared flow",
			snap{funcs: []string{"Pay!"}},
			snap{funcs: []string{"Pay!:pay", "qr:pay"}, flows: []*Flow{declared("pay", "Pay", "qr")}},
			[]string{"flow pay added: step-added Pay; step-added qr"}, 0},
		{"automatic flow outside the top and undescribed is ignored",
			snap{funcs: []string{"H!", "a"}, flows: []*Flow{{ID: "auto:H", Steps: steps("H", "a")}}},
			snap{funcs: []string{"H!", "b"}, flows: []*Flow{{ID: "auto:H", Steps: steps("H", "b")}}},
			nil, 0},
		{"automatic top flow is compared",
			snap{funcs: []string{"H!", "a"}, flows: []*Flow{{ID: "auto:H", Top: true, Steps: steps("H", "a")}}},
			snap{funcs: []string{"H!", "a", "b"}, flows: []*Flow{{ID: "auto:H", Top: true, Steps: steps("H", "b")}}},
			[]string{"flow auto:H changed: step-removed a (walk); step-added b"}, 0},
		{"automatic flow that left the top is not gone",
			snap{funcs: []string{"H!"}, flows: []*Flow{{ID: "auto:H", Top: true, Steps: steps("H")}}},
			snap{funcs: []string{"H!"}},
			nil, 0},
		{"automatic described flow whose entry point was deleted",
			snap{funcs: []string{"H!"}, flows: []*Flow{{ID: "auto:H", Described: true, Steps: steps("H")}}},
			snap{},
			[]string{"flow auto:H removed entry-deleted"}, 0},
		{"automatic flow that became declared is not gone",
			snap{funcs: []string{"H!"}, flows: []*Flow{{ID: "auto:H", Top: true, Steps: steps("H")}}},
			snap{funcs: []string{"H!:h"}, flows: []*Flow{declared("h", "H")}},
			[]string{"flow h added: step-added H"}, 0},
		{"journey loses a flow that is gone, and it is broken",
			snap{funcs: []string{"Pay!:pay", "Ord!:order"}, flows: []*Flow{declared("pay", "Pay"), declared("order", "Ord")},
				journeys: []*Journey{{ID: "guest", Declared: true, Flows: []string{"order", "pay"}}}},
			snap{funcs: []string{"Pay!", "Ord!:order"}, flows: []*Flow{declared("order", "Ord")},
				journeys: []*Journey{{ID: "guest", Declared: true, Flows: []string{"order"}}},
				broken:   []Broken{{Kind: BrokenJourneyFlow, Diagram: "guest", Ref: "pay"}}},
			[]string{"flow pay removed entry-unmarked", "journey guest changed: flow-removed pay (flow-gone)"}, 1},
		{"journey gains a flow",
			snap{flows: []*Flow{declared("a"), declared("b")}, journeys: []*Journey{{ID: "j", Declared: true, Flows: []string{"a"}}}},
			snap{flows: []*Flow{declared("a"), declared("b")}, journeys: []*Journey{{ID: "j", Declared: true, Flows: []string{"a", "b"}}}},
			[]string{"journey j changed: flow-added b"}, 0},
		{"declared journey with all flows gone",
			snap{flows: []*Flow{declared("a")}, journeys: []*Journey{{ID: "j", Declared: true, Flows: []string{"a"}}}},
			snap{broken: []Broken{{Kind: BrokenJourneyFlow, Diagram: "j", Ref: "a"}}},
			[]string{"flow a removed unmarked", "journey j removed flows-gone: flow-removed a (flow-gone)"}, 1},
		{"broken already in the base is not new",
			snap{broken: []Broken{{Kind: BrokenNoEntry, Diagram: "x", Ref: "f"}}},
			snap{broken: []Broken{{Kind: BrokenNoEntry, Diagram: "x", Ref: "f"}}},
			nil, 0},
		{"states: added state, transitions added, gone and moved",
			snap{funcs: []string{"pay", "cancel", "old"}, machines: []*Machine{{Entity: "Order", States: []string{"new", "paid", "cancelled"},
				Transitions: []Transition{{From: "new", To: "paid", Fids: []string{"old"}}, {From: "new", To: "cancelled", Fids: []string{"cancel"}}}}}},
			snap{funcs: []string{"pay", "cancel"}, machines: []*Machine{{Entity: "Order", States: []string{"new", "paid", "cancelled", "refunded"},
				Transitions: []Transition{{From: "new", To: "paid", Fids: []string{"pay"}}, {From: "paid", To: "refunded", Fids: []string{"pay"}}}}}},
			[]string{"states Order changed: state-added refunded; transition-funcs new>paid; transition-added paid>refunded; transition-removed new>cancelled"}, 0},
		{"states: machine removed",
			snap{machines: []*Machine{{Entity: "Sync", States: []string{"a"}}}},
			snap{},
			[]string{"states Sync removed: state-removed a"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Compare(tc.base.build(), tc.head.build())
			var got []string
			for _, d := range r.Diffs {
				got = append(got, line(d))
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "))
			}
			if len(r.Broken) != tc.broken {
				t.Fatalf("broken: %+v", r.Broken)
			}
			// every report renders in both languages and as JSON
			for _, l := range []string{LangEN, LangRU} {
				if md := Markdown(r, l); !strings.HasPrefix(md, "### ") || (r.Empty() != strings.Contains(md, dict[l].none)) {
					t.Errorf("markdown %s:\n%s", l, md)
				}
				if Text(r, l) == "" {
					t.Error("empty text")
				}
			}
			b, err := JSON(r)
			var back Report
			if err != nil || json.Unmarshal(b, &back) != nil || len(back.Diffs) != len(r.Diffs) {
				t.Fatalf("json: %v %s", err, b)
			}
		})
	}
}

func TestMarkdown(t *testing.T) {
	base := snap{funcs: []string{"Pay!:pay", "save:pay"}, flows: []*Flow{declared("pay", "Pay=start", "save=store")}}
	head := snap{funcs: []string{"Pay!:pay"}, flows: []*Flow{declared("pay", "Pay=start")},
		broken: []Broken{{Kind: BrokenJourneyFlow, Diagram: "guest", Ref: "qr", File: "a.go", Line: 3}}}
	r := Compare(base.build(), head.build())
	ru := Markdown(r, LangRU)
	for _, want := range []string{
		"### Диаграммы archmap",
		"Изменено: 1 · появилось: 0 · исчезло: 0 · битых: 1",
		"- путь `guest` ссылается на сценарий `qr`, которого нет (a.go:3)",
		"<details><summary>Сценарий <code>pay</code> — изменён</summary>",
		"- Шаг пропал: «store» — `save` (функция удалена)",
		"</details>",
	} {
		if !strings.Contains(ru, want) {
			t.Errorf("missing %q in\n%s", want, ru)
		}
	}
	if en := Markdown(r, LangEN); !strings.Contains(en, "- Step gone: «store» — `save` (the function was deleted)") {
		t.Errorf("en:\n%s", en)
	}
	if txt := Text(r, LangEN); strings.Contains(txt, "`") || strings.Contains(txt, "<details>") {
		t.Errorf("text must be plain:\n%s", txt)
	}
	if none := Markdown(Report{}, LangRU); none != "### Диаграммы archmap\n\nДиаграммы не изменились.\n" {
		t.Errorf("empty: %q", none)
	}
}
