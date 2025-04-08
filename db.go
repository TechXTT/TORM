package db

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	// _ "github.com/lib/pq" // PostgreSQL driver
)

// DB struct using standard sql.DB
type DB struct {
	Conn *sql.DB
}

type QueryBuilder struct {
	db           *DB
	tableName    string
	whereClauses []string
	args         []interface{}
}

// NewDB initializes a new database connection
func NewDB(dataSourceName string) (*DB, error) {
	conn, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{Conn: conn}, nil
}

func (db *DB) Query(tableName string) *QueryBuilder {
	return &QueryBuilder{db: db, tableName: tableName}
}

func (qb *QueryBuilder) Where(clause string, args ...interface{}) *QueryBuilder {
	for _, arg := range args {
		clause = replaceFirst(clause, "?", fmt.Sprintf("'%v'", arg))
	}
	qb.whereClauses = append(qb.whereClauses, clause)
	qb.args = append(qb.args, args...)
	return qb
}

func replaceFirst(str, old, new string) string {
	index := strings.Index(str, old)
	if index == -1 {
		return str
	}
	return str[:index] + new + str[index+len(old):]
}

func (qb *QueryBuilder) Select(dest interface{}) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("dest must be a pointer to a slice")
	}

	sliceVal := destVal.Elem()
	elemType := sliceVal.Type().Elem()

	query := fmt.Sprintf("SELECT * FROM %s", qb.tableName)
	if len(qb.whereClauses) > 0 {
		query += " WHERE " + qb.whereClauses[0]
		for i := 1; i < len(qb.whereClauses); i++ {
			query += " AND " + qb.whereClauses[i]
		}
	}

	rows, err := qb.db.Conn.Query(query, qb.args...)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}

	for rows.Next() {
		elemPtr := reflect.New(elemType)
		elemVal := elemPtr.Elem()

		fieldPtrs := make([]interface{}, len(columns))
		for i := range columns {
			field := elemVal.Field(i)
			if field.Kind() == reflect.Ptr {
				fieldPtrs[i] = field.Addr().Interface()
			} else {
				fieldPtrs[i] = reflect.New(field.Type()).Interface()
			}
		}

		if err := rows.Scan(fieldPtrs...); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		for i, _ := range columns {
			field := elemVal.Field(i)
			if field.Kind() == reflect.Ptr {
				field.Set(reflect.ValueOf(fieldPtrs[i]).Elem())
			} else {
				val := reflect.ValueOf(fieldPtrs[i]).Elem()
				if val.IsValid() {
					field.Set(val)
				}
			}
		}

		sliceVal.Set(reflect.Append(sliceVal, elemVal))
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	return nil
}

func (db *DB) AutoMigrate(dest interface{}) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("dest must be a pointer to a struct")
	}

	elemVal := destVal.Elem()
	elemType := elemVal.Type()

	tableName := ""
	for i, r := range elemType.Name() {
		if i == 0 {
			tableName += string(unicode.ToLower(r))
		} else {
			if unicode.IsUpper(r) {
				tableName += "_" + string(unicode.ToLower(r))
			} else {
				tableName += string(r)
			}
		}
	}

	type_dict := map[string]string{
		"int":    "INTEGER",
		"string": "TEXT",
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (", tableName)
	for i := 0; i < elemType.NumField(); i++ {
		field := elemType.Field(i)
		fieldName := ""
		for j, r := range field.Name {
			if j == 0 {
				fieldName += string(unicode.ToLower(r))
			} else {
				if unicode.IsUpper(r) {
					fieldName += "_" + string(unicode.ToLower(r))
				} else {
					fieldName += string(r)
				}
			}
		}

		query += fieldName + " " + type_dict[field.Type.Name()] + ","
	}
	query = query[:len(query)-1] + ");"

	if _, err := db.Conn.Exec(query); err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}

	return nil
}

func (db *DB) Close() error {
	return db.Conn.Close()
}
