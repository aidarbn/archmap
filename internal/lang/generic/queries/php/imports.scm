; archmap import query (original). `use` declarations are expanded in Go
; (expandPHPUse: groups, aliases); include/require of a plain string literal is
; a path relative to the file.
(namespace_use_declaration) @import
(include_expression (string (string_content) @import.relative))
(include_expression (encapsed_string (string_content) @import.relative))
(include_once_expression (string (string_content) @import.relative))
(include_once_expression (encapsed_string (string_content) @import.relative))
(require_expression (string (string_content) @import.relative))
(require_expression (encapsed_string (string_content) @import.relative))
(require_once_expression (string (string_content) @import.relative))
(require_once_expression (encapsed_string (string_content) @import.relative))
