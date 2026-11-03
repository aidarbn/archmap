package autosem

import (
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

// shop builds a small system: two plain CRUD handlers (entry -> service ->
// repository) and a checkout that reaches a payment client and queues a job
// handled by a worker which talks to a delivery client.
func shop() *facts.Facts {
	f := &facts.Facts{Repo: "shop", Modules: map[string]facts.Module{}, Functions: map[string]facts.Function{}}
	id := func(mod, name string) string { return mod + "/f.go::" + name }
	add := func(mod, name string, callees ...string) {
		f.Modules[mod] = facts.Module{}
		f.Functions[id(mod, name)] = facts.Function{Name: name, Start: 1, End: 30, Module: mod, File: mod + "/f.go", Lang: "go", Callees: callees}
	}
	entry := func(mod, name, kind string, callees ...string) {
		add(mod, name, callees...)
		fn := f.Functions[id(mod, name)]
		fn.Entry = &facts.Entry{Kind: kind, Labels: []string{name}}
		f.Functions[id(mod, name)] = fn
	}
	const (
		repo    = "internal/adapters/out/db/repository"
		pay     = "internal/adapters/out/stripe"
		ship    = "internal/adapters/out/courier"
		catalog = "internal/services/catalog"
		orders  = "internal/services/order"
		http    = "internal/adapters/in/http"
		workers = "internal/adapters/queue/workers"
		queue   = "internal/adapters/queue"
	)
	add(repo, "ListItems", id(repo, "query"))
	add(repo, "query")
	add(repo, "SaveOrder", id(repo, "exec"))
	add(repo, "exec")
	add(pay, "Charge", id(pay, "post"))
	add(pay, "post")
	add(ship, "Dispatch", id(ship, "post"))
	add(ship, "post")
	add(catalog, "List", id(repo, "ListItems"))
	add(catalog, "Get", id(repo, "ListItems"))
	add(orders, "Place", id(repo, "SaveOrder"), id(pay, "Charge"), id(orders, "enqueue"))
	add(orders, "enqueue", id(queue, "ScheduleDelivery"))
	add(queue, "ScheduleDelivery")
	fn := f.Functions[id(queue, "ScheduleDelivery")]
	fn.AsyncCallees = []string{id(workers, "DeliveryWorker.Work")}
	f.Functions[id(queue, "ScheduleDelivery")] = fn
	entry(workers, "DeliveryWorker.Work", "task", id(orders, "deliver"))
	add(orders, "deliver", id(ship, "Dispatch"), id(repo, "SaveOrder"))
	entry(http, "ListCatalog", "http", id(catalog, "List"))
	entry(http, "GetCatalogItem", "http", id(catalog, "Get"))
	entry(http, "Checkout", "http", id(orders, "Place"))
	return f
}

func flowByName(m Model, name string) (int, Flow) {
	for i, fl := range m.Flows {
		if fl.Name == name {
			return i, fl
		}
	}
	return -1, Flow{}
}

func TestTopPrefersBusinessFlowsOverCRUD(t *testing.T) {
	m := Build(shop())
	if len(m.Flows) == 0 || !m.Flows[0].Top || m.Flows[0].Name != "Checkout" {
		var names []string
		for _, fl := range m.Flows {
			names = append(names, fl.Name)
		}
		t.Fatalf("Checkout must be the first scenario, got %v", names)
	}
	ci, _ := flowByName(m, "ListCatalog")
	ki, _ := flowByName(m, "Checkout")
	if ki >= ci {
		t.Errorf("checkout (%d) must precede the CRUD list (%d)", ki, ci)
	}
}

func TestReachCountsExternalsAndQueues(t *testing.T) {
	f := shop()
	m := Build(f)
	layer := map[string]string{}
	for k, v := range m.Modules {
		layer[k] = v.Layer
	}
	g := newGraph(f, layer)
	co := g.reach("internal/adapters/in/http/f.go::Checkout")
	if len(co.ext) != 2 || co.async != 1 {
		t.Errorf("checkout reach: ext=%d async=%d, want 2 and 1 (payment and, through the queue, the courier)", len(co.ext), co.async)
	}
	if g.crud(co) {
		t.Error("checkout is not CRUD")
	}
	if !g.crud(g.reach("internal/adapters/in/http/f.go::ListCatalog")) {
		t.Error("ListCatalog (handler -> service -> repository) must be CRUD")
	}
}

func TestFlowCrossesQueue(t *testing.T) {
	m := Build(shop())
	_, fl := flowByName(m, "Checkout")
	joined := strings.Join(fl.Steps, " ")
	for _, want := range []string{"ScheduleDelivery", "DeliveryWorker.Work"} {
		if !strings.Contains(joined, want) {
			t.Errorf("checkout scenario lacks %s: %v", want, fl.Steps)
		}
	}
	// the worker follows the enqueuing function
	var iq, iw = -1, -1
	for i, s := range fl.Steps {
		switch {
		case strings.HasSuffix(s, "::ScheduleDelivery"):
			iq = i
		case strings.HasSuffix(s, "::DeliveryWorker.Work"):
			iw = i
		}
	}
	if iq < 0 || iw != iq+1 {
		t.Errorf("worker must directly follow the enqueuing step: %v", fl.Steps)
	}
}

func TestMainKeepsNoSeatWhenBusinessEntriesExist(t *testing.T) {
	f := shop()
	const mod = "cmd/app"
	f.Modules[mod] = facts.Module{}
	wire := []string{}
	for fid, fn := range f.Functions {
		if fn.Entry == nil {
			wire = append(wire, fid) // main "constructs" everything
		}
	}
	f.Functions[mod+"/main.go::main"] = facts.Function{Name: "main", Start: 1, End: 40, Module: mod, File: mod + "/main.go", Lang: "go",
		Callees: wire, Entry: &facts.Entry{Kind: "main", Labels: []string{"main"}}}
	m := Build(f)
	if m.Flows[0].Name == "main" {
		t.Error("main() wiring must not outrank business scenarios")
	}
}

func TestRustCrateLayers(t *testing.T) {
	cases := []struct {
		mod   string
		stats *moduleStats
		want  string
	}{
		{"crates/printer/src", &moduleStats{funcs: 9, lang: "rust"}, Domain},
		{"crates/cli/src", &moduleStats{funcs: 9, lang: "rust"}, Domain}, // the crate name "cli" is not a layer
		{"crates/ignore/src/walk", &moduleStats{funcs: 9, lang: "rust"}, Domain},
		{"crates/app/src", &moduleStats{funcs: 9, hasMain: true, entries: 1, lang: "rust"}, Entry},
		{"crates/globset/benches", &moduleStats{funcs: 3, lang: "rust"}, Test},
		{"crates/x/src/tests", &moduleStats{funcs: 3, lang: "rust"}, Test},
		{"crates/net/src/client", &moduleStats{funcs: 3, lang: "rust"}, Adapter},
	}
	for _, c := range cases {
		if got := classify(c.mod, c.stats); got != c.want {
			t.Errorf("classify(%q) = %s, want %s", c.mod, got, c.want)
		}
	}
}
