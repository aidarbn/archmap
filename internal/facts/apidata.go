package facts

// Endpoint is an externally reachable entry of the system: an HTTP route, a
// gRPC method, a websocket, a background task or a CLI command. It is built
// from code (proto files, router registrations, annotations), not from a model.
type Endpoint struct {
	Kind    string `json:"kind"` // http | grpc | ws | task | cli
	Service string `json:"service,omitempty"`
	Name    string `json:"name,omitempty"`
	Method  string `json:"method,omitempty"` // HTTP verb
	Path    string `json:"path,omitempty"`   // URL path, or the full gRPC method when there is no HTTP binding
	Fid     string `json:"fid,omitempty"`    // function that implements it
	// Auth is "public", "role:<x>", "authenticated" or "" when unknown.
	Auth     string `json:"auth,omitempty"`
	Request  string `json:"request,omitempty"`
	Response string `json:"response,omitempty"`
	Source   string `json:"source"` // proto | router | annotation | code
}

// DataModel is the persistent data schema found in the code.
type DataModel struct {
	Tables map[string]*Table `json:"tables"`
}

// Table is one table (or ORM model) with its fields.
type Table struct {
	Fields []Field    `json:"fields"`
	Source string     `json:"source"` // migration | orm | sqlc | jet
	Refs   []TableRef `json:"refs,omitempty"`
	// Desc is the description written in the code: COMMENT ON TABLE, a comment
	// above CREATE TABLE, the doc of the mapped struct / model. Empty when none.
	Desc string `json:"desc,omitempty"`
}

// Field is a column.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// NotNull is set for SQL columns declared NOT NULL or PRIMARY KEY (only SQL
	// sources know it; for ORM tables false means "unknown").
	NotNull bool `json:"notNull,omitempty"`
	// Desc is the description written in the code: COMMENT ON COLUMN, a `--`
	// comment on or above the column, the doc of the struct field, help_text.
	Desc string `json:"desc,omitempty"`
}

// TableRef is a foreign key: Field of this table points to To of Table.
type TableRef struct {
	Field string `json:"field"`
	Table string `json:"table"`
	To    string `json:"to,omitempty"`
}

// Access is how a function touches a table.
type Access struct {
	Table  string   `json:"table"`
	Op     string   `json:"op"` // read | insert | update | delete | upsert
	Fields []string `json:"fields,omitempty"`
	Line   int      `json:"line,omitempty"`
}
