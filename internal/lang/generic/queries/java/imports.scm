; archmap import query (original). `import a.b.C;` -> "a.b.C", `import a.b.*;` -> "a.b.*".
(import_declaration (scoped_identifier) @import . ) 
(import_declaration (scoped_identifier) @import (asterisk) @wild)
