package schema

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/scanner"
	"time"
	"unicode/utf8"
)

type Schema struct {
	Source Source
	Models []*Model
	Enums  []*Enum
	Joins  []Join
}

type Source struct {
	URL         string
	Environment string
}

type Model struct {
	position
	Name      string
	Table     string
	Columns   []*Field
	Relations []*Relation
	Keys      []Key
	Indexes   []Key
}

type Field struct {
	position
	Name        string
	Column      string
	Kind        string
	GoType      string
	SQLType     string
	Enum        *Enum
	Optional    bool
	List        bool
	Identity    bool
	Default     string
	UUIDVersion int
	UpdatedAt   bool
}

type Enum struct {
	Name   string
	Type   string
	Values []EnumValue
}

type EnumValue struct {
	Name  string
	Label string
}

type Key struct {
	Name     string
	Selector string
	Fields   []*Field
	Primary  bool
}

type Relation struct {
	position
	Name          string
	Target        *Model
	Many          bool
	Optional      bool
	Local         []*Field
	Remote        []*Field
	Join          *Join
	LinkLocal     string
	LinkRemote    string
	Constraint    string
	OnDelete      string
	OnUpdate      string
	label         string
	fieldList     *expression
	referenceList *expression
}

type Join struct {
	Table string
	A     *Model
	B     *Model
	AKey  *Field
	BKey  *Field
}

func Export(name string) string {
	return strings.ToUpper(name[:1]) + name[1:]
}

func Quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func Literal(value string) string {
	return `E'` + strings.NewReplacer(`\`, `\\`, "'", "''").Replace(value) + "'"
}

func Name(parts ...string) string {
	full := strings.Join(parts, "_")
	if len(full) <= 63 {
		return full
	}
	suffix := "_" + parts[len(parts)-1]
	prefix := strings.TrimSuffix(full, suffix)
	for len(prefix)+len(suffix) > 63 {
		_, size := utf8.DecodeLastRuneInString(prefix)
		prefix = prefix[:len(prefix)-size]
	}
	return prefix + suffix
}

var prismaIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var decimalLiteral = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
var uuidLiteral = regexp.MustCompile(`^[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

func physicalName(value expression) (string, error) {
	if value.kind != scanner.String || value.value == "" || len(value.value) > 63 || strings.ContainsRune(value.value, 0) {
		return "", diagnostic(value.position, "expected a PostgreSQL identifier from 1 to 63 bytes")
	}
	return value.value, nil
}

func attributes(all []attribute, supported ...string) (map[string]attribute, error) {
	byName := make(map[string]attribute)
	for _, attr := range all {
		name := attr.name
		if prefix, _, ok := strings.Cut(name, "."); ok {
			name = prefix + ".*"
		}
		if !slices.Contains(supported, name) {
			return nil, diagnostic(attr.position, "unsupported attribute %s", attr.name)
		}
		if _, found := byName[name]; found {
			return nil, diagnostic(attr.position, "duplicate attribute %s", attr.name)
		}
		byName[name] = attr
	}
	return byName, nil
}

func splitArgs(attr attribute, allowed ...string) ([]expression, map[string]expression, error) {
	var positional []expression

	named := make(map[string]expression)
	for _, arg := range attr.args {
		if arg.name == "" {
			if len(named) != 0 {
				return nil, nil, diagnostic(arg.position, "positional argument follows named argument")
			}
			positional = append(positional, arg)
			continue
		}
		valid := false
		for _, name := range allowed {
			if arg.name == name {
				valid = true
			}
		}
		if _, exists := named[arg.name]; exists || !valid {
			return nil, nil, diagnostic(arg.position, "unexpected or duplicate argument %q to %s", arg.name, attr.name)
		}
		named[arg.name] = arg
	}
	return positional, named, nil
}

func mapped(attr attribute) (string, error) {
	if len(attr.args) != 1 || attr.args[0].name != "" {
		return "", diagnostic(attr.position, "%s requires one string", attr.name)
	}
	return physicalName(attr.args[0])
}

func (m *Model) fields(expr expression) ([]*Field, error) {
	if !expr.list || len(expr.args) == 0 {
		return nil, diagnostic(expr.position, "expected a nonempty field list")
	}
	fields := make([]*Field, 0, len(expr.args))
	seen := make(map[string]bool)
	for _, item := range expr.args {
		var field *Field

		if item.kind == scanner.Ident && !item.call && item.name == "" {
			for _, column := range m.Columns {
				if item.value == column.Name {
					field = column
					break
				}
			}
		}
		if field == nil || seen[item.value] {
			return nil, diagnostic(item.position, "unknown, duplicate, or unsupported field %q in %s", item.value, m.Name)
		}
		seen[item.value] = true
		fields = append(fields, field)
	}
	return fields, nil
}

func columns(fields []*Field) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Column)
	}
	return names
}

