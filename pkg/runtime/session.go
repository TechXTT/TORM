package runtime

import "database/sql"

type Session struct {
	DB *sql.DB
}

func NewSession(db *sql.DB) *Session {
	return &Session{DB: db}
}
