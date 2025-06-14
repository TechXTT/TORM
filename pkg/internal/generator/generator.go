package generator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	// adjust to actual Prisma Go client import path
)

// formatFile formats the given file using gofmt.
func formatFile(filePath string) error {
	cmd := exec.Command("go", "fmt", filePath)
	return cmd.Run()
}

var modelTemplate = template.Must(template.New("model").
	Funcs(template.FuncMap{
		"hasTime": func(fields []Field) bool {
			for _, f := range fields {
				if f.Type == "time.Time" {
					return true
				}
			}
			return false
		},
		"hasUUID": func(fields []Field) bool {
			for _, f := range fields {
				if f.Type == "uuid.UUID" {
					return true
				}
			}
			return false
		},
		"goType": func(t string) string {
			if strings.HasSuffix(t, "[]") {
				return "[]" + strings.TrimSuffix(t, "[]")
			}
			return t
		},
		"export": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
	}).
	Parse(`package models

// Code generated by TORM; DO NOT EDIT.
		
import (
{{- if hasUUID .Fields }}
    "github.com/google/uuid"
{{- end }}
{{- if hasTime .Fields }}
    "time"
{{- end }}
)

type {{ .Name }} struct {
{{- range .Fields }}
    {{ export .Name }} {{ goType .Type }}
{{- end }}
{{- range .Relations }}
    {{ export .Name }} []{{ .Type }}
{{- end }}
}
`))

