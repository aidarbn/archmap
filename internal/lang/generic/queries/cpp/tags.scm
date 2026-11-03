; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-cpp v0.23.4
; (MIT License, Copyright (c) 2014 Max Brunsfeld), rewritten for archmap: the
; whole function_definition is the definition (upstream captures only the
; declarator), prototypes are not definitions. An out-of-line method
; `A::f() {}` also yields an @definition.impl named A with the same range as
; the function, so its owner (Func.Cls) is A. Namespaces are containers.
; ALL_CAPS names are macro invocations mistaken for a function head (no
; preprocessor) and are skipped.

(struct_specifier name: (type_identifier) @name body: (_)) @definition.class
(union_specifier name: (type_identifier) @name body: (_)) @definition.class
(class_specifier name: (type_identifier) @name body: (_)) @definition.class
(namespace_definition name: (_) @name) @definition.module

(function_definition declarator: (function_declarator declarator: [(identifier) (field_identifier) (destructor_name) (operator_name)] @name)
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (function_declarator declarator: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (function_declarator declarator: (qualified_identifier name: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name)))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (function_declarator declarator: [(identifier) (field_identifier) (destructor_name) (operator_name)] @name))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (function_declarator declarator: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name)))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (function_declarator declarator: (qualified_identifier name: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name))))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (reference_declarator (function_declarator declarator: [(identifier) (field_identifier) (destructor_name) (operator_name)] @name))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (reference_declarator (function_declarator declarator: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name)))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (reference_declarator (function_declarator declarator: (qualified_identifier name: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name))))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (pointer_declarator declarator: (function_declarator declarator: [(identifier) (field_identifier) (destructor_name) (operator_name)] @name)))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (pointer_declarator declarator: (function_declarator declarator: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name))))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (pointer_declarator declarator: (function_declarator declarator: (qualified_identifier name: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name)))))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (reference_declarator (function_declarator declarator: [(identifier) (field_identifier) (destructor_name) (operator_name)] @name)))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (reference_declarator (function_declarator declarator: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name))))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function
(function_definition declarator: (pointer_declarator declarator: (reference_declarator (function_declarator declarator: (qualified_identifier name: (qualified_identifier name: [(identifier) (destructor_name) (operator_name)] @name)))))
  (#not-match? @name "^[A-Z][A-Z0-9_]+$")) @definition.function

; owners of out-of-line methods
(function_definition declarator: (function_declarator declarator: (qualified_identifier scope: (_) @name))) @definition.impl
(function_definition declarator: (function_declarator declarator: (qualified_identifier name: (qualified_identifier scope: (_) @name)))) @definition.impl
(function_definition declarator: (pointer_declarator declarator: (function_declarator declarator: (qualified_identifier scope: (_) @name)))) @definition.impl
(function_definition declarator: (pointer_declarator declarator: (function_declarator declarator: (qualified_identifier name: (qualified_identifier scope: (_) @name))))) @definition.impl
(function_definition declarator: (reference_declarator (function_declarator declarator: (qualified_identifier scope: (_) @name)))) @definition.impl
(function_definition declarator: (reference_declarator (function_declarator declarator: (qualified_identifier name: (qualified_identifier scope: (_) @name))))) @definition.impl

(call_expression function: (identifier) @name) @reference.call
(call_expression function: (field_expression field: (field_identifier) @name)) @reference.call
(call_expression function: (field_expression field: (template_method name: (field_identifier) @name))) @reference.call
(call_expression function: (qualified_identifier name: (identifier) @name)) @reference.call
(call_expression function: (qualified_identifier name: (template_function name: (identifier) @name))) @reference.call
(call_expression function: (template_function name: (identifier) @name)) @reference.call
