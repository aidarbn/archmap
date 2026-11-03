// Package kotlin is the kotlin parser for archmap. NOT IMPLEMENTED YET: Parse returns an
// empty lang.File, so kotlin files contribute nothing to the map.
//
// Contract: see package lang. Use internal/ts for the tree-sitter layer.
//
// Reference behaviour is the Python original (arch.py, parse_*): fun declarations in classes/objects/interfaces and top-level (extension receiver is not the Cls); TStart includes annotations; imports = dotted import paths without the import keyword.
package kotlin

import "github.com/aidarbn/archmap/internal/lang"

// Parser implements lang.Parser for kotlin.
type Parser struct{}

// New returns the parser.
func New() *Parser { return &Parser{} }

// Name returns the language key used in facts.json.
func (*Parser) Name() string { return "kotlin" }

// Extensions lists handled file extensions.
func (*Parser) Extensions() []string { return []string{".kt"} }

// Parse is a stub.
func (*Parser) Parse(path string, src []byte) (lang.File, error) { return lang.File{}, nil }

var _ lang.Parser = (*Parser)(nil)
