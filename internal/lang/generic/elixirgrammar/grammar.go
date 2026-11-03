// Package elixirgrammar vendors the tree-sitter Elixir grammar
// (github.com/elixir-lang/tree-sitter-elixir v0.3.5, Apache-2.0, see LICENSE).
// The upstream module declares the path github.com/tree-sitter/tree-sitter-elixir,
// which cannot be required without a replace directive, and a replace breaks
// `go install ...@version`.
package elixirgrammar

// #cgo CFLAGS: -std=c11 -fPIC
// #include "src/parser.c"
// #include "src/scanner.c"
import "C"

import "unsafe"

// Language returns the tree-sitter Language for Elixir.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_elixir())
}
