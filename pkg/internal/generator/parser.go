package generator

import (
	"errors"
	"regexp"
	"strings"
)

// Field describes a single model field, including metadata for migration generation.
type Field struct {
	Name          string   // Go struct field name
	Type          string   // Go type (e.g., "string", "int", "time.Time", "uuid.UUID")
	Default       *string  // Default value expression, if any
	NotNull       bool     // True if the field is required (no null)
	PrimaryKey    bool     // True if this field is a primary key
	AutoIncrement bool     // True if this field uses auto-increment (serial)
	EnumValues    []string // List of enum options, if the field is an enum
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
	schema := string(input)
	modelRe := regexp.MustCompile(`model\s+(\w+)\s*{([^}]*)}`)
	matches := modelRe.FindAllStringSubmatch(schema, -1)
	if len(matches) == 0 {
		return AST{}, errors.New("no model definitions found")
	}

	var ast AST
	for _, m := range matches {
		name := m[1]
		block := m[2]
		lines := strings.Split(block, "\n")

		var fields []Field
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			fname := parts[0]
			ptype := parts[1]
			var goType string
			switch {
			case strings.HasPrefix(ptype, "String"):
				goType = "string"
			case strings.HasPrefix(ptype, "Int"):
				goType = "int"
			case strings.HasPrefix(ptype, "Float"):
				goType = "float64"
			case strings.HasPrefix(ptype, "Boolean"):
				goType = "bool"
			case strings.HasPrefix(ptype, "DateTime"):
				goType = "time.Time"
			case strings.HasPrefix(ptype, "Json"):
				goType = "map[string]interface{}"
			default:
				goType = ptype
			}
			f := Field{
				Name: fname,
				Type: goType,
			}
			// Nullability: optional if type ends with '?'
			if strings.HasSuffix(ptype, "?") {
				f.Default = nil
			}
			// Primary key: @id
			if strings.Contains(line, "@id") {
				f.PrimaryKey = true
			}
			// Default and AutoIncrement: @default(expr)
			if idx := strings.Index(line, "@default("); idx >= 0 {
				expr := line[idx+len("@default("):]
				if end := strings.LastIndex(expr, ")"); end >= 0 {
					def := expr[:end]
					if def == "autoincrement()" {
						f.AutoIncrement = true
					} else {
						f.Default = &def
					}
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
