package extract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

// fakeParser understands lines "def [Cls.]name -> call, recv.call, ..." (language "python").
type fakeParser struct{}

func (fakeParser) Name() string         { return "python" }
func (fakeParser) Extensions() []string { return []string{".py"} }
func (fakeParser) Parse(_ string, src []byte) (lang.File, error) {
	var f lang.File
	for i, l := range strings.Split(string(src), "\n") {
		switch {
		case strings.HasPrefix(l, "import "):
			f.Imports = append(f.Imports, lang.Import{Path: strings.TrimPrefix(l, "import ")})
		case strings.HasPrefix(l, "def "):
			head, calls, _ := strings.Cut(strings.TrimPrefix(l, "def "), "->")
			fn := lang.Func{Start: i + 1, End: i + 1, Sig: strings.TrimSpace(head)}
			fn.Cls, fn.Name = "", strings.TrimSpace(head)
			if c, n, ok := strings.Cut(fn.Name, "."); ok {
				fn.Cls, fn.Name = c, n
			}
			var cl lang.CallList
			for _, c := range strings.Split(calls, ",") {
				if c = strings.TrimSpace(c); c == "" {
					continue
				}
				if r, n, ok := strings.Cut(c, "."); ok {
					cl.Add(n, r)
				} else {
					cl.Add(c, "")
				}
			}
			fn.Calls = cl.Calls()
			f.Funcs = append(f.Funcs, fn)
		}
	}
	return f, nil
}

func TestRunResolvesCallsAndImports(t *testing.T) {
	root := t.TempDir()
	w := func(name, body string) {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("app/svc.py", "import lib.util\ndef Svc.run -> self.step, helper_fn, util_fn, x.get\ndef Svc.step\ndef helper_fn\n")
	w("lib/util.py", "def util_fn\ndef Svc.step\n")
	w("app/test_svc.py", "def test_x -> helper_fn\n")
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	f, err := Run(Options{Root: root, Parsers: []lang.Parser{fakeParser{}}})
	if err != nil {
		t.Fatal(err)
	}
	run := f.Functions["app/svc.py::Svc.run"]
	want := []string{"app/svc.py::Svc.step", "app/svc.py::helper_fn", "lib/util.py::util_fn"}
	if strings.Join(run.Callees, ",") != strings.Join(want, ",") {
		t.Errorf("callees = %v, want %v", run.Callees, want)
	}
	if got := f.Modules["app"].Imports; len(got) != 1 || got[0] != "lib" {
		t.Errorf("app imports = %v", got)
	}
	if _, ok := f.Functions["app/test_svc.py::test_x"]; ok {
		t.Error("test files must be skipped without --tests")
	}
	if f.Langs["python"] != 2 || f.Base != nil {
		t.Errorf("langs=%v base=%v", f.Langs, f.Base)
	}
	if !strings.Contains(Summary(f), "функций 5") {
		t.Errorf("summary: %s", Summary(f))
	}
}
