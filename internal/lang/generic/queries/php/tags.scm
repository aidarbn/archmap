; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-php v0.25.1
; (MIT License, Copyright (c) 2017 GitHub), adapted for archmap: methods and
; functions are definitions, enums are classes, `new X` and nullsafe/static
; calls are references.

(namespace_definition name: (namespace_name) @name) @definition.module
(interface_declaration name: (name) @name) @definition.interface
(trait_declaration name: (name) @name) @definition.interface
(class_declaration name: (name) @name) @definition.class
(enum_declaration name: (name) @name) @definition.class

(function_definition name: (name) @name) @definition.function
(method_declaration name: (name) @name) @definition.method

(object_creation_expression (name) @name) @reference.call
(object_creation_expression (qualified_name (name) @name .)) @reference.call
(function_call_expression function: (name) @name) @reference.call
(function_call_expression function: (qualified_name (name) @name .)) @reference.call
(scoped_call_expression name: (name) @name) @reference.call
(member_call_expression name: (name) @name) @reference.call
(nullsafe_member_call_expression name: (name) @name) @reference.call
