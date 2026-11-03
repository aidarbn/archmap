package extract

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// sourceRoots are directory names after which import paths start (Ruby $LOAD_PATH
// lib/, C include/, Dart package:x/ -> lib/, Haskell/Elixir src/ and lib/ ...).
var sourceRoots = map[string]bool{"lib": true, "src": true, "include": true, "app": true, "source": true, "sources": true}

func setFirst(m map[string]string, k, v string) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

// importStyle holds per-language tweaks of the generic path rule, keyed by
// generic.Spec.Name. Languages not listed use the defaults, so languages added
// to the generic parser are picked up without changes here.
var importStyle = map[string]struct {
	snakeCase bool // Elixir: MyApp.FooBar -> my_app/foo_bar
}{
	"elixir": {snakeCase: true},
}

// fileExts are extensions of included/required files that are not necessarily
// registered languages (C headers, shell scripts, ...).
var fileExts = map[string]bool{".h": true, ".hpp": true, ".hh": true, ".hxx": true, ".inc": true,
	".sh": true, ".bash": true, ".zsh": true, ".lua": true, ".php": true, ".dart": true, ".rb": true}

var dottedRe = regexp.MustCompile(`^[A-Za-z_][\w]*(\.[A-Za-z_][\w]*)+$`)

// pathLike is the generic import rule for languages without a dedicated one.
// The import text is normalised to a slash path:
//
//   - quotes, <>, a Dart "package:name/" prefix and a trailing ".*" / "::*" are
//     dropped; "\" (PHP namespaces) and "::" become "/"; a dotted name
//     (Elixir/Haskell/Scala/C#/Lua: a.b.C) becomes a/b/C (Elixir also
//     snake_cases CamelCase segments);
//   - a path starting with ./ or ../ is relative to the importing file; any
//     other path is tried relative to the file, then from the repo root, then
//     after a source root (lib/, src/, include/, app/ ... : Ruby $LOAD_PATH,
//     C include/ dir, Dart lib/), then as a trailing-segments suffix of a
//     file or directory;
//   - the last segment (a class inside a package) may be dropped for names
//     that are not file paths.
func (r *resolver) pathLike(imp, file, language string) string {
	s := strings.Trim(strings.TrimSpace(imp), "\"'`<>")
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "::*"), ".*"), "/*")
	if strings.HasPrefix(s, "package:") {
		s = s[len("package:"):]
		if i := strings.Index(s, "/"); i >= 0 {
			s = s[i+1:]
		}
	} else if strings.Contains(s, ":") && !strings.Contains(s, "::") {
		return "" // dart:async, http://, ...
	}
	s = strings.NewReplacer(`\`, "/", "::", "/").Replace(s)
	if s == "" || s == "*" {
		return ""
	}
	rel := strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
	isFile := fileExts[path.Ext(s)] || r.exts[path.Ext(s)]
	if !strings.Contains(s, "/") && !isFile && dottedRe.MatchString(s) {
		s = strings.ReplaceAll(s, ".", "/")
	}
	if importStyle[language].snakeCase {
		segs := strings.Split(s, "/")
		for i, g := range segs {
			segs[i] = snake(g)
		}
		s = strings.Join(segs, "/")
	}
	var cands []string
	if rel {
		cands = []string{path.Clean(path.Join(path.Dir(file), s))}
	} else {
		cands = []string{path.Clean(path.Join(path.Dir(file), s)), path.Clean(s)}
	}
	try := func(k string) string {
		if m, ok := r.files[k]; ok {
			return m
		}
		if m, ok := r.fileKeys[strings.TrimSuffix(k, path.Ext(k))]; ok && isFile {
			return m
		}
		if m, ok := r.fileKeys[k]; ok {
			return m
		}
		if r.modules[k] && k != "." {
			return k
		}
		if !rel {
			for _, mm := range []map[string]string{r.rootKeys, r.suffix, r.fileSuffix} {
				if m, ok := mm[k]; ok {
					return m
				}
				if isFile {
					if m, ok := mm[strings.TrimSuffix(k, path.Ext(k))]; ok {
						return m
					}
				}
			}
			// PSR-4/C#-style namespaces differ from directories only by case
			if m, ok := r.lowered()[strings.ToLower(k)]; ok {
				return m
			}
		}
		return ""
	}
	for _, k := range cands {
		if m := try(k); m != "" {
			return m
		}
	}
	if !isFile && !rel { // a.b.C: class inside package a.b
		if i := strings.LastIndex(s, "/"); i > 0 {
			return try(path.Clean(s[:i]))
		}
	}
	return ""
}

// lowered indexes every known path key by its lower-cased form (built lazily).
func (r *resolver) lowered() map[string]string {
	if r.low == nil {
		r.low = map[string]string{}
		for _, mm := range []map[string]string{r.rootKeys, r.suffix, r.fileSuffix, r.fileKeys} {
			for k, v := range mm {
				setFirst(r.low, strings.ToLower(k), v)
			}
		}
		for m := range r.modules {
			setFirst(r.low, strings.ToLower(m), m)
		}
	}
	return r.low
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

var cargoNameRe = regexp.MustCompile(`(?m)^name\s*=\s*"([^"]+)"`)

