package metadata

// Field describes a single model field.
type Field struct {
	Name    string
	Type    string
	Default *string
}

// Entity describes a model.
type Entity struct {
	Name   string
	Fields []Field
}

// AST is the parsed schema representation.
type AST struct {
	Entities []Entity
}

// ParseSchema parses a Prisma schema into an AST.
func ParseSchema(input []byte) (AST, error) {
	// TODO: use regex or parser
	return AST{}, nil
}
