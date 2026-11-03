; archmap import query (original).
(call method: (identifier) @fn
  arguments: (argument_list . (string (string_content) @import))
  (#eq? @fn "require"))
(call method: (identifier) @fn
  arguments: (argument_list . (string (string_content) @import.relative))
  (#eq? @fn "require_relative"))
