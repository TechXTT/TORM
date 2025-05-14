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
	tmpl := template.Must(template.
		New("entity").
		Funcs(template.FuncMap{
			"lower": func(s string) string {
				return strings.ToLower(s)
			},
		}).
		Parse(entityTemplate))
	return &Generator{Template: tmpl}
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

const entityTemplate = `package {{.Name}}
struct {
{{- range .Fields }}
{{.Name}} {{.Type}}  + "" + db:"{{.Name | lower}}" + "" + 
{{- end }}
} + "" + "`
