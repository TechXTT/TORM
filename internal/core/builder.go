// File: internal/core/builder.go
package core

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// QueryBuilder is a generics-based fluent query builder
type QueryBuilder[T any] struct {
	db          *sql.DB
	table       string
	selectCols  []string
	whereOps    []string
	args        []interface{}
	joinClauses []string // added for JOINs
	orderBy     string   // added for ORDER BY
	limit       int      // added for LIMIT
	offset      int      // added for OFFSET
}

func NewQueryBuilder[T any](db *sql.DB) *QueryBuilder[T] {
	return &QueryBuilder[T]{db: db}
}

func (qb *QueryBuilder[T]) From(table string) *QueryBuilder[T] {
	qb.table = table
	return qb
}

func (qb *QueryBuilder[T]) Select(cols ...string) *QueryBuilder[T] {
	qb.selectCols = cols
	return qb
}

func (qb *QueryBuilder[T]) Where(cond string, vals ...interface{}) *QueryBuilder[T] {
	qb.whereOps = append(qb.whereOps, cond)
	qb.args = append(qb.args, vals...)
	return qb
}

// Join adds a JOIN clause (e.g. "JOIN other_table ON ...")
func (qb *QueryBuilder[T]) Join(clause string) *QueryBuilder[T] {
	qb.joinClauses = append(qb.joinClauses, clause)
	return qb
}

// OrderBy sets the ORDER BY clause
func (qb *QueryBuilder[T]) OrderBy(order string) *QueryBuilder[T] {
	qb.orderBy = order
	return qb
}

// Limit sets the LIMIT clause
func (qb *QueryBuilder[T]) Limit(n int) *QueryBuilder[T] {
	qb.limit = n
	return qb
}

// Offset sets the OFFSET clause
func (qb *QueryBuilder[T]) Offset(n int) *QueryBuilder[T] {
	qb.offset = n
	return qb
}

// Build assembles the SQL query string and returns it with args
func (qb *QueryBuilder[T]) Build() (string, []interface{}) {
	parts := []string{"SELECT"}
	if len(qb.selectCols) > 0 {
		parts = append(parts, strings.Join(qb.selectCols, ", "))
	} else {
		parts = append(parts, "*")
	}
	parts = append(parts, "FROM", qb.table)
	if len(qb.joinClauses) > 0 {
		parts = append(parts, strings.Join(qb.joinClauses, " "))
	}
	if len(qb.whereOps) > 0 {
		parts = append(parts, "WHERE", strings.Join(qb.whereOps, " AND "))
	}
	if qb.orderBy != "" {
		parts = append(parts, "ORDER BY", qb.orderBy)
	}
	if qb.limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", qb.limit))
	}
	if qb.offset > 0 {
		parts = append(parts, fmt.Sprintf("OFFSET %d", qb.offset))
	}
	query := strings.Join(parts, " ")
	return query, qb.args
}

// All executes the built query and scans into a slice of T
func (qb *QueryBuilder[T]) All(ctx context.Context) ([]T, error) {
	query, args := qb.Build()
	rows, err := qb.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []T
	for rows.Next() {
		var item T
		// TODO: use reflection or a scanner to populate item
		if err := rows.Scan( /* fields of item */ ); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// One fetches a single record into T
func (qb *QueryBuilder[T]) One(ctx context.Context) (T, error) {
	qb.limit = 1
	items, err := qb.All(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	if len(items) == 0 {
		var zero T
		return zero, sql.ErrNoRows
	}
	return items[0], nil
}

// Count returns the count of matching records
func (qb *QueryBuilder[T]) Count(ctx context.Context) (int64, error) {
	// Temporarily override SELECT and ignore other clauses except WHERE and JOIN
	originalCols := qb.selectCols
	qb.selectCols = []string{"COUNT(*)"}
	query, args := qb.Build()
	qb.selectCols = originalCols

	row := qb.db.QueryRowContext(ctx, query, args...)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
