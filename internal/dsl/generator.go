// File: internal/dsl/generator.go
package dsl

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

// GenerateGo writes Go code for each entity in the AST
type Generator struct {
	Template *template.Template
}

func NewGenerator() *Generator {
	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasTime":   hasTime,
		"hasUUID":   hasUUID,
		"buildTags": buildTags,
	}
	tmpl := template.Must(template.
		New("entity").
		Funcs(funcMap).
		Parse(entityTemplate))
	return &Generator{Template: tmpl}
}

// hasTime returns true if any field uses time.Time.
func hasTime(fields []Field) bool {
	for _, f := range fields {
		if f.Type == "time.Time" {
			return true
		}
	}
	return false
}

// hasUUID returns true if any field uses uuid.UUID.
func hasUUID(fields []Field) bool {
	for _, f := range fields {
		if f.Type == "uuid.UUID" {
			return true
		}
	}
	return false
}

// buildTags is a placeholder for tag building logic.
func buildTags(f Field) string {
	return "  `db:\"" + strings.ToLower(f.Name) + "\"`"
}

func (g *Generator) Generate(ast AST, outDir string) error {
	for _, ent := range ast.Entities {
		path := fmt.Sprintf("%s/%s.go", outDir, ent.Name)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := g.Template.Execute(f, ent); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

const entityTemplate = `package {{ lower .Name }}

import (
{{- if hasTime .Fields }}
    "time"
{{- end }}
{{- if hasUUID .Fields }}
    uuid "github.com/google/uuid"
{{- end }}
)

type {{ .Name }} struct {
{{- range .Fields }}
    {{ .Name }} {{ .Type }}{{ buildTags . }}
{{- end }}
}
`