// cargoCrates maps crate names (dashes -> underscores) to the crate's source
// directory (<dir>/src when it holds sources, else <dir>) for every Cargo.toml.
func cargoCrates(root string, files []string) map[string]string {
	out := map[string]string{}
	dirs := map[string]bool{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".rs") {
			continue
		}
		for d := path.Dir(f); !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
			if d == "." || d == "/" {
				break
			}
		}
	}
	for d := range dirs {
		p := join(d, "Cargo.toml")
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.Index(s, "[package]")
		if i < 0 {
			continue
		}
		s = s[i:]
		if j := strings.Index(s[9:], "\n["); j >= 0 {
			s = s[:9+j]
		}
		m := cargoNameRe.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		d := path.Dir(p)
		if d == "." {
			d = ""
		}
		out[strings.ReplaceAll(m[1], "-", "_")] = d
	}
	return out
}

// rustCrateRoot is the source directory of the crate a file belongs to: the
// nearest ancestor directory holding lib.rs/main.rs, else the nearest "src".
func (r *resolver) rustCrateRoot(file string) string {
	for d := path.Dir(file); ; d = path.Dir(d) {
		for _, n := range []string{"lib.rs", "main.rs"} {
			if _, ok := r.files[join(d, n)]; ok {
				return d
			}
		}
		if path.Base(d) == "src" {
			return d
		}
		if d == "." || d == "/" || d == "" {
			return ""
		}
	}
}

func join(a, b string) string {
	if a == "" || a == "." {
		return b
	}
	return path.Join(a, b)
}

// rustModDir is the directory holding the child modules of the file's module:
// the file's own dir for mod.rs/lib.rs/main.rs, dir/stem for any other file.
func rustModDir(file string) string {
	switch path.Base(file) {
	case "mod.rs", "lib.rs", "main.rs":
		return path.Dir(file)
	}
	return join(path.Dir(file), strings.TrimSuffix(path.Base(file), ".rs"))
}

// rust resolves `crate::a::b`, `self::x` (also `mod x;`), `super::x`,
// `name::x` (a workspace crate by Cargo name) and globs `a::*`. The module is
// that of the longest prefix of the path that is a file (x.rs, x/mod.rs) or a
// directory under the base.
func (r *resolver) rust(imp, file string) string {
	imp = strings.TrimSuffix(strings.TrimSuffix(imp, "::*"), "*")
	var segs []string
	for _, g := range strings.Split(imp, "::") {
		if g != "" {
			segs = append(segs, g)
		}
	}
	if len(segs) == 0 {
		return ""
	}
	var bases []string
	atRoot := false // the path starts at a crate root: an unmatched item lives in lib.rs/main.rs
	switch segs[0] {
	case "crate":
		bases, segs, atRoot = []string{r.rustCrateRoot(file)}, segs[1:], true
	case "self":
		bases, segs = []string{rustModDir(file)}, segs[1:]
	case "super":
		b := rustModDir(file)
		for len(segs) > 0 && segs[0] == "super" {
			b, segs = path.Dir(b), segs[1:]
		}
		if b == "." {
			b = ""
		}
		bases = []string{b}
	default:
		if d, ok := r.crates[segs[0]]; ok {
			bases, segs, atRoot = []string{d}, segs[1:], true
			if _, ok := r.files[join(d, "src/lib.rs")]; ok {
				bases = []string{join(d, "src")}
			} else if _, ok := r.files[join(d, "src/main.rs")]; ok {
				bases = []string{join(d, "src")}
			}
		} else {
			bases = []string{rustModDir(file), r.rustCrateRoot(file)}
		}
	}
	for _, b := range bases {
		if len(segs) == 0 {
			if m, ok := r.files[join(b, "lib.rs")]; ok {
				return m
			}
			if m, ok := r.files[join(b, "mod.rs")]; ok {
				return m
			}
			if r.modules[b] || (b == "" && r.modules["."]) {
				if b == "" {
					return "."
				}
				return b
			}
			continue
		}
		for k := len(segs); k >= 1; k-- {
			p := join(b, strings.Join(segs[:k], "/"))
			if m, ok := r.files[p+".rs"]; ok {
				return m
			}
			if m, ok := r.files[p+"/mod.rs"]; ok {
				return m
			}
			if r.modules[p] {
				return p
			}
		}
	}
	if atRoot {
		for _, b := range bases {
			for _, n := range []string{"lib.rs", "main.rs", "mod.rs"} {
				if m, ok := r.files[join(b, n)]; ok {
					return m
				}
			}
		}
	}
	return ""
}
