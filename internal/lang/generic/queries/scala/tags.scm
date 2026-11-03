; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-scala v0.26.4
; (MIT License, Copyright (c) 2018 GitHub), adapted for archmap: only types and
; functions are definitions (objects, enums and traits are containers), calls
; are plain, member and `new` expressions.

(package_clause name: (package_identifier) @name) @definition.module
(trait_definition name: (identifier) @name) @definition.interface
(enum_definition name: (identifier) @name) @definition.class
(class_definition name: (identifier) @name) @definition.class
(object_definition name: (identifier) @name) @definition.class

(function_definition name: (identifier) @name) @definition.function
(function_declaration name: (identifier) @name) @definition.function

(call_expression function: (identifier) @name) @reference.call
(call_expression function: (field_expression field: (identifier) @name)) @reference.call
(call_expression function: (generic_function function: (identifier) @name)) @reference.call
(call_expression function: (generic_function function: (field_expression field: (identifier) @name))) @reference.call
(instance_expression (type_identifier) @name) @reference.call
(instance_expression (generic_type type: (type_identifier) @name)) @reference.call
