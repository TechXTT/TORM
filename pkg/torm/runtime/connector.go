package runtime

import (
	"database/sql"

	_ "github.com/lib/pq"
)

// Connect opens a database connection using the given DSN.
func Connect(dsn string) (*sql.DB, error) {
	return sql.Open("postgres", dsn)
}
