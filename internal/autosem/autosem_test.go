package autosem

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestPathLayer(t *testing.T) {
	cases := []struct{ mod, lang, want string }{
		{"cmd/app", "go", Entry},
		{"internal/adapters/in/grpc", "go", Entry},
		{"internal/adapters/out/kaspi", "go", Adapter},
		{"internal/adapters/out/db/repository", "go", Adapter},
		{"internal/services/order", "go", Domain},
		{"internal/usecase", "go", Domain},
		{"internal/domain", "go", Domain},
		{"pkg/ptr", "go", Infra},
		{"internal/config", "go", Infra},
		{"internal", "go", Infra},
		{"internal/testutil", "go", Test},
		{"tests/fixtures", "python", Test},
		{"app/views", "python", Entry},
		{"src/views", "js", UI},
		{"src/components/button", "js", UI},
		{"src/api", "js", Adapter},
		{"src/hooks", "js", Domain},
		{"crates/foo/benches", "rust", Test},
		{"internal/orders", "go", Other},
		{"clients/payments", "python", Adapter},
	}
	for _, c := range cases {
		if got := pathLayer(c.mod, c.lang); got != c.want {
			t.Errorf("pathLayer(%q,%q) = %s, want %s", c.mod, c.lang, got, c.want)
		}
	}
}

func TestClassifyByContent(t *testing.T) {
	if got := classify("foo/bar", &moduleStats{funcs: 4, entries: 3}); got != Entry {
		t.Errorf("entry-dominated unknown module = %s", got)
	}
	if got := classify("foo/bar", &moduleStats{funcs: 4, uiEntries: 3}); got != UI {
		t.Errorf("ui-entry-dominated unknown module = %s", got)
	}
	if got := classify("foo/bar", &moduleStats{funcs: 40, hasMain: true, entries: 1}); got != Entry {
		t.Errorf("module with main = %s", got)
	}
	if got := classify("services/x", &moduleStats{funcs: 4, entries: 4}); got != Domain {
		t.Errorf("path wins over content for known layers, got %s", got)
	}
	if got := classify("tests/x", &moduleStats{funcs: 4, hasMain: true}); got != Test {
		t.Errorf("test path must stay test, got %s", got)
	}
}

func TestRoleOf(t *testing.T) {
	cases := []struct{ mod, layer, want string }{
		{"internal/adapters/in/grpc", Entry, "gRPC"},
		{"internal/adapters/in/http", Entry, "HTTP"},
		{"internal/adapters/out/kaspi", Adapter, "Kaspi client"},
		{"internal/adapters/out/db/repository", Adapter, "Repository"},
		{"internal/services/file_service", Domain, "File service"},
		{"src/components/cardStats", UI, "Card stats"},
		{"pkg/db", Infra, "DB"},
		{"internal/api", Entry, "API"},
		{"crates/globset/src", Other, "Globset"},
		{"clients/payments", Adapter, "Payments client"},
		{".", Other, "Project root"},
	}
	for _, c := range cases {
		if got := roleOf(c.mod, c.layer); got != c.want {
			t.Errorf("roleOf(%q) = %q, want %q", c.mod, got, c.want)
		}
	}
}

