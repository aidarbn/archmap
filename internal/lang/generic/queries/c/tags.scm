; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-c v0.24.2
; (MIT License, Copyright (c) 2014 Max Brunsfeld), rewritten for archmap: the
; whole function_definition is the definition (upstream captures only the
; declarator), prototypes are not definitions, calls are references.

(struct_specifier name: (type_identifier) @name body: (_)) @definition.class
(union_specifier name: (type_identifier) @name body: (_)) @definition.class

; ALL_CAPS names are macro invocations that the parser mistook for a function
; head (no preprocessor), so they are not definitions.
(function_definition
  declarator: (function_declarator declarator: (identifier) @name)
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition
  declarator: (pointer_declarator
    declarator: (function_declarator declarator: (identifier) @name))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition
  declarator: (pointer_declarator
    declarator: (pointer_declarator
      declarator: (function_declarator declarator: (identifier) @name)))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function

(call_expression function: (identifier) @name) @reference.call
(call_expression function: (field_expression field: (field_identifier) @name)) @reference.call
