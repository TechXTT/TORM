// File: internal/core/connection.go
package core

import (
	"database/sql"
)

func Connect(driver, dsn string) (*sql.DB, error) {
	return sql.Open(driver, dsn)
}

func Close(db *sql.DB) error {
	return db.Close()
}
