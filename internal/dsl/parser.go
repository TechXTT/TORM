// File: internal/dsl/parser.go
package dsl

import (
	"errors"
	"regexp"
	"strings"
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
	text := string(input)
	// Find entity declarations
	entityRe := regexp.MustCompile(`entity\.(\w+)\s*\(\)`)
	entityMatches := entityRe.FindAllStringSubmatch(text, -1)
	if len(entityMatches) == 0 {
		return AST{}, errors.New("no entity definitions found")
	}

	var ast AST
	for _, em := range entityMatches {
		name := em[1]
		// Limit processing to text after this entity declaration
		idx := strings.Index(text, em[0])
		sub := text[idx:]
		lines := strings.Split(sub, "\n")

		var fieldsList []Field
		// Regex for Field definitions
		fieldRe := regexp.MustCompile(`Field\("([^"]+)",\s*"([^"]+)"\)`)
		// Regex for chained methods
		methodRe := regexp.MustCompile(`\.(\w+)\(([^)]*)\)`)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Field(") {
				continue
			}
			fm := fieldRe.FindStringSubmatch(line)
			if fm == nil {
				continue
			}
			fname := fm[1]
			ftype := fm[2]
			tagMap := make(map[string]string)

			// Extract chained method calls for field options
			for _, m := range methodRe.FindAllStringSubmatch(line, -1) {
				method := m[1]
				arg := m[2]
				switch method {
				case "NotNull":
					tagMap["NotNull"] = "true"
				case "PrimaryKey":
					tagMap["PrimaryKey"] = "true"
				case "AutoIncrement":
					tagMap["AutoIncrement"] = "true"
				case "Default":
					tagMap["Default"] = arg
				case "Enum":
					tagMap["Enum"] = arg
				}
			}
			fieldsList = append(fieldsList, Field{
				Name: fname,
				Type: ftype,
				Tag:  tagMap,
			})
		}
		ast.Entities = append(ast.Entities, Entity{
			Name:   name,
			Fields: fieldsList,
		})
	}

	return ast, nil
}
