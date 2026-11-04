package extract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang"
)

func TestMatchLib(t *testing.T) {
	cases := []struct{ path, name, kind string }{
		{"github.com/jackc/pgx/v5/pgxpool", "Postgres", KindDB},
		{"github.com/lib/pq", "Postgres", KindDB},
		{"github.com/go-jet/jet/v2/postgres", "Postgres", KindDB},
		{"github.com/go-jet/jet/v2", genericDB, KindDB},
		{"database/sql", genericDB, KindDB},
		{"github.com/riverqueue/river", "River", KindQueue},
		{"github.com/redis/go-redis/v9", "Redis", KindCache},
		{"github.com/minio/minio-go/v7", "MinIO", KindStorage},
		{"github.com/aws/aws-sdk-go-v2/service/s3", "S3", KindStorage},
		{"github.com/segmentio/kafka-go", "Kafka", KindQueue},
		{"github.com/nats-io/nats.go", "NATS", KindQueue},
		{"psycopg2", "Postgres", KindDB},
		{"sqlalchemy.orm", genericDB, KindDB},
		{"django.db.models", genericDB, KindDB},
		{"celery", "Celery", KindQueue},
		{"pg", "Postgres", KindDB},
		{"ioredis", "Redis", KindCache},
		{"bullmq", "BullMQ", KindQueue},
		{"@prisma/client", genericDB, KindDB},
		{"typeorm", genericDB, KindDB},
	}
	for _, c := range cases {
		r, ok := matchLib(c.path)
		if !ok || r.name != c.name || r.kind != c.kind {
			t.Errorf("%s -> %+v ok=%v, want %s/%s", c.path, r, ok, c.name, c.kind)
		}
	}
	for _, p := range []string{"net/http", "fmt", "pgx-helper-not", "react", "lodash", "pgrx"} {
		if r, ok := matchLib(p); ok {
			t.Errorf("%s must not match, got %+v", p, r)
		}
	}
}

func extFacts() (*facts.Facts, extInput) {
	f := &facts.Facts{Modules: map[string]facts.Module{}, Functions: map[string]facts.Function{}}
	addMod := func(m string, files ...string) { f.Modules[m] = facts.Module{Files: files} }
	addFn := func(fid, mod, file string, callees ...string) {
		f.Functions[fid] = facts.Function{Name: fid, Module: mod, File: file, Callees: callees, Lang: "go"}
	}
	addMod("internal/adapters/out/kaspi", "internal/adapters/out/kaspi/c.go")
	addMod("internal/adapters/out/kaspi/dto", "internal/adapters/out/kaspi/dto/d.go")
	addMod("internal/adapters/out/prg", "internal/adapters/out/prg/c.go")
	addMod("internal/adapters/out/db/repository", "internal/adapters/out/db/repository/r.go")
	addMod("internal/adapters/out/httpclient", "internal/adapters/out/httpclient/h.go")
	addMod("internal/services/order", "internal/services/order/s.go")
	addMod("internal/adapters/in/http", "internal/adapters/in/http/h.go")
	addFn("k::Pay", "internal/adapters/out/kaspi", "internal/adapters/out/kaspi/c.go")
	addFn("r::Save", "internal/adapters/out/db/repository", "internal/adapters/out/db/repository/r.go")
	addFn("s::Create", "internal/services/order", "internal/services/order/s.go", "k::Pay", "r::Save")
	addFn("h::Handle", "internal/adapters/in/http", "internal/adapters/in/http/h.go", "s::Create")
	in := extInput{
		Facts:  f,
		Layers: autosem.Layers(f),
		Imports: map[string][]lang.Import{
			"internal/adapters/out/db/repository/r.go": {{Path: "github.com/jackc/pgx/v5"}, {Path: "database/sql"}},
			"internal/adapters/out/prg/c.go":           {{Path: "net/http"}},
			"internal/adapters/out/httpclient/h.go":    {{Path: "net/http"}},
		},
		Calls: map[string][]lang.Call{},
		Text:  func(string) string { return "" },
	}
	return f, in
}

