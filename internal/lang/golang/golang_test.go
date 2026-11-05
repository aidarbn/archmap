package golang

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

const src = `package svc

import (
	"fmt"
	str "strings"
	_ "embed"
	"example.com/m/internal/db"
)

// Service does work.
type Service struct{}

//go:noinline
// Do runs.
// Second line.
func (s *Service) Do(
	ctx context.Context,
) error {
	s.helper()
	s.repo.Save(ctx)
	fmt.Println(len("x"), str.ToUpper("a"))
	db.New().Close()
	go func() { worker(s) }()
	_ = []byte("x")
	s.helper()
	return nil
}

func (Box[T]) Get() {}

func (b *Box[T, U]) Put(v T) { Map[T](v); b.items[0].Run() }

func decl(a int) int

func Map[T any](v T) T { return v }

func broken( {
`

func parse(t *testing.T, s string) lang.File {
	t.Helper()
	f, err := New().Parse("a.go", []byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func byName(f lang.File, n string) lang.Func {
	for _, fn := range f.Funcs {
		if fn.Name == n {
			return fn
		}
	}
	return lang.Func{}
}

func TestImports(t *testing.T) {
	f := parse(t, src)
	want := []lang.Import{{Path: "fmt"}, {Path: "strings", Alias: "str"}, {Path: "embed", Alias: "_"}, {Path: "example.com/m/internal/db"}}
	if !reflect.DeepEqual(f.Imports, want) {
		t.Fatalf("imports = %+v", f.Imports)
	}
}

func TestMethod(t *testing.T) {
	do := byName(parse(t, src), "Do")
	if do.Cls != "Service" || do.Recv != "s" {
		t.Errorf("cls/recv = %q/%q", do.Cls, do.Recv)
	}
	if do.Start != 16 || do.End != 27 {
		t.Errorf("bounds = %d-%d", do.Start, do.End)
	}
	if do.Sig != "func (s *Service) Do( ctx context.Context, ) error" {
		t.Errorf("sig = %q", do.Sig)
	}
	if do.Doc != "Do runs. Second line." {
		t.Errorf("doc = %q", do.Doc)
	}
	want := []lang.Call{
		{Name: "helper", Recv: "s"}, {Name: "Save", Recv: "s.repo"}, {Name: "Println", Recv: "fmt"},
		{Name: "ToUpper", Recv: "str"}, {Name: "New", Recv: "db"}, {Name: "Close", Recv: "?"}, {Name: "worker"},
	}
	if !reflect.DeepEqual(do.Calls, want) {
		t.Errorf("calls = %+v", do.Calls)
	}
}

func TestGenerics(t *testing.T) {
	f := parse(t, src)
	if g := byName(f, "Get"); g.Cls != "Box" || g.Recv != "" {
		t.Errorf("Get = %+v", g)
	}
	p := byName(f, "Put")
	if p.Cls != "Box" || p.Recv != "b" {
		t.Errorf("Put = %+v", p)
	}
	want := []lang.Call{{Name: "Map"}, {Name: "Run", Recv: "?"}}
	if !reflect.DeepEqual(p.Calls, want) {
		t.Errorf("calls = %+v", p.Calls)
	}
	if m := byName(f, "Map"); m.Sig != "func Map[T any](v T) T" || m.Cls != "" {
		t.Errorf("Map = %+v", m)
	}
}

func TestSkipsBodiless(t *testing.T) {
	if byName(parse(t, src), "decl").Name != "" {
		t.Error("declaration without body must be skipped")
	}
}

func TestBrokenInput(t *testing.T) {
	f := parse(t, src)
	if byName(f, "Do").Name == "" {
		t.Error("intact functions must survive a syntax error")
	}
	if _, err := New().Parse("x.go", []byte("")); err != nil {
		t.Fatal(err)
	}
}

func TestSigClipped(t *testing.T) {
	f := parse(t, "package a\nfunc F("+strings.Repeat("a int, ", 60)+") {\n}\n")
	if len(f.Funcs) != 1 || len(f.Funcs[0].Sig) != 200 {
		t.Errorf("funcs = %+v", f.Funcs)
	}
}

func TestTypesAndParams(t *testing.T) {
	f := parse(t, `package svc

type (
	Service struct {
		certs  *kaspi.CertificateService
		repo   Repo
		a, b   Box[int]
		items  []Item
		sync.Mutex
		hook   func()
	}
	Repo interface {
		Save(ctx context.Context) error
		Load() int
		io.Closer
	}
	ID string
)

func (s *Service) Do(ctx context.Context, r Repo, p *pkg.Plan, xs []Item, n, m int) {}
`)
	want := map[string]lang.TypeDecl{
		"Service": {Name: "Service", Kind: "struct", Fields: []lang.Field{
			{Name: "certs", Type: "CertificateService", Pkg: "kaspi"},
			{Name: "repo", Type: "Repo"},
			{Name: "a", Type: "Box"}, {Name: "b", Type: "Box"},
			{Type: "Mutex", Pkg: "sync"},
		}},
		"Repo": {Name: "Repo", Kind: "interface", Methods: []string{"Save", "Load"}, Embeds: []lang.Field{{Type: "Closer", Pkg: "io"}}},
		"ID":   {Name: "ID", Kind: "other"},
	}
	if len(f.Types) != len(want) {
		t.Fatalf("types = %+v", f.Types)
	}
	for _, d := range f.Types {
		if !reflect.DeepEqual(d, want[d.Name]) {
			t.Errorf("%s = %+v, want %+v", d.Name, d, want[d.Name])
		}
	}
	got := byName(f, "Do").Params
	wp := []lang.Field{{Name: "ctx", Type: "Context", Pkg: "context"}, {Name: "r", Type: "Repo"}, {Name: "p", Type: "Plan", Pkg: "pkg"}, {Name: "n", Type: "int"}, {Name: "m", Type: "int"}}
	if !reflect.DeepEqual(got, wp) {
		t.Errorf("params = %+v", got)
	}
}

func TestDocSkipsArchMarkup(t *testing.T) {
	f := parse(t, "package p\n\n// Pay charges the cart.\n// arch:step pay \"ask QR\"\n// arch:ext Kaspi\nfunc Pay() {}\n")
	if d := byName(f, "Pay").Doc; d != "Pay charges the cart." {
		t.Errorf("doc = %q", d)
	}
}

func TestDocSkipsDirectives(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"nolint after prose", "package p\n\n// Pay charges the cart.\n//nolint:gocyclo\nfunc Pay() {}\n", "Pay charges the cart."},
		{"nolint before prose", "package p\n\n// nolint:funlen\n// Pay charges the cart.\nfunc Pay() {}\n", "Pay charges the cart."},
		{"go directive", "package p\n\n// Pay charges the cart.\n//go:noinline\nfunc Pay() {}\n", "Pay charges the cart."},
		{"lint ignore and arch", "package p\n\n//lint:ignore U1000 kept\n// arch:flow pay \"Pay\"\nfunc Pay() {}\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if d := byName(parse(t, c.src), "Pay").Doc; d != c.want {
				t.Errorf("doc = %q, want %q", d, c.want)
			}
		})
	}
}