func resolveSource(b block, s *Schema) error {
	provider := b.settings["provider"]
	if provider.kind != scanner.String || (provider.value != "postgresql" && provider.value != "postgres") {
		return diagnostic(b.position, "TORM requires datasource provider postgresql")
	}
	for key, value := range b.settings {
		switch key {
		case "provider":
		case "relationMode":
			if value.kind != scanner.String || value.value != "foreignKeys" {
				return diagnostic(value.position, "only relationMode = \"foreignKeys\" is supported")
			}
		case "url":
			if value.kind == scanner.String && value.value != "" {
				s.Source.URL = value.value
			} else if value.call && value.value == "env" && len(value.args) == 1 && value.args[0].kind == scanner.String && value.args[0].name == "" && value.args[0].value != "" {
				s.Source.Environment = value.args[0].value
			} else {
				return diagnostic(value.position, "url must be a string or env(\"VARIABLE\")")
			}
		default:
			return diagnostic(value.position, "unsupported datasource setting %q", key)
		}
	}
	return nil
}

func resolveEnum(b block) (*Enum, error) {
	e := &Enum{Name: b.name, Type: b.name}
	attrs, err := attributes(b.attributes, "map")
	if err != nil {
		return nil, err
	}
	if attr, ok := attrs["map"]; ok {
		e.Type, err = mapped(attr)
		if err != nil {
			return nil, err
		}
	}
	labels := make(map[string]bool)
	names := make(map[string]bool)
	for _, entry := range b.fields {
		value := EnumValue{Name: entry.name, Label: entry.name}
		attrs, err := attributes(entry.attributes, "map")
		if err != nil {
			return nil, err
		}
		if attr, ok := attrs["map"]; ok {
			value.Label, err = mapped(attr)
			if err != nil {
				return nil, err
			}
		}
		if !prismaIdentifier.MatchString(value.Name) || names[Export(value.Name)] || labels[value.Label] || len(value.Label) > 63 {
			return nil, diagnostic(entry.position, "invalid or duplicate enum value %q", value.Name)
		}
		names[Export(value.Name)] = true
		labels[value.Label] = true
		e.Values = append(e.Values, value)
	}
	if len(e.Values) == 0 {
		return nil, diagnostic(b.position, "enum %s has no values", b.name)
	}
	return e, nil
}

