; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-c-sharp v0.23.5
; (MIT License, Copyright (c) 2014 Max Brunsfeld), adapted for archmap:
; constructors, local functions, records and structs are definitions, calls
; are plain, member, generic and `new` invocations.

(class_declaration name: (identifier) @name) @definition.class
(struct_declaration name: (identifier) @name) @definition.class
(record_declaration name: (identifier) @name) @definition.class
(interface_declaration name: (identifier) @name) @definition.interface
(namespace_declaration name: (_) @name) @definition.module
(file_scoped_namespace_declaration name: (_) @name) @definition.module

(method_declaration name: (identifier) @name) @definition.method
(constructor_declaration name: (identifier) @name) @definition.method
(property_declaration name: (identifier) @name value: (arrow_expression_clause)) @definition.method
(local_function_statement name: (identifier) @name) @definition.function

(invocation_expression function: (identifier) @name) @reference.call
(invocation_expression function: (generic_name (identifier) @name)) @reference.call
(invocation_expression function: (member_access_expression name: (identifier) @name)) @reference.call
(invocation_expression function: (member_access_expression name: (generic_name (identifier) @name))) @reference.call
(invocation_expression function: (conditional_access_expression (member_binding_expression name: (identifier) @name))) @reference.call
(object_creation_expression type: (identifier) @name) @reference.call
