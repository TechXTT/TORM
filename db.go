package db

import (
	"database/sql"
	"fmt"
	"reflect"
	"unicode"
	// _ "github.com/lib/pq" // PostgreSQL driver
)

// DB struct using standard sql.DB
type DB struct {
	Conn *sql.DB
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

// Select retrieves all rows from the table corresponding to the provided struct slice
func (db *DB) Select(dest interface{}) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("dest must be a pointer to a slice")
	}

	sliceVal := destVal.Elem()
	elemType := sliceVal.Type().Elem()

	// Use the struct name as the table name
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

	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := db.Conn.Query(query)
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
			fieldPtrs[i] = elemVal.Field(i).Addr().Interface()
		}

		if err := rows.Scan(fieldPtrs...); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		sliceVal.Set(reflect.Append(sliceVal, elemVal))
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	return nil
}
