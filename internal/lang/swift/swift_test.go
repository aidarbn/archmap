package swift

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

func parse(t *testing.T, src string) lang.File {
	t.Helper()
	f, err := New().Parse("A.swift", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func find(t *testing.T, f lang.File, cls, name string) lang.Func {
	t.Helper()
	for _, fn := range f.Funcs {
		if fn.Cls == cls && fn.Name == name {
			return fn
		}
	}
	t.Fatalf("func %s.%s not found in %+v", cls, name, f.Funcs)
	return lang.Func{}
}

const sample = `import Foundation
import struct Foo.Bar
import UIKit

/// Loads things.
/// More.
@objc(x) public final class Repo: Base, P {
    var computed: Int { return hidden() }
    var obs = 1 { didSet { observed() } }

    /// Fetch it
    @discardableResult
    public static func fetch<T>(_ a: T) async throws -> Int where T: P {
        self.load()
        obj.items[0].run(1).done()
        let c = { closure() }
        func inner() { deep() }
        a?.b!.c()
        return Self.make()
    }

    init?(a: Int) { setup() }
    deinit { cleanup() }
    subscript(i: Int) -> Int { get { return sub() } }
    func requirement()
}

extension Array.Foo where Element: P {
    func ext() {}
}

protocol Pr { func req(); func def() }
extension Pr { func def() { z() } }

enum E { case a; func m() {} }

func free() { Foo.bar(); foo.bar.baz(x); print("x") }
`

func TestFuncs(t *testing.T) {
	f := parse(t, sample)
	var got []string
	for _, fn := range f.Funcs {
		got = append(got, fn.Cls+"."+fn.Name)
	}
	want := []string{"Repo.fetch", "Repo.init", "Repo.deinit", "Foo.ext", "Pr.def", "E.m", ".free"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("funcs = %v, want %v", got, want)
	}
}

func TestFetchDetails(t *testing.T) {
	fn := find(t, parse(t, sample), "Repo", "fetch")
	if fn.Start != 13 || fn.TStart != 12 || fn.End != 20 {
		t.Errorf("lines = %d/%d/%d", fn.Start, fn.TStart, fn.End)
	}
	if fn.Sig != "public static func fetch<T>(_ a: T) async throws -> Int where T: P" {
		t.Errorf("sig = %q", fn.Sig)
	}
	if fn.Doc != "Fetch it" {
		t.Errorf("doc = %q", fn.Doc)
	}
	if !reflect.DeepEqual(fn.Decorators, []string{"discardableResult"}) {
		t.Errorf("decorators = %v", fn.Decorators)
	}
	wantCalls := []lang.Call{
		{Name: "load", Recv: "self"}, {Name: "run", Recv: "?"}, {Name: "done", Recv: "?"},
		{Name: "closure"}, {Name: "deep"}, {Name: "c", Recv: "a.b"}, {Name: "make", Recv: "Self"},
	}
	// `obj.items[0].run(1)` has a subscript receiver
	if !reflect.DeepEqual(fn.Calls, wantCalls) {
		t.Errorf("calls = %+v", fn.Calls)
	}
}

func TestInitDeinit(t *testing.T) {
	f := parse(t, sample)
	in := find(t, f, "Repo", "init")
	if in.Start != 22 || in.End != 22 || in.Sig != "init?(a: Int)" {
		t.Errorf("init = %+v", in)
	}
	if !reflect.DeepEqual(in.Calls, []lang.Call{{Name: "setup"}}) {
		t.Errorf("init calls = %+v", in.Calls)
	}
	if d := find(t, f, "Repo", "deinit"); d.Calls[0].Name != "cleanup" {
		t.Errorf("deinit = %+v", d)
	}
}

func TestExtensionAndFree(t *testing.T) {
	f := parse(t, sample)
	if e := find(t, f, "Foo", "ext"); e.Start != 29 {
		t.Errorf("ext = %+v", e)
	}
	free := find(t, f, "", "free")
	want := []lang.Call{{Name: "bar", Recv: "Foo"}, {Name: "baz", Recv: "foo.bar"}}
	if !reflect.DeepEqual(free.Calls, want) { // print is a keyword
		t.Errorf("free calls = %+v", free.Calls)
	}
}

func TestComputedNotFunctions(t *testing.T) {
	for _, fn := range parse(t, sample).Funcs {
		for _, c := range fn.Calls {
			if c.Name == "hidden" || c.Name == "observed" || c.Name == "sub" {
				t.Errorf("%s: call %v from a non-function body leaked", fn.Name, c)
			}
		}
	}
}

func TestImports(t *testing.T) {
	got := parse(t, sample).Imports
	want := []lang.Import{{Path: "Foundation"}, {Path: "Foo"}, {Path: "UIKit"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("imports = %+v", got)
	}
}

func TestEntryMarks(t *testing.T) {
	f := parse(t, `@main
struct Tool {
    static func main() { run() }
}
class AppDelegate: UIResponder, UIApplicationDelegate {
    func application(_ a: UIApplication, didFinishLaunchingWithOptions o: [Int: Any]?) -> Bool { true }
}
struct Plain: View { func x() {} }
`)
	if d := find(t, f, "Tool", "main").Decorators; !reflect.DeepEqual(d, []string{"type:main"}) {
		t.Errorf("Tool.main = %v", d)
	}
	if d := find(t, f, "AppDelegate", "application").Decorators; !reflect.DeepEqual(d, []string{"conforms:UIApplicationDelegate"}) {
		t.Errorf("delegate = %v", d)
	}
	if d := find(t, f, "Plain", "x"); len(d.Decorators) != 0 || len(d.Bases) != 0 {
		t.Errorf("Plain.x = %+v", d)
	}
}

func TestDocBlock(t *testing.T) {
	f := parse(t, "/**\n * Block doc.\n */\nfunc a() {}\n// MARK: x\nfunc b() {}\n")
	if d := find(t, f, "", "a").Doc; d != "Block doc." {
		t.Errorf("doc a = %q", d)
	}
	if d := find(t, f, "", "b").Doc; d != "" {
		t.Errorf("doc b = %q", d)
	}
}

func TestBrokenInput(t *testing.T) {
	f := parse(t, "class A { func ok() { x() }\n func broken( {{{ \n")
	_ = f // must not panic
	if got := parse(t, ""); len(got.Funcs) != 0 {
		t.Errorf("empty = %+v", got)
	}
}

func TestPrefixOperatorCalls(t *testing.T) {
	fn := find(t, parse(t, "func a() {\n guard !ok(x) else { return }\n if y, !s.has(1) { }\n let z: T = .make(1)\n}\n"), "", "a")
	want := []lang.Call{{Name: "ok"}, {Name: "has", Recv: "s"}}
	if !reflect.DeepEqual(fn.Calls, want) {
		t.Errorf("calls = %+v", fn.Calls)
	}
}

func TestInfixOperandCalls(t *testing.T) {
	fn := find(t, parse(t, "func a() -> Int {\n return c.reduce(0) { $0 + $1.calc() }\n}\n"), "", "a")
	want := []lang.Call{{Name: "reduce", Recv: "c"}, {Name: "calc", Recv: "$1"}}
	if !reflect.DeepEqual(fn.Calls, want) {
		t.Errorf("calls = %+v", fn.Calls)
	}
}
