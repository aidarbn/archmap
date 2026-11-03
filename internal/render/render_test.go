package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
	line := html[i : i+strings.Index(html[i:], "\n")]
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
	if !strings.Contains(string(out), "<title>demo — архитектура</title>") {
		t.Error("default title missing")
	}
	if !regexp.MustCompile(`const DATA = \{"facts":\{"repo":"demo","functions":\{\}\},"model":\{\}\};`).Match(out) {
		t.Errorf("unexpected payload shape")
	}
}

func TestFilesMissingModelIsEmpty(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "facts.json")
	if err := os.WriteFile(f, []byte(`{"repo":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "sub", "a.html")
	if err := Files(f, filepath.Join(dir, "nope.json"), out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), `"model":{}`) {
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
	line := html[i : i+strings.Index(html[i:], "\n")]
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
