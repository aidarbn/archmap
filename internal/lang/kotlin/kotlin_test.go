package kotlin

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

const sample = `package io.x.app

import a.b.C
import a.b.D as E
import a.b.*

/**
 * Screen of the app.
 * second line
 */
@Composable
@Preview(name = "x")
fun Screen(vm: VM = viewModel()) {
    Text("x")
    val v = foo<Int>(1)
    Screen()
}

class Repo @Inject constructor(
    private val api: Api,
) : Base(), Closeable {
    companion object {
        fun make() = Repo(Api())
    }

    // plain comment doc
    fun load(id: Long): Int {
        val r = this.fetch(id)
        x?.y!!.go()
        super.load(id)
        list.map { it.foo() }.filter { bar(it) }
        a.b().c()
        helper(
            1,
        )
        return r
    }

    private fun fetch(id: Long) = id.toInt()

    suspend fun <T> List<T>.ext(): Int = size()

    abstract fun ab(
        a: Int,
    ): Int
}

fun String.shout() = uppercase()
fun Map<String, List<Int>>?.opt(a: Int) {}

object Util {
    fun u() {
        fun local() { inner() }
        local()
    }
}

interface Svc {
    fun call(): Int
    fun def() = 1
}
`

func parse(t *testing.T, src string) lang.File {
	t.Helper()
	f, err := New().Parse("a/B.kt", []byte(src))
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
	t.Fatalf("func %q.%q not found in %+v", cls, name, f.Funcs)
	return lang.Func{}
}

func TestImports(t *testing.T) {
	f := parse(t, sample)
	want := []lang.Import{{Path: "a.b.C"}, {Path: "a.b.D", Alias: "E"}, {Path: "a.b.*"}}
	if !reflect.DeepEqual(f.Imports, want) {
		t.Fatalf("imports = %+v", f.Imports)
	}
}

func TestTopLevelAnnotatedWithDoc(t *testing.T) {
	f := parse(t, sample)
	s := find(t, f, "", "Screen")
	if s.Start != 13 || s.End != 17 || s.TStart != 11 {
		t.Errorf("lines = %d-%d tstart %d", s.Start, s.End, s.TStart)
	}
	if s.Sig != "fun Screen(vm: VM = viewModel())" {
		t.Errorf("sig = %q", s.Sig)
	}
	if s.Doc != "Screen of the app." {
		t.Errorf("doc = %q", s.Doc)
	}
	if !reflect.DeepEqual(s.Decorators, []string{"Composable", `Preview(name = "x")`}) {
		t.Errorf("decorators = %q", s.Decorators)
	}
	// Text, foo<Int>(1) recovered, recursive Screen() dropped
	want := []lang.Call{{Name: "Text"}, {Name: "foo"}}
	if !reflect.DeepEqual(s.Calls, want) {
		t.Errorf("calls = %+v", s.Calls)
	}
}

func TestClassMethodCalls(t *testing.T) {
	f := parse(t, sample)
	l := find(t, f, "Repo", "load")
	if l.Start != 27 || l.End != 37 || l.Doc != "plain comment doc" {
		t.Errorf("load = %d-%d doc %q", l.Start, l.End, l.Doc)
	}
	if !reflect.DeepEqual(l.Bases, []string{"Base", "Closeable"}) {
		t.Errorf("bases = %q", l.Bases)
	}
	want := []lang.Call{
		{Name: "fetch", Recv: "this"}, {Name: "go", Recv: "x.y"}, {Name: "load", Recv: "super"},
		{Name: "map", Recv: "list"}, {Name: "foo", Recv: "it"}, {Name: "filter", Recv: "?"},
		{Name: "bar"}, {Name: "b", Recv: "a"}, {Name: "c", Recv: "?"}, {Name: "helper"},
	}
	if !reflect.DeepEqual(l.Calls, want) {
		t.Errorf("calls = %+v", l.Calls)
	}
	// expression body
	fe := find(t, f, "Repo", "fetch")
	if fe.Start != 39 || fe.End != 39 || fe.Sig != "private fun fetch(id: Long)" {
		t.Errorf("fetch = %d-%d %q", fe.Start, fe.End, fe.Sig)
	}
	// unnamed companion is transparent
	if mk := find(t, f, "Repo", "make"); mk.Start != 23 {
		t.Errorf("make = %+v", mk)
	}
}

func TestExtensionFunctions(t *testing.T) {
	f := parse(t, sample)
	if e := find(t, f, "List", "ext"); e.Sig != "suspend fun <T> List<T>.ext(): Int" || len(e.Calls) != 1 {
		t.Errorf("ext = %+v", e)
	}
	if e := find(t, f, "String", "shout"); e.Calls[0].Name != "uppercase" {
		t.Errorf("shout = %+v", e)
	}
	find(t, f, "Map", "opt")
}

func TestBodylessAndLocal(t *testing.T) {
	f := parse(t, sample)
	ab := find(t, f, "Repo", "ab")
	if ab.Start != 43 || ab.End != 45 {
		t.Errorf("abstract = %d-%d", ab.Start, ab.End)
	}
	if c := find(t, f, "Svc", "call"); c.Start != c.End {
		t.Errorf("interface fun = %+v", c)
	}
	find(t, f, "Svc", "def")
	u := find(t, f, "Util", "u")
	for _, fn := range f.Funcs {
		if fn.Name == "local" {
			t.Errorf("local fun must not be a Func")
		}
	}
	if len(u.Calls) != 2 || u.Calls[0].Name != "inner" || u.Calls[1].Name != "local" {
		t.Errorf("u calls = %+v", u.Calls)
	}
}

func TestTrailingCommaDoesNotLoseFile(t *testing.T) {
	f := parse(t, `class A {
    fun f() {
        x.handle(
            onSuccess = { it.copy(a = 1,) },
        )
    }
    fun g() {}
}`)
	find(t, f, "A", "f")
	find(t, f, "A", "g")
}

func TestBrokenInput(t *testing.T) {
	for _, src := range []string{"", "fun (((", "class {", "import"} {
		if _, err := New().Parse("x.kt", []byte(src)); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestEntryPointFields(t *testing.T) {
	f := parse(t, `class MainActivity : AppCompatActivity() {
    override fun onCreate(b: Bundle?) { super.onCreate(b) }
}
class HomeViewModel(private val r: Repo) : ViewModel() {
    fun refresh() {}
}
fun main(args: Array<String>) {}
`)
	oc := find(t, f, "MainActivity", "onCreate")
	if !reflect.DeepEqual(oc.Bases, []string{"AppCompatActivity"}) {
		t.Errorf("bases = %q", oc.Bases)
	}
	if vm := find(t, f, "HomeViewModel", "refresh"); !reflect.DeepEqual(vm.Bases, []string{"ViewModel"}) {
		t.Errorf("bases = %q", vm.Bases)
	}
	find(t, f, "", "main")
}
