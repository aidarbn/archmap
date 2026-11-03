package extract

import (
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/lang"
)

// Go calls through typed fields and parameters.
//
// A call `s.repo.Save()` in a method of Service is resolved by the TYPE of the
// field repo, not by the name Save: the type is read from the struct
// declaration (and `a.b.M()` follows the chain of fields). A call `p.M()` on a
// parameter p is resolved by the parameter's type the same way.
//
//   - the type is a struct (or any named non-interface type) of the repository:
//     the method M of that type, nothing else; when the struct embeds other
//     types and has no M, the old by-name rules take over (M may be promoted);
//   - the type is an interface: the implementations are the repository types
//     that have every method of the interface (embedded interfaces of the
//     repository included; mocks are not implementations). One implementation
//     is linked, up to maxImpls too; more are ambiguous and not linked;
//   - the type comes from a package outside the repository (or is a builtin):
//     no link at all, the callee is not project code;
//   - the type is not found: the old by-name rules take over.
const maxImpls = 3

type typeKey struct{ module, name string }

type goType struct {
	decl lang.TypeDecl
	file string
}

type goTypes struct {
	decls   map[typeKey]goType
	methods map[typeKey]map[string]bool // type -> names of its methods
	aliases map[string]map[string]string
	impls   map[typeKey][]typeKey
}

func newGoTypes(order []string, fns map[string]*fn, decls map[typeKey]goType, aliases map[string]map[string]string) *goTypes {
	g := &goTypes{decls: decls, methods: map[typeKey]map[string]bool{}, aliases: aliases, impls: map[typeKey][]typeKey{}}
	for _, fid := range order {
		f := fns[fid]
		if f.lang != "go" || f.Cls == "" {
			continue
		}
		k := typeKey{f.module, f.Cls}
		if g.methods[k] == nil {
			g.methods[k] = map[string]bool{}
		}
		g.methods[k][f.Name] = true
	}
	return g
}

// ref is a type as written in some file: pkg.name, or name in the file's package.
type typeRef struct{ pkg, name, file, module string }

// target turns a type reference into a key; ext reports a package outside the repo.
func (g *goTypes) target(r typeRef) (k typeKey, ext bool) {
	if r.pkg == "" {
		return typeKey{r.module, r.name}, false
	}
	m, ok := g.aliases[r.file][r.pkg]
	if !ok {
		return typeKey{}, true
	}
	return typeKey{m, r.name}, false
}

// resolve finds the methods called as recv.name() in f. handled is false when
// the receiver is not a typed field/parameter chain or its type is unknown: the
// caller then falls back to name-based rules.
func (g *goTypes) resolve(f *fn, recv, name string, byCls map[string]map[string][]string, fns map[string]*fn) (out []string, handled bool) {
	parts := strings.Split(recv, ".")
	var cur typeRef
	switch root := parts[0]; {
	case f.Recv != "" && root == f.Recv && len(parts) > 1 && f.Cls != "":
		cur = typeRef{name: f.Cls, file: f.file, module: f.module}
	default:
		found := false
		for _, p := range f.Params {
			if p.Name == root {
				cur, found = typeRef{pkg: p.Pkg, name: p.Type, file: f.file, module: f.module}, true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	for _, field := range parts[1:] {
		k, ext := g.target(cur)
		if ext {
			return nil, true
		}
		t, ok := g.decls[k]
		if !ok || t.decl.Kind != "struct" {
			return nil, false
		}
		next, ok := fieldType(t.decl, field)
		if !ok {
			return nil, false
		}
		cur = typeRef{pkg: next.Pkg, name: next.Type, file: t.file, module: k.module}
	}
	k, ext := g.target(cur)
	if ext {
		return nil, true
	}
	t, ok := g.decls[k]
	if !ok {
		if g.methods[k] != nil { // methods of a type declared where we did not see it
			return methodsOf(byCls, fns, k, name), true
		}
		return nil, false
	}
	switch t.decl.Kind {
	case "interface":
		impls := g.implementations(k)
		var res []string
		for _, ik := range impls {
			res = append(res, methodsOf(byCls, fns, ik, name)...)
		}
		if len(impls) > maxImpls {
			return nil, true
		}
		return res, true
	default:
		res := methodsOf(byCls, fns, k, name)
		if len(res) == 0 && hasEmbedded(t.decl) {
			return nil, false
		}
		return res, true
	}
}

func fieldType(d lang.TypeDecl, name string) (lang.Field, bool) {
	for _, f := range d.Fields {
		if f.Name == name || (f.Name == "" && f.Type == name) {
			return f, true
		}
	}
	return lang.Field{}, false
}

func hasEmbedded(d lang.TypeDecl) bool {
	for _, f := range d.Fields {
		if f.Name == "" {
			return true
		}
	}
	return false
}

func methodsOf(byCls map[string]map[string][]string, fns map[string]*fn, k typeKey, name string) []string {
	var out []string
	for _, c := range byCls[k.name][name] {
		if fns[c].module == k.module && fns[c].lang == "go" {
			out = append(out, c)
		}
	}
	return out
}

// interfaceMethods collects the methods of an interface with the repository
// interfaces it embeds (depth-limited); ok is false for an empty method set.
func (g *goTypes) interfaceMethods(k typeKey, depth int) map[string]bool {
	out := map[string]bool{}
	t, ok := g.decls[k]
	if !ok || t.decl.Kind != "interface" || depth > 4 {
		return out
	}
	for _, m := range t.decl.Methods {
		out[m] = true
	}
	for _, e := range t.decl.Embeds {
		ek, ext := g.target(typeRef{pkg: e.Pkg, name: e.Type, file: t.file, module: k.module})
		if ext {
			continue
		}
		for m := range g.interfaceMethods(ek, depth+1) {
			out[m] = true
		}
	}
	return out
}

func isMock(k typeKey) bool {
	n := strings.ToLower(k.name)
	if strings.HasPrefix(n, "mock") || strings.HasPrefix(n, "fake") || strings.HasPrefix(n, "stub") {
		return true
	}
	for _, seg := range strings.Split(strings.ToLower(k.module), "/") {
		if strings.HasPrefix(seg, "mock") || strings.HasPrefix(seg, "fake") || seg == "stubs" {
			return true
		}
	}
	return false
}

// implementations lists the repository types that have all methods of interface k.
func (g *goTypes) implementations(k typeKey) []typeKey {
	if r, ok := g.impls[k]; ok {
		return r
	}
	need := g.interfaceMethods(k, 0)
	var res []typeKey
	if len(need) > 0 {
		for tk, have := range g.methods {
			if tk == k || isMock(tk) {
				continue
			}
			if d, ok := g.decls[tk]; ok && d.decl.Kind == "interface" {
				continue
			}
			all := true
			for m := range need {
				if !have[m] {
					all = false
					break
				}
			}
			if all {
				res = append(res, tk)
			}
		}
		sort.Slice(res, func(i, j int) bool {
			if res[i].module != res[j].module {
				return res[i].module < res[j].module
			}
			return res[i].name < res[j].name
		})
	}
	g.impls[k] = res
	return res
}
