// Package extract is the orchestrator: it walks the repository, runs the
// language parsers, resolves calls and imports across files, finds entry points
// and attaches change status, producing facts.Facts.
package extract

import (
	"crypto/sha256"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/gitstat"
	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/walk"
)

// Options configure Run.
type Options struct {
	Root      string
	Base      string // git ref; "" = merge-base with the default branch
	WithTests bool
	Parsers   []lang.Parser
	// Cache, if set, remembers parse results by file content between runs
	// (used by `archmap serve` so a rebuild re-parses only edited files).
	Cache *ParseCache
}

// ParseCache keeps parsed files keyed by path and a hash of their content.
// It is not safe for concurrent runs.
type ParseCache struct{ m map[string]cached }

type cached struct {
	sum [sha256.Size]byte
	f   lang.File
}

// NewParseCache returns an empty cache.
func NewParseCache() *ParseCache { return &ParseCache{m: map[string]cached{}} }

// fn is a parsed function plus the data the orchestrator attaches.
type fn struct {
	lang.Func
	file, module, lang string
	callees            []string
}

// commonNames are method names of standard libraries/ORMs/DOM: a call `x.get()`
// on an unknown object is not linked to a project method of the same name.
var commonNames = setOf(`get set add all first last count create update delete remove pop push append extend insert clear copy
filter exclude order_by values values_list annotate aggregate select_related prefetch_related exists only defer
get_or_create update_or_create bulk_create bulk_update save refresh_from_db is_valid validate
items keys values join split strip lower upper format replace startswith endswith encode decode find index sort
read write close open send run start stop wait cancel flush commit rollback execute fetch json text
post put patch head options dispatch list retrieve destroy partial_update
map forEach reduce some every includes slice splice concat then catch finally has toString valueOf
Close Error String Is As Unwrap Len Less Swap Read Write Get Set Add Delete Run Start Stop Wait Lock Unlock
Value Done Err Next Reset Clone Init Open Flush Sync Copy Format
description contains isEmpty count first last filter map flatMap compactMap sorted reversed
toList toSet plus minus let also apply run with takeIf
addSubview removeFromSuperview layoutIfNeeded setNeedsLayout reloadData present dismiss show hide`)

