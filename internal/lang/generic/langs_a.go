package generic

import (
	"regexp"
	"strings"
	"unsafe"

	dart "github.com/UserNobody14/tree-sitter-dart/bindings/go"
	elixir "github.com/aidarbn/archmap/internal/lang/generic/elixirgrammar"
	csharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	haskell "github.com/tree-sitter/tree-sitter-haskell/bindings/go"
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
)

// specsA lists the batch-A languages: C#, PHP, Scala, Dart, Elixir, Haskell.
// Objective-C is not here: tree-sitter-objc has no Go binding.
func specsA() []Spec {
	return []Spec{
		{Name: "csharp", Extensions: []string{".cs"}, Grammar: func() unsafe.Pointer { return csharp.Language() },
			Tags: q("csharp", "tags.scm"), Imports: q("csharp", "imports.scm")},
		{Name: "php", Extensions: []string{".php"}, Grammar: func() unsafe.Pointer { return php.LanguagePHP() },
			Tags: q("php", "tags.scm"), Imports: q("php", "imports.scm"), ExpandImport: expandPHPUse},
		{Name: "scala", Extensions: []string{".scala"}, Grammar: func() unsafe.Pointer { return scala.Language() },
			Tags: q("scala", "tags.scm"), Imports: q("scala", "imports.scm"), ExpandImport: expandScalaImport},
		{Name: "dart", Extensions: []string{".dart"}, Grammar: func() unsafe.Pointer { return dart.Language() },
			Tags: q("dart", "tags.scm"), Imports: q("dart", "imports.scm"), ExpandImport: expandDartImport,
			SiblingBody: "function_body"},
		{Name: "elixir", Extensions: []string{".ex"}, Grammar: func() unsafe.Pointer { return elixir.Language() },
			Tags: q("elixir", "tags.scm"), Imports: q("elixir", "imports.scm")},
		{Name: "haskell", Extensions: []string{".hs"}, Grammar: func() unsafe.Pointer { return haskell.Language() },
			Tags: q("haskell", "tags.scm"), Imports: q("haskell", "imports.scm")},
	}
}

// splitTop splits s on commas that are not inside braces.
func splitTop(s string) []string {
	var out []string
	depth, from := 0, 0
	for i := 0; i <= len(s); i++ {
		switch {
		case i == len(s) || (s[i] == ',' && depth == 0):
			if p := strings.TrimSpace(s[from:i]); p != "" {
				out = append(out, p)
			}
			from = i + 1
		case s[i] == '{':
			depth++
		case s[i] == '}':
			depth--
		}
	}
	return out
}

var (
	phpUseKw = regexp.MustCompile(`^use\s+(?:function\s+|const\s+)?`)
	phpAs    = regexp.MustCompile(`\s+as\s+\w+`)
)

// expandPHPUse expands a `use A\B, C\{D, E as F};` declaration into dotted
// paths ("A.B", "C.D", "C.E").
func expandPHPUse(s string) []string {
	s = strings.TrimSuffix(strings.TrimSpace(s), ";")
	s = phpUseKw.ReplaceAllString(s, "")
	var out []string
	for _, part := range splitTop(s) {
		part = strings.TrimSpace(phpAs.ReplaceAllString(part, ""))
		if i := strings.IndexByte(part, '{'); i >= 0 {
			base := strings.Trim(part[:i], `\ `)
			for _, m := range splitTop(part[i+1 : strings.LastIndexByte(part, '}')]) {
				m = strings.TrimSpace(phpAs.ReplaceAllString(m, ""))
				m = strings.TrimPrefix(strings.TrimPrefix(m, "function "), "const ")
				out = append(out, phpPath(base+`\`+m))
			}
			continue
		}
		out = append(out, phpPath(part))
	}
	return out
}

func phpPath(s string) string {
	return strings.ReplaceAll(strings.Trim(strings.Join(strings.Fields(s), ""), `\`), `\`, ".")
}

var scalaAs = regexp.MustCompile(`\s*(=>|\bas\b)\s*\w+`)

// expandScalaImport expands `import a.b.{C, D => E, _}, x.y._` into dotted
// paths; wildcards (`_`, `*`) become ".*".
func expandScalaImport(s string) []string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "import"))
	var out []string
	for _, part := range splitTop(s) {
		part = strings.Join(strings.Fields(part), "")
		if i := strings.IndexByte(part, '{'); i >= 0 {
			base := strings.TrimSuffix(part[:i], ".")
			for _, m := range splitTop(part[i+1 : strings.LastIndexByte(part, '}')]) {
				m = strings.TrimSpace(scalaAs.ReplaceAllString(m, ""))
				switch {
				case m == "" || strings.Contains(m, "=>"):
					// `A => _` hides a name: nothing to add
				case m == "_" || m == "*":
					out = append(out, base+".*")
				default:
					out = append(out, base+"."+m)
				}
			}
			continue
		}
		part = scalaAs.ReplaceAllString(part, "")
		if strings.HasSuffix(part, "._") {
			part = strings.TrimSuffix(part, "_") + "*"
		}
		out = append(out, part)
	}
	return out
}

// expandDartImport maps a Dart URI literal: `package:` and `dart:` URIs stay
// as written (scheme dropped for package:), a plain path is relative.
func expandDartImport(s string) []string {
	s = strings.Trim(s, `'"`)
	switch {
	case s == "":
		return nil
	case strings.HasPrefix(s, "package:"):
		return []string{strings.TrimPrefix(s, "package:")}
	case strings.HasPrefix(s, "dart:"):
		return []string{s}
	case strings.HasPrefix(s, "."):
		return []string{s}
	}
	return []string{"./" + s}
}
