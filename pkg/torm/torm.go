// File: pkg/torm/torm.go
package torm

import (
	"context"
	"database/sql"

	"github.com/TechXTT/TORM/internal/core"
)

// DB is the main handle for executing queries
type DB struct {
	conn *sql.DB
}

// Open connects to the database and returns a DB
func Open(dsn string) (*DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &DB{conn: db}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.conn.Close()
}

// Query returns a new query builder
func Query[T any](d *DB) *core.QueryBuilder[T] {
	return core.NewQueryBuilder[T](d.conn)
}

// Exec executes raw SQL with context
func (d *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.conn.ExecContext(ctx, query, args...)
}
