; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-java v0.23.5
; (MIT License, Copyright (c) 2017 Ayman Nadeem), adapted for archmap: constructors
; and enums/records are definitions, calls are the invocation nodes themselves.

(class_declaration name: (identifier) @name) @definition.class
(enum_declaration name: (identifier) @name) @definition.class
(record_declaration name: (identifier) @name) @definition.class
(interface_declaration name: (identifier) @name) @definition.interface

(method_declaration name: (identifier) @name) @definition.method
(constructor_declaration name: (identifier) @name) @definition.method

(method_invocation name: (identifier) @name) @reference.call
(object_creation_expression type: (type_identifier) @name) @reference.call