func resolve(blocks []block, legacyNames bool) (*Schema, error) {
	var s Schema
	var hasSource bool
	var modelBlocks []block

	models := make(map[string]*Model)
	enums := make(map[string]*Enum)
	tables := make(map[string]bool)
	goNames := make(map[string]bool)
	nativeTypes := "db.*"
	for _, b := range blocks {
		if b.kind == "datasource" {
			if hasSource {
				return nil, diagnostic(b.position, "only one datasource is supported")
			}
			if err := resolveSource(b, &s); err != nil {
				return nil, err
			}
			hasSource = true
			nativeTypes = b.name + ".*"
		}
	}
	legacyNames = legacyNames || s.Source.URL != "" || s.Source.Environment != ""
	for _, b := range blocks {
		if b.kind != "model" && b.kind != "enum" {
			continue
		}
		if !prismaIdentifier.MatchString(b.name) || token.Lookup(b.name).IsKeyword() || goNames[Export(b.name)] {
			return nil, diagnostic(b.position, "invalid or duplicate Go type name %q", b.name)
		}
		goNames[Export(b.name)] = true
		if b.kind == "enum" {
			e, err := resolveEnum(b)
			if err != nil {
				return nil, err
			}
			if tables[e.Type] || len(e.Type) > 63 {
				return nil, diagnostic(b.position, "invalid or duplicate PostgreSQL name %q", e.Type)
			}
			tables[e.Type] = true
			enums[e.Name] = e
			s.Enums = append(s.Enums, e)
			continue
		}
		table := b.name
		if legacyNames {
			table = strings.ToLower(table)
		}
		for _, attr := range b.attributes {
			if attr.name == "map" {
				var err error

				table, err = mapped(attr)
				if err != nil {
					return nil, err
				}
			}
		}
		if tables[table] || len(table) > 63 || table == "schema_migrations" {
			return nil, diagnostic(b.position, "invalid, reserved, or duplicate PostgreSQL name %q", table)
		}
		tables[table] = true
		model := &Model{position: b.position, Name: b.name, Table: table}
		models[b.name] = model
		s.Models = append(s.Models, model)
		modelBlocks = append(modelBlocks, b)
	}
	if len(modelBlocks) == 0 {
		return nil, diagnostic(blocks[0].position, "no model definitions found")
	}
	for i, model := range s.Models {
		if err := populate(model, modelBlocks[i], models, enums, legacyNames, nativeTypes); err != nil {
			return nil, err
		}
	}
	if err := resolveRelations(&s, tables); err != nil {
		return nil, err
	}
	for _, model := range s.Models {
		for _, key := range append(append([]Key{}, model.Keys...), model.Indexes...) {
			if tables[key.Name] {
				return nil, diagnostic(model.position, "duplicate PostgreSQL object name %q", key.Name)
			}
			tables[key.Name] = true
		}
	}
	return &s, nil
}

func populate(model *Model, b block, models map[string]*Model, enums map[string]*Enum, legacy bool, nativeTypes string) error {
	fieldNames := make(map[string]bool)
	columnNames := make(map[string]bool)
	var fieldAttrs []map[string]attribute

	for _, source := range b.fields {
		if !prismaIdentifier.MatchString(source.name) || fieldNames[Export(source.name)] {
			return diagnostic(source.position, "invalid or duplicate field %q in %s", source.name, model.Name)
		}
		fieldNames[Export(source.name)] = true
		if target := models[source.kind]; target != nil {
			relation, err := parseRelation(target, source)
			if err != nil {
				return err
			}
			model.Relations = append(model.Relations, relation)
			continue
		}
		field, attrs, err := scalar(source, enums[source.kind], legacy, nativeTypes)
		if err != nil {
			return err
		}
		if columnNames[field.Column] || len(field.Column) > 63 {
			return diagnostic(source.position, "invalid or duplicate column %q in %s", field.Column, model.Name)
		}
		columnNames[field.Column] = true
		model.Columns = append(model.Columns, field)
		fieldAttrs = append(fieldAttrs, attrs)
	}
	for i, field := range model.Columns {
		for _, name := range []string{"id", "unique"} {
			if attr, ok := fieldAttrs[i][name]; ok {
				if err := model.addKey(attr, []*Field{field}); err != nil {
					return err
				}
			}
		}
	}
	var seenMap bool
	for _, attr := range b.attributes {
		switch attr.name {
		case "map":
			if seenMap {
				return diagnostic(attr.position, "duplicate @@map")
			}
			seenMap = true
		case "id", "unique", "index":
			if err := model.addKey(attr, nil); err != nil {
				return err
			}
		default:
			return diagnostic(attr.position, "unsupported model attribute @@%s", attr.name)
		}
	}
	for _, key := range model.Keys {
		allRequired := true
		for _, field := range key.Fields {
			allRequired = allRequired && !field.Optional
		}
		if allRequired {
			return nil
		}
	}
	return diagnostic(model.position, "%s requires a primary key or a required unique key", model.Name)
}

