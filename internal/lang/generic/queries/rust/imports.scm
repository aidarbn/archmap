; archmap import query (original). `use` arguments are expanded in Go
; (expandRustUse); `mod foo;` is a child module of the current one.
(use_declaration argument: (_) @import)
(mod_item name: (identifier) @import.mod !body)
