// Package swift is the swift parser for archmap. NOT IMPLEMENTED YET: Parse returns an
// empty lang.File, so swift files contribute nothing to the map.
//
// Contract: see package lang. Use internal/ts for the tree-sitter layer.
//
// Reference behaviour is the Python original (arch.py, parse_*): func and init inside types/extensions (Cls = type name, extension target), free funcs; TStart includes attributes; imports = module names.
package swift

import "github.com/aidarbn/archmap/internal/lang"

// Parser implements lang.Parser for swift.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "swift" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".swift"} }

// Parse is a stub.
func (*Parser) Parse(path string, src []byte) (lang.File, error) { return lang.File{}, nil }

var _ lang.Parser = (*Parser)(nil)
