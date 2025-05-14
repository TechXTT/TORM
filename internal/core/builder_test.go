package core

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBuild_WithAllClauses(t *testing.T) {
	qb := NewQueryBuilder[any](nil).
		From("users").
		Select("id", "name").
		Where("active = ?", true).
		Join("JOIN orders ON orders.user_id = users.id").
		OrderBy("created_at DESC").
		Limit(10).
		Offset(5)

	sql, args := qb.Build()
	require.Equal(t,
		"SELECT id, name FROM users JOIN orders ON orders.user_id = users.id WHERE active = ? ORDER BY created_at DESC LIMIT 10 OFFSET 5",
		sql,
	)
	require.Equal(t, []interface{}{true}, args)
}

func TestBuild_Defaults(t *testing.T) {
	qb := NewQueryBuilder[any](nil).
		From("items")

	sql, args := qb.Build()
	require.Equal(t, "SELECT * FROM items", sql)
	require.Empty(t, args)
}

func TestCount(t *testing.T) {
	// Set up sqlmock
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect COUNT query
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM t WHERE x > \?`).
		WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	qb := NewQueryBuilder[any](db).
		From("t").
		Where("x > ?", 5)

	count, err := qb.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
	require.NoError(t, mock.ExpectationsWereMet())
}
