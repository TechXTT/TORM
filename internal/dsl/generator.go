package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// GenerateGo writes Go code for each entity in the AST
type Generator struct {
	Template *template.Template
}

type entityTemplateData struct {
	Package string
	Entity
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
	// Determine package name from output directory
	pkgName := filepath.Base(outDir)
	// Ensure directory exists
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, ent := range ast.Entities {
		data := entityTemplateData{
			Package: pkgName,
			Entity:  ent,
		}
		fileName := fmt.Sprintf("%s.go", strings.ToLower(ent.Name))
		filePath := filepath.Join(outDir, fileName)
		f, err := os.Create(filePath)
		if err != nil {
			return err
		}
		if err := g.Template.Execute(f, data); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

const entityTemplate = `package {{ .Package }}

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