var clientTemplate = template.Must(template.New("client").
	Funcs(template.FuncMap{
		"lower": strings.ToLower,
		"export": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
	}).
	Parse(`package models

// Code generated by TORM; DO NOT EDIT.

import (
    "context"
    "database/sql"
    "fmt"
    "io/ioutil"
    "os"
    "reflect"
    "regexp"
    "strings"
    "sync"
    "time"

    _ "github.com/lib/pq"
)


// Client wraps a database connection and provides per-model services.
type Client struct {
    dsn  string
    db   *sql.DB
    once sync.Once
    err  error
}

// NewClient reads DSN from prisma/schema.prisma and returns a Client.
func NewClient() *Client {
    data, err := ioutil.ReadFile("prisma/schema.prisma")
    if err != nil {
        panic(fmt.Sprintf("cannot read schema: %v", err))
    }
    re := regexp.MustCompile(` + "`url\\s*=\\s*(?:env\\(\"([^\"]+)\"\\)|\"([^\\\"]+)\")`" + `)
    m := re.FindStringSubmatch(string(data))
    var dsn string
    if m[1] != "" {
        dsn = strings.Trim(os.Getenv(m[1]), ` + `""` + `)
    } else {
        dsn = strings.Trim(m[2], ` + `""` + `)
    }
    // disable SSL if not set
    if strings.HasPrefix(dsn, "postgres://") && !strings.Contains(dsn, "sslmode=") {
        sep := "?"
        if strings.Contains(dsn, "?") { sep = "&" }
        dsn += sep + "sslmode=disable"
    }
    return &Client{dsn: dsn}
}

// connect opens the DB once.
func (c *Client) connect() (*sql.DB, error) {
    c.once.Do(func() {
        c.db, c.err = sql.Open("postgres", c.dsn)
        if c.err != nil { return }
        c.err = c.db.PingContext(context.Background())
    })
    return c.db, c.err
}
        

// TimeOrZero implements sql.Scanner for time.Time fields, converting NULL to zero time.
type TimeOrZero time.Time

// Scan implements the sql.Scanner interface.
func (t *TimeOrZero) Scan(value interface{}) error {
    if value == nil {
        *t = TimeOrZero(time.Time{})
        return nil
    }
    tm, ok := value.(time.Time)
    if !ok {
        return fmt.Errorf("cannot scan type %T into TimeOrZero", value)
    }
    *t = TimeOrZero(tm)
    return nil
}

// scanDest returns a slice of destination pointers for scanning into struct fields.
// It substitutes sql.NullString for any string field, and TimeOrZero for any time.Time field.
func scanDest(m interface{}) []interface{} {
    v := reflect.ValueOf(m)
    if v.Kind() != reflect.Ptr || v.IsNil() {
        return nil
    }
    v = v.Elem()
    if v.Kind() != reflect.Struct {
        return nil
    }

    var dest []interface{}
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := field.Type()

        switch fieldType {
        case reflect.TypeOf(time.Time{}):
            // Use a TimeOrZero placeholder
            var tmp TimeOrZero
            dest = append(dest, &tmp)

        case reflect.TypeOf(""):
            // Use a sql.NullString placeholder instead of a bare string
            var tmp sql.NullString
            dest = append(dest, &tmp)

        default:
            if field.CanAddr() {
                dest = append(dest, field.Addr().Interface())
            } else {
                var dummy interface{}
                dest = append(dest, &dummy)
            }
        }
    }
    return dest
}

{{- range .Entities }}

// {{ .Name }}Service provides DB operations for the {{ .Name }} model.
type {{ .Name }}Service struct {
    db *sql.DB
}

// {{ .Name }}Service returns a new service for {{ .Name }}.
func (c *Client) {{ .Name }}Service() (*{{ .Name }}Service, error) {
    db, err := c.connect()
    if err != nil {
        return nil, fmt.Errorf("connect: %w", err)
    }
    return &{{ .Name }}Service{db: db}, nil
}

// FindUnique retrieves a single {{ .Name }} by unique filter.
func (svc *{{ .Name }}Service) FindUnique(ctx context.Context, where map[string]interface{}) (*{{ .Name }}, error) {
    whereClause, args := buildWhere(where)
    cols := []string{ {{- range $i,$f := .Fields }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
    query := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", strings.Join(cols, ", "), "{{lower .Name}}", whereClause)
    row := svc.db.QueryRowContext(ctx, query, args...)
    var m {{ .Name }}
    dest := scanDest(&m)
    dest = dest[:len(cols)] // Ensure we only scan expected columns
    if err := row.Scan(dest...); err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, err
    }
    structVal := reflect.ValueOf(&m).Elem()
    for i := 0; i < len(cols); i++ {
        switch placeholder := dest[i].(type) {
        case *sql.NullString:
            if placeholder.Valid {
                structVal.Field(i).SetString(placeholder.String)
            } else {
                structVal.Field(i).SetString("")
            }
        case *TimeOrZero:
            t := time.Time(*placeholder)
            structVal.Field(i).Set(reflect.ValueOf(t))
        }
    }
    // Load one‐level relations
    {{- $ent := . }}
    {{- range .Relations }}
        {{- if .JoinTableName }}
            // load many-to-many {{ .Type }} via join table {{ .JoinTableName }}
            {
            colsRel := []string{ {{- range $i,$f := call $.EntitiesMap .Type }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
            rows{{ .Name }}, err := svc.db.QueryContext(ctx,
                fmt.Sprintf(
                    "SELECT %s FROM %s t JOIN %s jt ON t.id = jt.%s_id WHERE jt.%s_id = $1",
                    strings.Join(colsRel, ", "),
                    "{{ lower .Type }}",
                    "{{ lower .JoinTableName }}",
                    "{{ lower .Type }}",
                    "{{ lower $ent.Name }}",
                ),
                m.Id,
            )
            if err == nil {
                defer rows{{ .Name }}.Close()
                for rows{{ .Name }}.Next() {
                    var related {{ .Type }}
                    destRel := scanDest(&related)
                    destRel = destRel[:len(colsRel)]
                    if err := rows{{ .Name }}.Scan(destRel...); err == nil {
                        // Copy scanned values into struct fields
                        structValRel := reflect.ValueOf(&related).Elem()
                        for i := 0; i < len(colsRel); i++ {
                            switch placeholder := destRel[i].(type) {
                            case *sql.NullString:
                                if placeholder.Valid {
                                    structValRel.Field(i).SetString(placeholder.String)
                                } else {
                                    structValRel.Field(i).SetString("")
                                }
                            case *TimeOrZero:
                                t := time.Time(*placeholder)
                                structValRel.Field(i).Set(reflect.ValueOf(t))
                            }
                        }
                        m.{{ export .Name }} = append(m.{{ export .Name }}, related)
                    }
                }
            }
            }
        {{- else }}
            // one-to-many load of {{ .Type }}
            {
            colsRel := []string{ {{- range $i,$f := call $.EntitiesMap .Type }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
            rows{{ .Name }}, err := svc.db.QueryContext(ctx,
                fmt.Sprintf("SELECT %s FROM %s WHERE %sid = $1",
                    strings.Join(colsRel, ", "),
                    "{{ lower .Type }}",
                    "{{ lower $ent.Name }}",
                ),
                m.Id,
            )
            if err == nil {
                defer rows{{ .Name }}.Close()
                for rows{{ .Name }}.Next() {
                    var related {{ .Type }}
                    destRel := scanDest(&related)
                    destRel = destRel[:len(colsRel)]
                    if err := rows{{ .Name }}.Scan(destRel...); err == nil {
                        // Copy scanned values into struct fields
                        structValRel := reflect.ValueOf(&related).Elem()
                        for i := 0; i < len(colsRel); i++ {
                            switch placeholder := destRel[i].(type) {
                            case *sql.NullString:
                                if placeholder.Valid {
                                    structValRel.Field(i).SetString(placeholder.String)
                                } else {
                                    structValRel.Field(i).SetString("")
                                }
                            case *TimeOrZero:
                                t := time.Time(*placeholder)
                                structValRel.Field(i).Set(reflect.ValueOf(t))
                            }
                        }
                        m.{{ export .Name }} = append(m.{{ export .Name }}, related)
                    }
                }
            }
            }
        {{- end }}
    {{- end }}
    return &m, nil
}

// FindUniqueOrThrow retrieves a single {{ .Name }} or returns an error if not found.
func (svc *{{ .Name }}Service) FindUniqueOrThrow(ctx context.Context, where map[string]interface{}) (*{{ .Name }}, error) {
    rec, err := svc.FindUnique(ctx, where)
    if err != nil {
        return nil, err
    }
    if rec == nil {
        return nil, fmt.Errorf("{{ .Name }} not found")
    }
    return rec, nil
}

// FindFirst retrieves a single {{ .Name }} matching filters, or nil if none.
func (svc *{{ .Name }}Service) FindFirst(ctx context.Context, where map[string]interface{}) (*{{ .Name }}, error) {
    whereClause, args := buildWhere(where)
    cols := []string{ {{- range $i,$f := .Fields }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
    query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), "{{lower .Name}}")
    if whereClause != "" {
        query += " WHERE " + whereClause
    }
    row := svc.db.QueryRowContext(ctx, query, args...)
    var m {{ .Name }}
    dest := scanDest(&m)
    dest = dest[:len(cols)] // Ensure we only scan expected columns
    if err := row.Scan(dest...); err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, err
    }
    structVal := reflect.ValueOf(&m).Elem()
    for i := 0; i < len(cols); i++ {
        switch placeholder := dest[i].(type) {
        case *sql.NullString:
            if placeholder.Valid {
                structVal.Field(i).SetString(placeholder.String)
            } else {
                structVal.Field(i).SetString("")
            }
        case *TimeOrZero:
            t := time.Time(*placeholder)
            structVal.Field(i).Set(reflect.ValueOf(t))
        }
    }
    // Load one‐level relations
    {{- $ent := . }}
    {{- range .Relations }}
        {{- if .JoinTableName }}
            // load many-to-many {{ .Type }} via join table {{ .JoinTableName }}
            {
            colsRel := []string{ {{- range $i,$f := call $.EntitiesMap .Type }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
            rows{{ .Name }}, err := svc.db.QueryContext(ctx,
                fmt.Sprintf(
                    "SELECT %s FROM %s t JOIN %s jt ON t.id = jt.%s_id WHERE jt.%s_id = $1",
                    strings.Join(colsRel, ", "),
                    "{{ lower .Type }}",
                    "{{ lower .JoinTableName }}",
                    "{{ lower .Type }}",
                    "{{ lower $ent.Name }}",
                ),
                m.Id,
            )
            if err == nil {
                defer rows{{ .Name }}.Close()
                for rows{{ .Name }}.Next() {
                    var related {{ .Type }}
                    destRel := scanDest(&related)
                    destRel = destRel[:len(colsRel)]
                    if err := rows{{ .Name }}.Scan(destRel...); err == nil {
                        // Copy scanned values into struct fields
                        structValRel := reflect.ValueOf(&related).Elem()
                        for i := 0; i < len(colsRel); i++ {
                            switch placeholder := destRel[i].(type) {
                            case *sql.NullString:
                                if placeholder.Valid {
                                    structValRel.Field(i).SetString(placeholder.String)
                                } else {
                                    structValRel.Field(i).SetString("")
                                }
                            case *TimeOrZero:
                                t := time.Time(*placeholder)
                                structValRel.Field(i).Set(reflect.ValueOf(t))
                            }
                        }
                        m.{{ export .Name }} = append(m.{{ export .Name }}, related)
                    }
                }
            }
            }
        {{- else }}
            // one-to-many load of {{ .Type }}
            {
            colsRel := []string{ {{- range $i,$f := call $.EntitiesMap .Type }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
            rows{{ .Name }}, err := svc.db.QueryContext(ctx,
                fmt.Sprintf("SELECT %s FROM %s WHERE %sid = $1",
                    strings.Join(colsRel, ", "),
                    "{{ lower .Type }}",
                    "{{ lower $ent.Name }}",
                ),
                m.Id,
            )
            if err == nil {
                defer rows{{ .Name }}.Close()
                for rows{{ .Name }}.Next() {
                    var related {{ .Type }}
                    destRel := scanDest(&related)
                    destRel = destRel[:len(colsRel)]
                    if err := rows{{ .Name }}.Scan(destRel...); err == nil {
                        // Copy scanned values into struct fields
                        structValRel := reflect.ValueOf(&related).Elem()
                        for i := 0; i < len(colsRel); i++ {
                            switch placeholder := destRel[i].(type) {
                            case *sql.NullString:
                                if placeholder.Valid {
                                    structValRel.Field(i).SetString(placeholder.String)
                                } else {
                                    structValRel.Field(i).SetString("")
                                }
                            case *TimeOrZero:
                                t := time.Time(*placeholder)
                                structValRel.Field(i).Set(reflect.ValueOf(t))
                            }
                        }
                        m.{{ export .Name }} = append(m.{{ export .Name }}, related)
                    }
                }
            }
            }
        {{- end }}
    {{- end }}
    return &m, nil
}

// FindFirstOrThrow retrieves the first {{ .Name }} or errors if none.
func (svc *{{ .Name }}Service) FindFirstOrThrow(ctx context.Context, where map[string]interface{}) (*{{ .Name }}, error) {
    rec, err := svc.FindFirst(ctx, where)
    if err != nil {
        return nil, err
    }
    if rec == nil {
        return nil, fmt.Errorf("no {{ .Name }} found")
    }
    return rec, nil
}

// FindMany retrieves multiple {{ .Name }} records matching filters.
func (svc *{{ .Name }}Service) FindMany(ctx context.Context, where map[string]interface{}, orderBy []string, skip, take int) ([]*{{ .Name }}, error) {
    whereClause, args := buildWhere(where)
    {
    cols := []string{ {{- range $i,$f := .Fields }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
    query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), "{{lower .Name}}")
    if whereClause != "" {
        query += " WHERE " + whereClause
    }
    if len(orderBy) > 0 {
        query += " ORDER BY " + strings.Join(orderBy, ", ")
    }
    if take > 0 {
        query += fmt.Sprintf(" LIMIT %d", take)
    }
    if skip > 0 {
        query += fmt.Sprintf(" OFFSET %d", skip)
    }
    rows, err := svc.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var result []*{{ .Name }}
    for rows.Next() {
        var m {{ .Name }}
        dest := scanDest(&m)
        dest = dest[:len(cols)] // Ensure we only scan expected columns
        if err := rows.Scan(dest...); err != nil {
            return nil, err
        }
        structVal := reflect.ValueOf(&m).Elem()
        for i := 0; i < len(cols); i++ {
            switch placeholder := dest[i].(type) {
            case *sql.NullString:
                if placeholder.Valid {
                    structVal.Field(i).SetString(placeholder.String)
                } else {
                    structVal.Field(i).SetString("")
                }
            case *TimeOrZero:
                t := time.Time(*placeholder)
                structVal.Field(i).Set(reflect.ValueOf(t))
            }
        }
        // Load one‐level relations
        {{- $ent := . }}
        {{- range .Relations }}
            {{- if .JoinTableName }}
                // load many-to-many {{ .Type }} via join table {{ .JoinTableName }}
                {
                colsRel := []string{ {{- range $i,$f := call $.EntitiesMap .Type }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
                rows{{ .Name }}, err := svc.db.QueryContext(ctx,
                    fmt.Sprintf(
                        "SELECT %s FROM %s t JOIN %s jt ON t.id = jt.%s_id WHERE jt.%s_id = $1",
                        strings.Join(colsRel, ", "),
                        "{{ lower .Type }}",
                        "{{ lower .JoinTableName }}",
                        "{{ lower .Type }}",
                        "{{ lower $ent.Name }}",
                    ),
                    m.Id,
                )
                if err == nil {
                    defer rows{{ .Name }}.Close()
                    for rows{{ .Name }}.Next() {
                        var related {{ .Type }}
                        destRel := scanDest(&related)
                        destRel = destRel[:len(colsRel)]
                        if err := rows{{ .Name }}.Scan(destRel...); err == nil {
                            // Copy scanned values into struct fields
                            structValRel := reflect.ValueOf(&related).Elem()
                            for i := 0; i < len(colsRel); i++ {
                                switch placeholder := destRel[i].(type) {
                                case *sql.NullString:
                                    if placeholder.Valid {
                                        structValRel.Field(i).SetString(placeholder.String)
                                    } else {
                                        structValRel.Field(i).SetString("")
                                    }
                                case *TimeOrZero:
                                    t := time.Time(*placeholder)
                                    structValRel.Field(i).Set(reflect.ValueOf(t))
                                }
                            }
                            m.{{ export .Name }} = append(m.{{ export .Name }}, related)
                        } else {
                            fmt.Println("Error scanning {{ .Type }}:", err)
                        }
                    }
                }
                }
            {{- else }}
                // one-to-many load of {{ .Type }}
                {
                colsRel := []string{ {{- range $i,$f := call $.EntitiesMap .Type }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
                rows{{ .Name }}, err := svc.db.QueryContext(ctx,
                    fmt.Sprintf("SELECT %s FROM %s WHERE %sid = $1",
                        strings.Join(colsRel, ", "),
                        "{{ lower .Type }}",
                        "{{ lower $ent.Name }}",
                    ),
                    m.Id,
                )
                if err == nil {
                    defer rows{{ .Name }}.Close()
                    for rows{{ .Name }}.Next() {
                        var related {{ .Type }}
                        destRel := scanDest(&related)
                        destRel = destRel[:len(colsRel)]
                        if err := rows{{ .Name }}.Scan(destRel...); err == nil {
                            // Copy scanned values into struct fields
                            structValRel := reflect.ValueOf(&related).Elem()
                            for i := 0; i < len(colsRel); i++ {
                                switch placeholder := destRel[i].(type) {
                                case *sql.NullString:
                                    if placeholder.Valid {
                                        structValRel.Field(i).SetString(placeholder.String)
                                    } else {
                                        structValRel.Field(i).SetString("")
                                    }
                                case *TimeOrZero:
                                    t := time.Time(*placeholder)
                                    structValRel.Field(i).Set(reflect.ValueOf(t))
                                }
                            }
                            m.{{ export .Name }} = append(m.{{ export .Name }}, related)
                        } else {
                            fmt.Println("Error scanning {{ .Type }}:", err)
                        }
                    }
                }
                }
            {{- end }}
        {{- end }}
        result = append(result, &m)
    }
    return result, nil
    }
}

// Create inserts a new {{ .Name }} record and updates the passed model with any returned values.
func (svc *{{ .Name }}Service) Create(ctx context.Context, m *{{ .Name }}) error {
    // Extract values from the struct into a map
    data := make(map[string]interface{})
    {{- range .Fields }}
    {{- if not .PrimaryKey }}
    data["{{lower .Name}}"] = m.{{export .Name}}
    {{- end }}
    {{- end }}

    // Remove empty string entries so they won't be inserted
    for k, v := range data {
        if str, ok := v.(string); ok {
            if str == "" {
                delete(data, k)
            }
        }
    }

    cols, placeholders, args := buildInsert(data)
    colsList := strings.Join(cols, ", ")
    phList := strings.Join(placeholders, ", ")
    // Return all columns to repopulate the struct
    allCols := []string{ {{- range $i,$f := .Fields }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
    query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s", "{{lower .Name}}", colsList, phList, strings.Join(allCols, ", "))
    row := svc.db.QueryRowContext(ctx, query, args...)

    dest := scanDest(m)
    if err := row.Scan(dest...); err != nil {
        return err
    }
    structVal := reflect.ValueOf(m).Elem()
    for i := 0; i < structVal.NumField(); i++ {
        switch placeholder := dest[i].(type) {
        case *sql.NullString:
            if placeholder.Valid {
                structVal.Field(i).SetString(placeholder.String)
            } else {
                structVal.Field(i).SetString("")
            }
        case *TimeOrZero:
            t := time.Time(*placeholder)
            structVal.Field(i).Set(reflect.ValueOf(t))
        }
    }
    return nil
}

// Update modifies an existing {{ .Name }} record and updates the passed model pointer.
func (svc *{{ .Name }}Service) Update(ctx context.Context, where map[string]interface{}, m *{{ .Name }}) error {
    // Extract new values from the struct into a map
    data := make(map[string]interface{})
    {{- range .Fields }}
    {{- if not .PrimaryKey }}
    data["{{lower .Name}}"] = m.{{export .Name}}
    {{- end }}
    {{- end }}

    setClause, setArgs := buildSet(data, 1)
    whereClause, whereArgs := buildWhereOffset(where, len(setArgs)+1)
    args := append(setArgs, whereArgs...)
    allCols := []string{ {{- range $i,$f := .Fields }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
    query := fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING %s", "{{lower .Name}}", setClause, whereClause, strings.Join(allCols, ", "))
    row := svc.db.QueryRowContext(ctx, query, args...)

    dest := scanDest(m)
    if err := row.Scan(dest...); err != nil {
        return err
    }
    structVal := reflect.ValueOf(m).Elem()
    for i := 0; i < structVal.NumField(); i++ {
        switch placeholder := dest[i].(type) {
        case *sql.NullString:
            if placeholder.Valid {
                structVal.Field(i).SetString(placeholder.String)
            } else {
                structVal.Field(i).SetString("")
            }
        case *TimeOrZero:
            t := time.Time(*placeholder)
            structVal.Field(i).Set(reflect.ValueOf(t))
        }
    }
    return nil
}

// Upsert creates or updates a {{ .Name }} record and updates the passed model pointer.
func (svc *{{ .Name }}Service) Upsert(ctx context.Context, where map[string]interface{}, m *{{ .Name }}) error {
    tx, err := svc.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    existing, err := svc.FindUnique(ctx, where)
    if err != nil {
        tx.Rollback()
        return err
    }
    if existing == nil {
        if err := svc.Create(ctx, m); err != nil {
            tx.Rollback()
            return err
        }
    } else {
        if err := svc.Update(ctx, where, m); err != nil {
            tx.Rollback()
            return err
        }
    }
    if err := tx.Commit(); err != nil {
        return err
    }
    return nil
}

// Delete removes a {{ .Name }} record by unique filter.
func (svc *{{ .Name }}Service) Delete(ctx context.Context, where map[string]interface{}) error {
    whereClause, args := buildWhere(where)
    query := fmt.Sprintf("DELETE FROM %s WHERE %s", "{{lower .Name}}", whereClause)
    _, err := svc.db.ExecContext(ctx, query, args...)
    return err
}

// Count returns the number of {{ .Name }} records matching 'where'.
func (svc *{{ .Name }}Service) Count(ctx context.Context, where map[string]interface{}) (int64, error) {
    whereClause, args := buildWhere(where)
    query := fmt.Sprintf("SELECT COUNT(*) FROM %s", "{{lower .Name}}")
    if whereClause != "" {
        query += " WHERE " + whereClause
    }
    row := svc.db.QueryRowContext(ctx, query, args...)
    var count int64
    if err := row.Scan(&count); err != nil {
        return 0, err
    }
    return count, nil
}

// CreateMany inserts multiple {{ .Name }} records in a single statement.
func (svc *{{ .Name }}Service) CreateMany(ctx context.Context, data []map[string]interface{}) (int64, error) {
    if len(data) == 0 {
        return 0, nil
    }
    cols, _, _ := buildInsert(data[0])
    var placeholders []string
    var args []interface{}
    index := 1
    for _, row := range data {
        var ph []string
        for _, col := range cols {
            args = append(args, row[col])
            ph = append(ph, fmt.Sprintf("$%d", index))
            index++
        }
        placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(ph, ", ")))
    }
    query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", "{{lower .Name}}", strings.Join(cols, ", "), strings.Join(placeholders, ", "))
    res, err := svc.db.ExecContext(ctx, query, args...)
    if err != nil {
        return 0, err
    }
    return res.RowsAffected()
}

// UpdateMany modifies multiple {{ .Name }} records matching 'where' and updates each passed model pointer.
func (svc *{{ .Name }}Service) UpdateMany(ctx context.Context, where []map[string]interface{}, ms []*{{ .Name }}) (int64, error) {
    if len(where) != len(ms) {
        return 0, fmt.Errorf("mismatch between filter list and model list length")
    }
    var totalAffected int64 = 0
    for i, m := range ms {
        filter := where[i]
        data := make(map[string]interface{})
        {{- range .Fields }}
        {{- if not .PrimaryKey }}
        data["{{lower .Name}}"] = m.{{export .Name}}
        {{- end }}
        {{- end }}

        setClause, setArgs := buildSet(data, 1)
        whereClause, whereArgs := buildWhereOffset(filter, len(setArgs)+1)
        args := append(setArgs, whereArgs...)
        allCols := []string{ {{- range $i,$f := .Fields }}{{if $i}}, {{end}}"{{lower $f.Name}}"{{- end }} }
        query := fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING %s", "{{lower .Name}}", setClause, whereClause, strings.Join(allCols, ", "))
        row := svc.db.QueryRowContext(ctx, query, args...)

        dest := scanDest(m)
        if err := row.Scan(dest...); err != nil {
            return totalAffected, err
        }
        structVal := reflect.ValueOf(m).Elem()
        for i := 0; i < structVal.NumField(); i++ {
            switch placeholder := dest[i].(type) {
            case *sql.NullString:
                if placeholder.Valid {
                    structVal.Field(i).SetString(placeholder.String)
                } else {
                    structVal.Field(i).SetString("")
                }
            case *TimeOrZero:
                t := time.Time(*placeholder)
                structVal.Field(i).Set(reflect.ValueOf(t))
            }
        }
        totalAffected++
    }
    return totalAffected, nil
}

// DeleteMany removes multiple {{ .Name }} records.
func (svc *{{ .Name }}Service) DeleteMany(ctx context.Context, where map[string]interface{}) (int64, error) {
    whereClause, args := buildWhere(where)
    query := fmt.Sprintf("DELETE FROM %s WHERE %s", "{{lower .Name}}", whereClause)
    res, err := svc.db.ExecContext(ctx, query, args...)
    if err != nil {
        return 0, err
    }
    return res.RowsAffected()
}

// Aggregate computes SQL aggregates for {{ .Name }}.
func (svc *{{ .Name }}Service) Aggregate(ctx context.Context, where map[string]interface{}, agg map[string][]string) (map[string]interface{}, error) {
    selectClauses := []string{}
    for key, fields := range agg {
        for _, f := range fields {
            selectClauses = append(selectClauses, fmt.Sprintf("%s(%s) AS %s_%s", strings.TrimPrefix(key, "_"), f, key, f))
        }
    }
    whereClause, args := buildWhere(where)
    query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectClauses, ", "), "{{lower .Name}}")
    if whereClause != "" {
        query += " WHERE " + whereClause
    }
    row := svc.db.QueryRowContext(ctx, query, args...)
    cols := strings.Split(strings.Join(selectClauses, ", "), ", ")
    vals := make([]interface{}, len(cols))
    result := map[string]interface{}{}
    dest := []interface{}{}
    for range vals {
        var v interface{}
        dest = append(dest, &v)
    }
    if err := row.Scan(dest...); err != nil {
        return nil, err
    }
    for i, col := range cols {
        parts := strings.Split(col, " AS ")
        alias := strings.TrimSpace(parts[1])
        result[alias] = *(dest[i].(*interface{}))
    }
    return result, nil
}

// GroupBy groups {{ .Name }} by specified fields and computes aggregates.
func (svc *{{ .Name }}Service) GroupBy(ctx context.Context, by []string, where map[string]interface{}, agg map[string][]string) ([]map[string]interface{}, error) {
    groupClause := strings.Join(by, ", ")
    selectClauses := []string{}
    for _, g := range by {
        selectClauses = append(selectClauses, g)
    }
    for key, fields := range agg {
        for _, f := range fields {
            selectClauses = append(selectClauses, fmt.Sprintf("%s(%s) AS %s_%s", strings.TrimPrefix(key, "_"), f, key, f))
        }
    }
    whereClause, args := buildWhere(where)
    query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectClauses, ", "), "{{lower .Name}}")
    if whereClause != "" {
        query += " WHERE " + whereClause
    }
    query += " GROUP BY " + groupClause
    rows, err := svc.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var results []map[string]interface{}
    columns, _ := rows.Columns()
    for rows.Next() {
        vals := make([]interface{}, len(columns))
        dest := make([]interface{}, len(columns))
        for i := range vals {
            dest[i] = &vals[i]
        }
        if err := rows.Scan(dest...); err != nil {
            return nil, err
        }
        rowMap := map[string]interface{}{}
        for i, col := range columns {
            rowMap[col] = vals[i]
        }
        results = append(results, rowMap)
    }
    return results, nil
}

{{- end }}

// Helper functions used by services

// buildWhere assembles SQL WHERE clause and args
func buildWhere(where map[string]interface{}) (string, []interface{}) {
    var clauses []string
    var args []interface{}
    i := 1
    for k, v := range where {
        clauses = append(clauses, fmt.Sprintf("%s = $%d", k, i))
        args = append(args, v)
        i++
    }
    return strings.Join(clauses, " AND "), args
}

// buildWhereOffset is like buildWhere but starts binding at offset
func buildWhereOffset(where map[string]interface{}, start int) (string, []interface{}) {
    var clauses []string
    var args []interface{}
    i := start
    for k, v := range where {
        clauses = append(clauses, fmt.Sprintf("%s = $%d", k, i))
        args = append(args, v)
        i++
    }
    return strings.Join(clauses, " AND "), args
}

// buildInsert assembles INSERT columns, placeholders, and args
func buildInsert(data map[string]interface{}) ([]string, []string, []interface{}) {
    var cols []string
    var placeholders []string
    var args []interface{}
    i := 1
    for k, v := range data {
        cols = append(cols, k)
        placeholders = append(placeholders, fmt.Sprintf("$%d", i))
        args = append(args, v)
        i++
    }
    return cols, placeholders, args
}

// buildSet assembles SET clause and args
func buildSet(data map[string]interface{}, start int) (string, []interface{}) {
    var clauses []string
    var args []interface{}
    i := start
    for k, v := range data {
        clauses = append(clauses, fmt.Sprintf("%s = $%d", k, i))
        args = append(args, v)
        i++
    }
    return strings.Join(clauses, ", "), args
}
`))

