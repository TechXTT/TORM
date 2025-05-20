package runtime

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

// Connect opens a database connection using the given DSN.
func Connect(dsn string) (*sql.DB, error) {
	// Ensure SSL mode is disabled by default if not specified.
	if strings.HasPrefix(dsn, "postgres://") && !strings.Contains(dsn, "sslmode=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn = dsn + sep + "sslmode=disable"
	}
	// If the DSN is empty, throw an error.
	if dsn == "" {
		return nil, fmt.Errorf("DSN is empty")
	}
	return sql.Open("postgres", dsn)
}
