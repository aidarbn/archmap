package gitstat

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// parseFuncs is a toy parser: a function starts at "func Name" and runs until the next one.
func parseFuncs(path, src string) []Fn {
	var out []Fn
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "func ") {
			name := strings.Fields(l)[1]
			out = append(out, Fn{ID: path + "::" + name, Name: name, Sig: l, Start: i + 1, End: len(lines)})
		}
	}
	for i := 0; i+1 < len(out); i++ {
		out[i].End = out[i+1].Start - 1
	}
	return out
}

func cfg() Config {
	return Config{
		Parse:       func(p, s string) []Fn { return parseFuncs(p, s) },
		PathOK:      func(p string) bool { return strings.HasSuffix(p, ".go") },
		IsGenerated: func(string) bool { return false },
	}
}

func compute(t *testing.T, dir, base string, files ...string) (map[string]facts.Status, []facts.Deleted) {
	t.Helper()
	per := map[string][]Fn{}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		per[f] = parseFuncs(f, string(b))
	}
	return Compute(dir, base, files, per, cfg())
}

const baseSrc = "func A\n  a1\n  a2\nfunc B\n  b1\n  b2\nfunc C\n  c1\n"

func setup(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "x.go", baseSrc)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "init")
	return dir
}

func head(t *testing.T, dir string) string {
	t.Helper()
	out, ok := Git(dir, "rev-parse", "HEAD")
	if !ok {
		t.Fatal("rev-parse failed")
	}
	return strings.TrimSpace(out)
}

func TestOnlyEditedFunctionIsModified(t *testing.T) {
	dir := setup(t)
	base := head(t, dir)
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "x.go", strings.Replace(baseSrc, "b1", "b1 changed", 1))
	git(t, dir, "commit", "-qam", "edit B")
	st, del := compute(t, dir, base, "x.go")
	if len(st) != 1 || st["x.go::B"].Status != facts.Modified || st["x.go::B"].Uncommitted {
		t.Fatalf("want only B modified+committed, got %+v", st)
	}
	if len(del) != 0 {
		t.Fatalf("unexpected deleted: %+v", del)
	}
}

func TestUncommittedAndAdded(t *testing.T) {
	dir := setup(t)
	base := head(t, dir)
	write(t, dir, "x.go", strings.Replace(baseSrc, "a1", "a1 wip", 1)+"func D\n  d1\n")
	write(t, dir, "new.go", "func N\n  n1\n")
	st, _ := compute(t, dir, base, "x.go", "new.go")
	if s := st["x.go::A"]; s.Status != facts.Modified || !s.Uncommitted {
		t.Errorf("A: %+v", s)
	}
	if s := st["x.go::D"]; s.Status != facts.Added || !s.Uncommitted {
		t.Errorf("D: %+v", s)
	}
	if s := st["new.go::N"]; s.Status != facts.Added || !s.Uncommitted {
		t.Errorf("N (untracked file): %+v", s)
	}
	if _, ok := st["x.go::B"]; ok {
		t.Errorf("B must be untouched")
	}
}

func TestDeletedFunctionAndFile(t *testing.T) {
	dir := setup(t)
	write(t, dir, "y.go", "func Y\n  y1\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "add y")
	base := head(t, dir)
	write(t, dir, "x.go", "func A\n  a1\n  a2\nfunc B\n  b1\n  b2\n") // C removed
	if err := os.Remove(filepath.Join(dir, "y.go")); err != nil {
		t.Fatal(err)
	}
	_, del := compute(t, dir, base, "x.go")
	got := map[string]bool{}
	for _, d := range del {
		got[d.File+"::"+d.Name] = true
	}
	if len(del) != 2 || !got["x.go::C"] || !got["y.go::Y"] {
		t.Fatalf("deleted = %+v", del)
	}
}

func TestMergeBaseIgnoresBaseBranchProgress(t *testing.T) {
	dir := setup(t)
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "x.go", strings.Replace(baseSrc, "c1", "c1 feat", 1))
	git(t, dir, "commit", "-qam", "feat edits C")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "x.go", strings.Replace(baseSrc, "a1", "a1 main", 1))
	git(t, dir, "commit", "-qam", "main edits A")
	git(t, dir, "checkout", "-q", "feat")

	sha, name := DefaultBase(dir)
	if name != "main" || sha == "" {
		t.Fatalf("DefaultBase = %q, %q", sha, name)
	}
	st, _ := compute(t, dir, sha, "x.go")
	if len(st) != 1 || st["x.go::C"].Status != facts.Modified {
		t.Fatalf("want only C modified, got %+v", st)
	}
	if sha2, name2 := ResolveBase(dir, "main"); sha2 != sha || name2 != "main" {
		t.Fatalf("ResolveBase = %q, %q", sha2, name2)
	}
}

// The base is the merge-base with the remote's default branch, not with the
// branch the current upstream points at.
func TestDefaultBaseIsRemoteDefaultNotUpstream(t *testing.T) {
	dir := setup(t)
	git(t, dir, "branch", "-m", "develop")
	git(t, dir, "update-ref", "refs/remotes/origin/develop", "HEAD")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "x.go", strings.Replace(baseSrc, "a1", "a1 feat", 1))
	git(t, dir, "commit", "-qam", "feat")
	// the upstream of feat is another remote branch with extra work
	git(t, dir, "update-ref", "refs/remotes/origin/feat", "HEAD")
	git(t, dir, "config", "branch.feat.remote", "origin")
	git(t, dir, "config", "branch.feat.merge", "refs/heads/feat")

	sha, name := DefaultBase(dir)
	want, _ := Git(dir, "rev-parse", "origin/develop")
	if name != "origin/develop" || sha != strings.TrimSpace(want) {
		t.Fatalf("DefaultBase = %q, %q; want origin/develop %s", sha, name, want)
	}
}

func TestDefaultBaseFallsBackToOriginMain(t *testing.T) {
	dir := setup(t)
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	if _, name := DefaultBase(dir); name != "origin/main" {
		t.Fatalf("name = %q, want origin/main", name)
	}
}

func TestHunks(t *testing.T) {
	diff := "+++ b/f.go\n@@ -1,0 +2,3 @@\n@@ -9,2 +9 @@\n@@ -20,2 +18,0 @@\n+++ /dev/null\n@@ -1 +1 @@\n"
	got := Hunks(diff)["f.go"]
	want := []Range{{2, 4}, {9, 9}, {18.5, 18.5}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hunk %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}
