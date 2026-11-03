package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmokeExtractRenderMap(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	factsPath := filepath.Join(work, "facts.json")

	var out, errb bytes.Buffer
	if code := run([]string{"extract", "--root", repo, "--out", factsPath}, &out, &errb); code != 0 {
		t.Fatalf("extract exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "repo ") || !strings.Contains(out.String(), "точки входа") {
		t.Errorf("summary missing: %s", out.String())
	}
	b, err := os.ReadFile(factsPath)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("facts.json invalid: %v", err)
	}
	for _, k := range []string{"repo", "head", "branch", "base", "langs", "modules", "functions", "deleted"} {
		if _, ok := f[k]; !ok {
			t.Errorf("facts.json lacks %q", k)
		}
	}

	html := filepath.Join(work, "a.html")
	out.Reset()
	if code := run([]string{"render", "--facts", factsPath, "--out", html}, &out, &errb); code != 0 {
		t.Fatalf("render exit %d: %s", code, errb.String())
	}
	if hb, err := os.ReadFile(html); err != nil || !strings.Contains(string(hb), `"facts":{"repo"`) {
		t.Fatalf("render output wrong: %v", err)
	}

	mapped := filepath.Join(work, "m.html")
	if code := run([]string{"map", "--root", repo, "--out", mapped}, &out, &errb); code != 0 {
		t.Fatalf("map exit %d: %s", code, errb.String())
	}
	if _, err := os.Stat(mapped); err != nil {
		t.Fatal(err)
	}
}

func TestVersionServeAndUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "archmap ") {
		t.Errorf("version: %d %q", code, out.String())
	}
	errb.Reset()
	if code := run([]string{"serve", "--addr", "0.0.0.0:0"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "loopback") {
		t.Errorf("serve: %d %q", code, errb.String())
	}
	if code := run(nil, &out, &errb); code != 2 {
		t.Errorf("no args: %d", code)
	}
	if code := run([]string{"extract"}, &out, &errb); code != 1 {
		t.Errorf("extract without --out: %d", code)
	}
}
