package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/TechXTT/TORM/v2/pkg/internal/schema"
	"github.com/lib/pq"
)

type Decimal string

type Value[T any] struct {
	set   bool
	null  bool
	value T
}

func Set[T any](value T) Value[T] {
	return Value[T]{set: true, value: value}
}

func IsZero[T any](value T) bool {
	return reflect.ValueOf(&value).Elem().IsZero()
}

func Null[T any]() Value[T] {
	return Value[T]{set: true, null: true}
}

type Values map[string]any

func Put[T any](values Values, column string, input Value[T]) {
	if !input.set {
		return
	}
	if input.null {
		values[column] = nil
	} else {
		values[column] = input.value
	}
}

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type Scanner interface {
	Scan(...any) error
}

type Column struct {
	Name            string
	SQL             string
	List            bool
	Optional        bool
	DatabaseDefault bool
	CreateDefault   func() (any, error)
	UpdatedAt       bool
}

type Model[M any] struct {
	Table   string
	Columns []Column
	Unique  [][]string
	Scan    func(Scanner, ...any) (*M, error)
}

type Store[M any] struct {
	db    Queryer
	model *Model[M]
}

func NewStore[M any](db Queryer, model *Model[M]) Store[M] {
	return Store[M]{db: db, model: model}
}

func (s Store[M]) table() string {
	return `"public".` + schema.Quote(s.model.Table)
}

func (s Store[M]) columns(prefix string) string {
	names := make([]string, 0, len(s.model.Columns))
	for _, column := range s.model.Columns {
		names = append(names, prefix+schema.Quote(column.SQL))
	}
	return strings.Join(names, ", ")
}

func (s Store[M]) column(name string) (Column, error) {
	for _, column := range s.model.Columns {
		if column.Name == name {
			return column, nil
		}
	}
	for _, column := range s.model.Columns {
		if column.SQL == name {
			return column, nil
		}
	}
	return Column{}, fmt.Errorf("unknown field %q on %s", name, s.model.Table)
}

func bind(value any, list bool) any {
	if value == nil {
		return nil
	}
	if list {
		switch value := value.(type) {
		case []json.RawMessage:
			elements := make([]string, len(value))
			for i, element := range value {
				elements[i] = string(element)
			}
			return pq.Array(elements)
		}
		return pq.Array(value)
	}
	if raw, ok := value.(json.RawMessage); ok {
		return string(raw)
	}
	return value
}

type Filter[M any] struct {
	node *predicate
}

type predicate struct {
	column string
	op     string
	value  any
	terms  []*predicate
}

type Field[M, T any] struct {
	name string
}

func NewField[M, T any](name string) Field[M, T] {
	return Field[M, T]{name: name}
}

func (f Field[M, T]) Eq(value T) Filter[M] {
	return Filter[M]{&predicate{column: f.name, op: "=", value: value}}
}

func (f Field[M, T]) Ne(value T) Filter[M] {
	return Filter[M]{&predicate{column: f.name, op: "<>", value: value}}
}

func (f Field[M, T]) IsNull() Filter[M] {
	return Filter[M]{&predicate{column: f.name, op: "IS NULL"}}
}

func (f Field[M, T]) IsNotNull() Filter[M] {
	return Filter[M]{&predicate{column: f.name, op: "IS NOT NULL"}}
}

func And[M any](filters ...Filter[M]) Filter[M] {
	return combine("AND", filters)
}

func Or[M any](filters ...Filter[M]) Filter[M] {
	return combine("OR", filters)
}

func combine[M any](op string, filters []Filter[M]) Filter[M] {
	terms := make([]*predicate, 0, len(filters))
	for _, filter := range filters {
		if filter.node != nil {
			terms = append(terms, filter.node)
		}
	}
	if len(terms) == 0 {
		return Filter[M]{}
	}
	return Filter[M]{&predicate{op: op, terms: terms}}
}

func All[M any]() Filter[M] {
	return Filter[M]{&predicate{op: "TRUE"}}
}

