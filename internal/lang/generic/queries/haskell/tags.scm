; archmap tags query (original, no upstream tags.scm exists) for
; github.com/tree-sitter/tree-sitter-haskell v0.24.1 (MIT License). Each
; equation of a function is one definition; a haddock comment above the type
; signature is the doc. Types are classes, classes are interfaces, instances
; are impl blocks. Calls are applications of a (qualified) variable.

(data_type name: (name) @name) @definition.class
(newtype name: (name) @name) @definition.class
(class name: (name) @name) @definition.interface
(instance patterns: (type_patterns . (name) @name)) @definition.impl

((haddock) @doc . (signature) . (function name: (variable) @name) @definition.function)
(function name: (variable) @name) @definition.function
(declarations (bind name: (variable) @name) @definition.function)

(apply function: (variable) @name) @reference.call
(apply function: (qualified module: (module) id: (variable) @name)) @reference.call
