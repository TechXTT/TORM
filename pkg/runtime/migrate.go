package runtime

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Manager struct {
	db  *sql.DB
	dir string
}

type migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// ensureVersionTable creates the schema_migrations table if it doesn't exist.
func (m *Manager) ensureVersionTable() error {
	_, err := m.db.Exec(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY
        );
    `)
	return err
}

// loadMigrations reads all .up.sql and .down.sql files and returns a sorted slice.
func (m *Manager) loadMigrations() ([]migration, error) {
	files, err := ioutil.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	re := regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)
	tmp := map[int]*migration{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(f.Name())
		if matches == nil {
			continue
		}
		ver, _ := strconv.Atoi(matches[1])
		name := matches[2]
		direction := matches[3]
		content, err := ioutil.ReadFile(filepath.Join(m.dir, f.Name()))
		if err != nil {
			return nil, err
		}
		mig, exists := tmp[ver]
		if !exists {
			mig = &migration{Version: ver, Name: name}
			tmp[ver] = mig
		}
		if direction == "up" {
			mig.UpSQL = string(content)
		} else {
			mig.DownSQL = string(content)
		}
	}
	// Sort by version
	versions := make([]int, 0, len(tmp))
	for v := range tmp {
		versions = append(versions, v)
	}
	sort.Ints(versions)
	result := make([]migration, len(versions))
	for i, v := range versions {
		result[i] = *tmp[v]
	}
	return result, nil
}

// currentVersion fetches the highest applied migration version.
func (m *Manager) currentVersion() (int, error) {
	row := m.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`)
	var v sql.NullInt64
	if err := row.Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// recordVersion marks a migration as applied.
func (m *Manager) recordVersion(version int) error {
	_, err := m.db.Exec(`INSERT INTO schema_migrations(version) VALUES($1)`, version)
	return err
}

// deleteVersion removes a migration record (for rollback).
func (m *Manager) deleteVersion(version int) error {
	_, err := m.db.Exec(`DELETE FROM schema_migrations WHERE version = $1`, version)
	return err
}

// NewManager returns a migration manager pointed at a directory.
func NewManager(dsn, dir string) (*Manager, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create migrations dir: %w", err)
	}
	return &Manager{db: db, dir: dir}, nil
}

func (m *Manager) Dev() error {
	// apply any pending up migrations
	if err := m.ensureVersionTable(); err != nil {
		return fmt.Errorf("ensureVersionTable: %w", err)
	}
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("loadMigrations: %w", err)
	}
	current, err := m.currentVersion()
	if err != nil {
		return fmt.Errorf("currentVersion: %w", err)
	}
	for _, mig := range migrations {
		if mig.Version <= current {
			continue
		}
		if _, err := m.db.Exec(mig.UpSQL); err != nil {
			return fmt.Errorf("exec up migration %d_%s: %w", mig.Version, mig.Name, err)
		}
		if err := m.recordVersion(mig.Version); err != nil {
			return fmt.Errorf("recordVersion %d: %w", mig.Version, err)
		}
	}
	return nil
}

func (m *Manager) Deploy() error {
	// same as Dev but without drift detection
	return m.Dev()
}

func (m *Manager) Reset() error {
	// rollback all applied migrations, then reapply
	if err := m.ensureVersionTable(); err != nil {
		return fmt.Errorf("ensureVersionTable: %w", err)
	}
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("loadMigrations: %w", err)
	}
	current, err := m.currentVersion()
	if err != nil {
		return fmt.Errorf("currentVersion: %w", err)
	}
	// rollback in reverse order
	for i := len(migrations) - 1; i >= 0; i-- {
		mig := migrations[i]
		if mig.Version > current {
			continue
		}
		fmt.Printf("Reverting migration %d_%s.down.sql\n", mig.Version, mig.Name)
		if _, err := m.db.Exec(mig.DownSQL); err != nil {
			return fmt.Errorf("exec down migration %d_%s: %w", mig.Version, mig.Name, err)
		}
		if err := m.deleteVersion(mig.Version); err != nil {
			return fmt.Errorf("deleteVersion %d: %w", mig.Version, err)
		}
	}
	// reapply all
	return m.Dev()
}

func (m *Manager) Status() (string, error) {
	if err := m.ensureVersionTable(); err != nil {
		return "", fmt.Errorf("ensureVersionTable: %w", err)
	}
	migrations, err := m.loadMigrations()
	if err != nil {
		return "", fmt.Errorf("loadMigrations: %w", err)
	}
	current, err := m.currentVersion()
	if err != nil {
		return "", fmt.Errorf("currentVersion: %w", err)
	}
	statusLines := []string{fmt.Sprintf("Current version: %d", current)}
	for _, mig := range migrations {
		applied := "pending"
		if mig.Version <= current {
			applied = "applied"
		}
		statusLines = append(statusLines, fmt.Sprintf("%d_%s: %s", mig.Version, mig.Name, applied))
	}
	return strings.Join(statusLines, "\n"), nil
}
