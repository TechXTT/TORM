// File: internal/dsl/parser.go
package dsl

import (
	"errors"
)

// AST is the abstract syntax tree for schema definitions
type AST struct {
	Entities []Entity
}

type Entity struct {
	Name   string
	Fields []Field
}

type Field struct {
	Name string
	Type string
	Tag  map[string]string
}

// ParseSchema reads a DSL file and returns its AST
type ParseFunc func(input []byte) (AST, error)

func ParseSchema(input []byte) (AST, error) {
	// TODO: implement parser
	return AST{}, errors.New("not implemented")
}