func TestHumanize(t *testing.T) {
	for in, want := range map[string]string{
		"http_server":  "HTTP server",
		"orderItems":   "Order items",
		"card-stats":   "Card stats",
		"HTTPServer":   "HTTP server",
		"user_api_key": "User API key",
	} {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsHelper(t *testing.T) {
	cls := "Logger"
	for _, c := range []struct {
		fn   facts.Function
		want bool
	}{
		{facts.Function{Name: "NewNotFoundError", Module: "internal/x", File: "internal/x/a.go"}, true},
		{facts.Function{Name: "ptr", Module: "internal/x", File: "internal/x/a.go"}, true},
		{facts.Function{Name: "toProto", Module: "internal/x", File: "internal/x/a.go"}, true},
		{facts.Function{Name: "orderFromSqlc", Module: "internal/x", File: "internal/x/a.go"}, true},
		{facts.Function{Name: "Info", Cls: &cls, Module: "internal/x", File: "internal/x/a.go"}, true},
		{facts.Function{Name: "Wrap", Module: "pkg/errors", File: "pkg/errors/a.go"}, true},
		{facts.Function{Name: "Process", Module: "internal/x", File: "internal/x/errors.go"}, true},
		{facts.Function{Name: "PlaceOrder", Module: "internal/x", File: "internal/x/service.go"}, false},
		{facts.Function{Name: "validateOrderItems", Module: "internal/x", File: "internal/x/service_helpers.go"}, false},
	} {
		if got := isHelper(c.fn); got != c.want {
			t.Errorf("isHelper(%s) = %v, want %v", c.fn.Name, got, c.want)
		}
	}
}

// synthetic graph: handler -> service -> {helper, validate(->repo), save(repo), client(external)}
func synth() *facts.Facts {
	f := &facts.Facts{Repo: "demo", Modules: map[string]facts.Module{}, Functions: map[string]facts.Function{}}
	add := func(mod, name string, callees ...string) string {
		fid := mod + "/f.go::" + name
		cls := (*string)(nil)
		f.Modules[mod] = facts.Module{}
		f.Functions[fid] = facts.Function{Name: name, Cls: cls, Start: 1, End: 30, Module: mod, File: mod + "/f.go", Lang: "go", Callees: callees}
		return fid
	}
	id := func(mod, name string) string { return mod + "/f.go::" + name }
	add("internal/adapters/out/db/repository", "Insert", id("internal/adapters/out/db/repository", "exec"))
	add("internal/adapters/out/db/repository", "exec")
	add("internal/adapters/out/stripe", "Charge")
	add("internal/services/order", "newOrderError")
	add("internal/services/order", "validate", id("internal/services/order", "rules"))
	add("internal/services/order", "rules")
	add("internal/services/order", "Place",
		id("internal/services/order", "newOrderError"),
		id("internal/services/order", "validate"),
		id("internal/adapters/out/db/repository", "Insert"),
		id("internal/adapters/out/stripe", "Charge"))
	add("internal/adapters/in/http", "Create", id("internal/services/order", "Place"))
	add("internal/adapters/in/http", "Ping")
	e := f.Functions[id("internal/adapters/in/http", "Create")]
	e.Entry = &facts.Entry{Kind: "http", Labels: []string{"POST /orders"}}
	e.Doc = "Create places an order."
	f.Functions[id("internal/adapters/in/http", "Create")] = e
	p := f.Functions[id("internal/adapters/in/http", "Ping")]
	p.Entry = &facts.Entry{Kind: "http", Labels: []string{"GET /ping"}}
	f.Functions[id("internal/adapters/in/http", "Ping")] = p
	return f
}

func TestWalkPicksMeaningfulSteps(t *testing.T) {
	m := Build(synth())
	var fl *Flow
	for i := range m.Flows {
		if m.Flows[i].Name == "POST /orders" {
			fl = &m.Flows[i]
		}
	}
	if fl == nil {
		t.Fatalf("flow missing: %+v", m.Flows)
	}
	got := strings.Join(fl.Steps, " ")
	for _, want := range []string{"http/f.go::Create", "order/f.go::Place", "order/f.go::validate", "stripe/f.go::Charge", "repository/f.go::Insert"} {
		if !strings.Contains(got, want) {
			t.Errorf("steps lack %s: %v", want, fl.Steps)
		}
	}
	if strings.Contains(got, "newOrderError") {
		t.Errorf("error constructor must not be a step: %v", fl.Steps)
	}
	if fl.Steps[0] != "internal/adapters/in/http/f.go::Create" || fl.Steps[1] != "internal/services/order/f.go::Place" {
		t.Errorf("order of steps: %v", fl.Steps)
	}
	// the external adapter ends its branch: nothing below Charge
	if fl.Kind != "http" || fl.Desc != "Create places an order." {
		t.Errorf("kind/desc = %q/%q", fl.Kind, fl.Desc)
	}
}

func TestWalkLimitsAndNoRepeats(t *testing.T) {
	f := synth()
	// a long cyclic chain must stop at MaxSteps and never repeat a function
	prev := ""
	for i := 0; i < 20; i++ {
		fid := "internal/services/order/f.go::c" + string(rune('a'+i))
		fn := facts.Function{Name: "c" + string(rune('a'+i)), Start: 1, End: 30, Module: "internal/services/order", File: "internal/services/order/f.go", Lang: "go"}
		f.Functions[fid] = fn
		if prev != "" {
			p := f.Functions[prev]
			p.Callees = append(p.Callees, fid)
			f.Functions[prev] = p
		}
		prev = fid
	}
	last := f.Functions[prev]
	last.Callees = []string{"internal/services/order/f.go::ca"}
	f.Functions[prev] = last
	h := f.Functions["internal/adapters/in/http/f.go::Ping"]
	h.Callees = []string{"internal/services/order/f.go::ca"}
	f.Functions["internal/adapters/in/http/f.go::Ping"] = h
	m := Build(f)
	for _, fl := range m.Flows {
		if len(fl.Steps) > MaxSteps {
			t.Errorf("%s has %d steps", fl.Name, len(fl.Steps))
		}
		seen := map[string]bool{}
		for _, s := range fl.Steps {
			if seen[s] {
				t.Errorf("%s repeats %s", fl.Name, s)
			}
			seen[s] = true
		}
	}
}

func TestTopFlows(t *testing.T) {
	m := Build(synth())
	if len(m.Flows) != 2 {
		t.Fatalf("every entry point must get a flow, got %d", len(m.Flows))
	}
	if !m.Flows[0].Top || m.Flows[0].Name != "POST /orders" {
		t.Errorf("deep flow must come first and be top: %+v", m.Flows[0])
	}
	if m.Flows[1].Top || len(m.Flows[1].Steps) != 1 {
		t.Errorf("short flow is generated but not top: %+v", m.Flows[1])
	}
	if want := "http: path through internal/adapters/in/http"; m.Flows[1].Desc != want {
		t.Errorf("fallback desc = %q, want %q", m.Flows[1].Desc, want)
	}
}

func TestTopCapAndKinds(t *testing.T) {
	f := synth()
	// 12 more deep http entries and one task entry: top is capped, the task kind is represented
	for i := 0; i < 12; i++ {
		name := "H" + string(rune('a'+i))
		fid := "internal/adapters/in/http/f.go::" + name
		f.Functions[fid] = facts.Function{Name: name, Start: 1, End: 30, Module: "internal/adapters/in/http", File: "x", Lang: "go",
			Callees: []string{"internal/services/order/f.go::Place"}, Entry: &facts.Entry{Kind: "http", Labels: []string{name}}}
	}
	f.Functions["internal/adapters/in/http/f.go::Job"] = facts.Function{Name: "Job", Start: 1, End: 30, Module: "internal/adapters/in/http", File: "x", Lang: "go",
		Callees: []string{"internal/services/order/f.go::validate"}, Entry: &facts.Entry{Kind: "task", Labels: []string{"Job"}}}
	m := Build(f)
	top, task := 0, false
	for _, fl := range m.Flows {
		if fl.Top {
			top++
			task = task || fl.Kind == "task"
		}
	}
	if top > MaxTop || !task {
		t.Errorf("top = %d, task kind represented = %v", top, task)
	}
}

func TestBuildMeta(t *testing.T) {
	f := synth()
	m := Build(f)
	if m.Title != "demo" || m.Summary != "" {
		t.Errorf("no project: %q/%q", m.Title, m.Summary)
	}
	f.Project = &facts.Project{Name: "Shop", Description: "A shop."}
	f.Modules["internal/services/order"] = facts.Module{Doc: "Orders."}
	m = Build(f)
	if m.Title != "Shop" || m.Summary != "A shop." {
		t.Errorf("project: %q/%q", m.Title, m.Summary)
	}
	if mi := m.Modules["internal/services/order"]; mi.Desc != "Orders." || mi.Layer != Domain {
		t.Errorf("module = %+v", mi)
	}
	if mi := m.Modules["internal/adapters/out/stripe"]; mi.Desc != "" || mi.Role != "Stripe client" {
		t.Errorf("module = %+v", mi)
	}
	b, err := JSON(f)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if json.Unmarshal(b, &back) != nil || back["flows"] == nil || back["modules"] == nil {
		t.Errorf("json shape: %s", b)
	}
}
