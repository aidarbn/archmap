// Package facts defines the facts.json format consumed by viewer.html.
//
// The format is a compatibility contract with the original Python tool: field
// names, nullability and omitted-when-empty rules must not change.
package facts

import "path"

// Facts is the root of facts.json.
type Facts struct {
	Repo   string         `json:"repo"`
	Head   string         `json:"head"`
	Branch string         `json:"branch"`
	Base   *string        `json:"base"` // null when no base could be determined
	Langs  map[string]int `json:"langs"`
	// Unparsed counts files of code languages without a grammar by extension;
	// they are listed in Modules[].Files without functions.
	Unparsed  map[string]int      `json:"unparsed,omitempty"`
	Modules   map[string]Module   `json:"modules"`
	Functions map[string]Function `json:"functions"`
	Deleted   []Deleted           `json:"deleted"`
	Project   *Project            `json:"project,omitempty"`
	// Externals are the outside systems the code talks to (databases, queues,
	// payment and POS providers, ...), found from adapter modules and imports.
	Externals []External `json:"externals,omitempty"`
	// States are state machines found in the code, keyed by the type or enum name.
	States map[string]*StateMachine `json:"states,omitempty"`
}

// Project describes the analysed repository as a whole.
type Project struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Module is a directory that contains at least one source file.
type Module struct {
	Imports []string `json:"imports"`
	Files   []string `json:"files"`
	// Doc is the first 1-2 sentences of the module description taken from the
	// code (package comment, docstring, README); at most 300 characters.
	Doc string `json:"doc,omitempty"`
}

// Function is a function or method. Its map key (fid) is
// "path/file.ext::Class.method" (or "path/file.ext::name").
type Function struct {
	Name    string   `json:"name"`
	Cls     *string  `json:"cls"` // null for free functions
	Start   int      `json:"start"`
	End     int      `json:"end"`
	Sig     string   `json:"sig"`
	Doc     string   `json:"doc"`
	Callees []string `json:"callees"`
	// AsyncCallees are handlers of jobs this function puts on a queue
	// (enqueue -> worker); drawn apart from synchronous calls.
	AsyncCallees []string `json:"async_callees,omitempty"`
	File         string   `json:"file"`
	Module       string   `json:"module"`
	Lang         string   `json:"lang"`
	Entry        *Entry   `json:"entry,omitempty"`
	Status       string   `json:"status,omitempty"`      // added | modified
	Uncommitted  *bool    `json:"uncommitted,omitempty"` // set together with Status
	Decorators   []string `json:"decorators,omitempty"`
	Bases        []string `json:"bases,omitempty"`
	// Src is the function body as written, cut to 60 lines with a note.
	Src string `json:"src,omitempty"`
}

// Entry marks a function as an entry point.
type Entry struct {
	Kind   string   `json:"kind"` // http | task | main | lifecycle | ui
	Labels []string `json:"labels"`
}

// Status is the change status of a function against the base.
type Status struct {
	Status      string // "added" or "modified"
	Uncommitted bool   // the change is not committed yet
}

// Deleted describes a function that existed in the base and is gone now.
type Deleted struct {
	File   string  `json:"file"`
	Module string  `json:"module"`
	Name   string  `json:"name"`
	Cls    *string `json:"cls"`
	Sig    string  `json:"sig"`
}

// Status values.
const (
	Added    = "added"
	Modified = "modified"
)

// ModuleOf returns the module (directory) of a slash-separated file path; "." for the root.
func ModuleOf(p string) string {
	d := path.Dir(p)
	if d == "" {
		return "."
	}
	return d
}

// StrPtr returns nil for "" and a pointer to s otherwise.
func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// External is a system outside the repository that the code talks to.
type External struct {
	Name string `json:"name"`
	// Kind is payment | pos | auth | db | cache | queue | storage | messaging | http_api | other.
	Kind    string   `json:"kind"`
	Modules []string `json:"modules"`
	// Functions are the fids of functions outside Modules that call into them.
	Functions []string `json:"functions"`
}

// StateMachine is a set of states and the transitions seen in the code.
type StateMachine struct {
	States      []string     `json:"states"`
	Transitions []Transition `json:"transitions"`
}

// Transition is a state change made by the function Fid. From is empty when the
// source state is not visible. Source is "code" (found by the extractor) or
// "annotation" (declared with arch:state).
type Transition struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Fid    string `json:"fid"`
	Source string `json:"source"`
}
