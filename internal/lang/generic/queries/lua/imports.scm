; archmap import query (original): require("a.b") / require "a.b".
(function_call
  name: (identifier) @fn
  arguments: (arguments (string content: (string_content) @import))
  (#eq? @fn "require"))