var selfNames = setOf("self cls this super mcs")

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// Run builds the facts for the repository at opts.Root.
func Run(opts Options) (*facts.Facts, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	byExt := map[string]lang.Parser{}
	exts := map[string]bool{}
	for _, p := range opts.Parsers {
		for _, e := range p.Extensions() {
			byExt[e] = p
			exts[e] = true
		}
	}
	parse := func(p string, src []byte) lang.File {
		ps := byExt[path.Ext(p)]
		if ps == nil {
			return lang.File{}
		}
		var sum [sha256.Size]byte
		if opts.Cache != nil {
			sum = sha256.Sum256(src)
			if c, ok := opts.Cache.m[p]; ok && c.sum == sum {
				return c.f
			}
		}
		f, err := ps.Parse(p, src)
		if err != nil {
			return lang.File{}
		}
		if opts.Cache != nil {
			opts.Cache.m[p] = cached{sum, f}
		}
		return f
	}

	files := walk.List(root, exts, opts.WithTests)
	fns := map[string]*fn{}
	var order []string
	perFile := map[string][]gitstat.Fn{}
	rawImports := map[string]struct {
		lang string
		imps []lang.Import
	}{}
	defaultExport := map[string]string{}
	langs := map[string]int{}
	srcs := map[string]string{}
	fileLang := map[string]string{}
	for _, p := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		name := byExt[path.Ext(p)].Name()
		f := parse(p, b)
		langs[name]++
		fileLang[p] = name
		rawImports[p] = struct {
			lang string
			imps []lang.Import
		}{name, f.Imports}
		defaultExport[p] = f.DefaultExport
		if name == "go" {
			srcs[p] = string(b)
		}
		for _, pf := range f.Funcs {
			fid := p + "::"
			if pf.Cls != "" {
				fid += pf.Cls + "."
			}
			fid += pf.Name
			if _, dup := fns[fid]; dup { // overloads/duplicates are told apart by line
				fid += "@" + itoa(pf.Start)
			}
			x := &fn{Func: pf, file: p, module: facts.ModuleOf(p), lang: name}
			fns[fid] = x
			order = append(order, fid)
			perFile[p] = append(perFile[p], toGitFn(fid, pf))
		}
	}

	modSet := map[string]bool{}
	for _, p := range files {
		modSet[facts.ModuleOf(p)] = true
	}
	modules := sortedKeys(modSet)
	res := newResolver(files, modules, goModules(root))
	modImports := map[string]map[string]bool{}
	addImport := func(from, to string) {
		if modImports[from] == nil {
			modImports[from] = map[string]bool{}
		}
		modImports[from][to] = true
	}
	for p, ri := range rawImports {
		for _, imp := range ri.imps {
			if m := res.resolve(imp.Path, p, ri.lang); m != "" && m != facts.ModuleOf(p) {
				addImport(facts.ModuleOf(p), m)
			}
		}
	}
	aliases := map[string]map[string]string{}
	for p := range srcs {
		aliases[p] = goAliases(rawImports[p].imps, res)
	}

	byName := map[string][]string{}
	byCls := map[string]map[string][]string{}
	for _, fid := range order {
		f := fns[fid]
		byName[f.Name] = append(byName[f.Name], fid)
		if f.Cls != "" {
			if byCls[f.Cls] == nil {
				byCls[f.Cls] = map[string][]string{}
			}
			byCls[f.Cls][f.Name] = append(byCls[f.Cls][f.Name], fid)
		}
	}

	tiers := func(f *fn, cs []string) []string {
		var a, b, c []string
		for _, id := range cs {
			x := fns[id]
			if x.file == f.file {
				a = append(a, id)
			}
			if x.module == f.module {
				b = append(b, id)
			}
			if modImports[f.module][x.module] {
				c = append(c, id)
			}
		}
		switch {
		case len(a) > 0:
			return a
		case len(b) > 0:
			return b
		}
		return c
	}
	baseMethods := func(f *fn, name string) []string {
		for _, b := range f.Bases {
			parts := strings.Split(b, ".")
			if bc := byCls[parts[len(parts)-1]][name]; len(bc) > 0 {
				return bc
			}
		}
		return nil
	}
	ownClass := func(f *fn, name string) []string {
		if f.Cls == "" {
			return baseMethods(f, name)
		}
		cs := byCls[f.Cls][name]
		var same []string
		for _, c := range cs {
			if fns[c].file == f.file {
				same = append(same, c)
			}
		}
		if len(same) > 0 {
			return same
		}
		if len(cs) > 0 {
			return cs
		}
		return baseMethods(f, name)
	}
	without := func(ids []string, skip string) []string {
		var out []string
		for _, c := range ids {
			if c != skip {
				out = append(out, c)
			}
		}
		return out
	}
	resolveCall := func(fid string, f *fn, name, recv string) []string {
		cands := without(byName[name], fid)
		if recv == "" {
			if f.lang != "python" && f.lang != "go" && f.Cls != "" {
				if own := without(ownClass(f, name), fid); len(own) > 0 {
					return own
				}
			}
			var cs []string
			for _, c := range cands {
				if fns[c].Cls == "" {
					cs = append(cs, c)
				}
			}
			if len(cs) == 0 && f.lang == "python" { // Foo() -> Foo.__init__
				cs = byCls[name]["__init__"]
			}
			if t := tiers(f, cs); len(t) > 0 {
				return t
			}
			if len(cs) == 1 {
				return cs
			}
			return nil
		}
		if selfNames[recv] || (f.lang == "go" && f.Recv != "" && recv == f.Recv) {
			if recv == "super" {
				return baseMethods(f, name)
			}
			return without(ownClass(f, name), fid)
		}
		if f.lang == "go" && !strings.Contains(recv, ".") {
			if m, ok := aliases[f.file][recv]; ok {
				var out []string
				for _, c := range cands {
					if fns[c].module == m && fns[c].Cls == "" {
						out = append(out, c)
					}
				}
				return out
			}
		}
		if !strings.Contains(recv, ".") && unicode.IsUpper(firstRune(recv)) {
			if _, ok := byCls[recv]; ok { // Class.static()
				return byCls[recv][name]
			}
		}
		if commonNames[name] || recv == "?" {
			return nil
		}
		cs := tiers(f, cands)
		if len(cs) == 0 && len(cands) == 1 && distinctive(name) {
			cs = cands
		}
		return cs
	}

	for _, fid := range order {
		f := fns[fid]
		out := []string{}
		for _, c := range f.Calls {
			pick := resolveCall(fid, f, c.Name, c.Recv)
			if len(pick) > 3 {
				pick = nil
			}
			for _, p := range pick {
				if !contains(out, p) {
					out = append(out, p)
				}
			}
		}
		f.callees = out
		f.Calls = nil
	}

	// Swift: there are no imports inside a target, module dependencies show only through calls
	for _, fid := range order {
		if f := fns[fid]; f.lang == "swift" {
			for _, c := range f.callees {
				if m := fns[c].module; m != f.module {
					addImport(f.module, m)
				}
			}
		}
	}

	read := func(p string) (string, bool) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		return string(b), err == nil
	}
	entries := findEntries(files, read, fns, order, byName, defaultExport, func(p string) string { return fileLang[p] })

	var base, baseName string
	base, baseName = gitstat.ResolveBase(root, opts.Base)
	status := map[string]facts.Status{}
	deleted := []facts.Deleted{}
	if base != "" {
		status, deleted = gitstat.Compute(root, base, files, perFile, gitstat.Config{
			Parse: func(p string, src string) []gitstat.Fn {
				var out []gitstat.Fn
				for _, f := range parse(p, []byte(src)).Funcs {
					out = append(out, toGitFn("", f))
				}
				return out
			},
			PathOK:      func(p string) bool { return walk.PathOK(p, exts, opts.WithTests) },
			IsGenerated: walk.IsGeneratedText,
		})
	}

	out := &facts.Facts{
		Repo:      filepath.Base(root),
		Langs:     langs,
		Modules:   map[string]facts.Module{},
		Functions: map[string]facts.Function{},
		Deleted:   deleted,
	}
	if baseName != "" {
		out.Base = &baseName
	}
	if h, ok := gitstat.Git(root, "rev-parse", "--short", "HEAD"); ok {
		out.Head = strings.TrimSpace(h)
	}
	if b, ok := gitstat.Git(root, "rev-parse", "--abbrev-ref", "HEAD"); ok {
		out.Branch = strings.TrimSpace(b)
	}
	filesByMod := map[string]map[string]bool{}
	for _, fid := range order {
		f := fns[fid]
		o := facts.Function{
			Name: f.Name, Cls: facts.StrPtr(f.Cls), Start: f.Start, End: f.End,
			Sig: f.Sig, Doc: f.Doc, Callees: f.callees, File: f.file, Module: f.module, Lang: f.lang,
			Entry: entries[fid], Decorators: f.Decorators, Bases: f.Bases,
		}
		if s, ok := status[fid]; ok {
			u := s.Uncommitted
			o.Status, o.Uncommitted = s.Status, &u
		}
		out.Functions[fid] = o
		if filesByMod[f.module] == nil {
			filesByMod[f.module] = map[string]bool{}
		}
		filesByMod[f.module][f.file] = true
	}
	for _, m := range modules {
		out.Modules[m] = facts.Module{Imports: sortedKeys(modImports[m]), Files: sortedKeys(filesByMod[m])}
	}
	return out, nil
}

func toGitFn(id string, f lang.Func) gitstat.Fn {
	return gitstat.Fn{ID: id, Cls: f.Cls, Name: f.Name, Sig: f.Sig, Start: f.Start, End: f.End, TStart: f.TStart}
}

// goAliases maps package alias -> repository module for one Go file's imports.
func goAliases(imps []lang.Import, res *resolver) map[string]string {
	out := map[string]string{}
	for _, imp := range imps {
		m := res.goImp(imp.Path)
		if m == "" || imp.Alias == "_" || imp.Alias == "." {
			continue
		}
		alias := imp.Alias
		if alias == "" {
			parts := strings.Split(imp.Path, "/")
			alias = parts[len(parts)-1]
			if isMajorVersion(alias) && len(parts) > 1 {
				alias = parts[len(parts)-2]
			}
		}
		out[alias] = m
	}
	return out
}

func isMajorVersion(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func distinctive(name string) bool {
	if strings.Contains(name, "_") {
		return true
	}
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			return true
		}
	}
	return unicode.IsUpper(firstRune(name)) && utf8.RuneCountInString(name) >= 5
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }
