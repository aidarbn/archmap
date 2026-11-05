package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/diagdiff"
)

const diffRev1 = `package main

// arch:flow pay "Pay"
func Pay() { request(); save() }

// arch:step pay "ask the bank"
func request() {}

// arch:step pay "store"
func save() {}

// arch:flow order "Order"
// arch:journey guest "Guest": order > pay > refund
func Order() {}

// arch:flow refund "Refund"
func Refund() { undo() }

// arch:step refund "undo"
func undo() {}

func main() { Pay(); Order(); Refund() }
`

// rev 2: the step function save is deleted, the entry point Refund too (its
// step undo keeps the marker, the journey still names refund).
const diffRev2 = `package main

// arch:flow pay "Pay"
func Pay() { request() }

// arch:step pay "ask the bank"
func request() {}

// arch:flow order "Order"
// arch:journey guest "Guest": order > pay > refund
func Order() {}

// arch:step refund "undo"
func undo() {}

func main() { Pay(); Order() }
`

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestDiffGitBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	write := func(src string) {
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(diffRev1)
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-q", "-m", "rev1")
	write(diffRev2)
	gitT(t, dir, "commit", "-q", "-am", "rev2")

	var out, errb bytes.Buffer
	code := run([]string{"diff", "--root", dir, "--base", "HEAD~1", "--format", "json", "--fail-on", "broken"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit %d (want 1: broken)\n%s%s", code, out.String(), errb.String())
	}
	var r diagdiff.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	got := map[string]diagdiff.Diff{}
	for _, d := range r.Diffs {
		got[d.Kind+":"+d.ID] = d
	}
	pay := got["flow:pay"]
	if pay.Status != diagdiff.Changed || len(pay.Changes) != 1 || pay.Changes[0].Code != diagdiff.StepRemoved ||
		pay.Changes[0].Reason != diagdiff.ReasonDeleted || pay.Changes[0].Func.Name != "save" || pay.Changes[0].Text != "store" {
		t.Errorf("pay: %+v", pay)
	}
	refund := got["flow:refund"]
	if refund.Status != diagdiff.Removed || refund.Reason != diagdiff.ReasonEntryDeleted || refund.Entry == nil ||
		refund.Entry.Name != "Refund" || refund.Entry.File != "main.go" || refund.Entry.Line != 17 {
		t.Errorf("refund: %+v %+v", refund, refund.Entry)
	}
	guest := got["journey:guest"]
	if guest.Status != diagdiff.Changed || len(guest.Changes) != 1 || guest.Changes[0].Flow != "refund" || guest.Changes[0].Reason != diagdiff.ReasonFlowGone {
		t.Errorf("guest: %+v", guest)
	}
	kinds := map[string]bool{}
	for _, b := range r.Broken {
		kinds[b.Kind+":"+b.Diagram] = true
	}
	if !kinds[diagdiff.BrokenJourneyFlow+":guest"] || !kinds[diagdiff.BrokenNoEntry+":refund"] || len(r.Broken) != 2 {
		t.Errorf("broken: %+v", r.Broken)
	}
	// the temporary worktree is gone
	if wt := gitT(t, dir, "worktree", "list"); strings.Count(strings.TrimSpace(wt), "\n") != 0 {
		t.Errorf("worktree left behind:\n%s", wt)
	}

	// markdown in Russian, to a file; without --fail-on the exit code is 0
	md := filepath.Join(t.TempDir(), "diff.md")
	out.Reset()
	if code := run([]string{"diff", "--root", dir, "--base", "HEAD~1", "--format", "markdown", "--lang", "ru", "--out", md}, &out, &errb); code != 0 {
		t.Fatalf("exit %d\n%s", code, errb.String())
	}
	b, err := os.ReadFile(md)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### Диаграммы archmap", "исчезло: 1 · битых: 2", "удалена функция точки входа `Refund` (main.go:17 в базе)", "Шаг пропал: «store» — `save` (функция удалена)"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in\n%s", want, b)
		}
	}

	// the same tree as its own base: nothing changed
	out.Reset()
	if code := run([]string{"diff", "--root", dir, "--base", "HEAD", "--lang", "ru"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "Диаграммы не изменились.") {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
}

func TestDiffBaseRootAndFlags(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	for d, src := range map[string]string{base: diffRev1, head: diffRev1} {
		if err := os.WriteFile(filepath.Join(d, "main.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	if code := run([]string{"diff", "--root", head, "--base-root", base}, &out, &errb); code != 0 || !strings.Contains(out.String(), "Diagrams did not change.") {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
	for _, bad := range [][]string{{"--format", "html"}, {"--lang", "kz"}, {"--fail-on", "any"}} {
		if code := run(append([]string{"diff", "--root", head, "--base-root", base}, bad...), &out, &errb); code != 2 {
			t.Errorf("%v: exit %d", bad, code)
		}
	}
	// --base outside a git repository is an error, not a crash
	if code := run([]string{"diff", "--root", head, "--base", "main"}, &out, &errb); code != 1 {
		t.Errorf("no git: exit %d", code)
	}
}
