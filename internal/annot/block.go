package annot

import (
	"regexp"
	"strings"
)

// hsSigRe is a Haskell type signature line between a comment and its definition.
var hsSigRe = regexp.MustCompile(`^[a-z_][\w']*\s*::`)

// Block returns the comment lines directly above the declaration that starts at
// the 1-based line start, with comment markers removed. Decorator/annotation
// lines (`@x`, `#[x]`) between the comment and the declaration are skipped. A
// blank line ends the block. Supported: // /// //! # -- and /* */ blocks.
func Block(lines []string, start int) []Line {
	i := start - 2 // index of the line above
	for i >= 0 {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "@") || strings.HasPrefix(t, "#[") || hsSigRe.MatchString(t) {
			i--
			continue
		}
		break
	}
	var rev []Line
	for i >= 0 {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "":
			return reverse(rev)
		case strings.HasSuffix(t, "*/") || strings.HasSuffix(t, "*)"):
			// block comment: walk up to its opening line
			j := i
			for j >= 0 && !strings.Contains(lines[j], "/*") && !strings.Contains(lines[j], "(*") {
				j--
			}
			if j < 0 {
				return reverse(rev)
			}
			for k := i; k >= j; k-- {
				rev = append(rev, Line{k + 1, stripBlock(lines[k])})
			}
			i = j - 1
		case strings.HasPrefix(t, "//"):
			rev = append(rev, Line{i + 1, strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(t, "///"), "//!"), "//"), "/! ")})
			i--
		case strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#!"):
			rev = append(rev, Line{i + 1, strings.TrimLeft(t, "# ")})
			i--
		case strings.HasPrefix(t, "--"):
			rev = append(rev, Line{i + 1, strings.TrimLeft(t, "- ")})
			i--
		default:
			return reverse(rev)
		}
	}
	return reverse(rev)
}

func stripBlock(l string) string {
	t := strings.TrimSpace(l)
	t = strings.TrimPrefix(t, "(*")
	t = strings.TrimSuffix(t, "*)")
	t = strings.TrimPrefix(t, "/**")
	t = strings.TrimPrefix(t, "/*")
	t = strings.TrimSuffix(t, "*/")
	t = strings.TrimPrefix(t, "*")
	return strings.TrimSpace(t)
}

func reverse(l []Line) []Line {
	for a, b := 0, len(l)-1; a < b; a, b = a+1, b-1 {
		l[a], l[b] = l[b], l[a]
	}
	return l
}
