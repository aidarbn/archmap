// Package all lists every built-in language parser.
package all

import (
	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/lang/generic"
	"github.com/aidarbn/archmap/internal/lang/golang"
	"github.com/aidarbn/archmap/internal/lang/js"
	"github.com/aidarbn/archmap/internal/lang/kotlin"
	"github.com/aidarbn/archmap/internal/lang/python"
	"github.com/aidarbn/archmap/internal/lang/swift"
)

// Parsers returns one instance of each built-in parser.
func Parsers() []lang.Parser {
	ps := []lang.Parser{python.New(), golang.New(), js.New(), swift.New(), kotlin.New()}
	for _, g := range generic.Parsers() {
		ps = append(ps, g)
	}
	return ps
}
