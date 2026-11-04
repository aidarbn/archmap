package spec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

const sample = `states OrderStatus
  state draft
  validated -> payment_requested   on kaspi-pay      "запрошен QR"
  payment_requested -> accepted    on payment.paid
  * -> cancelled                   on cancel-order
  final created, rejected, cancelled

flow kaspi-pay "Оплата заказа через Kaspi QR"
  actor Планшет
  step TabletsServer.RequestKaspiQRForOrder          "Гость запросил QR"
  step Service.requestQR                    ext Kaspi   "Создаём QR в Kaspi"
  step plan "Проверить лимит платежей"      # запланированный шаг без кода
  state Order: validated -> payment_requested
  alt processed -> confirm-order
  alt expired "QR истёк"
  exit "Заказ оплачен"

journey guest-kaspi "Гость платит Kaspi": create-order > kaspi-pay > confirm-order
`

// sameDoc compares documents without positions and without the nil/empty slice difference.
func sameDoc(a, b *Doc) bool {
	ja, _ := json.Marshal(strip(a))
	jb, _ := json.Marshal(strip(b))
	norm := func(b []byte) string { return strings.ReplaceAll(string(b), "[]", "null") }
	return norm(ja) == norm(jb)
}

// strip drops positions so that parsed documents can be compared.
func strip(d *Doc) *Doc {
	c := copyJSON(*d)
	for _, s := range c.States {
		for i := range s.Transitions {
			s.Transitions[i].Line = 0
		}
		s.Line = 0
	}
	for _, f := range c.Flows {
		f.Line = 0
		for i := range f.Steps {
			f.Steps[i].Line = 0
		}
		for i := range f.Alts {
			f.Alts[i].Line = 0
		}
		for i := range f.States {
			f.States[i].Line = 0
		}
	}
	for _, j := range c.Journeys {
		j.Line = 0
	}
	return &c
}

func mustParse(t *testing.T, text string) *Doc {
	t.Helper()
	d, errs := Parse("arch/t.arch", text)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return d
}

func TestParseSample(t *testing.T) {
	d := mustParse(t, sample)
	st := d.States[0]
	if st.Entity != "OrderStatus" || len(st.Transitions) != 3 || !reflect.DeepEqual(st.Final, []string{"created", "rejected", "cancelled"}) || !reflect.DeepEqual(st.States, []string{"draft"}) {
		t.Fatalf("states: %+v", st)
	}
	if tr := st.Transitions[0]; tr.From != "validated" || tr.To != "payment_requested" || tr.On != "kaspi-pay" || tr.Label != "запрошен QR" || tr.Line != 3 {
		t.Fatalf("transition: %+v", tr)
	}
	if st.Transitions[2].From != "*" {
		t.Fatalf("wildcard: %+v", st.Transitions[2])
	}
	fl := d.Flows[0]
	if fl.ID != "kaspi-pay" || fl.Actor != "Планшет" || len(fl.Steps) != 3 || fl.Exit != "Заказ оплачен" || len(fl.Alts) != 2 || len(fl.States) != 1 {
		t.Fatalf("flow: %+v", fl)
	}
	if s := fl.Steps[1]; s.Ref != "Service.requestQR" || s.Ext != "Kaspi" || s.Text != "Создаём QR в Kaspi" {
		t.Fatalf("step: %+v", s)
	}
	if s := fl.Steps[2]; !s.Planned || s.Ref != "" || s.Text != "Проверить лимит платежей" {
		t.Fatalf("planned: %+v", s)
	}
	if a := fl.Alts[0]; a.Cond != "processed" || a.Flow != "confirm-order" {
		t.Fatalf("alt: %+v", a)
	}
	if a := fl.Alts[1]; a.Cond != "expired" || a.Text != "QR истёк" || a.Flow != "" {
		t.Fatalf("alt: %+v", a)
	}
	if j := d.Journeys[0]; j.ID != "guest-kaspi" || j.Name != "Гость платит Kaspi" || !reflect.DeepEqual(j.Flows, []string{"create-order", "kaspi-pay", "confirm-order"}) {
		t.Fatalf("journey: %+v", j)
	}
}

func TestFormatRoundTrip(t *testing.T) {
	d := mustParse(t, sample)
	out := Format(d)
	d2 := mustParse(t, out)
	if !reflect.DeepEqual(strip(d), strip(d2)) {
		t.Fatalf("parse(format(x)) != x\n%s", out)
	}
	if again := Format(d2); again != out {
		t.Fatalf("format is not stable:\n%s\n---\n%s", out, again)
	}
	// aligned columns, trailing newline, no trailing spaces
	if !strings.Contains(out, "  validated -> payment_requested   on kaspi-pay       \"запрошен QR\"\n") &&
		!strings.Contains(out, "validated -> payment_requested   on kaspi-pay") {
		t.Fatalf("columns not aligned:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasSuffix(l, " ") {
			t.Fatalf("trailing space in %q", l)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatal("no trailing newline")
	}
}

func TestFormatQuoting(t *testing.T) {
	d := &Doc{Flows: []*Flow{{ID: "f", Name: `He said "hi" # not a comment`, Actor: `A "b"`, Exit: "line1\nline2",
		Steps: []Step{{Ref: "a.go::F", Text: `q "x" \ y`}, {Planned: true, Text: "later"}}}}}
	out := Format(d)
	d2 := mustParse(t, out)
	if !reflect.DeepEqual(strip(d), strip(d2)) {
		t.Fatalf("round trip failed:\n%s\n%+v\n%+v", out, strip(d).Flows[0], strip(d2).Flows[0])
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		text string
		line int
		msg  string
	}{
		{"states\n", 1, "expected `states"},
		{"flow a\n  step\n", 2, "expected `step"},
		{"flow a\n  step plan\n", 2, "planned step needs a caption"},
		{"flow a\n  frobnicate x\n", 2, "unknown line"},
		{"  step x\n", 1, "outside"},
		{"states S\n  a - b\n", 2, "expected `from -> to"},
		{"states S\n  a -> b on\n", 2, "trigger"},
		{"states S\n  a -> b \"oops\n", 2, "unterminated"},
		{"journey j: a > > b\n", 1, "not a flow id"},
		{"journey j\n", 1, "expected `journey"},
		{"widget x\n", 1, "unknown block"},
		{"step x\n", 1, "indented"},
		{"flow a\n  state Order validated -> x\n", 2, "expected `state"},
		{"flow a\n  step x \"cap\" extra\n", 2, "unexpected"},
	}
	for _, c := range cases {
		_, errs := Parse("arch/e.arch", c.text)
		if len(errs) == 0 {
			t.Errorf("%q: no error", c.text)
			continue
		}
		if errs[0].Line != c.line || !strings.Contains(errs[0].Msg, c.msg) || errs[0].File != "arch/e.arch" {
			t.Errorf("%q: got %v, want line %d %q", c.text, errs[0], c.line, c.msg)
		}
	}
	// all errors are reported, parsing goes on
	d, errs := Parse("x.arch", "flow a\n  bad\n  step ok.F\n  worse\n")
	if len(errs) != 2 || len(d.Flows[0].Steps) != 1 {
		t.Fatalf("recovery: %v %+v", errs, d.Flows[0])
	}
	if e := errs[0]; e.Col != 3 || !strings.HasPrefix(e.Error(), "x.arch:2:3: ") {
		t.Fatalf("position: %v", e)
	}
}

func TestCommentsAndBlank(t *testing.T) {
	d := mustParse(t, "# header\n\nstates S   # trailing\n  a->b on x  # arrow without spaces\n\n  final b\n")
	st := d.States[0]
	if len(st.Transitions) != 1 || st.Transitions[0].On != "x" || st.Final[0] != "b" {
		t.Fatalf("%+v", st)
	}
	d = mustParse(t, `flow a "n # not comment"`+"\n")
	if d.Flows[0].Name != "n # not comment" {
		t.Fatalf("%+v", d.Flows[0])
	}
}

func setOf(t *testing.T, texts map[string]string) *Set {
	t.Helper()
	s := FromTexts(texts)
	if len(s.Errors) > 0 {
		t.Fatalf("errors: %v", s.Errors)
	}
	return s
}

func ip(i int) *int       { return &i }
func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

func TestOps(t *testing.T) {
	s := setOf(t, map[string]string{"arch/orders.arch": sample})
	run := func(ops ...Op) []string {
		t.Helper()
		files, err := s.Apply(ops)
		if err != nil {
			t.Fatalf("%v: %v", ops, err)
		}
		return files
	}
	files := run(Op{Op: "add_transition", Entity: "OrderStatus", From: "accepted", To: "refunded", On: "refund", Label: "возврат"})
	if !reflect.DeepEqual(files, []string{"arch/orders.arch"}) {
		t.Fatalf("files %v", files)
	}
	if st := s.Model().States[0]; len(st.Transitions) != 4 || st.Transitions[3].To != "refunded" {
		t.Fatalf("%+v", st)
	}
	// add_state then a transition makes the explicit `state` line disappear
	run(Op{Op: "add_state", Entity: "OrderStatus", State: "paused"})
	if st := s.Model().States[0]; !reflect.DeepEqual(st.States, []string{"draft", "paused"}) {
		t.Fatalf("%v", st.States)
	}
	run(Op{Op: "add_transition", Entity: "OrderStatus", From: "paused", To: "accepted"})
	if st := s.Model().States[0]; !reflect.DeepEqual(st.States, []string{"draft"}) {
		t.Fatalf("%v", st.States)
	}
	run(Op{Op: "rename_state", Entity: "OrderStatus", From: "accepted", To: "paid"})
	st := s.Model().States[0]
	if st.Transitions[1].To != "paid" || st.Transitions[3].From != "paid" {
		t.Fatalf("%+v", st.Transitions)
	}
	run(Op{Op: "set_final", Entity: "OrderStatus", State: "refunded", Final: bp(true)})
	run(Op{Op: "set_final", Entity: "OrderStatus", State: "cancelled", Final: bp(false)})
	st = s.Model().States[0]
	if !reflect.DeepEqual(st.Final, []string{"created", "rejected", "refunded"}) {
		t.Fatalf("final %v", st.Final)
	}
	run(Op{Op: "remove_transition", Entity: "OrderStatus", From: "paid", To: "refunded"})
	run(Op{Op: "remove_state", Entity: "OrderStatus", State: "paused"})
	st = s.Model().States[0]
	for _, tr := range st.Transitions {
		if tr.From == "paused" || tr.To == "paused" || tr.To == "refunded" {
			t.Fatalf("left over: %+v", tr)
		}
	}

	// steps
	run(Op{Op: "add_step", Flow: "kaspi-pay", Ref: "Svc.check", Text: "проверка", Index: ip(0)})
	run(Op{Op: "add_step", Flow: "kaspi-pay", Plan: true, Text: "later"})
	fl := s.Model().Flows[0]
	if len(fl.Steps) != 5 || fl.Steps[0].Ref != "Svc.check" || !fl.Steps[4].Planned {
		t.Fatalf("%+v", fl.Steps)
	}
	run(Op{Op: "move_step", Flow: "kaspi-pay", Index: ip(0), ToIdx: ip(2)})
	if fl = s.Model().Flows[0]; fl.Steps[2].Ref != "Svc.check" || fl.Steps[0].Ref != "TabletsServer.RequestKaspiQRForOrder" {
		t.Fatalf("%+v", fl.Steps)
	}
	run(Op{Op: "set_step", Flow: "kaspi-pay", Index: ip(2), Ref: "Svc.check2", Ext: "Kaspi"})
	run(Op{Op: "remove_step", Flow: "kaspi-pay", Index: ip(4)})
	fl = s.Model().Flows[0]
	if len(fl.Steps) != 4 || fl.Steps[2].Ref != "Svc.check2" || fl.Steps[2].Ext != "Kaspi" {
		t.Fatalf("%+v", fl.Steps)
	}
	run(Op{Op: "add_alt", Flow: "kaspi-pay", Cond: "declined", Target: "retry"})
	run(Op{Op: "remove_alt", Flow: "kaspi-pay", Index: ip(0)})
	run(Op{Op: "set_actor", Flow: "kaspi-pay", Actor: sp("Касса")}, Op{Op: "set_exit", Flow: "kaspi-pay", Exit: sp("")})
	fl = s.Model().Flows[0]
	if len(fl.Alts) != 2 || fl.Alts[1].Cond != "declined" || fl.Actor != "Касса" || fl.Exit != "" {
		t.Fatalf("%+v", fl)
	}

	// rename flow reaches journeys and alts
	run(Op{Op: "rename_flow", Flow: "kaspi-pay", To: "pay-kaspi"})
	if j := s.Model().Journeys[0]; j.Flows[1] != "pay-kaspi" {
		t.Fatalf("%+v", j)
	}

	// new entities go to a file by id prefix, else main.arch
	if f := run(Op{Op: "add_flow", ID: "refund", Name: sp("Возврат")}); !reflect.DeepEqual(f, []string{"arch/main.arch"}) {
		t.Fatalf("%v", f)
	}
	if f := run(Op{Op: "add_flow", ID: "kaspi-refund"}); !reflect.DeepEqual(f, []string{"arch/kaspi.arch"}) {
		t.Fatalf("%v", f)
	}
	if f := run(Op{Op: "add_journey", ID: "guest-refund", Flows: []string{"refund"}}); !reflect.DeepEqual(f, []string{"arch/guest.arch"}) {
		t.Fatalf("%v", f)
	}
	run(Op{Op: "add_transition", Entity: "Ticket", From: "open", To: "closed"})
	if s.File("arch/main.arch") == nil || len(s.File("arch/main.arch").Doc.States) != 1 {
		t.Fatal("Ticket not in main.arch")
	}
	run(Op{Op: "set_journey_flows", Jrny: "guest-refund", Flows: []string{"refund", "kaspi-refund"}})
	run(Op{Op: "remove_journey", Jrny: "guest-refund"})
	run(Op{Op: "remove_flow", Flow: "kaspi-refund"})

	// everything written out parses back to the same documents
	for p, text := range s.Texts() {
		d := mustParse(t, text)
		if !sameDoc(d, s.File(p).Doc) {
			t.Fatalf("%s does not round trip:\n%s", p, text)
		}
	}
}

func TestOpsErrorsKeepSet(t *testing.T) {
	s := setOf(t, map[string]string{"arch/orders.arch": sample})
	before := s.Texts()
	bads := []Op{
		{Op: "nope"},
		{Op: "add_transition", Entity: "OrderStatus", From: "validated", To: "payment_requested", On: "kaspi-pay"}, // duplicate
		{Op: "add_transition", Entity: "Order Status", From: "a", To: "b"},
		{Op: "add_transition", Entity: "X", From: "a b", To: "b"},
		{Op: "add_step", Flow: "nope", Ref: "x"},
		{Op: "add_step", Flow: "kaspi-pay", Ref: "x y"},
		{Op: "add_step", Flow: "kaspi-pay", Plan: true},
		{Op: "remove_step", Flow: "kaspi-pay", Index: ip(9)},
		{Op: "move_step", Flow: "kaspi-pay", Index: ip(0), ToIdx: ip(9)},
		{Op: "add_alt", Flow: "kaspi-pay", Cond: `a "b`},
		{Op: "add_flow", ID: "kaspi-pay"},
		{Op: "add_flow", ID: "x", File: "../etc/passwd"},
		{Op: "add_flow", ID: "x", File: "arch/sub/x.arch"},
		{Op: "rename_state", Entity: "OrderStatus", From: "validated", To: "accepted"},
		{Op: "remove_state", Entity: "OrderStatus", State: "ghost"},
		{Op: "add_journey", ID: "empty"},
	}
	for _, op := range bads {
		if _, err := s.Apply([]Op{op}); err == nil {
			t.Errorf("%+v: no error", op)
		}
	}
	// a batch is all or nothing
	_, err := s.Apply([]Op{{Op: "add_flow", ID: "okay"}, {Op: "remove_flow", Flow: "missing"}})
	if err == nil || !strings.Contains(err.Error(), "op 2") {
		t.Fatalf("batch: %v", err)
	}
	if !reflect.DeepEqual(before, s.Texts()) {
		t.Fatal("set changed after failed ops")
	}
	// ops are refused while the spec has syntax errors
	bad := FromTexts(map[string]string{"arch/b.arch": "flow\n"})
	if _, err := bad.Apply([]Op{{Op: "add_flow", ID: "x"}}); err == nil {
		t.Fatal("op accepted on a broken spec")
	}
}

func TestReplaceAndPath(t *testing.T) {
	s := setOf(t, map[string]string{"arch/a.arch": "flow a\n"})
	if errs := s.Replace("arch/a.arch", "flow\n"); len(errs) == 0 || s.File("arch/a.arch").Doc.Flows[0].ID != "a" {
		t.Fatalf("bad text must not replace: %v", errs)
	}
	if errs := s.Replace("arch/b.arch", "flow b\n"); len(errs) > 0 || len(s.Files) != 2 {
		t.Fatalf("%v", errs)
	}
	for _, p := range []string{"a.arch", "arch/../x.arch", "arch/x.txt", "arch/sub/x.arch", "/arch/x.arch", "arch/.arch", "arch//x.arch"} {
		if ValidPath(p) {
			t.Errorf("%q must be invalid", p)
		}
	}
	if !ValidPath("arch/orders-2.arch") {
		t.Error("orders-2.arch must be valid")
	}
}

func fn(file, name, cls string, callees ...string) facts.Function {
	f := facts.Function{Name: name, File: file, Start: 10, End: 20, Callees: callees}
	if cls != "" {
		c := cls
		f.Cls = &c
	}
	return f
}

func testFacts() *facts.Facts {
	f := &facts.Facts{Functions: map[string]facts.Function{
		"srv/tablets.go::TabletsServer.RequestKaspiQRForOrder": fn("srv/tablets.go", "RequestKaspiQRForOrder", "TabletsServer", "pay/svc.go::Service.requestQR"),
		"pay/svc.go::Service.requestQR":                        fn("pay/svc.go", "requestQR", "Service"),
		"pay/svc.go::Service.other":                            fn("pay/svc.go", "other", "Service"),
		"pay/other.go::Other.other":                            fn("pay/other.go", "other", "Other"),
		"util/util.go::helper":                                 fn("util/util.go", "helper", ""),
	}}
	f.States = map[string]*facts.StateMachine{"OrderStatus": {
		States: []string{"validated", "payment_requested", "accepted", "cancelled"},
		Transitions: []facts.Transition{
			{From: "validated", To: "payment_requested", Fid: "pay/svc.go::Service.requestQR", Source: "code"},
			{From: "payment_requested", To: "accepted", Fid: "pay/svc.go::Service.other", Source: "code"},
			{From: "accepted", To: "shipped", Fid: "pay/svc.go::Service.other", Source: "annotation"},
			{From: "", To: "cancelled", Fid: "pay/svc.go::Service.other", Source: "code"},
		},
	}}
	return f
}

func reachFrom(f *facts.Facts) func(string) map[string]bool {
	return func(entry string) map[string]bool {
		seen := map[string]bool{entry: true}
		var dfs func(string)
		dfs = func(x string) {
			for _, c := range f.Functions[x].Callees {
				if !seen[c] {
					seen[c] = true
					dfs(c)
				}
			}
		}
		dfs(entry)
		return seen
	}
}

func TestResolve(t *testing.T) {
	f := testFacts()
	ok := map[string]string{
		"pay/svc.go::Service.requestQR":        "pay/svc.go::Service.requestQR",
		"svc.go::Service.requestQR":            "pay/svc.go::Service.requestQR",
		"Service.requestQR":                    "pay/svc.go::Service.requestQR",
		"requestQR":                            "pay/svc.go::Service.requestQR",
		"helper":                               "util/util.go::helper",
		"TabletsServer.RequestKaspiQRForOrder": "srv/tablets.go::TabletsServer.RequestKaspiQRForOrder",
		"util.go::helper":                      "util/util.go::helper",
	}
	for ref, want := range ok {
		if got, c := Resolve(f, ref); got != want {
			t.Errorf("%s: got %q %v, want %q", ref, got, c, want)
		}
	}
	if got, c := Resolve(f, "other"); got != "" || len(c) != 2 {
		t.Errorf("ambiguous: %q %v", got, c)
	}
	if got, c := Resolve(f, "Service.other"); got != "pay/svc.go::Service.other" || c != nil {
		t.Errorf("exact beats suffix: %q %v", got, c)
	}
	if got, c := Resolve(f, "nothing"); got != "" || c != nil {
		t.Errorf("missing: %q %v", got, c)
	}
}

func codes(fs []Finding) map[string][]Finding {
	m := map[string][]Finding{}
	for _, x := range fs {
		m[x.Code] = append(m[x.Code], x)
	}
	return m
}

func TestCheck(t *testing.T) {
	f := testFacts()
	s := setOf(t, map[string]string{"arch/orders.arch": `states OrderStatus
  validated -> payment_requested
  payment_requested -> accepted
  payment_requested -> payment_failed   on payment.failed
  * -> cancelled
  final cancelled

flow kaspi-pay
  step TabletsServer.RequestKaspiQRForOrder
  step Service.requestQR   ext Kaspi
  step util.go::helper
  step plan "Проверить лимит"
  step Service.nothing
  step other
  state OrderStatus: validated -> bogus
  alt declined -> no-such-flow

journey j1: kaspi-pay > ghost
`})
	m, fs := Check(s, f, reachFrom(f))
	by := codes(fs)

	if n := len(by[CodeNotImpl]); n != 1 || !strings.Contains(by[CodeNotImpl][0].Msg, "payment_requested -> payment_failed") || by[CodeNotImpl][0].Line != 4 || by[CodeNotImpl][0].Severity != SevWarning {
		t.Errorf("not implemented: %+v", by[CodeNotImpl])
	}
	if n := len(by[CodeUndeclared]); n != 1 || !strings.Contains(by[CodeUndeclared][0].Msg, "accepted -> shipped") || by[CodeUndeclared][0].File != "pay/svc.go" || by[CodeUndeclared][0].Severity != SevError {
		t.Errorf("undeclared: %+v", by[CodeUndeclared])
	}
	if len(by[CodeNoFunc]) != 1 || by[CodeNoFunc][0].Line != 13 || len(by[CodeAmbiguous]) != 1 || !strings.Contains(by[CodeAmbiguous][0].Msg, "pay/other.go::Other.other") {
		t.Errorf("funcs: %+v %+v", by[CodeNoFunc], by[CodeAmbiguous])
	}
	if u := by[CodeUnreachable]; len(u) != 1 || u[0].Line != 11 || u[0].Severity != SevWarning {
		t.Errorf("unreachable: %+v", u)
	}
	if p := by[CodePlanned]; len(p) != 1 || p[0].Severity != SevInfo {
		t.Errorf("planned: %+v", p)
	}
	if n := by[CodeNoFlow]; len(n) != 2 {
		t.Errorf("flows: %+v", n)
	}
	if len(by[CodeStateRef]) != 1 {
		t.Errorf("state ref: %+v", by[CodeStateRef])
	}

	// enriched model
	st := m.States[0]
	if st.Code != "OrderStatus" {
		t.Errorf("code key %q", st.Code)
	}
	var drift []string
	for _, tr := range st.Transitions {
		drift = append(drift, tr.From+">"+tr.To+":"+tr.Source+":"+tr.Drift)
	}
	want := []string{"validated>payment_requested:spec:", "payment_requested>accepted:spec:", "payment_requested>payment_failed:spec:not_implemented", "*>cancelled:spec:", "accepted>shipped:annotation:undeclared"}
	if !reflect.DeepEqual(drift, want) {
		t.Errorf("drift\n got %v\nwant %v", drift, want)
	}
	fl := m.Flows[0]
	if fl.Planned != 1 || fl.Steps[0].Fid == "" || fl.Steps[3].Fid != "" || fl.Steps[4].Drift != DriftMissing || fl.Steps[5].Drift != DriftAmbiguous || fl.Steps[2].Drift != DriftUnreachable {
		t.Errorf("flow %+v", fl.Steps)
	}
	if j := m.Journeys[0]; !reflect.DeepEqual(j.Missing, []string{"ghost"}) {
		t.Errorf("journey %+v", j)
	}
}

func TestCheckNoSpecIsQuiet(t *testing.T) {
	m, fs := Check(&Set{}, testFacts(), nil)
	if len(fs) != 0 || len(m.States) != 0 {
		t.Fatalf("%v %v", m, fs)
	}
	m, fs = Check(nil, nil, nil)
	if len(fs) != 0 || m.Flows == nil {
		t.Fatalf("nil set: %+v %v", m, fs)
	}
}

func TestCheckDuplicatesAndSpecFlows(t *testing.T) {
	s := setOf(t, map[string]string{
		"arch/a.arch": "flow x\nflow x\njourney j: x > y\nstates S\n",
		"arch/b.arch": "flow y\n  alt c -> x\nstates S\n",
	})
	_, fs := Check(s, nil, nil)
	by := codes(fs)
	if len(by[CodeDuplicate]) != 2 { // flow x, states S
		t.Fatalf("duplicates: %+v", by[CodeDuplicate])
	}
	if len(by[CodeNoFlow]) != 0 {
		t.Fatalf("spec flows are known: %+v", by[CodeNoFlow])
	}
}

func TestCheckSyntaxErrors(t *testing.T) {
	s := FromTexts(map[string]string{"arch/a.arch": "flow a\n  wat\n"})
	_, fs := Check(s, nil, nil)
	if len(fs) != 1 || fs[0].Code != CodeSyntax || fs[0].Line != 2 || fs[0].File != "arch/a.arch" {
		t.Fatalf("%+v", fs)
	}
}

func TestMergeStates(t *testing.T) {
	f := testFacts()
	s := setOf(t, map[string]string{"arch/o.arch": `states OrderStatus
  validated -> payment_requested   on kaspi-pay   "QR"
  payment_requested -> payment_failed
  * -> refunded
  final cancelled, refunded
states Brand
  a -> b
`})
	g := MergeStates(f, s)
	if len(f.States["OrderStatus"].Transitions) != 4 {
		t.Fatal("input facts were modified")
	}
	sm := g.States["OrderStatus"]
	var got []string
	for _, tr := range sm.Transitions {
		got = append(got, tr.From+">"+tr.To+":"+tr.Source+":"+tr.Drift+":"+tr.On)
	}
	has := func(x string) bool {
		for _, y := range got {
			if y == x {
				return true
			}
		}
		return false
	}
	for _, w := range []string{
		"validated>payment_requested:code::kaspi-pay",
		"payment_requested>payment_failed:spec:not_implemented:",
		"accepted>refunded:spec:not_implemented:",
		"validated>refunded:spec:not_implemented:",
		"payment_requested>accepted:code:undeclared:",
	} {
		if !has(w) {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	for _, x := range got {
		if strings.HasPrefix(x, "cancelled>") || strings.HasPrefix(x, "refunded>refunded") {
			t.Errorf("final states do not move on: %s", x)
		}
	}
	if sm.Transitions[0].Label != "QR" {
		t.Errorf("label: %+v", sm.Transitions[0])
	}
	if b := g.States["Brand"]; b == nil || len(b.Transitions) != 1 || b.Transitions[0].Source != "spec" {
		t.Errorf("spec-only machine: %+v", b)
	}
	if MergeStates(f, &Set{}) != f {
		t.Error("no spec must return the same facts")
	}
}
