package generic

import (
	"embed"
	"regexp"
	"strings"
	"unsafe"

	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

//go:embed queries
var queries embed.FS

func q(lang, file string) string {
	b, err := queries.ReadFile("queries/" + lang + "/" + file)
	if err != nil {
		return ""
	}
	return string(b)
}

// Specs lists the languages that run on the generic parser. To add a language:
// add its grammar module to go.mod, put queries/<name>/tags.scm (and optionally
// imports.scm) next to this file, and append one Spec here.
func Specs() []Spec {
	specs := []Spec{
		{Name: "java", Extensions: []string{".java"}, Grammar: func() unsafe.Pointer { return java.Language() },
			Tags: q("java", "tags.scm"), Imports: q("java", "imports.scm")},
		{Name: "rust", Extensions: []string{".rs"}, Grammar: func() unsafe.Pointer { return rust.Language() },
			Tags: q("rust", "tags.scm"), Imports: q("rust", "imports.scm"), ExpandImport: expandRustUse},
		{Name: "ruby", Extensions: []string{".rb"}, Grammar: func() unsafe.Pointer { return ruby.Language() },
			Tags: q("ruby", "tags.scm"), Imports: q("ruby", "imports.scm")},
	}
	specs = append(specs, specsA()...)
	return specs
}

// Parsers returns a parser for every Spec.
func Parsers() []*Parser {
	var out []*Parser
	for _, s := range Specs() {
		out = append(out, New(s))
	}
	return out
}

// expandRustUse expands a `use` argument: braces are flattened, `as` renames
// dropped, `self` in a list means the list prefix, globs end in "::*".
var rustAs = regexp.MustCompile(`\s+as\s+\w+`)

func expandRustUse(s string) []string {
	s = strings.Join(strings.Fields(rustAs.ReplaceAllString(s, "")), "")
	var out []string
	var rec func(prefix, s string)
	rec = func(prefix, s string) {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			out = append(out, join(prefix, s))
			return
		}
		base := join(prefix, strings.TrimSuffix(s[:i], "::"))
		inner := s[i+1 : strings.LastIndexByte(s, '}')]
		depth, from := 0, 0
		for k := 0; k <= len(inner); k++ {
			if k == len(inner) || (inner[k] == ',' && depth == 0) {
				if part := inner[from:k]; part != "" {
					if part == "self" {
						out = append(out, base)
					} else {
						rec(base, part)
					}
				}
				from = k + 1
			} else if inner[k] == '{' {
				depth++
			} else if inner[k] == '}' {
				depth--
			}
		}
	}
	rec("", s)
	return out
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "::" + b
}
