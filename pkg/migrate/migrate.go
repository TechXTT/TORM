package migrate

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// Migration holds one versioned migration
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// Manager applies and rolls back migrations
type Manager struct {
	db            *sql.DB
	migrationsDir string
	migrations    []Migration
}

// NewManager loads migration files from the specified directory
func NewManager(db *sql.DB, migrationsDir string) (*Manager, error) {
	m := &Manager{db: db, migrationsDir: migrationsDir}
	if err := m.loadMigrations(); err != nil {
		return nil, err
	}
	return m, nil
}

// loadMigrations reads .up.sql/.down.sql files and organizes them by version
func (m *Manager) loadMigrations() error {
	entries, err := ioutil.ReadDir(m.migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	re := regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)
	tmp := map[int]*Migration{}
	for _, fi := range entries {
		if fi.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(fi.Name())
		if len(matches) != 4 {
			continue
		}
		ver, _ := strconv.Atoi(matches[1])
		name := matches[2]
		dir := matches[3]
		path := filepath.Join(m.migrationsDir, fi.Name())
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", fi.Name(), err)
		}
		mig, exists := tmp[ver]
		if !exists {
			mig = &Migration{Version: ver, Name: name}
			tmp[ver] = mig
		}
		if dir == "up" {
			mig.UpSQL = string(data)
		} else {
			mig.DownSQL = string(data)
		}
	}
	// sort and assign
	versions := make([]int, 0, len(tmp))
	for v := range tmp {
		versions = append(versions, v)
	}
	sort.Ints(versions)
	for _, v := range versions {
		m.migrations = append(m.migrations, *tmp[v])
	}
	return nil
}

// EnsureVersionTable creates schema_migrations if missing
func (m *Manager) EnsureVersionTable() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY);`)
	return err
}

// currentVersion returns the highest applied migration version
func (m *Manager) currentVersion() (int, error) {
	var v sql.NullInt64
	row := m.db.QueryRow(`SELECT MAX(version) FROM schema_migrations;`)
	if err := row.Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// recordVersion inserts a version record
func (m *Manager) recordVersion(version int) error {
	_, err := m.db.Exec(`INSERT INTO schema_migrations(version) VALUES($1);`, version)
	return err
}

// deleteVersion removes a version record
func (m *Manager) deleteVersion(version int) error {
	_, err := m.db.Exec(`DELETE FROM schema_migrations WHERE version = $1;`, version)
	return err
}

// Up applies all pending migrations
func (m *Manager) Up() error {
	if err := m.EnsureVersionTable(); err != nil {
		return err
	}
	current, err := m.currentVersion()
	if err != nil {
		return err
	}

	for _, mig := range m.migrations {
		if mig.Version <= current {
			continue
		}
		fmt.Printf("Applying %04d%s.up.sql\n", mig.Version, mig.Name)
		if _, err := m.db.Exec(mig.UpSQL); err != nil {
			return fmt.Errorf("apply up %d: %w", mig.Version, err)
		}
		if err := m.recordVersion(mig.Version); err != nil {
			return fmt.Errorf("record version %d: %w", mig.Version, err)
		}
	}
	return nil
}

// Down rolls back the latest migration
func (m *Manager) Down() error {
	if err := m.EnsureVersionTable(); err != nil {
		return err
	}

	current, err := m.currentVersion()
	if err != nil {
		return err
	}
	if current == 0 {
		fmt.Println("No migrations to roll back.")
		return nil
	}
	var toRoll *Migration
	for i := len(m.migrations) - 1; i >= 0; i-- {
		if m.migrations[i].Version == current {
			toRoll = &m.migrations[i]
			break
		}
	}
	if toRoll == nil {
		return fmt.Errorf("migration not found for version %d", current)
	}
	fmt.Printf("Rolling back %04d_%s.down.sql\n", toRoll.Version, toRoll.Name)
	if _, err := m.db.Exec(toRoll.DownSQL); err != nil {
		return fmt.Errorf("apply down %d: %w", toRoll.Version, err)
	}
	return m.deleteVersion(toRoll.Version)
}
