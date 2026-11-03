// Package js is the js parser for archmap. NOT IMPLEMENTED YET: Parse returns an
// empty lang.File, so js files contribute nothing to the map.
//
// Contract: see package lang. Use internal/ts for the tree-sitter layer.
//
// Reference behaviour is the Python original (arch.py, parse_*): function declarations, const f = (...) => and function expressions, React memo/forwardRef components, class methods and class-property arrows; Cls = enclosing class; DefaultExport for export default; imports = module specifiers of import/require/dynamic import/export-from; pick the tsx dialect by extension.
package js

import "github.com/aidarbn/archmap/internal/lang"

// Parser implements lang.Parser for js.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "js" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".js", ".jsx", ".ts", ".tsx", ".mjs"} }

// Parse is a stub.
func (*Parser) Parse(path string, src []byte) (lang.File, error) { return lang.File{}, nil }

var _ lang.Parser = (*Parser)(nil)
