package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aidarbn/archmap/internal/diagdiff"
	"github.com/aidarbn/archmap/internal/spec"
)

// cmdDiff compares the diagrams (scenarios, journeys, state machines) of the
// tree at --root with those of a base revision, for a pull request comment.
// Exit code: 0 — no changes or only changes; 1 — with --fail-on broken, the
// change breaks a diagram (or an error); 2 — bad flags.
func cmdDiff(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("diff", stderr)
	root := fs.String("root", ".", "repository root (the current tree)")
	base := fs.String("base", "origin/main", "git ref of the base; checked out into a temporary worktree")
	baseRoot := fs.String("base-root", "", "a directory with the base tree (instead of --base)")
	format := fs.String("format", "text", "output format: text | markdown | json")
	lang := fs.String("lang", "en", "language of text and markdown: en | ru")
	out := fs.String("out", "", "write the report to this file instead of stdout")
	failOn := fs.String("fail-on", "", `"broken": exit 1 when the change breaks a diagram`)
	tests := fs.Bool("tests", false, "include test files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	switch {
	case *format != "text" && *format != "markdown" && *format != "json":
		fmt.Fprintln(stderr, "archmap: --format must be text, markdown or json")
		return 2
	case *lang != diagdiff.LangEN && *lang != diagdiff.LangRU:
		fmt.Fprintln(stderr, "archmap: --lang must be en or ru")
		return 2
	case *failOn != "" && *failOn != "broken":
		fmt.Fprintln(stderr, "archmap: --fail-on must be broken")
		return 2
	}
	rep, err := diffTrees(*root, *base, *baseRoot, *tests)
	if err != nil {
		fmt.Fprintln(stderr, "archmap:", err)
		return 1
	}
	var text []byte
	switch *format {
	case "markdown":
		text = []byte(diagdiff.Markdown(rep, *lang))
	case "json":
		if text, err = diagdiff.JSON(rep); err != nil {
			fmt.Fprintln(stderr, "archmap:", err)
			return 1
		}
	default:
		text = []byte(diagdiff.Text(rep, *lang))
	}
	if *out != "" {
		err := os.MkdirAll(filepath.Dir(*out), 0o755)
		if err == nil {
			err = os.WriteFile(*out, text, 0o644)
		}
		if err != nil {
			fmt.Fprintln(stderr, "archmap:", err)
			return 1
		}
	} else {
		_, _ = stdout.Write(text)
	}
	if *failOn == "broken" && len(rep.Broken) > 0 {
		fmt.Fprintf(stderr, "archmap: the change breaks %d diagram(s)\n", len(rep.Broken))
		return 1
	}
	return 0
}

// diffTrees builds the snapshots of the base and of root and compares them.
func diffTrees(root, baseRef, baseRoot string, tests bool) (diagdiff.Report, error) {
	label := baseRoot
	if baseRoot == "" {
		dir, cleanup, err := checkoutBase(root, baseRef)
		if err != nil {
			return diagdiff.Report{}, err
		}
		defer cleanup()
		baseRoot, label = dir, baseRef
	}
	b, err := snapshot(baseRoot, tests)
	if err != nil {
		return diagdiff.Report{}, fmt.Errorf("base: %w", err)
	}
	h, err := snapshot(root, tests)
	if err != nil {
		return diagdiff.Report{}, err
	}
	rep := diagdiff.Compare(b, h)
	rep.Base, rep.Head = label, root
	return rep, nil
}

func snapshot(root string, tests bool) (*diagdiff.Snapshot, error) {
	f, err := extractFacts(root, "", tests, false)
	if err != nil {
		return nil, err
	}
	set, err := spec.Load(root)
	if err != nil {
		return nil, err
	}
	return diagdiff.Build(f, set), nil
}

// checkoutBase puts revision ref of the repository holding root into a
// temporary detached worktree and returns the directory that matches root in
// it; cleanup removes the worktree.
func checkoutBase(root, ref string) (string, func(), error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	top, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, fmt.Errorf("--base needs a git repository (or use --base-root): %w", err)
	}
	if _, err := git(abs, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return "", nil, fmt.Errorf("no such revision %q (fetch it, e.g. actions/checkout with fetch-depth: 0)", ref)
	}
	topReal, err := filepath.EvalSymlinks(top)
	if err != nil {
		return "", nil, err
	}
	absReal, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(topReal, absReal)
	if err != nil {
		return "", nil, err
	}
	tmp, err := os.MkdirTemp("", "archmap-base-")
	if err != nil {
		return "", nil, err
	}
	wt := filepath.Join(tmp, "base")
	if _, err := git(top, "worktree", "add", "--detach", "--quiet", wt, ref); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	cleanup := func() {
		_, _ = git(top, "worktree", "remove", "--force", wt)
		os.RemoveAll(tmp)
		_, _ = git(top, "worktree", "prune")
	}
	return filepath.Join(wt, rel), cleanup, nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var errb strings.Builder
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(string(out)), nil
}
