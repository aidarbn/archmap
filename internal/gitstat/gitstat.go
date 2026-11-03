// Package gitstat computes per-function change status (added / modified /
// uncommitted / deleted) of the working tree against the merge-base with a base
// ref. Git is invoked via exec without a shell.
package gitstat

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Git runs `git -C root args...` and returns stdout; ok is false when git fails
// or is not installed.
func Git(root string, args ...string) (out string, ok bool) {
	full := append([]string{"-c", "core.quotepath=off", "-C", root}, args...)
	cmd := exec.Command("git", full...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return buf.String(), true
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// DefaultBase returns the merge-base of HEAD with the repository's default
// branch and the ref's short name; ("", "") if none is found.
func DefaultBase(root string) (sha, name string) {
	var cands []string
	if head, ok := Git(root, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); ok && strings.TrimSpace(head) != "" {
		cands = append(cands, strings.TrimSpace(head))
	}
	cands = append(cands, "refs/remotes/origin/main", "refs/remotes/origin/master", "main", "master")
	for _, c := range cands {
		if mb, ok := Git(root, "merge-base", "HEAD", c); ok && strings.TrimSpace(mb) != "" {
			return strings.TrimSpace(mb), strings.TrimPrefix(c, "refs/remotes/")
		}
	}
	return "", ""
}

// ResolveBase turns an explicit --base ref into (merge-base sha, name); an empty
// ref falls back to DefaultBase. When the ref has no merge-base with HEAD it is
// used as is.
func ResolveBase(root, ref string) (sha, name string) {
	if ref == "" {
		return DefaultBase(root)
	}
	if mb, ok := Git(root, "merge-base", "HEAD", ref); ok && strings.TrimSpace(mb) != "" {
		return strings.TrimSpace(mb), ref
	}
	return ref, ref
}

// Range is an inclusive line range in the new version of a file. A pure
// deletion is a point between lines (s+0.5) so it does not touch a neighbour.
type Range struct{ A, B float64 }

var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// Hunks parses `git diff -U0` output into new-side line ranges per file.
func Hunks(diff string) map[string][]Range {
	res := map[string][]Range{}
	cur := ""
	for _, ln := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(ln, "+++ "):
			if strings.HasPrefix(ln, "+++ b/") {
				cur = ln[6:]
			} else {
				cur = ""
			}
		case strings.HasPrefix(ln, "@@") && cur != "":
			m := hunkRe.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			s, _ := strconv.Atoi(m[1])
			n := 1
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
			}
			if n > 0 {
				res[cur] = append(res[cur], Range{float64(s), float64(s + n - 1)})
			} else {
				res[cur] = append(res[cur], Range{float64(s) + 0.5, float64(s) + 0.5})
			}
		}
	}
	return res
}

func touches(rs []Range, a, b int) bool {
	for _, r := range rs {
		if r.A <= float64(b) && r.B >= float64(a) {
			return true
		}
	}
	return false
}

// Fn is a function as needed for change detection.
type Fn struct {
	ID         string
	Cls, Name  string
	Sig        string
	Start, End int
	TStart     int // first line including decorators; 0 = Start
}

// Config wires the language-aware pieces in without an import cycle.
type Config struct {
	// Parse returns the functions of a file's content (used for the base version).
	Parse func(path string, src string) []Fn
	// PathOK reports whether a repo-relative path belongs on the map (language,
	// skipped dirs, generated and test files).
	PathOK func(path string) bool
	// IsGenerated reports whether file content looks machine generated.
	IsGenerated func(head string) bool
}

// Compute returns the status of every changed function (keyed by fid) and the
// list of functions deleted since base. perFile maps repo-relative path to its
// current functions; only those files are considered "alive".
func Compute(root, base string, files []string, perFile map[string][]Fn, cfg Config) (map[string]facts.Status, []facts.Deleted) {
	status := map[string]facts.Status{}
	deleted := []facts.Deleted{}
	dargs := []string{"diff", "-U0", "--no-color", "--no-renames", "--no-ext-diff"}
	bd, _ := Git(root, append(dargs, base)...)
	ld, _ := Git(root, append(dargs, "HEAD")...)
	branchH, localH := Hunks(bd), Hunks(ld)
	ut, _ := Git(root, "ls-files", "--others", "--exclude-standard")
	untracked := map[string]bool{}
	for _, p := range lines(ut) {
		untracked[p] = true
	}
	lt, _ := Git(root, "ls-tree", "-r", "--name-only", base)
	inBase := map[string]bool{}
	for _, p := range lines(lt) {
		inBase[p] = true
	}
	changed := map[string]bool{}
	for p := range branchH {
		changed[p] = true
	}
	for p := range untracked {
		changed[p] = true
	}
	for _, p := range files {
		fns := perFile[p]
		if !changed[p] {
			continue
		}
		if !inBase[p] {
			for _, f := range fns {
				status[f.ID] = facts.Status{Status: facts.Added, Uncommitted: untracked[p] || touches(localH[p], f.Start, f.End)}
			}
			continue
		}
		old, _ := Git(root, "show", base+":"+p)
		oldFns := cfg.Parse(p, old)
		type key struct{ cls, name string }
		oldKeys := map[key]bool{}
		for _, f := range oldFns {
			oldKeys[key{f.Cls, f.Name}] = true
		}
		newKeys := map[key]bool{}
		for _, f := range fns {
			k := key{f.Cls, f.Name}
			newKeys[k] = true
			a := f.TStart
			if a == 0 {
				a = f.Start
			}
			touched := touches(branchH[p], a, f.End)
			local := touches(localH[p], a, f.End)
			if !oldKeys[k] {
				status[f.ID] = facts.Status{Status: facts.Added, Uncommitted: local}
			} else if touched {
				status[f.ID] = facts.Status{Status: facts.Modified, Uncommitted: local}
			}
		}
		for _, f := range oldFns {
			if !newKeys[key{f.Cls, f.Name}] {
				deleted = append(deleted, delRec(p, f))
			}
		}
	}
	// files deleted entirely, with the same filters as live ones
	gone, _ := Git(root, "diff", "--name-only", "--no-renames", "--diff-filter=D", base)
	for _, p := range lines(gone) {
		if !cfg.PathOK(p) {
			continue
		}
		old, _ := Git(root, "show", base+":"+p)
		head := old
		if len(head) > 600 {
			head = head[:600]
		}
		if cfg.IsGenerated(head) {
			continue
		}
		for _, f := range cfg.Parse(p, old) {
			deleted = append(deleted, delRec(p, f))
		}
	}
	return status, deleted
}

func delRec(p string, f Fn) facts.Deleted {
	return facts.Deleted{File: p, Module: facts.ModuleOf(p), Name: f.Name, Cls: facts.StrPtr(f.Cls), Sig: f.Sig}
}
