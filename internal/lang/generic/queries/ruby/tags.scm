; Based on queries/tags.scm of github.com/tree-sitter/tree-sitter-ruby v0.23.1
; (MIT License, Copyright (c) 2016 Rob Rix), adapted for archmap: the upstream
; "every identifier is a call" pattern is dropped (it cannot tell locals from
; calls without locals.scm), doc comments are found by the generic code, and
; require/load are not calls.

(method name: (_) @name) @definition.method
(singleton_method name: (_) @name) @definition.method

(class name: [(constant) @name (scope_resolution name: (_) @name)]) @definition.class
(singleton_class value: [(constant) @name (scope_resolution name: (_) @name)]) @definition.class
(module name: [(constant) @name (scope_resolution name: (_) @name)]) @definition.module

(call method: (identifier) @name
  (#not-match? @name "^(require|require_relative|load|lambda|proc|raise|attr_reader|attr_writer|attr_accessor|include|extend|prepend|private|public|protected)$")) @reference.call
