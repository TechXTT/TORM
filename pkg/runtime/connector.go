package runtime

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

func Connect(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL connection string is empty")
	}
	connector, err := pq.NewConnector(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL connection configuration")
	}
	return sql.OpenDB(connector), nil
}
