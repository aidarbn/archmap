package render

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// planMachine (viewer/views/status.js, between "plan:begin" and "plan:end") decides the layout of a state
// machine: the main path, the finals band, the brackets "many into one", glued parallel transitions. It is pure
// JavaScript with no DOM, so the test runs it in node on table cases; without node the test is skipped.

type stTr struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Fid    string `json:"fid,omitempty"`
	Insert bool   `json:"insert,omitempty"`
}

type stMachine struct {
	States    []string `json:"states"`
	Trs       []stTr   `json:"trs"`
	CodeFinal []string `json:"codeFinal,omitempty"`
}

type stEdge struct {
	From, To, Cls, Text string
	N                   int
	Grouped             bool
}

type stPlan struct {
	Pseudo, Initial, Main, Loose, NoEntry []string
	Finals                                []struct{ N, Kind string }
	Side                                  []struct {
		N     string
		Below bool
	}
	Edges  []stEdge
	Groups []struct {
		To   string
		From []string
		Text string
	}
	Any map[string][]stTr
}

func runPlans(t *testing.T, ms []stMachine, texts map[string]string) []stPlan {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	src, err := os.ReadFile("viewer/views/status.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	a, b := strings.Index(s, "/* plan:begin"), strings.Index(s, "/* plan:end */")
	if a < 0 || b < a {
		t.Fatal("plan markers not found in status.js")
	}
	in, _ := json.Marshal(map[string]any{"ms": ms, "texts": texts})
	script := s[a:b] + `
const inp = JSON.parse(require('fs').readFileSync(0, 'utf8'));
const res = inp.ms.map((m) => planMachine(m, { text: (x) => (inp.texts || {})[x.fid] || '', short: (f) => f.split('.').pop() }));
process.stdout.write(JSON.stringify(res.map((p) => ({
  pseudo: p.pseudo, initial: p.initial, main: p.main, loose: p.loose, noEntry: p.noEntry, any: p.any,
  finals: p.finals.map((f) => ({ n: f.n, kind: f.kind })), side: p.side.map((x) => ({ n: x.n, below: x.below })),
  edges: p.edges.map((e) => ({ from: e.from, to: e.to, cls: e.cls, text: e.text, n: e.n, grouped: !!e.grouped })),
  groups: p.groups.map((g) => ({ to: g.to, from: g.from, text: g.text })),
}))));
`
	cmd := exec.Command(node, "-e", script)
	cmd.Stdin = strings.NewReader(string(in))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	var ps []stPlan
	if err := json.Unmarshal(out, &ps); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return ps
}

func tr(from, to, fid string) stTr { return stTr{From: from, To: to, Fid: fid} }

// order is the Taply OrderStatus machine in short: 13 statuses, the main path through the QR payment,
// cancellation from four statuses, expiry from three, a return qr_scanned → payment_requested.
var order = stMachine{
	States: []string{"validation_requested", "validated", "validation_failed", "payment_requested", "qr_scanned", "payment_failed", "accepted", "created", "rejected", "failed", "cancelled", "expired", "нет"},
	Trs: []stTr{
		tr("нет", "validation_requested", "svc.CreateOrder"),
		tr("validation_requested", "validated", "repo.UpdateValidationResult"),
		tr("validation_requested", "validation_failed", "repo.UpdateValidationResult"),
		tr("validated", "payment_requested", "svc.RequestQR"), tr("validated", "payment_requested", "repo.SetPaymentRequested"),
		tr("payment_requested", "qr_scanned", "svc.onQRScanned"),
		tr("qr_scanned", "payment_requested", "svc.RequestQR"),
		tr("validated", "accepted", "svc.ConfirmOrder"), tr("payment_requested", "accepted", "svc.ConfirmOrder"),
		tr("qr_scanned", "accepted", "svc.ConfirmOrder"), tr("qr_scanned", "accepted", "svc.onPaymentSuccess"),
		tr("accepted", "created", "svc.finishOrder"), tr("accepted", "rejected", "svc.finishOrder"),
		tr("accepted", "failed", "svc.failOrder"),
		tr("validated", "cancelled", "svc.CancelOrder"), tr("payment_requested", "cancelled", "svc.CancelOrder"),
		tr("qr_scanned", "cancelled", "svc.CancelOrder"), tr("accepted", "cancelled", "svc.CancelOrder"),
		tr("", "cancelled", "svc.CancelOrder"), // the same function from a known status: dropped
		tr("validated", "expired", "repo.CancelExpired"), tr("payment_requested", "expired", "repo.CancelExpired"),
		tr("accepted", "expired", "repo.CancelExpired"),
		tr("", "expired", "worker.Cleanup"), // from any status, another function: kept
	},
	CodeFinal: []string{"created", "rejected", "failed", "cancelled", "expired", "payment_failed", "validation_failed"},
}

func TestPlanMainPath(t *testing.T) {
	cases := []struct {
		name    string
		m       stMachine
		main    []string
		initial []string
		finals  []string
	}{
		{"order: longest chain to the successful final", order,
			[]string{"validation_requested", "validated", "payment_requested", "qr_scanned", "accepted", "created"},
			[]string{"validation_requested"}, []string{"validation_failed", "rejected", "failed", "cancelled", "expired"}},
		{"no pseudo start: the status nothing leads into", stMachine{
			States: []string{"pending", "running", "completed", "failed"},
			Trs:    []stTr{tr("pending", "running", "a"), tr("running", "completed", "b"), tr("running", "failed", "c"), tr("pending", "failed", "d")},
		}, []string{"pending", "running", "completed"}, []string{"pending"}, []string{"failed"}},
		{"success before failure even when the failure chain is longer", stMachine{
			States: []string{"new", "a", "b", "c", "error", "done"},
			Trs:    []stTr{tr("new", "a", "1"), tr("a", "b", "2"), tr("b", "c", "3"), tr("c", "error", "4"), tr("new", "done", "5")},
		}, []string{"new", "done"}, []string{"new"}, []string{"error"}},
		{"a cycle and a neutral end (lifecycle)", stMachine{
			States: []string{"нет", "active", "inactive", "deleted", "purged"},
			Trs:    []stTr{tr("нет", "active", "c"), tr("active", "inactive", "u"), tr("inactive", "active", "u"), tr("active", "deleted", "d"), tr("inactive", "deleted", "d"), tr("deleted", "purged", "h")},
		}, []string{"active", "inactive", "deleted", "purged"}, []string{"active"}, nil},
		{"a new row born in a status (INSERT) is the initial one", stMachine{
			States: []string{"pending", "running", "completed", "failed"},
			Trs:    []stTr{{To: "pending", Fid: "repo.Create", Insert: true}, tr("pending", "running", "a"), tr("running", "completed", "b"), tr("", "failed", "c")},
		}, []string{"pending", "running", "completed"}, []string{"pending"}, []string{"failed"}},
		{"a failure-like function name does not turn a middle status into a failure", stMachine{
			States: []string{"draft", "review", "published", "rejected"},
			Trs:    []stTr{tr("draft", "review", "svc.CancelOrder"), tr("review", "published", "svc.CancelOrder"), tr("review", "rejected", "svc.CancelOrder")},
		}, []string{"draft", "review", "published"}, []string{"draft"}, []string{"rejected"}},
		{"two statuses", stMachine{States: []string{"off", "on"}, Trs: []stTr{tr("off", "on", "x")}},
			[]string{"off", "on"}, []string{"off"}, nil},
		{"only from any: no initial, no main path", stMachine{
			States: []string{"created", "wait", "error", "expired"},
			Trs:    []stTr{tr("", "error", "x"), tr("", "expired", "y")},
		}, nil, nil, []string{"error", "expired"}},
		{"several initials: the one with the longest path", stMachine{
			States: []string{"draft", "imported", "review", "published"},
			Trs:    []stTr{tr("draft", "review", "a"), tr("review", "published", "b"), tr("imported", "published", "c")},
		}, []string{"draft", "review", "published"}, []string{"draft", "imported"}, nil},
	}
	ms := make([]stMachine, len(cases))
	for i, c := range cases {
		ms[i] = c.m
	}
	ps := runPlans(t, ms, nil)
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := ps[i]
			if !reflect.DeepEqual(p.Main, c.main) && !(len(p.Main) == 0 && len(c.main) == 0) {
				t.Errorf("main %v, want %v", p.Main, c.main)
			}
			if !reflect.DeepEqual(p.Initial, c.initial) && !(len(p.Initial) == 0 && len(c.initial) == 0) {
				t.Errorf("initial %v, want %v", p.Initial, c.initial)
			}
			var fin []string
			for _, f := range p.Finals {
				fin = append(fin, f.N)
			}
			if !sameSet(fin, c.finals) {
				t.Errorf("finals %v, want %v", fin, c.finals)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}

func TestPlanGroupsAndGlue(t *testing.T) {
	texts := map[string]string{"svc.CancelOrder": "Заказ отменяется", "svc.RequestQR": "Создаётся QR"}
	twoInto := stMachine{
		States: []string{"a", "b", "c", "x"},
		Trs:    []stTr{tr("a", "b", "1"), tr("b", "c", "2"), tr("a", "x", "f"), tr("b", "x", "f")},
	}
	ps := runPlans(t, []stMachine{order, twoInto}, texts)
	p := ps[0]
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"cancelled: one bracket from four statuses, in main-path order",
			groupOf(p, "cancelled"), []string{"validated", "payment_requested", "qr_scanned", "accepted"}},
		{"expired: three statuses are enough", groupOf(p, "expired"), []string{"validated", "payment_requested", "accepted"}},
		{"failed: one source, no bracket", groupOf(p, "failed"), []string(nil)},
		{"two into one stay arrows", len(ps[1].Groups), 0},
		{"bracket caption from the code", groupText(p, "cancelled"), "Заказ отменяется"},
		{"parallel transitions glued ×2", edgeOf(p, "validated", "payment_requested").N, 2},
		{"glued caption: the human one", edgeOf(p, "validated", "payment_requested").Text, "Создаётся QR"},
		{"no caption in the code: the short function name", edgeOf(p, "accepted", "created").Text, "finishOrder"},
		{"main edge", edgeOf(p, "payment_requested", "qr_scanned").Cls, "main"},
		{"skip along the main path", edgeOf(p, "validated", "accepted").Cls, "skip"},
		{"return", edgeOf(p, "qr_scanned", "payment_requested").Cls, "back"},
		{"creation", edgeOf(p, "нет", "validation_requested").Cls, "start"},
		{"grouped edge is not drawn alone", edgeOf(p, "accepted", "cancelled").Grouped, true},
		{"from any: duplicate of a known transition dropped", len(p.Any["cancelled"]), 0},
		{"from any: another function kept", len(p.Any["expired"]), 1},
		{"no entry: payment_failed", p.Loose, []string{"payment_failed"}},
		{"pseudo start is not a status", p.Pseudo, []string{"нет"}},
		// barycentre of the sources on the main path: expired (1, 2, 4) before cancelled (1, 2, 3, 4)
		{"finals ordered by their sources", finalsOf(p), []string{"validation_failed", "expired", "cancelled", "rejected", "failed"}},
		{"kinds", kindsOf(p), map[string]string{"validation_failed": "fail", "cancelled": "fail", "expired": "expired", "rejected": "fail", "failed": "fail"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !reflect.DeepEqual(c.got, c.want) {
				t.Errorf("got %#v, want %#v", c.got, c.want)
			}
		})
	}
}

func groupOf(p stPlan, to string) []string {
	for _, g := range p.Groups {
		if g.To == to {
			return g.From
		}
	}
	return nil
}

func groupText(p stPlan, to string) string {
	for _, g := range p.Groups {
		if g.To == to {
			return g.Text
		}
	}
	return ""
}

func edgeOf(p stPlan, from, to string) stEdge {
	for _, e := range p.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	return stEdge{}
}

func finalsOf(p stPlan) []string {
	var out []string
	for _, f := range p.Finals {
		out = append(out, f.N)
	}
	return out
}

func kindsOf(p stPlan) map[string]string {
	out := map[string]string{}
	for _, f := range p.Finals {
		out[f.N] = f.Kind
	}
	return out
}
