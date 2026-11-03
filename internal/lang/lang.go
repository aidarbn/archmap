// Package lang defines the contract between language parsers and the extract
// orchestrator.
//
// # Division of labour
//
// A Parser is purely syntactic and per file. For one file it returns:
//
//   - every function and method (Func): name, owning class/receiver type, 1-based
//     inclusive line range, one-line signature, first doc line, the calls made in
//     its body BY NAME (Call), plus decorators/bases where the language has them;
//   - the file's imports (Import);
//   - optionally DefaultExport (JS/TS: name of the `export default` symbol).
//
// A Parser never resolves anything across files and never looks at the file
// system, git or other parsers. It must not panic on broken input: on a syntax
// error return what could be recovered (possibly an empty File) and a nil error;
// return a non-nil error only for internal failures (the orchestrator then treats
// the file as empty).
//
// The orchestrator (package extract) owns everything cross-file:
//
//   - function ids ("path::Class.name", duplicates get "@<start line>");
//   - call resolution, using the rules of the original arch.py: self/this/cls
//     -> methods of the own class (then of Bases); `super` -> methods of the base
//     classes; Go receiver variable (Func.Recv) -> methods of the receiver type;
//     Go `pkg.Func` -> function of the module imported as pkg; `Class.static()` ->
//     methods of that class; names in the "common method" list (get, Close, ...)
//     and calls on unknown expressions (Recv "?") are not linked; everything else
//     by proximity: same file -> same module -> imported modules; a unique,
//     "distinctive" name elsewhere; more than 3 candidates -> dropped;
//   - import -> module resolution (go.mod module paths, Python dotted/relative
//     paths, JS relative/alias paths, JVM package suffixes);
//   - entry points (HTTP routes, gRPC handlers, tasks, main, Next.js pages) from
//     Decorators/Bases/Sig and route-registration patterns in the source text;
//   - change status against the git base.
//
// # Conventions parsers must follow
//
// Func.Calls is ordered by position of appearance (the call's name, not its
// start), deduplicated by (Name, Recv), contains no language keywords
// (see Keywords) and no calls to the function's own nested definitions' bodies
// that belong to other Funcs. Recv is the source text before the final dot:
// "" for a bare call `f()`, "self"/"s.repo"/"pkg" for `self.f()`/`s.repo.f()`/
// `pkg.f()`, "?" when the receiver is not a plain dotted name (call result,
// index, literal, ...).
//
// Imports are raw strings the orchestrator resolves per language:
//
//   - python: dotted module ("a.b", relative with leading dots "..x"); for
//     `from m import n` emit both "m" and "m.n" (`from . import n` -> ".n");
//   - go: Path is the import path, Alias the explicit alias ("" if none);
//   - js: the module specifier as written ("./x", "@/lib/y", "react");
//   - kotlin: dotted package/class path without `import`; swift: module name.
package lang

// Call is a call made inside a function body, by name.
type Call struct {
	Name string
	Recv string
}

// Func is a function or method found in one file.
type Func struct {
	Name string
	Cls  string // enclosing class/struct/receiver type; "" for free functions
	// Start and End are 1-based, inclusive line numbers of the whole definition
	// (for Go/Python: through the closing line of the body).
	Start, End int
	// TStart is the first line including decorators/annotations/attributes
	// attached to the function; 0 means "same as Start". Used only for change
	// detection.
	TStart int
	Sig    string // one line, whitespace-collapsed, at most 200 chars
	Doc    string // first line of the doc comment/docstring, at most 200 chars
	Calls  []Call
	// Decorators are decorator/annotation expressions without the leading "@"
	// (python: `app.get("/x")`, `shared_task`).
	Decorators []string
	// Bases are base classes of the enclosing class as written (python).
	Bases []string
	// Recv is the receiver variable name of a Go method ("" otherwise).
	Recv string
	// Ext marks a Kotlin extension function (`fun Foo.bar()`); Cls is then the
	// extended type, not a declaring class, so the orchestrator also resolves
	// bare calls `bar()` to it.
	Ext bool
}

// Import is one import of a file; see the package documentation for the
// per-language meaning of Path.
type Import struct {
	Path  string
	Alias string // Go only
}

// File is the result of parsing one source file.
type File struct {
	Funcs   []Func
	Imports []Import
	// DefaultExport is the name of the `export default` function/component
	// (JS/TS), "" if none. Used for Next.js page detection.
	DefaultExport string
}

// Parser parses source files of one language.
type Parser interface {
	// Name is the language key written to facts.json: "python", "go", "js",
	// "swift" or "kotlin".
	Name() string
	// Extensions lists file extensions the parser handles, with the dot.
	Extensions() []string
	// Parse parses one file. path is repo-relative with forward slashes (useful
	// for dialect choice such as .tsx); it is not read from disk. Calls are made
	// sequentially, never concurrently.
	Parse(path string, src []byte) (File, error)
}

// Keywords are language keywords and builtins that look like calls and must not
// appear in Func.Calls.
var Keywords = toSet(`if for while switch return func function catch defer go select case new make len cap append
print super self this typeof sizeof await async elif lambda not and or in is assert del yield with
range map int str float bool list dict set tuple string error panic recover delete copy close else when
guard let var val fun init`)

func toSet(s string) map[string]bool {
	m := map[string]bool{}
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' || s[i] == '\n' || s[i] == '\t' {
			if start >= 0 {
				m[s[start:i]] = true
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	return m
}

// CallList accumulates calls in order, dropping duplicates and keywords.
type CallList struct {
	list []Call
	seen map[Call]bool
}

// Add appends a call unless it is a keyword or was already added.
func (c *CallList) Add(name, recv string) {
	if name == "" || (recv == "" && Keywords[name]) {
		return
	}
	k := Call{name, recv}
	if c.seen == nil {
		c.seen = map[Call]bool{}
	}
	if c.seen[k] {
		return
	}
	c.seen[k] = true
	c.list = append(c.list, k)
}

// Calls returns the accumulated calls.
func (c *CallList) Calls() []Call { return c.list }
