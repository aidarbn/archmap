// Package lint checks the `arch:` scenario markup of a repository: syntax,
// references between flows and reachability of steps along the call graph.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/annot"
	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
)

// Problem is one finding.
type Problem struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

func (p Problem) String() string { return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Msg) }

// Result is the outcome of a lint run.
type Result struct {
	Problems  []Problem `json:"problems"`
	Annotated int       `json:"annotated"` // functions with arch: markup
	Flows     int       `json:"flows"`     // distinct declared flow ids
}

// Run checks the markup of the functions in f; root is the repository root the
// file paths of f are relative to.
func Run(root string, f *facts.Facts) Result {
	var res Result
	add := func(file string, line int, format string, a ...any) {
		res.Problems = append(res.Problems, Problem{file, line, fmt.Sprintf(format, a...)})
	}
	fids := make([]string, 0, len(f.Functions))
	for fid := range f.Functions {
		fids = append(fids, fid)
	}
	sort.Strings(fids)

	src := map[string][]string{}
	lines := func(file string) []string {
		if l, ok := src[file]; ok {
			return l
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		var l []string
		if err == nil {
			l = strings.Split(string(b), "\n")
		}
		src[file] = l
		return l
	}
	sites := map[string][]annot.Line{}
	for _, fid := range fids {
		fn := f.Functions[fid]
		bl := annot.Block(lines(fn.File), fn.Start)
		sites[fid] = bl
		if _, errs := annot.Parse(bl); len(errs) > 0 {
			for _, e := range errs {
				add(fn.File, e.Line, "%s", e.Msg)
			}
		}
	}
	// where a directive sits: the first comment line holding all of the parts
	at := func(fid string, parts ...string) int {
		for _, l := range sites[fid] {
			ok := strings.Contains(l.Text, annot.Prefix)
			for _, p := range parts {
				ok = ok && strings.Contains(l.Text, p)
			}
			if ok {
				return l.N
			}
		}
		return f.Functions[fid].Start
	}

	// declared flows
	declaredAt := map[string]string{} // flow id -> entry fid
	for _, fid := range fids {
		if !f.Functions[fid].Arch.Empty() {
			res.Annotated++
		}
	}
	for _, fid := range autosem.DeclOrder(f) {
		fn := f.Functions[fid]
		for _, d := range fn.Arch.Flows {
			if prev, dup := declaredAt[d.ID]; dup {
				add(fn.File, at(fid, "arch:flow", " "+d.ID), "flow %q is already declared at %s", d.ID, prev)
				continue
			}
			declaredAt[d.ID] = fmt.Sprintf("%s:%d", fn.File, at(fid, "arch:flow", " "+d.ID))
			res.Flows++
		}
	}
	entryOf := map[string]string{}
	for _, fid := range autosem.DeclOrder(f) {
		for _, d := range f.Functions[fid].Arch.Flows {
			if _, ok := entryOf[d.ID]; !ok {
				entryOf[d.ID] = fid
			}
		}
	}
	reach := map[string]map[string]bool{}
	for _, fid := range fids {
		fn := f.Functions[fid]
		a := fn.Arch
		if a == nil {
			continue
		}
		for _, s := range a.Steps {
			entry, ok := entryOf[s.Flow]
			if !ok {
				add(fn.File, at(fid, "arch:step", " "+s.Flow), "arch:step refers to flow %q, which has no arch:flow (no entry point)", s.Flow)
				continue
			}
			if entry == fid {
				continue
			}
			if reach[entry] == nil {
				reach[entry] = autosem.Reachable(f, entry)
			}
			if !reach[entry][fid] {
				add(fn.File, at(fid, "arch:step", " "+s.Flow), "step of flow %q is not reachable by calls from its entry point %s", s.Flow, f.Functions[entry].Name)
			}
		}
		for _, o := range a.On {
			if o.Flow != "" {
				if _, ok := entryOf[o.Flow]; !ok {
					add(fn.File, at(fid, "arch:on", o.Cond), "arch:on -> %q: no such flow", o.Flow)
				}
			}
		}
		for _, e := range a.Ext {
			if strings.ContainsAny(e, ":>\"") {
				add(fn.File, at(fid, "arch:ext"), "arch:ext: %q does not look like a system name", e)
			}
		}
		for _, s := range a.State {
			if s.From == s.To {
				add(fn.File, at(fid, "arch:state", s.Entity), "arch:state %s: %q -> %q changes nothing", s.Entity, s.From, s.To)
			}
		}
	}
	sort.SliceStable(res.Problems, func(i, j int) bool {
		a, b := res.Problems[i], res.Problems[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	if res.Problems == nil {
		res.Problems = []Problem{}
	}
	return res
}
