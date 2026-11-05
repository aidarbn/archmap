// Package facts defines the facts.json format consumed by the viewer (internal/render/viewer).
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
	States    map[string]*StateMachine `json:"states,omitempty"`
	Endpoints []Endpoint               `json:"endpoints,omitempty"`
	Data      *DataModel               `json:"data,omitempty"`
	// Services are the endpoint groups (gRPC services, handler types) with
	// their descriptions, keyed by Endpoint.Service.
	Services map[string]Service `json:"services,omitempty"`
	// Actors are the actor descriptions given in the markup.
	Actors []ActorDoc `json:"actors,omitempty"`
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
	// Arch holds the scenario annotations (`arch:` comments) above the function.
	Arch *ArchNote `json:"arch,omitempty"`
	// Data lists the tables and stores the function reads or writes.
	Data []Access `json:"data,omitempty"`
}

// ArchNote is the parsed `arch:` markup of one function (see package annot).
type ArchNote struct {
	Flows []FlowDecl  `json:"flows,omitempty"`
	Steps []StepNote  `json:"steps,omitempty"`
	Ext   []string    `json:"ext,omitempty"`
	On    []OnNote    `json:"on,omitempty"`
	State []StateNote `json:"state,omitempty"`
	// Journeys declare end-to-end paths over several flows; Then appends flows
	// to a journey declared elsewhere.
	Journeys []JourneyDecl `json:"journeys,omitempty"`
	Then     []ThenNote    `json:"then,omitempty"`
	// Actors are `arch:actor <Name> "description"` lines (File is not set here).
	Actors []ActorDoc `json:"actors,omitempty"`
}

// JourneyDecl declares a journey (`arch:journey`). Flows holds the flows named
// right in the declaration: the nearest `arch:flow` of the same comment (block
// form) or the whole `a > b > c` list (list form).
type JourneyDecl struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Actor string   `json:"actor,omitempty"`
	Exit  string   `json:"exit,omitempty"`
	Flows []string `json:"flows,omitempty"`
}

// ThenNote appends flows to journey Journey (`arch:then`).
type ThenNote struct {
	Journey string   `json:"journey"`
	Flows   []string `json:"flows"`
}

// FlowDecl declares a scenario that starts at the annotated function.
type FlowDecl struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Actor string `json:"actor,omitempty"`
	Exit  string `json:"exit,omitempty"`
}

// StepNote makes the function a step of flow Flow with a caption.
type StepNote struct {
	Flow string `json:"flow"`
	Text string `json:"text,omitempty"`
}

// OnNote is a branch or outcome of the step: either a jump to flow Flow or a text.
type OnNote struct {
	Cond string `json:"cond"`
	Flow string `json:"flow,omitempty"`
	Text string `json:"text,omitempty"`
}

// StateNote is a state transition of an entity.
type StateNote struct {
	Entity string `json:"entity"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Empty reports a note with no content.
func (a *ArchNote) Empty() bool {
	return a == nil || len(a.Flows)+len(a.Steps)+len(a.Ext)+len(a.On)+len(a.State)+len(a.Journeys)+len(a.Then)+len(a.Actors) == 0
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
	// Desc is the package doc of the client module of the system, when it has one.
	Desc string `json:"desc,omitempty"`
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
	// Confirmed is set on a code transition that is also declared with arch:state.
	Confirmed bool `json:"confirmed,omitempty"`
	// On, Label and Drift come from the .arch specification (see internal/spec):
	// the trigger and caption of the declared transition; Drift is
	// "not_implemented" (Source "spec": declared, absent in the code) or
	// "undeclared" (in the code, absent in the spec).
	On    string `json:"on,omitempty"`
	Label string `json:"label,omitempty"`
	Drift string `json:"drift,omitempty"`
}