// Generate reads a Prisma schema and outputs Go client code.
func Generate(schemaPath, outDir string) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	ast, err := ParseSchema(data)
	if err != nil {
		return err
	}
	fmt.Printf("Parsed %d entities\n", len(ast.Entities))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Generate enums.go with Go enum types
	enumFile := filepath.Join(outDir, "enums.go")
	ef, err := os.Create(enumFile)
	if err != nil {
		return err
	}
	defer ef.Close()

	// Write package and import (none needed for simple string enums)
	fmt.Fprintln(ef, "package models")
	fmt.Fprintln(ef, "\n// Code generated by TORM; DO NOT EDIT.")

	for _, enum := range ast.Enums {
		// Define type as string
		fmt.Fprintf(ef, "type %s string\n\n", enum.Name)
		// Define constants
		fmt.Fprintf(ef, "const (\n")
		for _, val := range enum.Values {
			// Convert enum value to upper-case or CamelCase
			// Use strings.Title(strings.ToLower(val))
			constName := enum.Name + strings.Title(strings.ToLower(val))
			fmt.Fprintf(ef, "    %s %s = \"%s\"\n", constName, enum.Name, val)
		}
		fmt.Fprintf(ef, ")\n\n")
	}
	if err := formatFile(enumFile); err != nil {
		return err
	}
	fmt.Printf("Generated enums %s\n", enumFile)

	// generate one file per entity
	for _, ent := range ast.Entities {
		filePath := filepath.Join(outDir, strings.ToLower(ent.Name)+".go")
		f, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		if err := modelTemplate.Execute(f, ent); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}

		// format the file
		if err := formatFile(filePath); err != nil {
			return err
		}

		fmt.Printf("Generated model %s\n", filePath)
	}

	// Generate client.go with service methods for all models
	clientPath := filepath.Join(outDir, "client.go")
	cf, err := os.Create(clientPath)
	if err != nil {
		return err
	}
	defer cf.Close()

	// Build EntitiesMap: function for lookup of entity fields by name
	dataMap := map[string]interface{}{
		"Entities": ast.Entities,
		"EntitiesMap": func(name string) []Field {
			for _, e := range ast.Entities {
				if e.Name == name {
					return e.Fields
				}
			}
			return nil
		},
	}
	if err := clientTemplate.Execute(cf, dataMap); err != nil {
		return err
	}
	if err := formatFile(clientPath); err != nil {
		return err
	}
	fmt.Printf("Generated client %s\n", clientPath)

	// Run go mod tidy in the output directory
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = outDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w\nOutput:\n%s", err, string(out))
	}
	fmt.Printf("Ran go mod tidy in %s\n", outDir)

	return nil
}
