// Package spec reads, checks, edits and writes `.arch` files: the optional
// text specification of behaviour (state machines, scenarios, journeys) that
// lives in <root>/arch/*.arch next to the code. The language says what the
// system does, never how; archmap lint compares it with the code.
//
// The types in this file are also the JSON contract for the editor in the
// viewer (see docs/spec.md). Fields marked "enrich" are filled by Check and
// are never written to files.
package spec

// Dir is the directory of spec files relative to the repository root.
const Dir = "arch"

// Ext is the extension of spec files.
const Ext = ".arch"

// Transition sources and drift values (enrich).
const (
	SourceSpec       = "spec"
	SourceCode       = "code"
	SourceAnnotation = "annotation"

	DriftNotImplemented = "not_implemented" // in the spec, not in the code
	DriftUndeclared     = "undeclared"      // in the code, not in the spec

	DriftMissing     = "missing"     // step: no such function
	DriftAmbiguous   = "ambiguous"   // step: several functions match
	DriftUnreachable = "unreachable" // step: not reachable by calls from the previous one
)

// Transition is `from -> to [on trigger] ["label"]`. From may be "*" (any state).
type Transition struct {
	From  string `json:"from"`
	To    string `json:"to"`
	On    string `json:"on,omitempty"`
	Label string `json:"label,omitempty"`
	Line  int    `json:"line,omitempty"`
	// enrich:
	Source string `json:"source,omitempty"` // spec | code | annotation
	Drift  string `json:"drift,omitempty"`  // not_implemented | undeclared
	Fid    string `json:"fid,omitempty"`    // function that makes the transition in the code
}

// States is a state machine of one entity (`states Entity`).
type States struct {
	Entity string `json:"entity"`
	// States lists states declared with `state a, b` (those that have no
	// transition); every other state is implied by Transitions and Final.
	States      []string     `json:"states"`
	Transitions []Transition `json:"transitions"`
	Final       []string     `json:"final"`
	Comment     []string     `json:"comment,omitempty"` // `#` lines right above the block, kept when the file is rewritten
	File        string       `json:"file,omitempty"`
	Line        int          `json:"line,omitempty"`
	Code        string       `json:"code,omitempty"` // enrich: key of the matching machine in facts.states
}

// Step is `step <ref> [ext System] ["caption"]` or `step plan "caption" [in module]`.
type Step struct {
	Ref     string `json:"ref,omitempty"` // Type.Method | Func | path::fid; empty for a planned step
	Planned bool   `json:"planned,omitempty"`
	Module  string `json:"module,omitempty"` // planned steps only: the module the step will live in (`in services/kaspi`)
	Ext     string `json:"ext,omitempty"`
	Text    string `json:"text,omitempty"`
	Line    int    `json:"line,omitempty"`
	// enrich:
	Fid   string `json:"fid,omitempty"`   // resolved function
	Drift string `json:"drift,omitempty"` // missing | ambiguous | unreachable
}

// Alt is `alt <cond> [-> flow] ["text"]`: a branch or an outcome of the flow.
type Alt struct {
	Cond string `json:"cond"`
	Flow string `json:"flow,omitempty"`
	Text string `json:"text,omitempty"`
	Line int    `json:"line,omitempty"`
}

// StateRef is `state Entity: from -> to`: the flow moves an entity between states.
type StateRef struct {
	Entity string `json:"entity"`
	From   string `json:"from"`
	To     string `json:"to"`
	Line   int    `json:"line,omitempty"`
}

// Flow is a scenario (`flow id ["name"]`).
type Flow struct {
	ID     string     `json:"id"`
	Name   string     `json:"name,omitempty"`
	Actor  string     `json:"actor,omitempty"`
	Steps  []Step     `json:"steps"`
	States []StateRef `json:"states"`
	Alts   []Alt      `json:"alts"`
	Exit   string     `json:"exit,omitempty"`
	// Comment: `#` lines right above the block, kept when the file is rewritten.
	Comment []string `json:"comment,omitempty"`
	File    string   `json:"file,omitempty"`
	Line    int      `json:"line,omitempty"`
	// enrich:
	Code    bool `json:"code,omitempty"`    // the code declares a flow with this id (arch:flow)
	Planned int  `json:"planned,omitempty"` // number of planned steps
}

// Journey is `journey id ["name"]: a > b > c`.
type Journey struct {
	ID    string   `json:"id"`
	Name  string   `json:"name,omitempty"`
	Flows []string `json:"flows"`
	// Comment: `#` lines right above the block, kept when the file is rewritten.
	Comment []string `json:"comment,omitempty"`
	File    string   `json:"file,omitempty"`
	Line    int      `json:"line,omitempty"`
	// enrich:
	Code    bool     `json:"code,omitempty"`    // the code declares a journey with this id
	Missing []string `json:"missing,omitempty"` // flow ids that exist nowhere
}

// Doc is the content of one .arch file. Kinds are kept apart; Format writes
// states, then flows, then journeys.
//
// Comments: a `#` line at column 0 belongs to the next block (Comment); those
// after the last block are Tail. Comments at the end of a line and indented
// comment lines are dropped when archmap rewrites the file.
type Doc struct {
	States   []*States  `json:"states"`
	Flows    []*Flow    `json:"flows"`
	Journeys []*Journey `json:"journeys"`
	Tail     []string   `json:"tail,omitempty"`
}

// Model is everything the spec files declare (all files, in file order).
type Model struct {
	States   []States  `json:"states"`
	Flows    []Flow    `json:"flows"`
	Journeys []Journey `json:"journeys"`
}

// Error is a syntax or validation error with a position.
type Error struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col,omitempty"`
	Msg  string `json:"msg"`
}

func (e Error) Error() string {
	if e.Col > 0 {
		return e.File + ":" + itoa(e.Line) + ":" + itoa(e.Col) + ": " + e.Msg
	}
	return e.File + ":" + itoa(e.Line) + ": " + e.Msg
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