type Order[M any] struct {
	field string
	desc  bool
}

func (f Field[M, T]) Asc() Order[M] {
	return Order[M]{field: f.name}
}

func (f Field[M, T]) Desc() Order[M] {
	return Order[M]{field: f.name, desc: true}
}

type FindOptions[M any] struct {
	Filter Filter[M]
	Order  []Order[M]
	Offset int
	Limit  int
}

func (s Store[M]) filter(node *predicate, args *[]any) (string, error) {
	if node == nil {
		return "", nil
	}
	switch node.op {
	case "TRUE":
		return "TRUE", nil
	case "AND", "OR":
		terms := make([]string, 0, len(node.terms))
		for _, child := range node.terms {
			part, err := s.filter(child, args)
			if err != nil {
				return "", err
			}
			terms = append(terms, part)
		}
		return "(" + strings.Join(terms, " "+node.op+" ") + ")", nil
	}
	column, err := s.column(node.column)
	if err != nil {
		return "", err
	}
	name := schema.Quote(column.SQL)
	switch node.op {
	case "IS NULL", "IS NOT NULL":
		return name + " " + node.op, nil
	default:
		*args = append(*args, bind(node.value, column.List))
		return fmt.Sprintf("%s %s $%d", name, node.op, len(*args)), nil
	}
}

func (s Store[M]) LegacyFilter(where map[string]any) (Filter[M], error) {
	filters := make([]Filter[M], 0, len(where))
	for _, name := range sortedKeys(where) {
		column, err := s.column(name)
		if err != nil {
			return Filter[M]{}, err
		}
		term := &predicate{column: column.Name, op: "=", value: where[name]}
		if where[name] == nil {
			term.op = "IS NULL"
		}
		filters = append(filters, Filter[M]{term})
	}
	return And(filters...), nil
}

func (s Store[M]) LegacyOptions(where map[string]any, order []string, offset, limit int) (FindOptions[M], error) {
	filter, err := s.LegacyFilter(where)
	options := FindOptions[M]{Filter: filter, Offset: offset, Limit: limit}
	if err != nil {
		return options, err
	}
	for _, value := range order {
		parts := strings.Fields(value)
		if len(parts) == 0 || len(parts) > 2 || (len(parts) == 2 && !strings.EqualFold(parts[1], "asc") && !strings.EqualFold(parts[1], "desc")) {
			return options, fmt.Errorf("invalid sort expression %q", value)
		}
		column, err := s.column(parts[0])
		if err != nil {
			return options, err
		}
		options.Order = append(options.Order, Order[M]{field: column.Name, desc: len(parts) == 2 && strings.EqualFold(parts[1], "desc")})
	}
	return options, nil
}

func (s Store[M]) Find(ctx context.Context, options FindOptions[M]) ([]*M, error) {
	if options.Offset < 0 || options.Limit < 0 {
		return nil, fmt.Errorf("offset and limit must be nonnegative")
	}
	var args []any

	where, err := s.filter(options.Filter.node, &args)
	if err != nil {
		return nil, err
	}
	query := "SELECT " + s.columns("") + " FROM " + s.table()
	if where != "" {
		query += " WHERE " + where
	}
	if len(options.Order) > 0 {
		order := make([]string, 0, len(options.Order))
		for _, item := range options.Order {
			column, err := s.column(item.field)
			if err != nil {
				return nil, err
			}
			direction := " ASC"
			if item.desc {
				direction = " DESC"
			}
			order = append(order, schema.Quote(column.SQL)+direction)
		}
		query += " ORDER BY " + strings.Join(order, ", ")
	}
	for _, clause := range []struct {
		keyword string
		value   int
	}{{"LIMIT", options.Limit}, {"OFFSET", options.Offset}} {
		if clause.value > 0 {
			args = append(args, clause.value)
			query += fmt.Sprintf(" %s $%d", clause.keyword, len(args))
		}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*M, 0)
	for rows.Next() {
		entry, err := s.model.Scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s Store[M]) unique(values Values, args *[]any) (string, error) {
	resolved, err := s.resolveValues(values)
	if err != nil {
		return "", err
	}
	key, err := s.uniqueColumns(resolved)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(key))
	for _, name := range key {
		*args = append(*args, bind(resolved[name], false))
		parts = append(parts, fmt.Sprintf("%s = $%d", schema.Quote(name), len(*args)))
	}
	return strings.Join(parts, " AND "), nil
}

