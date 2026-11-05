package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEscapesScriptBreakouts(t *testing.T) {
	facts := `{"repo":"r</script>x","functions":{"a::f":{"sig":"</script><script>alert(1)</script>","doc":"<!--<script>"}}}`
	model := `{"title":"T <b>&","summary":"</SCRIPT>"}`
	out, err := HTML([]byte(facts), []byte(model))
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	i := strings.Index(html, "const DATA = ")
	if i < 0 {
		t.Fatal("DATA assignment not found")
	}
	line := strings.TrimSuffix(html[i:i+strings.Index(html[i:], "\n")], "\r")
	if strings.Contains(line, "<") {
		t.Errorf("raw '<' left in payload: %.200s", line)
	}
	esc := "\\" + "u003c"
	if !strings.Contains(line, esc+"/script>") || !strings.Contains(line, esc+"!--"+esc+"script>") {
		t.Errorf("payload not escaped as \\u003c: %.300s", line)
	}
	if strings.Contains(html, "alert(1)</script>") {
		t.Error("script breakout present")
	}
	if strings.Contains(html, "<!--TITLE-->") || strings.Contains(html, "/*ARCH_DATA*/null") {
		t.Error("placeholders not substituted")
	}
	if !strings.Contains(html, "<title>T &lt;b>&</title>") {
		t.Error("title not escaped")
	}
}

