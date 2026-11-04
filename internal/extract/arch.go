package extract

import (
	"strings"

	"github.com/aidarbn/archmap/internal/annot"
	"github.com/aidarbn/archmap/internal/facts"
)

// archNotes parses the `arch:` markup above every function of one file and
// returns it by function start line. Syntax errors are skipped here; they are
// reported by `archmap lint`.
func archNotes(src []byte, funcs map[int]bool) map[int]*facts.ArchNote {
	if !strings.Contains(string(src), annot.Prefix) {
		return nil
	}
	lines := strings.Split(string(src), "\n")
	out := map[int]*facts.ArchNote{}
	for start := range funcs {
		if n, _ := annot.Parse(annot.Block(lines, start)); n != nil {
			out[start] = n
		}
	}
	return out
}