func (m *Model) addKey(attr attribute, fields []*Field) error {
	args, named, err := splitArgs(attr, "map", "name")
	if err != nil {
		return err
	}
	if fields == nil {
		if len(args) != 1 {
			return diagnostic(attr.position, "@@%s requires one field list", attr.name)
		}
		fields, err = m.fields(args[0])
		if err != nil {
			return err
		}
	} else if len(args) != 0 {
		return diagnostic(attr.position, "@%s accepts named arguments only", attr.name)
	}
	if name, ok := named["name"]; ok && (len(args) == 0 || attr.name == "index" || name.kind != scanner.String || !prismaIdentifier.MatchString(name.value)) {
		return diagnostic(name.position, "invalid selector name")
	}
	key := Key{Fields: fields, Primary: attr.name == "id"}
	if name, ok := named["name"]; ok {
		key.Selector = name.value
	}
	for _, field := range fields {
		if (key.Primary && field.Optional) || field.List || field.Kind == "Json" {
			return diagnostic(attr.position, "field %s cannot be used in @@%s", field.Name, attr.name)
		}
	}
	if key.Primary {
		for _, other := range m.Keys {
			if other.Primary {
				return diagnostic(attr.position, "multiple primary keys on %s", m.Name)
			}
		}
		key.Name = Name(m.Table, "pkey")
	} else {
		suffix := "key"
		if attr.name == "index" {
			suffix = "idx"
		}
		key.Name = Name(m.Table, strings.Join(columns(fields), "_"), suffix)
	}
	if value, ok := named["map"]; ok {
		key.Name, err = physicalName(value)
		if err != nil {
			return err
		}
	}
	if attr.name == "index" {
		m.Indexes = append(m.Indexes, key)
	} else {
		m.Keys = append(m.Keys, key)
	}
	return nil
}

func scalar(source fieldSyntax, enum *Enum, legacy bool, nativeTypes string) (*Field, map[string]attribute, error) {
	attrs, err := attributes(source.attributes, "id", "unique", "map", "default", "updatedAt", nativeTypes)
	if err != nil {
		return nil, nil, err
	}
	f := &Field{position: source.position, Name: source.name, Column: source.name, Kind: source.kind, Optional: source.optional, List: source.list, Enum: enum}
	if legacy {
		f.Column = strings.ToLower(f.Column)
	}
	if attr, ok := attrs["map"]; ok {
		f.Column, err = mapped(attr)
		if err != nil {
			return nil, nil, err
		}
	}
	switch f.Kind {
	case "String":
		f.GoType, f.SQLType = "string", "text"
	case "Int":
		f.GoType, f.SQLType = "int", "integer"
	case "BigInt":
		f.GoType, f.SQLType = "int64", "bigint"
	case "Boolean":
		f.GoType, f.SQLType = "bool", "boolean"
	case "Float":
		f.GoType, f.SQLType = "float64", "double precision"
	case "Decimal":
		f.GoType, f.SQLType = "runtime.Decimal", "numeric(65,30)"
	case "DateTime":
		f.GoType, f.SQLType = "time.Time", "timestamp(3) without time zone"
	case "Bytes":
		f.GoType, f.SQLType = "[]byte", "bytea"
	case "Json":
		f.GoType, f.SQLType = "json.RawMessage", "jsonb"
	default:
		if enum == nil {
			return nil, nil, diagnostic(source.position, "unknown or unsupported type %q", f.Kind)
		}
		f.GoType, f.SQLType = Export(enum.Name), Quote(enum.Type)
	}
	if attr, ok := attrs[nativeTypes]; ok {
		if err := nativeType(f, attr); err != nil {
			return nil, nil, err
		}
	}
	if attr, ok := attrs["default"]; ok {
		args, _, err := splitArgs(attr)
		if err != nil {
			return nil, nil, err
		}
		if len(args) != 1 {
			return nil, nil, diagnostic(attr.position, "@default requires one value")
		}
		if err := resolveDefault(f, args[0]); err != nil {
			return nil, nil, err
		}
	}
	if attr, ok := attrs["updatedAt"]; ok {
		if f.Kind != "DateTime" || f.List || len(attr.args) != 0 {
			return nil, nil, diagnostic(attr.position, "@updatedAt requires a DateTime field")
		}
		f.UpdatedAt = true
	}
	return f, attrs, nil
}

