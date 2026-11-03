; archmap import query (original): `source file` and `. file` with a literal path.
(command name: (command_name (word) @fn) . argument: (word) @import.relative
  (#any-of? @fn "source" "."))
(command name: (command_name (word) @fn) . argument: (string . (string_content) @import.relative .)
  (#any-of? @fn "source" "."))