func TestDefaultTitleAndEmptyModel(t *testing.T) {
	out, err := HTML([]byte(`{"repo":"demo","functions":{}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<title>demo</title>") {
		t.Error("default title missing")
	}
	if !regexp.MustCompile(`const DATA = \{"facts":\{"repo":"demo","functions":\{\}\},"model":\{\}\};`).Match(out) {
		t.Errorf("unexpected payload shape")
	}
}

func TestFilesBuildsAutoModel(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "facts.json")
	if err := os.WriteFile(f, []byte(`{"repo":"demo","project":{"name":"Demo","description":"d"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "sub", "a.html")
	if err := Files(f, out, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), `"title":"Demo","summary":"d"`) {
		t.Fatalf("render output wrong: %v", err)
	}
}

// The static page must not know about serve mode: placeholder intact in the
// template, no "serve" key in the rendered data.
func TestStaticRenderHasNoServeKey(t *testing.T) {
	if strings.Count(Viewer(), "/*ARCH_DATA*/null") != 1 {
		t.Fatal("template must contain the /*ARCH_DATA*/null placeholder exactly once")
	}
	out, err := HTML([]byte(`{"repo":"r","functions":{}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	i := strings.Index(html, "const DATA = ")
	line := strings.TrimSuffix(html[i:i+strings.Index(html[i:], "\n")], "\r")
	if strings.Contains(line, `"serve"`) {
		t.Errorf("static data carries a serve key: %s", line)
	}
	if line != `const DATA = {"facts":{"repo":"r","functions":{}},"model":{}};` {
		t.Errorf("static data line changed: %s", line)
	}
	live, _ := HTMLLive([]byte(`{"repo":"r"}`), nil, Live{Seq: 3, Events: "/e", Data: "/d"})
	if !strings.Contains(string(live), `"serve":{"seq":3,"events":"/e","data":"/d"}`) {
		t.Error("live page lacks the serve key")
	}
}

// The embedded viewer is one self-contained page: CSS and scripts inlined in the
// declared order, views wrapped in blocks, no external resources.
func TestViewerAssembledFromParts(t *testing.T) {
	v := Viewer()
	for _, s := range []string{cssSlot, jsSlot} {
		if strings.Contains(v, s) {
			t.Errorf("slot %s left in the page", s)
		}
	}
	if strings.Count(v, titleTag) != 2 || strings.Count(v, dataSlot) != 1 {
		t.Error("placeholders must survive assembly: title twice, data once")
	}
	order := append(append([]string{}, coreJS...), "views/map.js", lastJS)
	last := -1
	for _, name := range order {
		i := strings.Index(v, "/* --- "+name+" --- */")
		if i < 0 {
			t.Fatalf("%s missing from the page", name)
		}
		if i < last {
			t.Errorf("%s out of order", name)
		}
		last = i
	}
	if !strings.Contains(v, "/* --- views/map.js --- */\n{\n") {
		t.Error("a view must be wrapped in its own block")
	}
	if strings.Count(strings.ToLower(v), "<script") != 1 || strings.Count(strings.ToLower(v), "</script") != 1 {
		t.Error("the page must have exactly one script block")
	}
	if strings.Index(v, "const DATA = ") > strings.Index(v, "/* --- core.js --- */") {
		t.Error("DATA must be defined before the scripts")
	}
	for _, ext := range []string{"src=\"http", "href=\"http", "@import", "url(http"} {
		if strings.Contains(v, ext) {
			t.Errorf("external resource %q in the page", ext)
		}
	}
}

func TestAssembleOrderAndChecks(t *testing.T) {
	shell := "<style-slot><!--@CSS--></style-slot>\n<script>\nconst DATA = /*ARCH_DATA*/null;\n<!--@JS-->\n</script>"
	base := fstest.MapFS{"viewer/shell.html": {Data: []byte(shell)}, "viewer/viewer.css": {Data: []byte("a{}")}, "viewer/" + lastJS: {Data: []byte("main();")}}
	for _, n := range coreJS {
		base["viewer/"+n] = &fstest.MapFile{Data: []byte("// " + n)}
	}
	base["viewer/views/b.js"] = &fstest.MapFile{Data: []byte("const x = 2;")}
	base["viewer/views/a.js"] = &fstest.MapFile{Data: []byte("const x = 1;")}
	base["viewer/views/a.css"] = &fstest.MapFile{Data: []byte(".a{}")}
	out, err := assemble(base)
	if err != nil {
		t.Fatal(err)
	}
	ia, ib, im := strings.Index(out, "views/a.js"), strings.Index(out, "views/b.js"), strings.Index(out, "main();")
	if ia < 0 || ib < ia || im < ib || strings.Index(out, "// panel.js") > ia {
		t.Errorf("wrong order:\n%s", out)
	}
	if !strings.Contains(out, "{\nconst x = 1;\n}") || !strings.Contains(out, ".a{}") {
		t.Error("view not wrapped in a block or view css missing")
	}
	bad := func(name, src string) {
		fs := fstest.MapFS{}
		for k, v := range base {
			fs[k] = v
		}
		fs[name] = &fstest.MapFile{Data: []byte(src)}
		if _, err := assemble(fs); err == nil {
			t.Errorf("%s with %q must be rejected", name, src)
		}
	}
	bad("viewer/views/c.js", "s = '</SCRIPT>';")
	bad("viewer/core.js", "x = '/*ARCH_DATA*/null';")
	bad("viewer/views/c.css", "</style>")
	bad("viewer/shell.html", "<!--@CSS--><!--@JS-->")
	bad("viewer/shell.html", "<!--@CSS--><!--@JS-->/*ARCH_DATA*/null")
}

func TestFilesNoSrcDropsBodies(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "facts.json")
	facts := `{"repo":"demo","functions":{"a.py::f":{"name":"f","cls":null,"start":1,"end":1,"sig":"","doc":"","callees":[],"file":"a.py","module":".","lang":"python","src":"SECRET-BODY"}}}`
	if err := os.WriteFile(f, []byte(facts), 0o644); err != nil {
		t.Fatal(err)
	}
	for noSrc, want := range map[bool]bool{false: true, true: false} {
		out := filepath.Join(dir, "o.html")
		if err := Files(f, out, noSrc); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(out)
		if got := strings.Contains(string(b), "SECRET-BODY"); got != want {
			t.Errorf("noSrc=%v: body present=%v, want %v", noSrc, got, want)
		}
	}
}

// A live edit must be visible: the flash lasts at least 1.5 s, a marker stays until reload, and
// prefers-reduced-motion keeps the marker without the animation.
func TestLiveFlashVisibleAndPersistent(t *testing.T) {
	v := strings.ReplaceAll(Viewer(), "\r\n", "\n")
	for _, s := range []string{"const FLASH_MS = 3200", "const SES = ", ".ldot", "g.tb[data-id]", ".api-row[data-i]", "li[data-fn]"} {
		if !strings.Contains(v, s) {
			t.Errorf("viewer lacks %q", s)
		}
	}
	i := strings.Index(v, "@media (prefers-reduced-motion: reduce) {\n  .n.lflash")
	if i < 0 || !strings.Contains(v[i:i+300], "animation: none") {
		t.Error("reduced motion must switch the flash animation off")
	}
	if strings.Contains(v, "lflashn 2.6s") || !strings.Contains(v, "lflashn 3.2s") {
		t.Error("the flash animation must last 3.2 s")
	}
}

// The minimal interface: walk, editor, the sequence dock and the scenario sheet stay in the sources but are not
// put into the page; there is no auto play, no tabs, lists or language switch in the markup — one ⌘K field.
func TestMinimalInterface(t *testing.T) {
	v := Viewer()
	for _, name := range []string{"editor", "walk", "sequence", "sheet"} {
		for _, ext := range []string{".js", ".css"} {
			if strings.Contains(v, "/* --- views/"+name+ext+" --- */") {
				t.Errorf("views/%s%s must not be in the page", name, ext)
			}
		}
	}
	for _, s := range []string{"/* --- views/api.js --- */", "/* --- views/data.js --- */", "/* --- views/status.js --- */", "flowSteps", "planMod"} {
		if !strings.Contains(v, s) {
			t.Errorf("%q missing from the page", s)
		}
	}
	for _, id := range []string{`id="tabs"`, `id="listBtn"`, `id="flowsBtn"`, `id="walkBtn"`, `id="editBtn"`, `id="langSeg"`, `id="pAuto"`, `id="pSpeed"`, `id="btnAuto"`, `id="themeBtn"`, `id="panelBtn"`} {
		if strings.Contains(v, id) {
			t.Errorf("%s must be gone from the markup", id)
		}
	}
	for _, s := range []string{"auto()", "state.playing = true", "playing = !state.playing"} {
		if strings.Contains(v, s) {
			t.Errorf("auto play left in the page: %q", s)
		}
	}
	if strings.Count(v, `id="q"`) != 1 {
		t.Error("the page must have exactly one ⌘K field")
	}
}
