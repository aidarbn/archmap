; Based on queries/tags.scm of github.com/tree-sitter-grammars/tree-sitter-lua
; v0.5.0 (MIT License, Copyright (c) 2021 Munif Tanjim), extended for archmap:
; `function T.f` / `function T:m` get the table T as owner through an
; @definition.impl of the same range as the function.

(function_declaration
  name: [
    (identifier) @name
    (dot_index_expression
      field: (identifier) @name)
  ]) @definition.function

(function_declaration
  name: (method_index_expression
    method: (identifier) @name)) @definition.method

(function_declaration
  name: (dot_index_expression
    table: (identifier) @name)) @definition.impl

(function_declaration
  name: (method_index_expression
    table: (identifier) @name)) @definition.impl

(assignment_statement
  (variable_list
    .
    name: [
      (identifier) @name
      (dot_index_expression
        field: (identifier) @name)
    ])
  (expression_list
    .
    value: (function_definition))) @definition.function

(table_constructor
  (field
    name: (identifier) @name
    value: (function_definition))) @definition.function

(function_call
  name: [
    (identifier) @name
    (dot_index_expression
      field: (identifier) @name)
    (method_index_expression
      method: (identifier) @name)
  ]) @reference.call