func (s Store[M]) uniqueColumns(resolved Values) ([]string, error) {
	for _, key := range s.model.Unique {
		if len(key) != len(resolved) {
			continue
		}
		match := true
		for _, name := range key {
			if value, exists := resolved[name]; !exists || value == nil {
				match = false
				break
			}
		}
		if match {
			return key, nil
		}
	}
	return nil, fmt.Errorf("select one declared unique or primary key on %s", s.model.Table)
}

func (s Store[M]) Get(ctx context.Context, key Values) (*M, error) {
	var args []any

	where, err := s.unique(key, &args)
	if err != nil {
		return nil, err
	}
	record, err := s.model.Scan(s.db.QueryRowContext(ctx, "SELECT "+s.columns("")+" FROM "+s.table()+" WHERE "+where, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return record, err
}

func (s Store[M]) resolveValues(values Values) (Values, error) {
	resolved := make(Values, len(values))
	for _, name := range sortedKeys(values) {
		column, err := s.column(name)
		if err != nil {
			return nil, err
		}
		if _, exists := resolved[column.SQL]; exists {
			return nil, fmt.Errorf("duplicate value for %s", column.Name)
		}
		resolved[column.SQL] = values[name]
	}
	return resolved, nil
}

type writeParts struct {
	columns      []string
	placeholders []string
	assignments  []string
	args         []any
}

func (s Store[M]) writeValues(values Values, insert bool, offset int) (writeParts, error) {
	resolved, err := s.resolveValues(values)
	if err != nil {
		return writeParts{}, err
	}
	return s.writeColumns(resolved, insert, offset)
}

func (s Store[M]) writeColumns(resolved Values, insert bool, offset int) (writeParts, error) {
	var parts writeParts
	var now time.Time

	for _, column := range s.model.Columns {
		value, set := resolved[column.SQL]
		if !set && insert && column.CreateDefault != nil {
			generated, err := column.CreateDefault()
			if err != nil {
				return parts, err
			}
			value, set = generated, true
		}
		if !set && column.UpdatedAt && (insert || len(resolved) != 0) {
			if now.IsZero() {
				now = time.Now().UTC()
			}
			value, set = now, true
		}
		if !set {
			if insert && !column.Optional && !column.DatabaseDefault {
				return parts, fmt.Errorf("required field %s.%s is missing", s.model.Table, column.Name)
			}
			continue
		}
		if value == nil && !column.Optional && !column.List {
			return parts, fmt.Errorf("required field %s.%s cannot be NULL", s.model.Table, column.Name)
		}
		if column.List && value != nil {
			valueOf := reflect.ValueOf(value)
			if valueOf.Kind() == reflect.Slice && valueOf.IsNil() {
				value = reflect.MakeSlice(valueOf.Type(), 0, 0).Interface()
			}
		}
		if value == nil && column.List {
			return parts, fmt.Errorf("list field %s.%s cannot be NULL", s.model.Table, column.Name)
		}
		parts.args = append(parts.args, bind(value, column.List))
		name := schema.Quote(column.SQL)
		placeholder := fmt.Sprintf("$%d", offset+len(parts.args))
		parts.columns = append(parts.columns, name)
		parts.placeholders = append(parts.placeholders, placeholder)
		parts.assignments = append(parts.assignments, name+" = "+placeholder)
	}
	return parts, nil
}

func (s Store[M]) insertSQL(parts writeParts) string {
	query := "INSERT INTO " + s.table()
	if len(parts.columns) == 0 {
		return query + " DEFAULT VALUES"
	}
	return query + " (" + strings.Join(parts.columns, ", ") + ") VALUES (" + strings.Join(parts.placeholders, ", ") + ")"
}

func (s Store[M]) Insert(ctx context.Context, values Values) (*M, error) {
	parts, err := s.writeValues(values, true, 0)
	if err != nil {
		return nil, err
	}
	return s.model.Scan(s.db.QueryRowContext(ctx, s.insertSQL(parts)+" RETURNING "+s.columns(""), parts.args...))
}

func (s Store[M]) Patch(ctx context.Context, key, values Values) (*M, error) {
	if len(values) == 0 {
		return s.Get(ctx, key)
	}
	parts, err := s.writeValues(values, false, 0)
	if err != nil {
		return nil, err
	}
	where, err := s.unique(key, &parts.args)
	if err != nil {
		return nil, err
	}
	query := "UPDATE " + s.table() + " SET " + strings.Join(parts.assignments, ", ") + " WHERE " + where + " RETURNING " + s.columns("")
	return s.model.Scan(s.db.QueryRowContext(ctx, query, parts.args...))
}

func (s Store[M]) Upsert(ctx context.Context, key, create, update Values) (*M, error) {
	resolvedKey, err := s.resolveValues(key)
	if err != nil {
		return nil, err
	}
	conflict, err := s.uniqueColumns(resolvedKey)
	if err != nil {
		return nil, err
	}
	resolvedCreate, err := s.resolveValues(create)
	if err != nil {
		return nil, err
	}
	for _, column := range conflict {
		if value, exists := resolvedCreate[column]; exists && !reflect.DeepEqual(value, resolvedKey[column]) {
			return nil, fmt.Errorf("create value for %s differs from the upsert key", column)
		}
		resolvedCreate[column] = resolvedKey[column]
	}
	ins, err := s.writeColumns(resolvedCreate, true, 0)
	if err != nil {
		return nil, err
	}
	upd, err := s.writeValues(update, false, len(ins.args))
	if err != nil {
		return nil, err
	}
	if len(upd.assignments) == 0 {
		name := schema.Quote(conflict[0])
		upd.assignments = append(upd.assignments, name+" = "+schema.Quote(s.model.Table)+"."+name)
	}
	query := s.insertSQL(ins) + " ON CONFLICT (" + schema.Quoted(conflict) + ") DO UPDATE SET " + strings.Join(upd.assignments, ", ") + " RETURNING " + s.columns("")
	return s.model.Scan(s.db.QueryRowContext(ctx, query, append(ins.args, upd.args...)...))
}

func (s Store[M]) Delete(ctx context.Context, key Values) error {
	var args []any

	where, err := s.unique(key, &args)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM "+s.table()+" WHERE "+where, args...)
	return err
}

func (s Store[M]) Count(ctx context.Context, filter Filter[M]) (int64, error) {
	var args []any
	var count int64

	where, err := s.filter(filter.node, &args)
	if err != nil {
		return 0, err
	}
	query := "SELECT COUNT(*) FROM " + s.table()
	if where != "" {
		query += " WHERE " + where
	}
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (s Store[M]) UpdateMatching(ctx context.Context, filter Filter[M], values Values) (int64, error) {
	if filter.node == nil {
		return 0, fmt.Errorf("update requires a filter; use runtime.All to target every row")
	}
	if len(values) == 0 {
		return 0, nil
	}
	parts, err := s.writeValues(values, false, 0)
	if err != nil {
		return 0, err
	}
	where, err := s.filter(filter.node, &parts.args)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, "UPDATE "+s.table()+" SET "+strings.Join(parts.assignments, ", ")+" WHERE "+where, parts.args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s Store[M]) DeleteMatching(ctx context.Context, filter Filter[M]) (int64, error) {
	if filter.node == nil {
		return 0, fmt.Errorf("delete requires a filter; use runtime.All to target every row")
	}
	var args []any

	where, err := s.filter(filter.node, &args)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM "+s.table()+" WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func WithTx(ctx context.Context, db Queryer, run func(Queryer) error) (err error) {
	switch db := db.(type) {
	case interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	}:
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := run(tx); err != nil {
			return err
		}
		return tx.Commit()
	case *sql.Tx:
		if _, err := db.ExecContext(ctx, "SAVEPOINT torm_operation"); err != nil {
			return err
		}
		released := false
		defer func() {
			if released {
				return
			}
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, rollbackErr := db.ExecContext(cleanup, "ROLLBACK TO SAVEPOINT torm_operation; RELEASE SAVEPOINT torm_operation")
			if rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
		}()
		if err := run(db); err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, "RELEASE SAVEPOINT torm_operation")
		released = err == nil
		return err
	default:
		return run(db)
	}
}

func (s Store[M]) InsertMany(ctx context.Context, values []Values) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	err := WithTx(ctx, s.db, func(executor Queryer) error {
		for _, entry := range values {
			parts, err := s.writeValues(entry, true, 0)
			if err != nil {
				return err
			}
			result, err := executor.ExecContext(ctx, s.insertSQL(parts), parts.args...)
			if err != nil {
				return err
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if inserted == 0 {
				return sql.ErrNoRows
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int64(len(values)), nil
}

func (s Store[M]) PatchMany(ctx context.Context, keys, values []Values) ([]*M, error) {
	if len(keys) != len(values) {
		return nil, fmt.Errorf("mismatch between filter and model counts")
	}
	result := make([]*M, 0, len(values))
	if len(values) == 0 {
		return result, nil
	}
	err := WithTx(ctx, s.db, func(executor Queryer) error {
		store := NewStore(executor, s.model)
		for i, entry := range values {
			updated, err := store.Patch(ctx, keys[i], entry)
			if err != nil {
				return err
			}
			if updated == nil {
				return sql.ErrNoRows
			}
			result = append(result, updated)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s Store[M]) LegacyAggregate(ctx context.Context, group []string, where Values, aggregations map[string][]string) ([]map[string]any, error) {
	filter, err := s.LegacyFilter(where)
	if err != nil {
		return nil, err
	}
	var args []any
	var groups []string
	var expressions []string
	var labels []string

	clause, err := s.filter(filter.node, &args)
	if err != nil {
		return nil, err
	}
	for _, name := range group {
		column, err := s.column(name)
		if err != nil {
			return nil, err
		}
		quoted := schema.Quote(column.SQL)
		groups = append(groups, quoted)
		labels = append(labels, name)
	}
	expressions = append(expressions, groups...)
	for _, name := range sortedKeys(aggregations) {
		function := strings.ToLower(strings.TrimPrefix(name, "_"))
		if !slices.Contains([]string{"avg", "count", "max", "min", "sum"}, function) {
			return nil, fmt.Errorf("unsupported aggregate %q", name)
		}
		for _, field := range aggregations[name] {
			expression := "*"
			if function != "count" || field != "*" {
				column, err := s.column(field)
				if err != nil {
					return nil, err
				}
				expression = schema.Quote(column.SQL)
			}
			expressions = append(expressions, function+"("+expression+")")
			labels = append(labels, name+"_"+field)
		}
	}
	if len(expressions) == 0 {
		return nil, fmt.Errorf("aggregate requires at least one field")
	}
	query := "SELECT " + strings.Join(expressions, ", ") + " FROM " + s.table()
	if clause != "" {
		query += " WHERE " + clause
	}
	if len(groups) > 0 {
		query += " GROUP BY " + strings.Join(groups, ", ")
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []map[string]any
	pointers := make([]any, len(labels))
	values := make([]any, len(labels))
	for i := range pointers {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		result := make(map[string]any, len(labels))
		for i, name := range labels {
			result[name] = values[i]
		}
		results = append(results, result)
	}
	return results, rows.Err()
}