func TestDetectExternals(t *testing.T) {
	_, in := extFacts()
	got := detectExternals(in)
	by := map[string]facts.External{}
	for _, e := range got {
		by[e.Name] = e
	}
	k, ok := by["Kaspi"]
	if !ok || k.Kind != KindPayment {
		t.Fatalf("Kaspi missing or wrong kind: %+v", got)
	}
	if !reflect.DeepEqual(k.Modules, []string{"internal/adapters/out/kaspi", "internal/adapters/out/kaspi/dto"}) {
		t.Errorf("Kaspi modules: %v", k.Modules)
	}
	if !reflect.DeepEqual(k.Functions, []string{"s::Create"}) {
		t.Errorf("Kaspi callers must be outside functions only: %v", k.Functions)
	}
	if p := by["Postgres"]; p.Kind != KindDB || !reflect.DeepEqual(p.Functions, []string{"s::Create"}) {
		t.Errorf("Postgres: %+v", p)
	}
	if _, ok := by[genericDB]; ok {
		t.Error("generic Database must merge into the only dialect")
	}
	if p := by["Prg"]; p.Kind != KindHTTPAPI {
		t.Errorf("unknown client under out/ that speaks HTTP is http_api: %+v", p)
	}
	for _, bad := range []string{"Httpclient", "Db", "Http"} {
		if _, ok := by[bad]; ok {
			t.Errorf("%s is not an outside system", bad)
		}
	}
}

func TestExternalsDirectUse(t *testing.T) {
	f := &facts.Facts{
		Modules: map[string]facts.Module{"app/orders": {Files: []string{"app/orders/s.go"}}},
		Functions: map[string]facts.Function{
			"s::Open": {Module: "app/orders", File: "app/orders/s.go", Lang: "go"},
			"s::Else": {Module: "app/orders", File: "app/orders/s.go", Lang: "go"},
		},
	}
	in := extInput{Facts: f, Layers: map[string]string{"app/orders": autosem.Domain},
		Imports: map[string][]lang.Import{"app/orders/s.go": {{Path: "github.com/redis/go-redis/v9"}}},
		Calls:   map[string][]lang.Call{"s::Open": {{Name: "NewClient", Recv: "redis"}}}}
	got := detectExternals(in)
	if len(got) != 1 || got[0].Name != "Redis" || !reflect.DeepEqual(got[0].Functions, []string{"s::Open"}) {
		t.Errorf("a service that calls the client itself: %+v", got)
	}
	in.Calls = nil
	if got := detectExternals(in); len(got) != 0 {
		t.Errorf("a mere import in a service is not evidence: %+v", got)
	}
}

func TestDialectFromText(t *testing.T) {
	f := &facts.Facts{
		Modules:   map[string]facts.Module{"app/db": {Files: []string{"app/db/db.go"}}},
		Functions: map[string]facts.Function{},
	}
	in := extInput{Facts: f, Layers: map[string]string{"app/db": autosem.Adapter},
		Imports: map[string][]lang.Import{"app/db/db.go": {{Path: "gorm.io/gorm"}}},
		Text:    func(string) string { return `dsn := "postgres://u@h/db"` }}
	got := detectExternals(in)
	if len(got) != 1 || got[0].Name != "Postgres" {
		t.Errorf("dialect from connection string: %+v", got)
	}
	in.Text = func(string) string { return "" }
	if got := detectExternals(in); len(got) != 1 || got[0].Name != genericDB {
		t.Errorf("unknown dialect stays generic: %+v", got)
	}
}

func TestBaseURL(t *testing.T) {
	f := &facts.Facts{
		Modules:   map[string]facts.Module{"src/api": {Files: []string{"src/api/c.ts"}}},
		Functions: map[string]facts.Function{},
	}
	text := `const BASE_URL = "https://api.weather-service.io/v2"; const x = "http://localhost:3000"`
	in := extInput{Facts: f, Layers: map[string]string{"src/api": autosem.Adapter}, Text: func(string) string { return text }}
	got := detectExternals(in)
	if len(got) != 1 || got[0].Name != "weather-service.io" || got[0].Kind != KindHTTPAPI {
		t.Errorf("base url: %+v", got)
	}
}

func goFile(text string) stFile {
	return stFile{Path: "app/order.go", Module: "app", Lang: "go", Text: text}
}

