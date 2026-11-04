package extract

import (
	"fmt"
	"strings"
)

// MaxSrcLines is the longest function body kept in facts (Function.Src).
const MaxSrcLines = 60

// SourceSlice returns lines start..end (1-based, inclusive) of text. A body
// longer than max lines is cut and ends with a note in the language's comment
// style; max <= 0 means no limit.
func SourceSlice(text string, start, end, max int, lang string) string {
	lines := strings.Split(text, "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	body := lines[start-1 : end]
	more := 0
	if max > 0 && len(body) > max {
		more = len(body) - max
		body = body[:max]
	}
	s := strings.TrimRight(strings.Join(body, "\n"), "\r\n \t")
	if more > 0 {
		c := "//"
		if lang == "python" {
			c = "#"
		}
		s += fmt.Sprintf("\n%s … (+%d more lines)", c, more)
	}
	return s
}
