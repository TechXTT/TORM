package db

import (
	"database/sql"
	"fmt"
)

type DB struct {
	Conn *sql.DB
}

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

func (db *DB) Query(query string) {
	rows, err := db.Conn.Query(query)
	if err != nil {
		fmt.Println(err)
	}

	defer rows.Close()
}
