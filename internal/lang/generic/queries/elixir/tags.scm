; Based on queries/tags.scm of github.com/elixir-lang/tree-sitter-elixir v0.3.5
; (Apache License 2.0), adapted for archmap: modules are containers,
; def/defp/defmacro... are functions (a clause head is not a call; archmap drops
; calls on a definition name), special forms are not calls, a preceding
; `@doc "..."` is the doc.

; Definitions

(call
  target: (identifier) @_k
  (arguments (alias) @name)
  (#any-of? @_k "defmodule" "defprotocol")) @definition.module

(call
  target: (identifier) @_k
  (arguments (alias) @name)
  (#eq? @_k "defimpl")) @definition.impl

((unary_operator
   operand: (call
     target: (identifier) @_d
     (arguments [(string (quoted_content) @doc) (sigil (quoted_content) @doc)])
     (#any-of? @_d "doc" "moduledoc")))
 .
 (call
   target: (identifier) @_k
   (arguments
     [
       (identifier) @name
       (call target: (identifier) @name)
       (binary_operator
         left: (call target: (identifier) @name)
         operator: "when")
     ])
   (#any-of? @_k "def" "defp" "defdelegate" "defguard" "defguardp" "defmacro" "defmacrop")) @definition.function)

(call
  target: (identifier) @_k
  (arguments
    [
      (identifier) @name
      (call target: (identifier) @name)
      (binary_operator
        left: (call target: (identifier) @name)
        operator: "when")
    ])
  (#any-of? @_k "def" "defp" "defdelegate" "defguard" "defguardp" "defmacro" "defmacrop")) @definition.function

; References: local, remote (`Mod.fun`) and piped (`|> fun`) calls

(call
  target: (identifier) @name
  (#not-match? @name "^(def|defp|defdelegate|defguard|defguardp|defmacro|defmacrop|defmodule|defprotocol|defimpl|defstruct|defexception|defoverridable|alias|case|cond|else|for|if|import|quote|raise|receive|require|reraise|super|throw|try|unless|unquote|unquote_splicing|use|with|fn|when|and|or|not|in)$")) @reference.call

(call
  target: (dot right: (identifier) @name)) @reference.call

(binary_operator
  operator: "|>"
  right: (identifier) @name) @reference.call
