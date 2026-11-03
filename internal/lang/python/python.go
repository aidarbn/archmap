// Package python is the python parser for archmap. NOT IMPLEMENTED YET: Parse returns an
// empty lang.File, so python files contribute nothing to the map.
//
// Contract: see package lang. Use internal/ts for the tree-sitter layer.
//
// Reference behaviour is the Python original (arch.py, parse_*): functions and methods of top-level classes (also inside top-level if/try/with), nested defs are not separate Funcs; Decorators (unparsed text) and Bases of the class; TStart = first decorator line; Sig = def line(s) up to the body; Doc = first docstring line; imports from every import statement in the file, including lazy ones inside functions.
package python

import "github.com/aidarbn/archmap/internal/lang"

// Parser implements lang.Parser for python.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "python" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".py"} }

// Parse is a stub.
func (*Parser) Parse(path string, src []byte) (lang.File, error) { return lang.File{}, nil }

var _ lang.Parser = (*Parser)(nil)
