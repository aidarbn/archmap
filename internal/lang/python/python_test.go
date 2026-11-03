package python

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

func parse(t *testing.T, src string) lang.File {
	t.Helper()
	f, err := New().Parse("a.py", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func byName(f lang.File, name string) lang.Func {
	for _, fn := range f.Funcs {
		if fn.Name == name {
			return fn
		}
	}
	return lang.Func{}
}

const sample = `import os, a.b as c
from . import n
from ..pkg import (x, y as z)
from m import *

@app.get("/items")
@other
async def top(a, b=helper(1)):
    """First line.

    More."""
    os.path.join(a)
    self.x()
    obj.m().n()
    f(a)
    def inner():
        deep()
    return len([1])


class A(Base, metaclass=M):
    class Inner:
        def skipped(self): pass

    @staticmethod
    def s(
        a,  # c
        b,
    ) -> int:
        return super().init(), cls.k(), self.repo.save()

    def one(self): return 1


if True:
    def cond(): pass
try:
    def t(): pass
except E:
    def h(): pass
`

func TestFuncs(t *testing.T) {
	f := parse(t, sample)
	var names []string
	for _, fn := range f.Funcs {
		names = append(names, fn.Cls+"."+fn.Name)
	}
	want := ".top A.s A.one .cond .t .h"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("funcs %q want %q", got, want)
	}
	top := byName(f, "top")
	if top.Start != 8 || top.TStart != 6 || top.End != 18 {
		t.Errorf("top bounds %d %d %d", top.Start, top.TStart, top.End)
	}
	if top.Sig != "async def top(a, b=helper(1))" {
		t.Errorf("sig %q", top.Sig)
	}
	if top.Doc != "First line." {
		t.Errorf("doc %q", top.Doc)
	}
	if !reflect.DeepEqual(top.Decorators, []string{`app.get("/items")`, "other"}) {
		t.Errorf("decorators %v", top.Decorators)
	}
	got := top.Calls
	exp := []lang.Call{{Name: "get", Recv: "app"}, {Name: "helper"}, {Name: "join", Recv: "os.path"}, {Name: "x", Recv: "self"}, {Name: "m", Recv: "obj"}, {Name: "n", Recv: "?"}, {Name: "f"}, {Name: "deep"}}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("calls %v want %v", got, exp)
	}
}

func TestClassMethods(t *testing.T) {
	f := parse(t, sample)
	s := byName(f, "s")
	if s.Cls != "A" || !reflect.DeepEqual(s.Bases, []string{"Base"}) {
		t.Errorf("cls/bases %q %v", s.Cls, s.Bases)
	}
	if s.Sig != "def s( a, # c b, ) -> int" {
		t.Errorf("sig %q", s.Sig)
	}
	if !reflect.DeepEqual(s.Decorators, []string{"staticmethod"}) || s.TStart != 25 || s.Start != 26 {
		t.Errorf("deco %v %d %d", s.Decorators, s.TStart, s.Start)
	}
	exp := []lang.Call{{Name: "init", Recv: "super"}, {Name: "k", Recv: "cls"}, {Name: "save", Recv: "self.repo"}}
	if !reflect.DeepEqual(s.Calls, exp) {
		t.Errorf("calls %v", s.Calls)
	}
	one := byName(f, "one")
	if one.Sig != "def one(self)" || one.Start != one.End {
		t.Errorf("one-liner %q %d-%d", one.Sig, one.Start, one.End)
	}
}

func TestImports(t *testing.T) {
	f := parse(t, sample)
	var got []string
	for _, i := range f.Imports {
		got = append(got, i.Path)
	}
	want := []string{"os", "a.b", ".", ".n", "..pkg", "..pkg.x", "..pkg.y", "m"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("imports %v want %v", got, want)
	}
}

func TestEndIgnoresTrailingComments(t *testing.T) {
	f := parse(t, "def f():\n    if x:\n        a()\n        # tail\n    # tail2\n\n# top\ndef g(): pass\n")
	if fn := byName(f, "f"); fn.End != 3 {
		t.Errorf("end %d want 3", fn.End)
	}
}

func TestLazyImportAndBroken(t *testing.T) {
	f := parse(t, "def f():\n    import json\n    return json.dumps(1)\n")
	if len(f.Imports) != 1 || f.Imports[0].Path != "json" {
		t.Errorf("lazy import %v", f.Imports)
	}
	f = parse(t, "def ok():\n    a()\n\ndef broken(:\n")
	if byName(f, "ok").Name != "ok" {
		t.Errorf("no recovery: %v", f.Funcs)
	}
	parse(t, "")
}
