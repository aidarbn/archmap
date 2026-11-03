; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-rust v0.23.2
; (MIT License, Copyright (c) 2017 Maxim Sokolov), adapted for archmap: the
; function node itself is the definition (upstream captures the declaration_list
; for methods), impl blocks are @definition.impl named after the implemented
; type, scoped and generic calls are references.

(struct_item name: (type_identifier) @name) @definition.class
(enum_item name: (type_identifier) @name) @definition.class
(union_item name: (type_identifier) @name) @definition.class
(trait_item name: (type_identifier) @name) @definition.interface
(mod_item name: (identifier) @name) @definition.module

(impl_item type: (type_identifier) @name) @definition.impl
(impl_item type: (generic_type type: (type_identifier) @name)) @definition.impl
(impl_item type: (scoped_type_identifier name: (type_identifier) @name)) @definition.impl

(function_item name: (identifier) @name) @definition.function

(call_expression function: (identifier) @name) @reference.call
(call_expression function: (field_expression field: (field_identifier) @name)) @reference.call
(call_expression function: (scoped_identifier name: (identifier) @name)) @reference.call
(call_expression function: (generic_function function: (identifier) @name)) @reference.call
(call_expression function: (generic_function function: (field_expression field: (field_identifier) @name))) @reference.call
(call_expression function: (generic_function function: (scoped_identifier name: (identifier) @name))) @reference.call