func nativeType(f *Field, attr attribute) error {
	_, name, _ := strings.Cut(attr.name, ".")
	var kind string
	var sqlType string
	var count int

	switch name {
	case "Uuid":
		kind, sqlType = "String", "uuid"
		f.GoType = "uuid.UUID"
	case "Text":
		kind, sqlType = "String", "text"
	case "VarChar", "Char":
		kind, sqlType = "String", "character"
		if name == "VarChar" {
			sqlType = "character varying"
		}
		if len(attr.args) > 0 {
			sqlType += "(%d)"
			count = 1
		}
	case "SmallInt":
		kind, sqlType = "Int", "smallint"
	case "Integer":
		kind, sqlType = "Int", "integer"
	case "BigInt":
		kind, sqlType = "BigInt", "bigint"
	case "Real":
		kind, sqlType = "Float", "real"
	case "DoublePrecision":
		kind, sqlType = "Float", "double precision"
	case "Boolean":
		kind, sqlType = "Boolean", "boolean"
	case "Decimal":
		kind, sqlType, count = "Decimal", "numeric(%d,%d)", 2
		if len(attr.args) == 0 {
			sqlType, count = "numeric", 0
		}
	case "Date":
		kind, sqlType = "DateTime", "date"
	case "Timestamp", "Timestamptz", "Time", "Timetz":
		kind, count = "DateTime", 1
		base := "timestamp"
		if name == "Time" || name == "Timetz" {
			base = "time"
		}
		zone := "without"
		if name == "Timestamptz" || name == "Timetz" {
			zone = "with"
		}
		sqlType = base + "(%d) " + zone + " time zone"
		if len(attr.args) == 0 {
			sqlType, count = base+" "+zone+" time zone", 0
		}
	case "Json":
		kind, sqlType = "Json", "json"
	case "JsonB":
		kind, sqlType = "Json", "jsonb"
	case "ByteA":
		kind, sqlType = "Bytes", "bytea"
	default:
		return diagnostic(attr.position, "unsupported native type @%s", attr.name)
	}
	if f.Kind != kind || len(attr.args) != count {
		return diagnostic(attr.position, "@%s requires %s and %d arguments in this form", attr.name, kind, count)
	}
	args := make([]interface{}, 0, count)
	for _, arg := range attr.args {
		number, err := strconv.Atoi(arg.value)
		if err != nil || arg.kind != scanner.Int || arg.name != "" || number < 0 {
			return diagnostic(arg.position, "expected a nonnegative integer")
		}
		args = append(args, number)
	}
	switch name {
	case "Decimal":
		if count == 2 && (args[0].(int) < 1 || args[0].(int) > 1000 || args[1].(int) > args[0].(int)) {
			return diagnostic(attr.position, "invalid decimal precision or scale")
		}
	case "VarChar", "Char":
		if count == 1 && (args[0].(int) < 1 || args[0].(int) > 10485760) {
			return diagnostic(attr.position, "invalid character length")
		}
	case "Timestamp", "Timestamptz", "Time", "Timetz":
		if count == 1 && args[0].(int) > 6 {
			return diagnostic(attr.position, "time precision must be from 0 to 6")
		}
	}
	f.SQLType = fmt.Sprintf(sqlType, args...)
	return nil
}

