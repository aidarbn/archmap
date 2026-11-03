; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-ocaml v0.24.2
; (MIT License, Copyright (c) 2018 Max Brunsfeld), rewritten for archmap: the
; value_definition is the function definition (its let_binding gives the name),
; doc comments come from the comment above, references are reduced to calls
; (application, `|>` and `@@`).

(module_binding (module_name) @name) @definition.module
(module_type_definition (module_type_name) @name) @definition.interface
(class_binding (class_name) @name) @definition.class
(class_type_binding (class_type_name) @name) @definition.class

(method_definition (method_name) @name) @definition.method

(value_definition (let_binding pattern: (value_name) @name (parameter))) @definition.function
(value_definition (let_binding
  pattern: (value_name) @name
  body: [(fun_expression) (function_expression)])) @definition.function
(external (value_name) @name) @definition.function

(application_expression function: (value_path (value_name) @name)) @reference.call
(infix_expression
  left: (value_path (value_name) @name)
  operator: (concat_operator) @op
  (#eq? @op "@@")) @reference.call
(infix_expression
  operator: (rel_operator) @op
  right: (value_path (value_name) @name)
  (#eq? @op "|>")) @reference.call
(method_invocation (method_name) @name) @reference.call
