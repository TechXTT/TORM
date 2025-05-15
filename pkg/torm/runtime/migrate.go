package runtime

import (
	"database/sql"
	"fmt"
)

type Manager struct {
	db  *sql.DB
	dir string
}

// NewManager returns a migration manager pointed at a directory.
func NewManager(db *sql.DB, dir string) (*Manager, error) {
	return &Manager{db: db, dir: dir}, nil
}

func (m *Manager) Up() error {
	fmt.Println("Running migrations up in", m.dir)
	// TODO: apply .sql files in sequence
	return nil
}

func (m *Manager) Down() error {
	fmt.Println("Reverting migrations in", m.dir)
	// TODO: revert last migration
	return nil
}

func (m *Manager) Status() (string, error) {
	// TODO: inspect migrations table
	return "OK", nil
}
