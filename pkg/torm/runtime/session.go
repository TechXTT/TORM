package runtime

import "database/sql"

// Session holds a DB connection and provides query context.
type Session struct {
	DB *sql.DB
}

// NewSession creates a new session from an existing DB.
func NewSession(db *sql.DB) *Session {
	return &Session{DB: db}
}

// TODO: add Exec, Query, Transaction helpers
