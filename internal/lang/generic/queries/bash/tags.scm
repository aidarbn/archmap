; archmap tags query (original; tree-sitter-bash v0.25.1 ships no tags.scm,
; grammar MIT License, Copyright (c) 2017 Max Brunsfeld). Shell functions are
; definitions, every simple command is a call (calls to external programs
; and builtins are dropped later when they do not resolve to a function).

(function_definition name: (word) @name) @definition.function

(command name: (command_name (word) @name)
  (#not-match? @name "^(echo|printf|cd|test|true|false|exit|return|local|export|read|set|shift|source|\\.|\\[|\\[\\[|:)$")) @reference.call
