package extract

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/gitstat"
	"github.com/aidarbn/archmap/internal/walk"
)

// apiCtx is what the endpoint and data detectors work on: the repository text
// and the functions found by the language parsers.
type apiCtx struct {
	root    string
	paths   []string // every file of the repository (slash paths), junk directories excluded
	fns     map[string]*fn
	order   []string
	entries map[string]*facts.Entry

	text      map[string]string
	byName    map[string][]string
	byCls     map[string][]string // class -> fids of its methods
	byFile    map[string][]string
	schema    *facts.DataModel
	modelTbl  map[string]string // ORM model name -> table
	sqlcQ     map[string][]facts.Access
	sqlcNames []string
	pyClasses map[string][]*pyClass
	djObjRE   *regexp.Regexp
	prismaAcc map[string]string
	ormRepo   map[string]string
}

var apiSkipDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, "dist": true, "build": true, ".next": true,
	"__pycache__": true, ".venv": true, "venv": true, "Pods": true, "target": true, "site-packages": true,
	"coverage": true, "DerivedData": true, "storybook-static": true,
}

func newAPICtx(root string, fns map[string]*fn, order []string, entries map[string]*facts.Entry) *apiCtx {
	c := &apiCtx{root: root, fns: fns, order: order, entries: entries, text: map[string]string{},
		byName: map[string][]string{}, byCls: map[string][]string{}, byFile: map[string][]string{}}
	for _, fid := range order {
		f := fns[fid]
		c.byName[f.Name] = append(c.byName[f.Name], fid)
		if f.Cls != "" {
			c.byCls[f.Cls] = append(c.byCls[f.Cls], fid)
		}
		c.byFile[f.file] = append(c.byFile[f.file], fid)
	}
	var all []string
	if out, ok := gitstat.Git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard"); ok {
		for _, p := range strings.Split(out, "\x00") {
			if p != "" {
				all = append(all, p)
			}
		}
	} else {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != root && (apiSkipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if rel, err := filepath.Rel(root, p); err == nil {
				all = append(all, rel)
			}
			return nil
		})
	}
	for _, p := range all {
		p = filepath.ToSlash(p)
		skip := false
		parts := strings.Split(p, "/")
		for _, part := range parts[:len(parts)-1] {
			if apiSkipDirs[part] || strings.HasPrefix(part, ".") {
				skip = true
				break
			}
		}
		if !skip {
			c.paths = append(c.paths, p)
		}
	}
	sort.Strings(c.paths)
	return c
}

// read returns the text of a repository file ("" when unreadable); results are cached.
func (c *apiCtx) read(p string) string {
	if t, ok := c.text[p]; ok {
		return t
	}
	b, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(p)))
	if err != nil || len(b) > 2<<20 {
		c.text[p] = ""
		return ""
	}
	c.text[p] = string(b)
	return c.text[p]
}

// pathsWhere lists the repository files accepted by ok.
func (c *apiCtx) pathsWhere(ok func(string) bool) []string {
	var out []string
	for _, p := range c.paths {
		if ok(p) {
			out = append(out, p)
		}
	}
	return out
}

// body returns the source of a function and the 1-based line of its first line.
func (c *apiCtx) body(fid string) (string, int) {
	f := c.fns[fid]
	if f == nil {
		return "", 0
	}
	lines := strings.Split(c.read(f.file), "\n")
	s, e := f.Start, f.End
	if s < 1 || s > len(lines) {
		return "", 0
	}
	if e > len(lines) || e < s {
		e = len(lines)
	}
	return strings.Join(lines[s-1:e], "\n"), s
}

// handler resolves a handler expression (`h.Create`, `views.list`, `Create`) written in file to a fid.
func (c *apiCtx) handler(file, expr string) string {
	expr = strings.TrimSpace(expr)
	parts := strings.Split(expr, ".")
	name := parts[len(parts)-1]
	if name == "" {
		return ""
	}
	cands := c.byName[name]
	if len(cands) == 0 {
		return ""
	}
	if len(parts) > 1 {
		recv := strings.ToLower(parts[len(parts)-2])
		var m []string
		for _, id := range cands {
			cl := strings.ToLower(c.fns[id].Cls)
			if cl != "" && (cl == recv || strings.Contains(cl, recv) || strings.Contains(recv, cl)) {
				m = append(m, id)
			}
		}
		if len(m) == 1 {
			return m[0]
		}
		if len(m) > 1 {
			cands = m
		}
	}
	pick := func(keep func(*fn) bool) []string {
		var m []string
		for _, id := range cands {
			if keep(c.fns[id]) {
				m = append(m, id)
			}
		}
		return m
	}
	if m := pick(func(f *fn) bool { return f.file == file }); len(m) == 1 {
		return m[0]
	}
	mod := facts.ModuleOf(file)
	if m := pick(func(f *fn) bool { return f.module == mod }); len(m) == 1 {
		return m[0]
	}
	if len(cands) == 1 {
		return cands[0]
	}
	return ""
}

func lineAt(text string, off int) int { return 1 + strings.Count(text[:off], "\n") }

// stripComments blanks C-style comments (keeping line numbers) in text.
var cCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)

func blankKeepLines(s string) string {
	return cCommentRE.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("\n", strings.Count(m, "\n"))
	})
}

func isCodeComment(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "--")
}

func joinPath(a, b string) string {
	if b == "" {
		if a == "" {
			return "/"
		}
		if !strings.HasPrefix(a, "/") {
			return "/" + a
		}
		return a
	}
	s := strings.TrimRight(a, "/") + "/" + strings.TrimLeft(b, "/")
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}

func isTestPath(p string) bool { return walk.IsTest(p) }

var _ = path.Join

// analyzeAPIData runs the endpoint and data detectors.
func analyzeAPIData(c *apiCtx) ([]facts.Endpoint, *facts.DataModel, map[string][]facts.Access) {
	c.schema = c.buildSchema()
	access := c.buildAccess()
	eps := c.buildEndpoints()
	var dm *facts.DataModel
	if len(c.schema.Tables) > 0 {
		dm = c.schema
	}
	return eps, dm, access
}
