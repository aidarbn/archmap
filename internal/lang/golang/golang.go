// Package golang is the go parser for archmap. NOT IMPLEMENTED YET: Parse returns an
// empty lang.File, so go files contribute nothing to the map.
//
// Contract: see package lang. Use internal/ts for the tree-sitter layer.
//
// Reference behaviour is the Python original (arch.py, parse_*): top-level funcs and methods with bodies (declarations without a body are skipped); Cls = receiver type without * and type parameters, Recv = receiver variable; Doc = leading // comment lines (not //go: directives); imports with optional alias; calls inside closures belong to the enclosing func.
package golang

import "github.com/aidarbn/archmap/internal/lang"

// Parser implements lang.Parser for go.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "go" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".go"} }

// Parse is a stub.
func (*Parser) Parse(path string, src []byte) (lang.File, error) { return lang.File{}, nil }

var _ lang.Parser = (*Parser)(nil)
