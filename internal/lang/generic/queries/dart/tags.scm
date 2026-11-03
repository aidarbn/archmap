; Based on queries/tags.scm of github.com/UserNobody14/tree-sitter-dart
; (MIT License, Copyright (c) 2019 Nick Tsaplin), adapted for archmap. In this
; grammar a function body is the sibling of its signature (Spec.SiblingBody), so
; the signature is the definition node. Calls are runs of sibling nodes: an
; identifier followed by an argument part, with the receiver chain in @recv.

(class_definition name: (identifier) @name) @definition.class
(mixin_declaration (mixin) (identifier) @name) @definition.class
(extension_declaration name: (identifier) @name) @definition.class
(enum_declaration name: (identifier) @name) @definition.class

(program (function_signature name: (identifier) @name) @definition.function)

(class_body (method_signature [
  (function_signature name: (identifier) @name)
  (getter_signature name: (identifier) @name)
  (setter_signature name: (identifier) @name)
  (constructor_signature name: (identifier) @name)
  (factory_constructor_signature (identifier) @name)
]) @definition.method)
(extension_body (method_signature (function_signature name: (identifier) @name)) @definition.method)

; bare call: `f(x)`, `Foo(x)`
((identifier) @name @reference.call . (selector (argument_part)))

; member call: `a.b.f(x)`, `this.f()`, `a?.f()`
([(identifier) (this) (super)] @recv . (selector (unconditional_assignable_selector))* . (selector [
    (unconditional_assignable_selector (identifier) @name @reference.call)
    (conditional_assignable_selector (identifier) @name @reference.call)
  ]) . (selector (argument_part)))
([(identifier) (this) (super)] @recv . (selector (unconditional_assignable_selector))* . [
    (unconditional_assignable_selector (identifier) @name @reference.call)
    (conditional_assignable_selector (identifier) @name @reference.call)
  ] . (selector (argument_part)))

; `new Foo()`
(new_expression (type_identifier) @name @reference.call)
