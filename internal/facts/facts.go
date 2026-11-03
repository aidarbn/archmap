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
}

// Module is a directory that contains at least one source file.
type Module struct {
	Imports []string `json:"imports"`
	Files   []string `json:"files"`
}

// Function is a function or method. Its map key (fid) is
// "path/file.ext::Class.method" (or "path/file.ext::name").
type Function struct {
	Name        string   `json:"name"`
	Cls         *string  `json:"cls"` // null for free functions
	Start       int      `json:"start"`
	End         int      `json:"end"`
	Sig         string   `json:"sig"`
	Doc         string   `json:"doc"`
	Callees     []string `json:"callees"`
	File        string   `json:"file"`
	Module      string   `json:"module"`
	Lang        string   `json:"lang"`
	Entry       *Entry   `json:"entry,omitempty"`
	Status      string   `json:"status,omitempty"`      // added | modified
	Uncommitted *bool    `json:"uncommitted,omitempty"` // set together with Status
	Decorators  []string `json:"decorators,omitempty"`
	Bases       []string `json:"bases,omitempty"`
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
