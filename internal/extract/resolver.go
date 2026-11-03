package extract

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/gitstat"
)

type goMod struct{ name, dir string }

var goModRe = regexp.MustCompile(`(?m)^module\s+(\S+)`)

// goModules lists (module path, dir) of every go.mod in the repo, longest dirs first.
func goModules(root string) []goMod {
	var res []goMod
	out, _ := gitstat.Git(root, "ls-files", "--cached", "--others", "--exclude-standard", "*go.mod")
	for _, p := range strings.Split(out, "\n") {
		if p == "" || path.Base(p) != "go.mod" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		if m := goModRe.FindSubmatch(b); m != nil {
			d := path.Dir(p)
			if d == "." {
				d = ""
			}
			res = append(res, goMod{strings.Trim(string(m[1]), `"`), d})
		}
	}
	if len(res) == 0 {
		if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
			if m := goModRe.FindSubmatch(b); m != nil {
				res = append(res, goMod{string(m[1]), ""})
			}
		}
	}
	sort.SliceStable(res, func(i, j int) bool { return len(res[i].dir) > len(res[j].dir) })
	return res
}

// resolver maps an import to a repository module (directory), or "" for external packages.
type resolver struct {
	modules map[string]bool
	gomods  []goMod
	pyfiles map[string]string
	suffix  map[string]string
}

func newResolver(files, modules []string, gomods []goMod) *resolver {
	r := &resolver{modules: map[string]bool{}, gomods: gomods, pyfiles: map[string]string{}, suffix: map[string]string{}}
	for _, m := range modules {
		r.modules[m] = true
	}
	for _, p := range files {
		if !strings.HasSuffix(p, ".py") {
			continue
		}
		key := strings.TrimSuffix(p, ".py")
		if strings.HasSuffix(key, "/__init__") {
			key = strings.TrimSuffix(key, "/__init__")
		}
		r.pyfiles[key] = facts.ModuleOf(p)
		if strings.HasPrefix(key, "src/") {
			if _, ok := r.pyfiles[key[4:]]; !ok {
				r.pyfiles[key[4:]] = facts.ModuleOf(p)
			}
		}
	}
	for _, m := range modules {
		parts := strings.Split(m, "/")
		for k := range parts {
			if len(parts)-k >= 2 {
				key := strings.Join(parts[k:], "/")
				if _, ok := r.suffix[key]; !ok {
					r.suffix[key] = m
				}
			}
		}
	}
	return r
}

func (r *resolver) goImp(imp string) string {
	for _, gm := range r.gomods {
		if imp == gm.name || strings.HasPrefix(imp, gm.name+"/") {
			rest := strings.TrimLeft(imp[len(gm.name):], "/")
			var parts []string
			for _, x := range []string{gm.dir, rest} {
				if x != "" {
					parts = append(parts, x)
				}
			}
			rel := strings.Join(parts, "/")
			if rel == "" {
				rel = "."
			}
			if r.modules[rel] {
				return rel
			}
			return ""
		}
	}
	return ""
}

func (r *resolver) python(imp, file string) string {
	var key string
	if strings.HasPrefix(imp, ".") {
		trimmed := strings.TrimLeft(imp, ".")
		lvl := len(imp) - len(trimmed)
		base := path.Dir(file)
		for i := 0; i < lvl-1; i++ {
			base = path.Dir(base)
		}
		parts := []string{base}
		for _, p := range strings.Split(trimmed, ".") {
			if p != "" {
				parts = append(parts, p)
			}
		}
		key = path.Join(parts...)
	} else {
		key = strings.ReplaceAll(imp, ".", "/")
	}
	cands := []string{key}
	if i := strings.LastIndex(key, "/"); i >= 0 {
		cands = append(cands, key[:i])
	}
	for _, k := range cands {
		if k == "" {
			continue
		}
		if m, ok := r.pyfiles[k]; ok {
			return m
		}
		if r.modules[k] && k != "." {
			return k
		}
		if !strings.HasPrefix(imp, ".") {
			if m, ok := r.suffix[k]; ok {
				return m
			}
		}
	}
	return ""
}

func (r *resolver) js(imp, file string) string {
	var c string
	pres := []string{""}
	if strings.HasPrefix(imp, ".") {
		c = path.Clean(path.Join(path.Dir(file), imp))
		if path.Dir(file) == "." {
			c = path.Clean(imp)
		}
	} else {
		c = imp
		for _, a := range []string{"@/", "~/", "#/"} {
			c = strings.TrimPrefix(c, a)
		}
		pres = []string{"", "src/", "app/"}
	}
	for _, pre := range pres {
		cc := path.Clean(pre + c)
		for _, s := range []string{cc, path.Dir(cc)} {
			if r.modules[s] {
				return s
			}
		}
	}
	return ""
}

func (r *resolver) jvm(imp string) string {
	parts := strings.Split(imp, ".")
	for cut := 0; cut <= 1; cut++ {
		if len(parts)-cut <= 0 {
			continue
		}
		key := strings.Join(parts[:len(parts)-cut], "/")
		if m, ok := r.suffix[key]; ok {
			return m
		}
		if r.modules[key] {
			return key
		}
	}
	return ""
}

func (r *resolver) resolve(imp, file, language string) string {
	switch language {
	case "go":
		return r.goImp(imp)
	case "python":
		return r.python(imp, file)
	case "js":
		return r.js(imp, file)
	case "kotlin":
		return r.jvm(imp)
	}
	return ""
}
