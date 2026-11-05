package autosem

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func strp(s string) *string { return &s }

func TestTechnical(t *testing.T) {
	type tc struct {
		name      string
		fn        facts.Function
		flow      Flow
		inJourney bool
		want      bool
	}
	entry := func(kind string, labels ...string) *facts.Entry { return &facts.Entry{Kind: kind, Labels: labels} }
	two := []string{"e", "x"}
	cases := []tc{
		{"health handler", facts.Function{Name: "Health", Entry: entry("http"), Doc: "Health check"}, Flow{Kind: KindDeclared, Steps: two}, false, true},
		{"version alone", facts.Function{Name: "GetVersion", Entry: entry("http"), Doc: "v"}, Flow{Kind: KindDeclared, Steps: two}, false, true},
		{"state version is business", facts.Function{Name: "GetStateVersion", Entry: entry("grpc"), Doc: "versions of the tablet state"}, Flow{Kind: KindDeclared, Steps: two}, false, false},
		{"doc route", facts.Function{Name: "serveSpec", Entry: entry("http", "GET /doc"), Doc: "openapi page"}, Flow{Kind: "http", Steps: two, Top: true}, false, true},
		{"root catch-all", facts.Function{Name: "index", Entry: entry("http", "GET /{$}"), Doc: "index"}, Flow{Kind: "http", Steps: two, Top: true}, false, true},
		{"metrics", facts.Function{Name: "Handler", Cls: strp("MetricsServer"), Entry: entry("http"), Doc: "x"}, Flow{Kind: "http", Steps: two, Top: true}, false, true},
		{"cleanup worker", facts.Function{Name: "Work", Cls: strp("CleanupTestRestaurantsWorker"), Entry: entry("task"), Doc: "deletes"}, Flow{Kind: KindDeclared, Steps: two}, false, true},
		{"bootstrap by flow id", facts.Function{Name: "Run", Entry: entry("task"), Doc: "x"}, Flow{ID: "keycloak-bootstrap", Kind: KindDeclared, Steps: two}, false, true},
		{"technical but on a through path", facts.Function{Name: "Health", Entry: entry("http"), Doc: "x"}, Flow{Kind: KindDeclared, Steps: two}, true, false},
		{"one step without a description", facts.Function{Name: "ListLanguages", Entry: entry("grpc")}, Flow{Kind: KindDeclared, Steps: []string{"e"}}, false, true},
		{"one step with a description", facts.Function{Name: "GetProfile", Entry: entry("grpc"), Doc: "the tablet profile"}, Flow{Kind: KindDeclared, Steps: []string{"e"}}, false, false},
		{"one step with an exit", facts.Function{Name: "ListLanguages", Entry: entry("grpc")}, Flow{Kind: KindDeclared, Exit: "list shown", Steps: []string{"e"}}, false, false},
		{"auto without a description", facts.Function{Name: "CreateOrder", Entry: entry("http")}, Flow{Kind: "http", Steps: two}, false, true},
		{"auto without a description but top", facts.Function{Name: "CreateOrder", Entry: entry("http")}, Flow{Kind: "http", Steps: two, Top: true}, false, false},
		{"auto with a description", facts.Function{Name: "CreateOrder", Entry: entry("http"), Doc: "creates an order"}, Flow{Kind: "http", Steps: two}, false, false},
		{"declared business flow", facts.Function{Name: "RequestKaspiQRForOrder", Entry: entry("grpc"), Doc: "QR"}, Flow{ID: "kaspi-pay", Kind: KindDeclared, Steps: two}, false, false},
		{"latest is not test", facts.Function{Name: "GetLatestOrders", Entry: entry("http"), Doc: "x"}, Flow{Kind: "http", Steps: two}, false, false},
	}
	for _, c := range cases {
		f := &facts.Facts{Functions: map[string]facts.Function{"e": c.fn, "x": {Name: "x"}}}
		if got := Technical(c.flow, f, c.inJourney); got != c.want {
			t.Errorf("%s: Technical = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMinorSteps(t *testing.T) {
	fn := func(mod, name string, mut func(*facts.Function)) facts.Function {
		f := facts.Function{Name: name, Module: mod, File: mod + "/x.go", Start: 1, End: 20}
		if mut != nil {
			mut(&f)
		}
		return f
	}
	f := &facts.Facts{
		Functions: map[string]facts.Function{
			"h":      fn("internal/adapters/in/http", "CreateOrder", func(f *facts.Function) { f.Entry = &facts.Entry{Kind: "http"} }),
			"svc":    fn("internal/services/order", "Create", nil),
			"same":   fn("internal/services/order", "validate", nil),
			"err":    fn("internal/domain", "NewValidationError", nil),
			"log":    fn("internal/services/order", "logf", nil),
			"conv":   fn("internal/services/order", "ToProto", nil),
			"strut":  fn("pkg/strutil", "Trim", nil),
			"getter": fn("internal/services/order", "GetID", func(f *facts.Function) { f.End = 3 }),
			"repo":   fn("internal/adapters/out/db", "Insert", func(f *facts.Function) { f.Data = []facts.Access{{Table: "orders", Op: "insert"}} }),
			"state":  fn("internal/services/order", "markPaid", nil),
			"arch":   fn("internal/services/order", "audit", func(f *facts.Function) { f.Arch = &facts.ArchNote{Ext: []string{"Kaspi"}} }),
			"enq":    fn("internal/services/order", "schedule", func(f *facts.Function) { f.AsyncCallees = []string{"wrk"} }),
			"wrk":    fn("internal/adapters/dbqueue/workers", "Work", nil),
			"wrk2":   fn("internal/adapters/dbqueue/workers", "next", nil),
		},
		States: map[string]*facts.StateMachine{"Order": {Transitions: []facts.Transition{{To: "paid", Fid: "state"}}}},
	}
	layer := map[string]string{}
	for _, fn := range f.Functions {
		layer[fn.Module] = classify(fn.Module, nil)
	}
	g := newGraph(f, layer)
	cases := []struct {
		name string
		flow Flow
		want []string
	}{
		{"helpers and same-module calls fold", Flow{Steps: []string{"h", "svc", "same", "err", "log", "conv", "strut", "getter"}},
			[]string{"same", "err", "log", "conv", "strut", "getter"}},
		{"data, state, markup, queue stay", Flow{Steps: []string{"h", "svc", "repo", "state", "arch", "enq", "wrk", "wrk2"}},
			[]string{"wrk2"}},
		{"a label keeps a step", Flow{Steps: []string{"h", "svc", "same"}, Labels: map[string]string{"same": "checks the order"}}, nil},
		{"declared steps are all kept", Flow{Kind: KindDeclared, Steps: []string{"h", "svc", "same", "err"}}, nil},
		{"back in a module after a hop is a move", Flow{Steps: []string{"h", "svc", "repo", "same"}}, nil},
	}
	for _, c := range cases {
		if got := g.minorSteps(c.flow); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: minor = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTrimFlowsFoldsDeadEndBranches(t *testing.T) {
	f := &facts.Facts{Functions: map[string]facts.Function{
		"a": {Name: "Pay", Module: "m", Entry: &facts.Entry{Kind: "http"}, Doc: "pay"},
		"b": {Name: "Check", Module: "m"},
	}}
	g := newGraph(f, map[string]string{"m": Domain})
	flows := []Flow{{ID: "pay", Kind: KindDeclared, Steps: []string{"a", "b"}, Branches: []Branch{
		{At: "b", Cond: "paid", Flow: "send"},
		{At: "b", Cond: "QR expired", Text: "payment failed"},
		{At: "a", Cond: "bad status", Text: "rejected"},
	}}}
	g.trimFlows(flows, nil)
	var fold []bool
	for _, b := range flows[0].Branches {
		fold = append(fold, b.Fold)
	}
	if !reflect.DeepEqual(fold, []bool{false, true, true}) {
		t.Errorf("fold = %v, want [false true true]", fold)
	}
	if flows[0].Hidden {
		t.Error("a declared business flow must stay visible")
	}
}