func resolveDefault(f *Field, value expression) error {
	if f.List {
		if !value.list {
			return diagnostic(value.position, "list default must be an array")
		}
		values := make([]string, 0, len(value.args))
		for _, element := range value.args {
			scalar := *f
			scalar.List = false
			if err := resolveDefault(&scalar, element); err != nil {
				return err
			}
			if element.call {
				return diagnostic(element.position, "default list elements must be literals")
			}
			values = append(values, scalar.Default)
		}
		f.Default = "ARRAY[" + strings.Join(values, ", ") + "]::" + f.SQLType + "[]"
		return nil
	}
	if value.call {
		switch value.value {
		case "autoincrement":
			if len(value.args) == 0 && !f.Optional && (f.Kind == "Int" || f.Kind == "BigInt") {
				f.Identity = true
				return nil
			}
		case "uuid":
			version := 4
			if len(value.args) == 1 && value.args[0].kind == scanner.Int && value.args[0].name == "" {
				version, _ = strconv.Atoi(value.args[0].value)
			} else if len(value.args) != 0 {
				version = 0
			}
			if f.Kind == "String" && (version == 4 || version == 7) {
				f.UUIDVersion = version
				return nil
			}
		case "now":
			if len(value.args) == 0 && f.Kind == "DateTime" {
				f.Default = "CURRENT_TIMESTAMP"
				return nil
			}
		case "dbgenerated":
			if len(value.args) == 1 && value.args[0].kind == scanner.String && value.args[0].name == "" && f.SQLType == "uuid" {
				switch value.args[0].value {
				case "gen_random_uuid()", "uuid_generate_v4()":
					f.Default = value.args[0].value
					return nil
				}
			}
		}
		return diagnostic(value.position, "unsupported or incompatible default %s for %s", value.value, f.Name)
	}
	if strings.ContainsRune(value.value, 0) || value.name != "" {
		return diagnostic(value.position, "invalid default")
	}
	switch f.Kind {
	case "String", "Json", "DateTime":
		if value.kind == scanner.String {
			if (f.SQLType == "uuid" && !uuidLiteral.MatchString(value.value)) || (f.Kind == "Json" && !json.Valid([]byte(value.value))) {
				break
			}
			if f.Kind == "DateTime" {
				if _, err := time.Parse(time.RFC3339Nano, value.value); err != nil {
					break
				}
			}
			f.Default = Literal(value.value)
		}
	case "Boolean":
		if value.kind == scanner.Ident && (value.value == "true" || value.value == "false") {
			f.Default = value.value
		}
	case "Int", "BigInt":
		bits := 32
		if f.SQLType == "smallint" {
			bits = 16
		} else if f.Kind == "BigInt" {
			bits = 64
		}
		if _, err := strconv.ParseInt(value.value, 10, bits); err == nil && value.kind == scanner.Int {
			f.Default = value.value
		}
	case "Float", "Decimal":
		if decimalLiteral.MatchString(value.value) && (value.kind == scanner.Int || value.kind == scanner.Float || (f.Kind == "Decimal" && value.kind == scanner.String)) {
			f.Default = value.value
		}
	case "Bytes":
		if value.kind == scanner.String {
			if _, err := base64.StdEncoding.DecodeString(value.value); err == nil {
				f.Default = "decode(" + Literal(value.value) + ", 'base64')"
			}
		}
	default:
		for _, enum := range f.Enum.Values {
			if value.kind == scanner.Ident && enum.Name == value.value {
				f.Default = Literal(enum.Label)
				break
			}
		}
	}
	if f.Default == "" {
		return diagnostic(value.position, "incompatible default for %s", f.Name)
	}
	return nil
}
