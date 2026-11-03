; archmap tags query (original; tree-sitter-zig v1.1.2 ships no tags.scm,
; grammar MIT License, Copyright (c) 2021 Amaan Qureshi). Functions (also
; those inside struct/enum/union declarations, owned by the const they are
; assigned to), containers and calls.

(function_declaration name: (identifier) @name) @definition.function

(variable_declaration (identifier) @name
  [(struct_declaration) (enum_declaration) (union_declaration) (opaque_declaration)]) @definition.class

(call_expression function: (identifier) @name) @reference.call
(call_expression function: (field_expression member: (identifier) @name)) @reference.call
