package generator

import (
	"errors"
	"regexp"
	"strings"
)

// Index describes a model-level index on one or more fields.
type Index struct {
	Fields []string // field names in the index
}

// Enum describes a Prisma enum type with possible values.
type Enum struct {
	Name   string
	Values []string
}

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

// Relation describes a list‐based relation field (e.g., votesAsNetwork Vote[]).
type Relation struct {
	Name string // Go struct field name (from schema line, e.g. "votesAsNetwork")
	Type string // Target model name (e.g. "Vote")
	JoinTableName string // new field for many-to-many join table name
}

// Entity describes a model.
type Entity struct {
	Name      string
	Fields    []Field
	Indexes   []Index    // added to capture @@index definitions
	Relations []Relation // list of related model relations (fieldName + target type)
}

// AST is the parsed schema representation.
type AST struct {
	Enums    []Enum
	Entities []Entity
}

// ParseSchema parses a Prisma schema into an AST.
func ParseSchema(input []byte) (AST, error) {
	schema := string(input)

	var ast AST

	// First, parse enum blocks
	enumRe := regexp.MustCompile(`enum\s+(\w+)\s*{([^}]*)}`)
	enumMatches := enumRe.FindAllStringSubmatch(schema, -1)
	for _, em := range enumMatches {
		enumName := em[1]
		block := em[2]
		lines := strings.Split(block, "\n")
		var values []string
		for _, line := range lines {
			val := strings.TrimSpace(line)
			if val == "" {
				continue
			}
			values = append(values, val)
		}
		ast.Enums = append(ast.Enums, Enum{
			Name:   enumName,
			Values: values,
		})
	}

	// Remove enum blocks from schema to avoid parsing them as models
	schemaWithoutEnums := enumRe.ReplaceAllString(schema, "")

	modelRe := regexp.MustCompile(`model\s+(\w+)\s*{([^}]*)}`)
	matches := modelRe.FindAllStringSubmatch(schemaWithoutEnums, -1)
	if len(matches) == 0 {
		return AST{}, errors.New("no model definitions found")
	}

	for _, m := range matches {
		name := m[1]
		block := m[2]
		lines := strings.Split(block, "\n")

		var indexes []Index
		// First pass: capture any @@index(...) definitions
		indexRe := regexp.MustCompile(`^\s*@@index\(\[([^\]]+)\]\)`)
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if m := indexRe.FindStringSubmatch(trimmed); m != nil {
				// m[1] contains comma-separated field names
				cols := strings.Split(m[1], ",")
				for i := range cols {
					cols[i] = strings.TrimSpace(strings.Trim(cols[i], `"`))
				}
				indexes = append(indexes, Index{Fields: cols})
			}
		}

		var fields []Field
		var relations []Relation // accumulate list-based relations for this entity
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			// Skip model-level index lines (already handled)
			if indexRe.MatchString(line) {
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
				// If ptype matches an enum name, use the enum Go type
				foundEnum := false
				for _, enum := range ast.Enums {
					if ptype == enum.Name {
						goType = enum.Name
						foundEnum = true
						break
					}
				}
				if !foundEnum {
					goType = ptype
				}
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
			// Handle PostgreSQL UUID annotation: @db.Uuid
			if strings.Contains(line, "@db.Uuid") {
				f.Type = "uuid.UUID"
			}

			// Handle @updatedAt like not null with default now()
			if strings.Contains(line, "@updatedAt") {
				f.NotNull = true
				if f.Default == nil {
					now := "now()"
					f.Default = &now
				}
			}

			// Detect list-based relation fields, e.g. "votesAsNetwork Vote[]"
			if strings.HasSuffix(ptype, "[]") {
				// Base type is the model name without the "[]"
				base := strings.TrimSuffix(ptype, "[]")
				// Record a list relation field with field name fname and target base
				relations = append(relations, Relation{
					Name: fname,
					Type: base,
				})
				continue
			}
			// Skip explicit @relation annotations (handled via list fields)
			if strings.Contains(line, "@relation") {
				continue
			}

			fields = append(fields, f)
		}

		// After building 'fields' slice:
		ent := Entity{
			Name:      name,
			Fields:    fields,
			Indexes:   indexes, // assign parsed indexes
			Relations: relations,
		}
		ast.Entities = append(ast.Entities, ent)
	}

	// Compute JoinTableName for many-to-many relations
	for i, ent := range ast.Entities {
		for ri, rel := range ent.Relations {
			// find target entity index
			for j, otherEnt := range ast.Entities {
				if otherEnt.Name != rel.Type {
					continue
				}
				// check if other has reciprocal list relation back to ent.Name
				hasReciprocal := false
				for _, r2 := range otherEnt.Relations {
					if r2.Type == ent.Name {
						hasReciprocal = true
						break
					}
				}
				if hasReciprocal {
					// use lexicographically smaller names to form table name
					nameA := strings.ToLower(ent.Name)
					nameB := strings.ToLower(otherEnt.Name)
					if nameA < nameB {
						join := nameA + "_" + nameB
						ast.Entities[i].Relations[ri].JoinTableName = join
						// set for the reciprocal side
						for rj, r2 := range ast.Entities[j].Relations {
							if r2.Type == ent.Name {
								ast.Entities[j].Relations[rj].JoinTableName = join
								break
							}
						}
					}
				}
				break
			}
		}
	}

	return ast, nil
}
