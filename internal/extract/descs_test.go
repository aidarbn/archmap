package extract

import (
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestProtoServiceAndRPCDescriptions(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod": apiGoMod,
		"api/shop.proto": `syntax = "proto3";
package shop.v1;

// Админский сервис управления
// рестораном и продуктами
service ShopService {
  // Получить заказ по id
  rpc GetOrder(GetOrderRequest) returns (GetOrderResponse) {
    option (google.api.http) = {get: "/v1/orders/{id}"};
  }

  /* Оплатить заказ */
  rpc Pay(PayRequest) returns (PayResponse);
  rpc Refund(RefundRequest) returns (RefundResponse);
  rpc Bare(BareRequest) returns (BareResponse);
}

service TabletService {
  rpc Ping(PingRequest) returns (PingResponse);
}
`,
		"srv/doc.go": "// Package srv — входящий gRPC-API магазина.\npackage srv\n",
		"srv/server.go": `package srv

// ShopServer serves the shop.
//nolint:revive
type ShopServer struct{}

func (s *ShopServer) GetOrder(ctx context.Context, req *shopv1.GetOrderRequest) (*shopv1.GetOrderResponse, error) {
	return nil, nil
}

// Refund returns the money to the guest.
func (s *ShopServer) Refund(ctx context.Context, req *shopv1.RefundRequest) (*shopv1.RefundResponse, error) {
	return nil, nil
}

type TabletServer struct{}

func (s *TabletServer) Ping(ctx context.Context, req *tabletv1.PingRequest) (*tabletv1.PingResponse, error) {
	return nil, nil
}
`,
	})
	eps := []struct{ name, desc string }{
		{"GetOrder", "Получить заказ по id"},                 // comment above the rpc
		{"Pay", "Оплатить заказ"},                            // block comment
		{"Refund", "Refund returns the money to the guest."}, // no proto comment: the handler doc
		{"Bare", ""},
	}
	for _, c := range eps {
		e := findEP(f, func(e facts.Endpoint) bool { return e.Name == c.name })
		if e == nil {
			t.Fatalf("no %s", c.name)
		}
		if e.Desc != c.desc {
			t.Errorf("%s desc = %q, want %q", c.name, e.Desc, c.desc)
		}
	}
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Name == "GetOrder" }); e.Line != 8 {
		t.Errorf("GetOrder line = %d, want 8", e.Line)
	}
	svcs := []struct {
		name, desc, from, file string
		line                   int
		impl                   string
	}{
		{"ShopService", "Админский сервис управления рестораном и продуктами", "proto", "api/shop.proto", 6, "ShopServer"},
		{"TabletService", "Package srv — входящий gRPC-API магазина.", "package", "api/shop.proto", 18, "TabletServer"},
	}
	for _, c := range svcs {
		s, ok := f.Services[c.name]
		if !ok {
			t.Fatalf("no service %s: %+v", c.name, f.Services)
		}
		if s.Desc != c.desc || s.DescFrom != c.from || s.File != c.file || s.Line != c.line || s.Impl != c.impl {
			t.Errorf("%s = %+v", c.name, s)
		}
	}
}

func TestServiceDescFromImplType(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod":      apiGoMod,
		"api/a.proto": "syntax = \"proto3\";\nservice AuthService {\n  rpc Login(LoginRequest) returns (LoginResponse);\n}\n",
		"srv/auth.go": "package srv\n\n// AuthServer authenticates terminals and staff.\ntype AuthServer struct{}\n\nfunc (s *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) { return nil, nil }\n",
	})
	s := f.Services["AuthService"]
	if s.Desc != "AuthServer authenticates terminals and staff." || s.DescFrom != "type" || s.ImplFile != "srv/auth.go" || s.ImplLine != 4 {
		t.Errorf("service = %+v", s)
	}
}

func TestActorDocsAndExternalDesc(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod": apiGoMod,
		"srv/h.go": `package srv

// Create makes an order.
// arch:flow order "Create an order"
// arch:actor Планшет "Планшет в зале ресторана"
func Create() {}

// arch:actor Гость "Посетитель"
func Other() {}
`,
	})
	want := []facts.ActorDoc{{Name: "Планшет", Desc: "Планшет в зале ресторана", File: "srv/h.go", Line: 5}, {Name: "Гость", Desc: "Посетитель", File: "srv/h.go", Line: 8}}
	if len(f.Actors) != 2 || f.Actors[0] != want[0] || f.Actors[1] != want[1] {
		t.Errorf("actors = %+v", f.Actors)
	}
	if d := f.Functions["srv/h.go::Create"].Doc; d != "Create makes an order." {
		t.Errorf("doc = %q", d)
	}
}

func TestExternalDoc(t *testing.T) {
	mods := map[string]facts.Module{
		"internal/adapters/out/kaspi":         {Doc: "Package kaspi — клиент Kaspi Pay."},
		"internal/adapters/dbqueue":           {Doc: "Package dbqueue — очередь River."},
		"internal/adapters/out/db/repository": {Doc: "Package repository — запросы к Postgres."},
	}
	cases := []struct {
		x    facts.External
		want string
	}{
		{facts.External{Name: "Kaspi", Kind: "payment", Modules: []string{"internal/adapters/out/kaspi"}}, "Package kaspi — клиент Kaspi Pay."},
		{facts.External{Name: "Postgres", Kind: "db", Modules: []string{"internal/adapters/dbqueue", "internal/adapters/out/db/repository"}}, "Package repository — запросы к Postgres."},
		{facts.External{Name: "Redis", Kind: "cache", Modules: []string{"a", "b"}}, ""},
	}
	for _, c := range cases {
		if got := externalDoc(&c.x, mods); got != c.want {
			t.Errorf("%s: %q, want %q", c.x.Name, got, c.want)
		}
	}
}
