// File: internal/dsl/parser.go
package dsl

import (
	"errors"
	"regexp"
	"strings"
)

// AST represents the abstract syntax tree of parsed schema definitions.
type AST struct {
	Entities []Entity
}

// Entity describes a database entity from the DSL.
type Entity struct {
	Name   string
	Fields []Field
}

// Field describes a single field/column in an entity, including optional tags.
type Field struct {
	Name          string
	Type          string
	NotNull       bool
	PrimaryKey    bool
	AutoIncrement bool
	Default       *string
	EnumValues    []string
}

// ParseSchema reads a DSL file and returns its AST.
func ParseSchema(input []byte) (AST, error) {
	text := string(input)
	// Regex to find entity declarations: entity.<Name>()
	entityRe := regexp.MustCompile(`entity\.(\w+)\s*\(\)`)
	entityMatches := entityRe.FindAllStringSubmatch(text, -1)
	if len(entityMatches) == 0 {
		return AST{}, errors.New("no entity definitions found")
	}

	var ast AST
	for _, em := range entityMatches {
		name := em[1]
		// Scope text to after this entity declaration
		idx := strings.Index(text, em[0])
		if idx < 0 {
			continue
		}
		block := text[idx:]
		lines := strings.Split(block, "\n")

		var fields []Field
		// Regex for Field("name", "Type")
		fieldRe := regexp.MustCompile(`Field\("([^"]+)",\s*"([^"]+)"\)`)
		// Regex for chained calls: .Method(args)
		chainRe := regexp.MustCompile(`\.(\w+)\(([^)]*)\)`)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Field(") {
				continue
			}
			fm := fieldRe.FindStringSubmatch(line)
			if fm == nil {
				continue
			}
			f := Field{
				Name: fm[1],
				Type: fm[2],
			}
			// Process chained methods
			for _, m := range chainRe.FindAllStringSubmatch(line, -1) {
				method := m[1]
				arg := m[2]
				switch method {
				case "NotNull":
					f.NotNull = true
				case "PrimaryKey":
					f.PrimaryKey = true
				case "AutoIncrement":
					f.AutoIncrement = true
				case "Default":
					// strip quotes if present
					val := strings.Trim(arg, `"`)
					f.Default = &val
				case "Enum":
					// parse comma-separated values inside quotes
					args := strings.Split(arg, ",")
					for i := range args {
						args[i] = strings.Trim(strings.TrimSpace(args[i]), `"`)
					}
					f.EnumValues = args
				}
			}
			fields = append(fields, f)
		}
		ast.Entities = append(ast.Entities, Entity{
			Name:   name,
			Fields: fields,
		})
	}
	return ast, nil
}
