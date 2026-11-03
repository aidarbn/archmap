package extract

import (
	"sort"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

// callsOf lists "Caller -> Callee" (Cls.Name) for the callees of one function.
func callsOf(f *facts.Facts, caller string) []string {
	var out []string
	for _, fn := range f.Functions {
		name := fn.Name
		if fn.Cls != nil {
			name = *fn.Cls + "." + name
		}
		if name != caller {
			continue
		}
		for _, c := range fn.Callees {
			to := f.Functions[c]
			n := to.Name
			if to.Cls != nil {
				n = *to.Cls + "." + n
			}
			out = append(out, n+"@"+to.Module)
		}
	}
	sort.Strings(out)
	return out
}

func wantCalls(t *testing.T, f *facts.Facts, caller string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := callsOf(f, caller); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s calls %v, want %v", caller, got, want)
	}
}

func TestGoFieldTypeStruct(t *testing.T) {
	f := runFiles(t, map[string]string{
		"go.mod": "module ex.com/m\n\ngo 1.22\n",
		"pay/pay.go": `package pay

import "ex.com/m/cert"

type Service struct {
	certs *cert.Service
	repo  *Repo
	db    *pgxpool.Pool
}

type Repo struct{}

func (r *Repo) Create() {}

func (s *Service) Pay() {
	s.repo.Create()
	s.certs.Revoke()
	s.db.Create()
}
`,
		"cert/cert.go": `package cert

type Service struct{}

func (c *Service) Create() {}
func (c *Service) Revoke() {}
`,
	})
	// s.repo.Create is Repo.Create, not cert's Create of the same name; the
	// pool is not project code.
	wantCalls(t, f, "Service.Pay", "Repo.Create@pay", "Service.Revoke@cert")
}

func TestGoFieldChainAndParam(t *testing.T) {
	f := runFiles(t, map[string]string{
		"go.mod": "module ex.com/m\n\ngo 1.22\n",
		"a/a.go": `package a

type Outer struct{ in *Inner }
type Inner struct{ st *Store }
type Store struct{}

func (s *Store) Save() {}

type Other struct{}

func (o *Other) Save() {}

func (o *Outer) Run(st *Store, ot Other) {
	o.in.st.Save()
	st.Save()
	ot.Save()
}
`,
	})
	wantCalls(t, f, "Outer.Run", "Store.Save@a", "Other.Save@a")
}

func TestGoFieldInterfaceImplementations(t *testing.T) {
	f := runFiles(t, map[string]string{
		"go.mod": "module ex.com/m\n\ngo 1.22\n",
		"svc/svc.go": `package svc

type Sender interface {
	Send(id int)
	Flush()
}

type Many interface{ Ping() }

type Svc struct {
	out  Sender
	many Many
}

func (s *Svc) Do() {
	s.out.Send(1)
	s.many.Ping()
}
`,
		"smtp/smtp.go": `package smtp

type Client struct{}

func (c *Client) Send(id int) {}
func (c *Client) Flush()      {}

// Partial has Send only: not an implementation.
type Partial struct{}

func (p *Partial) Send(id int) {}
`,
		"mocks/m.go": `package mocks

type MockSender struct{}

func (m *MockSender) Send(id int) {}
func (m *MockSender) Flush()      {}
`,
		"p/p.go": `package p

type A struct{}
type B struct{}
type C struct{}
type D struct{}

func (A) Ping() {}
func (B) Ping() {}
func (C) Ping() {}
func (D) Ping() {}
`,
	})
	// one implementation (Client; Partial lacks Flush, the mock is skipped);
	// four implementations of Many are ambiguous and not linked.
	wantCalls(t, f, "Svc.Do", "Client.Send@smtp")
}

func TestGoFieldEmbeddedFallsBack(t *testing.T) {
	f := runFiles(t, map[string]string{
		"go.mod": "module ex.com/m\n\ngo 1.22\n",
		"a/a.go": `package a

type Base struct{}

func (b *Base) Touch() {}

type Wrapper struct{ Base }

type User struct{ w *Wrapper }

func (u *User) Use() { u.w.Touch() }
`,
	})
	// Touch is promoted from the embedded Base: found by the by-name rules.
	wantCalls(t, f, "User.Use", "Base.Touch@a")
}
