package extract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aidarbn/archmap/internal/lang"
)

// stubParser returns canned functions for a file regardless of its contents.
type stubParser struct {
	name, ext string
	funcs     []lang.Func
}

func (s stubParser) Name() string         { return s.name }
func (s stubParser) Extensions() []string { return []string{s.ext} }
func (s stubParser) Parse(string, []byte) (lang.File, error) {
	return lang.File{Funcs: s.funcs}, nil
}

func runStubs(t *testing.T, files map[string]string, ps ...lang.Parser) map[string]entryInfo {
	t.Helper()
	root := t.TempDir()
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(root, n), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	f, err := Run(Options{Root: root, Parsers: ps})
	if err != nil {
		t.Fatal(err)
	}
	res := map[string]entryInfo{}
	for id, fn := range f.Functions {
		ei := entryInfo{callees: fn.Callees}
		if fn.Entry != nil {
			ei.kind, ei.labels = fn.Entry.Kind, fn.Entry.Labels
		}
		res[id] = ei
	}
	return res
}

type entryInfo struct {
	kind    string
	labels  []string
	callees []string
}

func fnl(name, cls string, line int, calls []lang.Call, decos, bases []string) lang.Func {
	return lang.Func{Name: name, Cls: cls, Start: line, End: line, Sig: "func " + name, Calls: calls, Decorators: decos, Bases: bases}
}

func TestSwiftEntriesAndCalls(t *testing.T) {
	app := []string{"type:main", "conforms:App"}
	del := []string{"conforms:UIApplicationDelegate"}
	sw := stubParser{"swift", ".swift", []lang.Func{
		fnl("helper", "", 1, nil, nil, nil),
		fnl("foo", "MyApp", 2, nil, app, nil),
		fnl("init", "MyApp", 3, nil, app, nil),
		fnl("application", "AppDelegate", 4, nil, del, nil),
		fnl("sceneDidBecomeActive", "SceneDel", 5, nil, []string{"conforms:UIWindowSceneDelegate"}, nil),
		fnl("scene", "Other", 6, nil, nil, nil), // not a delegate
		fnl("init", "Model", 7, nil, nil, nil),
		fnl("tick", "Model", 8, nil, nil, nil),
		fnl("build", "Maker", 9, []lang.Call{{Name: "Model", Recv: ""}, {Name: "tick", Recv: "Self"}}, nil, nil),
		fnl("tick", "Maker", 10, nil, nil, nil),
	}}
	r := runStubs(t, map[string]string{"a.swift": "x\n"}, sw)
	if e := r["a.swift::MyApp.init"]; e.kind != "main" {
		t.Errorf("App init: %+v", e)
	}
	if e := r["a.swift::MyApp.foo"]; e.kind != "" {
		t.Errorf("App foo must not be an entry: %+v", e)
	}
	if e := r["a.swift::AppDelegate.application"]; e.kind != "lifecycle" || e.labels[0] != "iOS AppDelegate.application" {
		t.Errorf("delegate: %+v", e)
	}
	if e := r["a.swift::SceneDel.sceneDidBecomeActive"]; e.kind != "lifecycle" {
		t.Errorf("scene delegate: %+v", e)
	}
	if e := r["a.swift::Other.scene"]; e.kind != "" {
		t.Errorf("non-delegate scene: %+v", e)
	}
	b := r["a.swift::Maker.build"].callees
	want := map[string]bool{"a.swift::Model.init": true, "a.swift::Maker.tick": true}
	if len(b) != 2 || !want[b[0]] || !want[b[1]] {
		t.Errorf("callees = %v (Type() -> init, Self. -> own class)", b)
	}
}

func TestSwiftMainPrefersMainFunc(t *testing.T) {
	sw := stubParser{"swift", ".swift", []lang.Func{
		fnl("init", "A", 1, nil, []string{"type:main"}, nil),
		fnl("main", "A", 2, nil, []string{"type:main"}, nil),
	}}
	r := runStubs(t, map[string]string{"a.swift": "x\n"}, sw)
	if r["a.swift::A.main"].kind != "main" || r["a.swift::A.init"].kind != "" {
		t.Errorf("%+v", r)
	}
}

func TestKotlinEntriesAndExtensions(t *testing.T) {
	comp := []string{"Composable"}
	ext := fnl("shout", "String", 20, nil, nil, nil)
	ext.Ext = true
	kt := stubParser{"kotlin", ".kt", []lang.Func{
		fnl("main", "", 1, nil, nil, nil),
		fnl("onCreate", "MainActivity", 2, nil, nil, []string{"AppCompatActivity"}),
		fnl("onCreate", "Plain", 3, nil, nil, nil),
		fnl("onReceive", "Rx", 4, nil, nil, []string{"BroadcastReceiver"}),
		fnl("HomeScreen", "", 5, nil, comp, nil),
		fnl("HomeButton", "", 6, nil, comp, nil),
		fnl("load", "HomeViewModel", 7, nil, nil, []string{"ViewModel"}),
		{Name: "hidden", Cls: "HomeViewModel", Start: 8, End: 8, Sig: "private fun hidden", Bases: []string{"ViewModel"}},
		fnl("useExt", "Holder", 9, []lang.Call{{Name: "shout"}}, nil, nil),
		ext,
	}}
	r := runStubs(t, map[string]string{"a.kt": "x\n"}, kt)
	for id, kind := range map[string]string{
		"a.kt::main":                  "main",
		"a.kt::MainActivity.onCreate": "lifecycle",
		"a.kt::Plain.onCreate":        "",
		"a.kt::Rx.onReceive":          "lifecycle",
		"a.kt::HomeScreen":            "ui",
		"a.kt::HomeButton":            "",
		"a.kt::HomeViewModel.load":    "ui",
		"a.kt::HomeViewModel.hidden":  "",
	} {
		if r[id].kind != kind {
			t.Errorf("%s: kind %q, want %q", id, r[id].kind, kind)
		}
	}
	if c := r["a.kt::Holder.useExt"].callees; len(c) != 1 || c[0] != "a.kt::String.shout" {
		t.Errorf("extension call: %v", c)
	}
}