func TestGoMachines(t *testing.T) {
	ms := collectMachines([]stFile{goFile(`package app
type OrderStatus string
const (
	StatusNew  OrderStatus = "new"
	StatusPaid OrderStatus = "paid"
)
type Stage int
const (
	StageOne Stage = iota
	StageTwo
	StageThree
)
const (
	PaymentStatusOpen = "open"
	PaymentStatusDone = "done"
	MaxRetries = "3"
)
`)})
	got := map[string][]string{}
	for _, m := range ms {
		got[m.key] = m.labels
	}
	want := map[string][]string{
		"OrderStatus":   {"new", "paid"},
		"Stage":         {"One", "Two", "Three"},
		"PaymentStatus": {"open", "done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("machines: %v", got)
	}
}

const goOrder = `package app
type OrderStatus string
const (
	OrderStatusNew      OrderStatus = "new"
	OrderStatusPaid     OrderStatus = "paid"
	OrderStatusShipped  OrderStatus = "shipped"
	OrderStatusCanceled OrderStatus = "canceled"
)

func (o *Order) Pay() {
	if o.Status != OrderStatusNew {
		return
	}
	o.Status = OrderStatusPaid
}

func (s *Svc) Ship(ctx context.Context, id int) error {
	// o.Status = OrderStatusCanceled in a comment does not count
	return s.repo.UpdateStatusIfPaid(ctx, id, OrderStatusShipped)
}

func (s *Svc) Cancel(ctx context.Context, o *Order) error {
	switch o.Status {
	case OrderStatusNew, OrderStatusPaid:
		o.Status = OrderStatusCanceled
	}
	return nil
}

func (s *Svc) Create() *Order {
	return &Order{Status: OrderStatusNew}
}

func (s *Svc) Report() string {
	return fmt.Sprint(OrderStatusPaid, o.Status == OrderStatusShipped)
}

func (r *Repo) Touch(ctx context.Context) {
	r.db.Exec(ctx, "UPDATE orders SET status = 'paid' WHERE id = $1 AND status = 'new'", 1)
}
`

func TestGoTransitions(t *testing.T) {
	f := goFile(goOrder)
	var funcs []stFunc
	for _, name := range []string{"Pay", "Ship", "Cancel", "Create", "Report", "Touch"} {
		i := strings.Index(f.Text, ") "+name+"(")
		start := strings.Count(f.Text[:i], "\n") + 1
		end := start + strings.Count(f.Text[i:], "\n}\n")*0
		rest := f.Text[i:]
		end = start + strings.Count(rest[:strings.Index(rest, "\n}\n")], "\n") + 1
		funcs = append(funcs, stFunc{Fid: name, File: f.Path, Start: start, End: end})
	}
	st := detectStates([]stFile{f}, funcs, nil)["OrderStatus"]
	if st == nil {
		t.Fatal("no OrderStatus machine")
	}
	var got []string
	for _, tr := range st.Transitions {
		if tr.Source != "code" {
			t.Errorf("source: %+v", tr)
		}
		got = append(got, tr.Fid+":"+tr.From+">"+tr.To)
	}
	want := []string{
		"Cancel:new>canceled", "Cancel:paid>canceled", // switch case lists are sources
		"Pay:new>paid",      // != guard is the source
		"Ship:paid>shipped", // ...IfPaid in the call name is the source
		"Touch:new>paid",    // SQL SET/WHERE
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("transitions:\n got %v\nwant %v", got, want)
	}
}

func TestSQLQueriesByName(t *testing.T) {
	f := goFile(`package app
type OrderStatus string
const (
	OrderStatusNew  OrderStatus = "new"
	OrderStatusPaid OrderStatus = "paid"
)
func (r *Repo) Pay(ctx context.Context, id int) error {
	_, err := r.q.MarkPaid(ctx, id)
	return err
}
`)
	q := map[string]string{"MarkPaid": "UPDATE orders\nSET status = 'paid'\nWHERE id = $1 AND status = 'new'"}
	st := detectStates([]stFile{f}, []stFunc{{Fid: "Pay", File: f.Path, Start: 7, End: 10}}, q)["OrderStatus"]
	if st == nil || len(st.Transitions) != 1 || st.Transitions[0].From != "new" || st.Transitions[0].To != "paid" {
		t.Errorf("%+v", st)
	}
}

func TestSQLSetBeforeWhere(t *testing.T) {
	// SET status = $1 is dynamic: the literal in WHERE is a source, not a target
	values := map[string][]*machine{}
	m := &machine{key: "OrderStatus", values: map[string]string{"accepted": "accepted", "failed": "failed"}}
	values["accepted"], values["failed"] = []*machine{m}, []*machine{m}
	var ev funcEvidence
	sqlEvidence(&ev, "UPDATE orders SET status = $1 WHERE id = $2 AND status = 'accepted'", values)
	if len(ev.to) != 0 {
		t.Errorf("no target is visible: %+v", ev.to)
	}
}

func TestPythonMachines(t *testing.T) {
	f := stFile{Path: "shop/models.py", Module: "shop", Lang: "python", Text: `from django.db import models

class OrderStatus(models.TextChoices):
    NEW = "new", "New"
    PAID = "paid", "Paid"
    DONE = "done", "Done"

class Color(models.TextChoices):
    RED = "red"

def pay(order):
    if order.status == OrderStatus.NEW:
        order.status = OrderStatus.PAID

def finish(qs):
    qs.filter(status=OrderStatus.PAID).update(status=OrderStatus.DONE)
`}
	funcs := []stFunc{{Fid: "pay", File: f.Path, Start: 12, End: 14}, {Fid: "finish", File: f.Path, Start: 16, End: 17}}
	st := detectStates([]stFile{f}, funcs, nil)
	m := st["OrderStatus"]
	if m == nil || len(st) != 1 || !reflect.DeepEqual(m.States, []string{"new", "paid", "done"}) {
		t.Fatalf("machines: %+v", st)
	}
	var got []string
	for _, tr := range m.Transitions {
		got = append(got, tr.Fid+":"+tr.From+">"+tr.To)
	}
	if !reflect.DeepEqual(got, []string{"finish:paid>done", "pay:new>paid"}) {
		t.Errorf("transitions: %v", got)
	}
}

func TestTSMachines(t *testing.T) {
	f := stFile{Path: "src/o.ts", Module: "src", Lang: "js", Text: `export type OrderStatus = 'new' | 'paid' | 'done'
export enum PayState { Open = 'open', Closed = 'closed' }
type Mode = 'a' | 'b'
function pay(o) {
  if (o.status === 'new') {
    o.status = 'paid'
  }
}
function close(p) { p.state = PayState.Closed }
`}
	funcs := []stFunc{{Fid: "pay", File: f.Path, Start: 4, End: 8}, {Fid: "close", File: f.Path, Start: 9, End: 9}}
	st := detectStates([]stFile{f}, funcs, nil)
	if st["OrderStatus"] == nil || len(st["OrderStatus"].Transitions) != 1 || st["OrderStatus"].Transitions[0].From != "new" {
		t.Errorf("union: %+v", st["OrderStatus"])
	}
	if st["PayState"] == nil || st["PayState"].Transitions[0].To != "closed" {
		t.Errorf("enum: %+v", st["PayState"])
	}
	if st["Mode"] != nil {
		t.Error("a union without a status-like name is not a state machine")
	}
}

func TestSameNameMachinesGetModule(t *testing.T) {
	a := stFile{Path: "a/s.go", Module: "a", Lang: "go", Text: "const (\nSA Status = \"x\"\nSB Status = \"y\"\n)\ntype Status string\n"}
	b := stFile{Path: "b/s.go", Module: "b", Lang: "go", Text: "const (\nTA Status = \"p\"\nTB Status = \"q\"\n)\ntype Status string\n"}
	var keys []string
	for _, m := range collectMachines([]stFile{a, b}) {
		keys = append(keys, m.key)
	}
	if !reflect.DeepEqual(keys, []string{"Status (a)", "Status (b)"}) {
		t.Errorf("%v", keys)
	}
}

func TestStatements(t *testing.T) {
	got := statements("a := 1 // x = 2\nf(a,\n  \"b;c\")\nq := `sel\nect`; z()\n", "go")
	var raw []string
	for _, s := range got {
		raw = append(raw, strings.TrimSpace(s.raw))
	}
	want := []string{"a := 1", `f(a,   "b;c")`, "q := `sel\nect`", "z()"}
	if !reflect.DeepEqual(raw, want) {
		t.Errorf("%q", raw)
	}
	if len(got[1].lits) != 1 || got[1].lits[0].val != "b;c" {
		t.Errorf("literals: %+v", got[1].lits)
	}
	if strings.Contains(got[1].masked, "b;c") {
		t.Error("string contents must be masked")
	}
}

func TestSourceSlice(t *testing.T) {
	text := "l1\nl2\nl3\nl4\nl5\n"
	if got := SourceSlice(text, 2, 3, 60, "go"); got != "l2\nl3" {
		t.Errorf("%q", got)
	}
	if got := SourceSlice(text, 1, 5, 2, "go"); got != "l1\nl2\n// … (+3 more lines)" {
		t.Errorf("%q", got)
	}
	if got := SourceSlice(text, 1, 5, 2, "python"); !strings.HasSuffix(got, "# … (+3 more lines)") {
		t.Errorf("%q", got)
	}
	if got := SourceSlice(text, 9, 12, 60, "go"); got != "" {
		t.Errorf("out of range: %q", got)
	}
}

func TestVendorOfSegmentAndEnvName(t *testing.T) {
	for seg, want := range map[string]string{"kaspi": "Kaspi", "twilio_service": "Twilio", "iiko-client": "iiko", "orders": ""} {
		v, ok := vendorOfSegment(seg)
		if v.name != want || ok != (want != "") {
			t.Errorf("%s -> %+v", seg, v)
		}
	}
	for env, want := range map[string]string{
		"NEXT_PUBLIC_API_URL": "Backend API", "VITE_BACKEND_URL": "Backend API", "NEXT_PUBLIC_MEDIA_URL": "Media API",
		"NEXT_PUBLIC_SENTRY_URL": "", "NEXT_PUBLIC_SITE_URL": "", "REACT_APP_PAYMENT_API_URL": "Payment API",
	} {
		if got := envAPIName(env); got != want {
			t.Errorf("%s -> %q, want %q", env, got, want)
		}
	}
}
