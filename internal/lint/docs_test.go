package lint

import (
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestDocsCoverage(t *testing.T) {
	cls := "TabletsServer"
	f := &facts.Facts{
		Modules: map[string]facts.Module{
			"srv": {Doc: "Package srv — gRPC.", Files: []string{"srv/a.go"}},
			"cmd": {Files: []string{"cmd/main.go"}},
		},
		Functions: map[string]facts.Function{
			"srv/a.go::TabletsServer.Pay": {Name: "Pay", Cls: &cls, File: "srv/a.go", Start: 10, Module: "srv", Doc: "Pay charges.", Entry: &facts.Entry{Kind: "http"},
				Arch: &facts.ArchNote{Flows: []facts.FlowDecl{{ID: "pay", Name: "Pay", Actor: "Планшет"}}}},
			"srv/a.go::TabletsServer.Menu": {Name: "Menu", Cls: &cls, File: "srv/a.go", Start: 20, Module: "srv", Entry: &facts.Entry{Kind: "http"}},
			"cmd/main.go::main":            {Name: "main", File: "cmd/main.go", Start: 3, Module: "cmd", Entry: &facts.Entry{Kind: "main"}},
		},
		Endpoints: []facts.Endpoint{
			{Kind: "grpc", Service: "TabletsService", Name: "Pay", Fid: "srv/a.go::TabletsServer.Pay", Auth: "role:tablet", Source: "proto", Line: 5, Desc: "Оплатить"},
			{Kind: "grpc", Service: "TabletsService", Name: "Menu", Fid: "srv/a.go::TabletsServer.Menu", Auth: "role:tablet", Source: "proto", Line: 7},
			{Kind: "grpc", Service: "AdminService", Name: "Ban", Auth: "role:admin", Source: "proto", Line: 3},
		},
		Services: map[string]facts.Service{
			"TabletsService": {Kind: "grpc", Desc: "API планшетов", File: "api/t.proto", Line: 2},
			"AdminService":   {Kind: "grpc", File: "api/a.proto", Line: 1},
		},
		Actors:    []facts.ActorDoc{{Name: "Планшет", Desc: "Планшет в зале", File: "srv/a.go", Line: 8}},
		Externals: []facts.External{{Name: "Kaspi", Modules: []string{"out/kaspi"}, Desc: "клиент"}, {Name: "Redis", Modules: []string{"out/cache"}}},
	}
	d := Docs(f)
	cases := []struct {
		name string
		got  Count
		want Count
	}{
		{"modules", d.Modules, Count{1, 2}},
		{"entries", d.Entries, Count{1, 3}},
		{"endpoints", d.Endpoints, Count{1, 3}},
		{"actors", d.Actors, Count{2, 3}}, // Планшет (markup), tablet (service), admin (none)
		{"externals", d.Externals, Count{1, 2}},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	var lines []string
	for _, m := range d.Missing {
		lines = append(lines, m.String())
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{
		"cmd/main.go: no description (module cmd)",
		"api/a.proto:1: no description (actor admin)",
		"api/t.proto:7: no description (endpoint TabletsService.Menu)",
		"srv/a.go:20: no description (function TabletsServer.Menu)",
		"out/cache: no description (external Redis)",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if !strings.HasPrefix(d.Line(), "descriptions: modules 1/2, entry points 1/3, endpoints 1/3, actors 2/3") {
		t.Errorf("line = %q", d.Line())
	}
}
