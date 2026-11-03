; archmap import query (original): @import("file.zig"); @import("std") and
; other package names are not files of the repository.
(builtin_function (builtin_identifier) @fn
  (arguments . (string (string_content) @import.relative))
  (#eq? @fn "@import")
  (#match? @import.relative "\\.zig$"))
