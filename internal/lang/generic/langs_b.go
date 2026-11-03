package generic

import (
	"unsafe"

	lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	zig "github.com/tree-sitter-grammars/tree-sitter-zig/bindings/go"
	bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	ocaml "github.com/tree-sitter/tree-sitter-ocaml/bindings/go"
)

// specsB lists the group B languages: C, C++, Lua, Bash, Zig, OCaml.
//
// Header rule: ".h" is parsed as C++. A C header is almost always valid C++
// for tags purposes (declarations, static inline functions), and a header in a
// C++ project may contain classes/namespaces that the C grammar would turn
// into errors. ".c" is C.
func specsB() []Spec {
	return []Spec{
		{Name: "c", Extensions: []string{".c"}, Grammar: func() unsafe.Pointer { return c.Language() },
			Tags: q("c", "tags.scm"), Imports: q("c", "imports.scm")},
		{Name: "cpp", Extensions: []string{".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx", ".h"}, Grammar: func() unsafe.Pointer { return cpp.Language() },
			Tags: q("cpp", "tags.scm"), Imports: q("cpp", "imports.scm")},
		{Name: "lua", Extensions: []string{".lua"}, Grammar: func() unsafe.Pointer { return lua.Language() },
			Tags: q("lua", "tags.scm"), Imports: q("lua", "imports.scm")},
		{Name: "bash", Extensions: []string{".sh", ".bash"}, Grammar: func() unsafe.Pointer { return bash.Language() },
			Tags: q("bash", "tags.scm"), Imports: q("bash", "imports.scm")},
		{Name: "zig", Extensions: []string{".zig"}, Grammar: func() unsafe.Pointer { return zig.Language() },
			Tags: q("zig", "tags.scm"), Imports: q("zig", "imports.scm")},
		{Name: "ocaml", Extensions: []string{".ml"}, Grammar: func() unsafe.Pointer { return ocaml.LanguageOCaml() },
			Tags: q("ocaml", "tags.scm"), Imports: q("ocaml", "imports.scm")},
	}
}
