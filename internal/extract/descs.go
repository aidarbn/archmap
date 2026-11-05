package extract

import (
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang"
)

// Descriptions of endpoints, services, actors and external systems. Every
// text comes from the code: comments of .proto files, doc comments of the
// handler functions and types, package docs and `arch:actor` markup.

// commentAbove returns the text of the comment block that ends on the line
// right above line (1-based) of lines: `//`, `#` or `/* … */` lines, without
// the comment markers. A blank line ends the block.
func commentAbove(lines []string, line int) string {
	var out []string
	inBlock := false
	for i := line - 2; i >= 0 && i < len(lines); i-- {
		t := strings.TrimSpace(lines[i])
		switch {
		case inBlock:
			if strings.HasPrefix(t, "/*") {
				inBlock = false
				t = strings.TrimPrefix(strings.TrimLeft(t, "/*"), "*")
			}
			out = append(out, strings.TrimSpace(strings.TrimLeft(t, "*")))
		case strings.HasSuffix(t, "*/"):
			t = strings.TrimSpace(strings.TrimSuffix(t, "*/"))
			if strings.HasPrefix(t, "/*") {
				t = strings.TrimLeft(t, "/*")
			} else {
				inBlock = true
			}
			out = append(out, strings.TrimSpace(strings.TrimLeft(t, "*")))
		case strings.HasPrefix(t, "//"):
			out = append(out, strings.TrimSpace(strings.TrimLeft(t, "/")))
		case strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#[") && !strings.HasPrefix(t, "#include"):
			out = append(out, strings.TrimSpace(strings.TrimLeft(t, "#")))
		default:
			i = -1
			continue
		}
	}
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return strings.Join(out, "\n")
}

// addService records a service seen in a .proto file (the first one wins; a
// later one fills only an empty description).
func (c *apiCtx) addService(name string, s facts.Service) {
	if c.services == nil {
		c.services = map[string]*facts.Service{}
	}
	if o := c.services[name]; o != nil {
		if o.Desc == "" && s.Desc != "" {
			o.Desc, o.File, o.Line = s.Desc, s.File, s.Line
		}
		return
	}
	if s.Desc != "" {
		s.DescFrom = "proto"
	}
	c.services[name] = &s
}

// describe fills the descriptions that need the whole facts: endpoints
// without one get the doc of their handler, services the doc of the type that
// implements them (else of its package), externals the doc of their client
// package, and the `arch:actor` descriptions are collected.
func describe(out *facts.Facts, protoSvcs map[string]*facts.Service, fns map[string]*fn, order []string, texts map[string]string) {
	svcs := map[string]*facts.Service{}
	for k, s := range protoSvcs {
		cp := *s
		svcs[k] = &cp
	}
	for i := range out.Endpoints {
		e := &out.Endpoints[i]
		f, ok := out.Functions[e.Fid]
		if e.Desc == "" && ok {
			e.Desc = lang.Summary(f.Doc)
		}
		if e.Service == "" || e.Kind == "task" || e.Kind == "cli" {
			continue
		}
		s := svcs[e.Service]
		if s == nil {
			s = &facts.Service{Kind: e.Kind}
			svcs[e.Service] = s
		}
		if s.Impl == "" && ok && f.Cls != nil {
			s.Impl = *f.Cls
			s.ImplFile, s.ImplLine = typeDecl(*f.Cls, f.Module, out, texts)
		}
	}
	for _, s := range svcs {
		if s.Desc != "" || s.Impl == "" {
			continue
		}
		if s.ImplFile != "" {
			if d := lang.Summary(commentAbove(strings.Split(texts[s.ImplFile], "\n"), s.ImplLine)); d != "" {
				s.Desc, s.DescFrom = d, "type"
				continue
			}
		}
		if m := facts.ModuleOf(s.ImplFile); s.ImplFile != "" && out.Modules[m].Doc != "" {
			s.Desc, s.DescFrom = out.Modules[m].Doc, "package"
		}
	}
	if len(svcs) > 0 {
		out.Services = map[string]facts.Service{}
		for k, s := range svcs {
			out.Services[k] = *s
		}
	}

	for i := range out.Externals {
		x := &out.Externals[i]
		x.Desc = externalDoc(x, out.Modules)
	}

	seen := map[string]bool{}
	for _, fid := range order {
		f := fns[fid]
		if f.arch == nil {
			continue
		}
		for _, a := range f.arch.Actors {
			k := strings.ToLower(a.Name)
			if seen[k] {
				continue
			}
			seen[k] = true
			a.File = f.file
			out.Actors = append(out.Actors, a)
		}
	}
	sort.SliceStable(out.Actors, func(i, j int) bool {
		if out.Actors[i].File != out.Actors[j].File {
			return out.Actors[i].File < out.Actors[j].File
		}
		return out.Actors[i].Line < out.Actors[j].Line
	})
}

// typeDecl finds the declaration of type name among the files of module.
func typeDecl(name, module string, out *facts.Facts, texts map[string]string) (string, int) {
	re := regexp.MustCompile(`^\s*(?:export\s+)?(?:(?:public|internal|private|open|final|abstract|data)\s+)*(?:type|class|struct|object)\s+` + regexp.QuoteMeta(name) + `\b`)
	for _, p := range out.Modules[module].Files {
		t, ok := texts[p]
		if !ok || !strings.Contains(t, name) {
			continue
		}
		for i, l := range strings.Split(t, "\n") {
			if re.MatchString(l) {
				return p, i + 1
			}
		}
	}
	return "", 0
}

// externalDoc is the package doc of the client module of x: the module named
// after the system, else its only module, else (a database) a module under db.
func externalDoc(x *facts.External, mods map[string]facts.Module) string {
	low := strings.ToLower(strings.ReplaceAll(x.Name, " ", ""))
	for _, m := range x.Modules {
		seg := strings.ToLower(m[strings.LastIndex(m, "/")+1:])
		if (seg == low || strings.Contains(seg, low)) && mods[m].Doc != "" {
			return mods[m].Doc
		}
	}
	if len(x.Modules) == 1 {
		return mods[x.Modules[0]].Doc
	}
	if x.Kind == "db" {
		for _, m := range x.Modules { // the storage module itself first
			switch strings.ToLower(m[strings.LastIndex(m, "/")+1:]) {
			case "db", "database", "postgres", "pg", "repository", "repo", "store", "storage":
				if mods[m].Doc != "" {
					return mods[m].Doc
				}
			}
		}
		for _, m := range x.Modules {
			if (strings.HasSuffix(m, "/db") || strings.Contains(m, "/db/")) && mods[m].Doc != "" {
				return mods[m].Doc
			}
		}
	}
	return ""
}
