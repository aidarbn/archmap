package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestSlug(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Оплата Kaspi", "оплата-kaspi"},
		{"  Menu sync / POS  ", "menu-sync-pos"},
		{"a__b", "a__b"},
		{"!!!", ""},
	} {
		if got := Slug(c.in); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParsePages(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []Page
	}{
		{
			name: "name only",
			text: "page \"Оплата Kaspi\"\n  journey guest-pay-kaspi\n  flow    kaspi-pay\n  states  Order, Payment\n  tables  orders, payments,kaspi_payment_details\n  api     RequestKaspiQRForOrder, \"POST /orders\"\n  module  services/kaspi\n  note    \"Текст пояснения\"\n",
			want: []Page{{ID: "оплата-kaspi", Name: "Оплата Kaspi", Frames: []Frame{
				{Kind: FrameJourney, Refs: []string{"guest-pay-kaspi"}},
				{Kind: FrameFlow, Refs: []string{"kaspi-pay"}},
				{Kind: FrameStates, Refs: []string{"Order", "Payment"}},
				{Kind: FrameTables, Refs: []string{"orders", "payments", "kaspi_payment_details"}},
				{Kind: FrameAPI, Refs: []string{"RequestKaspiQRForOrder", "POST /orders"}},
				{Kind: FrameModule, Refs: []string{"services/kaspi"}},
				{Kind: FrameNote, Text: "Текст пояснения"},
			}}},
		},
		{
			name: "id and name, two pages",
			text: "page kaspi \"Оплата\"\n  tables orders\n\npage sync\n  module internal/services/sync\n",
			want: []Page{
				{ID: "kaspi", Name: "Оплата", Frames: []Frame{{Kind: FrameTables, Refs: []string{"orders"}}}},
				{ID: "sync", Name: "sync", Frames: []Frame{{Kind: FrameModule, Refs: []string{"internal/services/sync"}}}},
			},
		},
		{
			name: "next to other blocks",
			text: "journey j: a > b\npage p\n  journey j\nflow a\n  step A\n",
			want: []Page{{ID: "p", Name: "p", Frames: []Frame{{Kind: FrameJourney, Refs: []string{"j"}}}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := mustParse(t, c.text)
			var got []Page
			for _, p := range d.Pages {
				q := copyJSON(*p)
				q.Line, q.Comment = 0, nil
				for i := range q.Frames {
					q.Frames[i].Line = 0
				}
				got = append(got, q)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("pages:\n got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestParsePageErrors(t *testing.T) {
	cases := []struct {
		text, want string
		line       int
	}{
		{"page\n", "expected `page \"Name\"`", 1},
		{"page a b c\n", "expected `page \"Name\"`", 1},
		{"page \"!!\"\n", "empty id", 1},
		{"page p\n  journey a, b\n", "one journey per line", 2},
		{"page p\n  flow\n", "needs at least one name", 2},
		{"page p\n  tables \"orders\"\n", "without quotes", 2},
		{"page p\n  diagram x\n", "unknown line \"diagram\" in page", 2},
		{"page p\n  note\n", "`note` needs a value", 2},
		{"tables orders\n", "must be indented inside a page", 1},
		{"  tables orders\n", "indented line outside", 1},
	}
	for _, c := range cases {
		_, errs := Parse("arch/pages/p.arch", c.text)
		if len(errs) == 0 {
			t.Errorf("%q: no error, want %q", c.text, c.want)
			continue
		}
		if !strings.Contains(errs[0].Msg, c.want) || errs[0].Line != c.line {
			t.Errorf("%q: got %s, want line %d %q", c.text, errs[0].Error(), c.line, c.want)
		}
		if !strings.HasPrefix(errs[0].Error(), "arch/pages/p.arch:") {
			t.Errorf("%q: error without file:line: %s", c.text, errs[0].Error())
		}
	}
}

func TestPageFormatRoundTrip(t *testing.T) {
	text := "# the kaspi page\npage \"Оплата Kaspi\"\n  journey guest-pay-kaspi\n  states  Order, Payment\n  api     CancelOrder, \"POST /orders\"\n  note    \"Пояснение \\\"в кавычках\\\"\"\n\npage sync \"Синхронизация меню\"\n  module services/sync\n\npage x\n  tables orders\n"
	d := mustParse(t, text)
	out := Format(d)
	if out != text {
		t.Errorf("format:\n%s\nwant:\n%s", out, text)
	}
	d2 := mustParse(t, out)
	if !reflect.DeepEqual(copyJSON(d.Pages), copyJSON(d2.Pages)) {
		t.Errorf("round trip changed the pages")
	}
}

func TestLayoutPage(t *testing.T) {
	fr := func(kinds ...string) []Frame {
		out := make([]Frame, len(kinds))
		for i, k := range kinds {
			out[i] = Frame{Kind: k}
		}
		return out
	}
	cases := []struct {
		name   string
		frames []Frame
		want   []Slot
	}{
		{"empty", nil, []Slot{}},
		{"the example page", fr("journey", "states", "tables", "api", "module", "note"), []Slot{
			{Frame: 0, Row: 0, Col: 0, Span: 2}, {Frame: 1, Row: 0, Col: 2, Span: 2},
			{Frame: 2, Row: 1, Col: 0, Span: 1}, {Frame: 3, Row: 1, Col: 1, Span: 1}, {Frame: 4, Row: 1, Col: 2, Span: 1}, {Frame: 5, Row: 1, Col: 3, Span: 1},
		}},
		{"large ones first, the rest in line order", fr("note", "tables", "flow", "states", "journey"), []Slot{
			{Frame: 2, Row: 0, Col: 0, Span: 2}, {Frame: 4, Row: 0, Col: 2, Span: 2},
			{Frame: 3, Row: 1, Col: 0, Span: 2}, {Frame: 0, Row: 1, Col: 2, Span: 1}, {Frame: 1, Row: 1, Col: 3, Span: 1},
		}},
		{"a wide frame that does not fit starts a row", fr("journey", "api", "states"), []Slot{
			{Frame: 0, Row: 0, Col: 0, Span: 2}, {Frame: 2, Row: 0, Col: 2, Span: 2}, {Frame: 1, Row: 1, Col: 0, Span: 1},
		}},
		{"small ones wrap after four", fr("api", "api", "api", "api", "api"), []Slot{
			{Frame: 0, Row: 0, Col: 0, Span: 1}, {Frame: 1, Row: 0, Col: 1, Span: 1}, {Frame: 2, Row: 0, Col: 2, Span: 1}, {Frame: 3, Row: 0, Col: 3, Span: 1}, {Frame: 4, Row: 1, Col: 0, Span: 1},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := LayoutPage(c.frames); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func pageFacts() *facts.Facts {
	f := testFacts()
	f.Modules = map[string]facts.Module{"internal/services/kaspi": {}, "internal/services/sync": {}, "internal/adapters/sync": {}}
	f.Data = &facts.DataModel{Tables: map[string]*facts.Table{"orders": {}, "payments": {}}}
	fn := f.Functions["pay/svc.go::Service.requestQR"]
	fn.Data = []facts.Access{{Table: "kaspi_payment_details", Op: "insert"}}
	f.Functions["pay/svc.go::Service.requestQR"] = fn
	f.Functions["srv/tablets.go::TabletsServer.RequestKaspiQRForOrder"] = withArch(f.Functions["srv/tablets.go::TabletsServer.RequestKaspiQRForOrder"], &facts.ArchNote{
		Flows:    []facts.FlowDecl{{ID: "kaspi-pay"}},
		Journeys: []facts.JourneyDecl{{ID: "guest-pay-kaspi", Flows: []string{"kaspi-pay"}}},
	})
	f.Endpoints = []facts.Endpoint{
		{Kind: "grpc", Service: "TabletsService", Name: "RequestKaspiQRForOrder", Method: "POST", Path: "/tablet/v2/orders/{order_id}/kaspi-qr"},
		{Kind: "grpc", Service: "TabletsService", Name: "CancelOrder", Method: "POST", Path: "/tablet/v2/order/{order_id}/cancel"},
		{Kind: "grpc", Service: "AdminService", Name: "CancelOrder", Method: "POST", Path: "/admin/order/{order_id}/cancel"},
	}
	return f
}

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withArch(fn facts.Function, a *facts.ArchNote) facts.Function { fn.Arch = a; return fn }

func TestCheckPageRefs(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		missing []string
		msg     string
		check   func(t *testing.T, fr Frame)
	}{
		{name: "journey from arch:journey", line: "journey guest-pay-kaspi"},
		{name: "journey from the spec", line: "journey spec-j"},
		{name: "no such journey", line: "journey nope", missing: []string{"nope"}, msg: `journey "nope": no such journey`},
		{name: "flow from arch:flow", line: "flow kaspi-pay"},
		{name: "no such flow", line: "flow nope", missing: []string{"nope"}, msg: "no such flow"},
		{name: "states by entity", line: "states Order, Ghost", missing: []string{"Ghost"}, msg: "no state machine",
			check: func(t *testing.T, fr Frame) {
				if !reflect.DeepEqual(fr.Machines, []string{"OrderStatus", ""}) {
					t.Errorf("machines %v", fr.Machines)
				}
			}},
		{name: "tables: schema and data access", line: "tables orders, kaspi_payment_details, ghosts", missing: []string{"ghosts"}, msg: `tables "ghosts"`},
		{name: "api by name, service.name and path", line: "api RequestKaspiQRForOrder, AdminService.CancelOrder, /tablet/v2/order/{order_id}/cancel",
			check: func(t *testing.T, fr Frame) {
				if !reflect.DeepEqual(fr.Endpoints, []int{0, 2, 1}) {
					t.Errorf("endpoints %v", fr.Endpoints)
				}
			}},
		{name: "api ambiguous", line: "api CancelOrder", missing: []string{"CancelOrder"}, msg: "matches several endpoints"},
		{name: "api missing", line: "api Nope", missing: []string{"Nope"}, msg: `api "Nope": no such endpoint`},
		{name: "module by suffix", line: "module services/kaspi",
			check: func(t *testing.T, fr Frame) {
				if !reflect.DeepEqual(fr.Modules, []string{"internal/services/kaspi"}) {
					t.Errorf("modules %v", fr.Modules)
				}
			}},
		{name: "module ambiguous suffix", line: "module sync", missing: []string{"sync"}, msg: "matches several modules"},
		{name: "module missing", line: "module nope/x", missing: []string{"nope/x"}, msg: "no such module"},
		{name: "note needs nothing", line: "note \"text\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := setOf(t, map[string]string{
				"arch/flows.arch":     "journey spec-j: kaspi-pay\n",
				"arch/pages/one.arch": "page p\n  " + c.line + "\n",
			})
			m, fs := Check(s, pageFacts(), nil)
			if len(m.Pages) != 1 || len(m.Pages[0].Frames) != 1 {
				t.Fatalf("pages %+v", m.Pages)
			}
			fr := m.Pages[0].Frames[0]
			if !reflect.DeepEqual(fr.Missing, c.missing) {
				t.Errorf("missing %v, want %v", fr.Missing, c.missing)
			}
			refs := codes(fs)[CodePageRef]
			if c.msg == "" && len(refs) > 0 {
				t.Errorf("unexpected findings %+v", refs)
			}
			if c.msg != "" {
				if len(refs) == 0 || !strings.Contains(refs[0].Msg, c.msg) || refs[0].Severity != SevError || refs[0].File != "arch/pages/one.arch" || refs[0].Line != 2 {
					t.Errorf("findings %+v, want %q at arch/pages/one.arch:2", refs, c.msg)
				}
			}
			if c.check != nil {
				c.check(t, fr)
			}
			if len(m.Pages[0].Layout) != 1 {
				t.Errorf("layout %+v", m.Pages[0].Layout)
			}
		})
	}
}

func TestDuplicatePage(t *testing.T) {
	s := setOf(t, map[string]string{
		"arch/a.arch":       "page p\n  note x\n",
		"arch/pages/b.arch": "page p \"Again\"\n  note y\n",
	})
	m, fs := Check(s, pageFacts(), nil)
	if len(m.Pages) != 1 || len(codes(fs)[CodeDuplicate]) != 1 {
		t.Errorf("pages %d, findings %+v", len(m.Pages), fs)
	}
	if s.Empty() {
		t.Error("a set with pages only is not empty")
	}
}

func TestLoadPagesDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "arch/main.arch", "page a\n  note x\n")
	writeFile(t, root, "arch/pages/kaspi.arch", "page \"Оплата Kaspi\"\n  tables orders\n")
	writeFile(t, root, "arch/pages/notes.txt", "ignored")
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range s.Files {
		paths = append(paths, f.Path)
	}
	if !reflect.DeepEqual(paths, []string{"arch/main.arch", "arch/pages/kaspi.arch"}) {
		t.Errorf("paths %v", paths)
	}
	if got := s.Model().Pages; len(got) != 2 || got[1].File != "arch/pages/kaspi.arch" {
		t.Errorf("pages %+v", got)
	}
	if !ValidPath("arch/pages/x.arch") || ValidPath("arch/pages/sub/x.arch") || ValidPath("arch/other/x.arch") {
		t.Error("ValidPath")
	}
}
