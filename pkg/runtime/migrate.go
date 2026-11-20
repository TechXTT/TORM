package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/TechXTT/TORM/v2/pkg/internal/schema"
)

const migrationLock = 453968184766172851
const shadowLock = migrationLock + 1

type Manager struct {
	db  *sql.DB
	dir string
}

func NewManager(dsn, dir string) (*Manager, error) {
	db, err := Connect(dsn)
	if err != nil {
		return nil, err
	}
	return &Manager{db: db, dir: dir}, nil
}

func (m *Manager) Close() error {
	return m.db.Close()
}

func locked(ctx context.Context, db *sql.DB, lockID int64, run func(*sql.Conn) error) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, unlockErr := conn.ExecContext(cleanup, "SELECT pg_advisory_unlock($1)", lockID); unlockErr != nil {
			conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, unlockErr)
		}
	}()
	return run(conn)
}

func transaction(ctx context.Context, conn *sql.Conn, namespace string, readOnly bool, run func(*sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: readOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SET LOCAL search_path = "+schema.Quote(namespace)+", public; SET LOCAL standard_conforming_strings = on"); err != nil {
		return err
	}
	if err := run(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func runMigration(ctx context.Context, tx *sql.Tx, item migration) error {
	if _, err := tx.ExecContext(ctx, item.Up); err != nil {
		return fmt.Errorf("apply migration %04d_%s: %w", item.Version, item.Name, err)
	}
	return nil
}

func withShadow(ctx context.Context, dsn, targetName string, run func(shadowSchema) error) error {

	if dsn == "" {
		return fmt.Errorf("set TORM_SHADOW_DATABASE_URL to a separate, dedicated shadow database")
	}
	db, err := Connect(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return locked(ctx, db, shadowLock, func(conn *sql.Conn) error {
		var name string
		var initial catalog

		if err := conn.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil {
			return err
		}
		if name == targetName {
			return fmt.Errorf("the shadow database must have a different database name from the target")
		}
		if err := transaction(ctx, conn, "public", true, func(tx *sql.Tx) error {
			state, err := readCatalog(ctx, tx)
			if err != nil {
				return err
			}
			applied, err := history(ctx, tx)
			if err != nil {
				return err
			}
			if len(applied)+len(state.Tables)+len(state.Enums)+len(state.Other) > 0 {
				return fmt.Errorf("the shadow database must have an empty public schema")
			}
			initial = state
			return nil
		}); err != nil {
			return err
		}
		return inShadowSchema(ctx, conn, initial, run)
	})
}

type shadowSchema struct {
	conn      *sql.Conn
	namespace string
	public    catalog
}

func inShadowSchema(ctx context.Context, conn *sql.Conn, public catalog, run func(shadowSchema) error) (err error) {
	var random [12]byte

	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	namespace := "torm_shadow_" + hex.EncodeToString(random[:])
	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA "+schema.Quote(namespace)); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, dropErr := conn.ExecContext(cleanup, "DROP SCHEMA "+schema.Quote(namespace)+" CASCADE")
		err = errors.Join(err, dropErr)
	}()
	return run(shadowSchema{conn: conn, namespace: namespace, public: public})
}

func (s shadowSchema) replay(ctx context.Context, files []migration) error {
	for _, item := range files {
		if err := transaction(ctx, s.conn, s.namespace, false, func(tx *sql.Tx) error {
			if err := runMigration(ctx, tx, item); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "SET LOCAL search_path = public"); err != nil {
				return err
			}
			public, err := readCatalog(ctx, tx)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(s.public, public) {
				return fmt.Errorf("migration %04d_%s changes the shadow database's public schema; use unqualified names", item.Version, item.Name)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("shadow replay: %w", err)
		}
	}
	return nil
}

func (s shadowSchema) catalog(ctx context.Context, ddl string) (catalog, error) {
	var result catalog

	err := transaction(ctx, s.conn, s.namespace, ddl == "", func(tx *sql.Tx) error {
		if ddl != "" {
			if _, err := tx.ExecContext(ctx, ddl); err != nil {
				return fmt.Errorf("evaluate schema in shadow database: %w", err)
			}
		}
		var err error

		result, err = readCatalog(ctx, tx)
		return err
	})
	return result, err
}

func (s shadowSchema) verify(ctx context.Context, files []migration, live catalog) (catalog, error) {
	if err := s.replay(ctx, files); err != nil {
		return catalog{}, err
	}
	expected, err := s.catalog(ctx, "")
	if err != nil {
		return catalog{}, err
	}
	return expected, verifyCatalog(expected, live)
}

type liveState struct {
	Name    string
	Schema  catalog
	History []appliedMigration
}

func inspect(ctx context.Context, conn *sql.Conn) (liveState, error) {
	var state liveState

	err := transaction(ctx, conn, "public", true, func(tx *sql.Tx) error {
		var err error

		if err := tx.QueryRowContext(ctx, "SELECT current_database()").Scan(&state.Name); err != nil {
			return err
		}
		state.History, err = history(ctx, tx)
		if err != nil {
			return err
		}
		state.Schema, err = readCatalog(ctx, tx)
		return err
	})
	return state, err
}

func verifyCatalog(historyState, live catalog) error {
	if reflect.DeepEqual(historyState, live) {
		return nil
	}
	for _, name := range sortedKeys(live.Tables) {
		if !reflect.DeepEqual(live.Tables[name], historyState.Tables[name]) {
			return fmt.Errorf("schema drift: table %s differs from migration history", name)
		}
	}
	for _, name := range sortedKeys(historyState.Tables) {
		if live.Tables[name] == nil {
			return fmt.Errorf("schema drift: table %s from migration history is missing", name)
		}
	}
	return fmt.Errorf("schema drift: enums or other PostgreSQL objects differ from migration history")
}

func planMigration(ctx context.Context, conn *sql.Conn, files []migration, shadowDSN, ddl string) (MigrationPlan, error) {
	var plan MigrationPlan

	if err := validateSQL(ddl); err != nil {
		return plan, fmt.Errorf("desired schema SQL: %w", err)
	}
	live, err := inspect(ctx, conn)
	if err != nil {
		return plan, err
	}
	if err := checkHistory(files, live.History, false); err != nil {
		return plan, err
	}
	applied := len(live.History)
	err = withShadow(ctx, shadowDSN, live.Name, func(history shadowSchema) error {
		expected, err := history.verify(ctx, files[:applied], live.Schema)
		if err != nil {
			return err
		}
		if applied < len(files) {
			if err := history.replay(ctx, files[applied:]); err != nil {
				return err
			}
			expected, err = history.catalog(ctx, "")
			if err != nil {
				return err
			}
		}
		var desired catalog

		if err := inShadowSchema(ctx, history.conn, history.public, func(s shadowSchema) error {
			desired, err = s.catalog(ctx, ddl)
			return err
		}); err != nil {
			return err
		}
		plan, err = diff(expected, desired)
		if err != nil || plan.SQL == "" {
			return err
		}
		if err := validateSQL(plan.SQL); err != nil {
			return err
		}
		preview, err := history.catalog(ctx, plan.SQL)
		if err != nil {
			return fmt.Errorf("generated SQL could not be replayed; use a reviewed manual migration: %w", err)
		}
		if err := verifyCatalog(desired, preview); err != nil {
			return fmt.Errorf("generated SQL did not produce the desired schema: %w", err)
		}
		return nil
	})
	return plan, err
}

func (m *Manager) Plan(ctx context.Context, shadowDSN, ddl, name string, acceptDataLoss bool) (MigrationPlan, string, error) {
	var plan MigrationPlan
	var path string

	err := locked(ctx, m.db, migrationLock, func(conn *sql.Conn) error {
		files, err := loadMigrations(m.dir)
		if err != nil {
			return err
		}
		plan, err = planMigration(ctx, conn, files, shadowDSN, ddl)
		if err != nil || plan.SQL == "" || name == "" {
			return err
		}
		if len(plan.Review) != 0 && !acceptDataLoss {
			return fmt.Errorf("review SQL and pass --accept-data-loss to write this migration")
		}
		path, err = writePlan(m.dir, name, files, plan)
		return err
	})
	return plan, path, err
}

func deploy(ctx context.Context, conn *sql.Conn, files []migration) error {
	var applied []appliedMigration

	if err := transaction(ctx, conn, "public", true, func(tx *sql.Tx) error {
		var err error

		applied, err = history(ctx, tx)
		if err != nil {
			return err
		}
		if err := checkHistory(files, applied, false); err != nil {
			return err
		}
		if len(applied) == 0 {
			state, err := readCatalog(ctx, tx)
			if err != nil {
				return err
			}
			if len(state.Tables)+len(state.Enums)+len(state.Other) != 0 {
				return fmt.Errorf("database has schema objects without migration history; verify and run migrate baseline")
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, item := range files[len(applied):] {
		if err := transaction(ctx, conn, "public", false, func(tx *sql.Tx) error {
			if err := ensureHistory(ctx, tx); err != nil {
				return err
			}
			if err := runMigration(ctx, tx, item); err != nil {
				return err
			}
			return record(ctx, tx, item)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Develop(ctx context.Context, shadowDSN, ddl, name string, acceptDataLoss bool) (MigrationPlan, string, error) {
	var plan MigrationPlan
	var path string

	err := locked(ctx, m.db, migrationLock, func(conn *sql.Conn) error {
		files, err := loadMigrations(m.dir)
		if err != nil {
			return err
		}
		plan, err = planMigration(ctx, conn, files, shadowDSN, ddl)
		if err != nil {
			return err
		}
		if plan.SQL != "" {
			if len(plan.Review) != 0 && !acceptDataLoss {
				return fmt.Errorf("review the plan and pass --accept-data-loss to apply this migration")
			}
			path, err = writePlan(m.dir, name, files, plan)
			if err != nil {
				return err
			}
			files, err = loadMigrations(m.dir)
			if err != nil {
				return err
			}
		}
		return deploy(ctx, conn, files)
	})
	return plan, path, err
}

func (m *Manager) DeployContext(ctx context.Context) error {
	return locked(ctx, m.db, migrationLock, func(conn *sql.Conn) error {
		files, err := loadMigrations(m.dir)
		if err != nil {
			return err
		}
		return deploy(ctx, conn, files)
	})
}

func (m *Manager) Baseline(ctx context.Context, shadowDSN string, version int) error {
	return locked(ctx, m.db, migrationLock, func(conn *sql.Conn) error {
		files, err := loadMigrations(m.dir)
		if err != nil {
			return err
		}
		live, err := inspect(ctx, conn)
		if err != nil {
			return err
		}
		if err := checkHistory(files, live.History, true); err != nil {
			return err
		}
		count := len(live.History)
		if version != 0 {
			count = 0
			for i, item := range files {
				if item.Version == version {
					count = i + 1
					break
				}
			}
			if count == 0 || count < len(live.History) {
				return fmt.Errorf("baseline version must exist on disk and include all existing history")
			}
		} else if count == 0 {
			count = len(files)
		}
		if count == 0 {
			return fmt.Errorf("baseline requires at least one reviewed migration")
		}
		if err := withShadow(ctx, shadowDSN, live.Name, func(s shadowSchema) error {
			_, err := s.verify(ctx, files[:count], live.Schema)
			return err
		}); err != nil {
			return err
		}
		return transaction(ctx, conn, "public", false, func(tx *sql.Tx) error {
			if err := ensureHistory(ctx, tx); err != nil {
				return err
			}
			for _, item := range files[:count] {
				if err := record(ctx, tx, item); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (m *Manager) ResetContext(ctx context.Context, shadowDSN string) error {
	return locked(ctx, m.db, migrationLock, func(conn *sql.Conn) error {
		files, err := loadMigrations(m.dir)
		if err != nil {
			return err
		}
		live, err := inspect(ctx, conn)
		if err != nil {
			return err
		}
		if err := checkHistory(files, live.History, false); err != nil {
			return err
		}
		applied := len(live.History)
		if err := withShadow(ctx, shadowDSN, live.Name, func(s shadowSchema) error {
			if _, err := s.verify(ctx, files[:applied], live.Schema); err != nil {
				return err
			}
			return s.replay(ctx, files[applied:])
		}); err != nil {
			return err
		}
		drop, err := diff(live.Schema, catalog{})
		if err != nil {
			return err
		}
		if err := transaction(ctx, conn, "public", false, func(tx *sql.Tx) error {
			if err := ensureHistory(ctx, tx); err != nil {
				return err
			}
			if drop.SQL != "" {
				if _, err := tx.ExecContext(ctx, drop.SQL); err != nil {
					return err
				}
			}
			_, err := tx.ExecContext(ctx, "DELETE FROM public.schema_migrations")
			return err
		}); err != nil {
			return err
		}
		return deploy(ctx, conn, files)
	})
}

func (m *Manager) StatusContext(ctx context.Context) (string, error) {
	var lines []string

	err := locked(ctx, m.db, migrationLock, func(conn *sql.Conn) error {
		files, err := loadMigrations(m.dir)
		if err != nil {
			return err
		}
		var applied []appliedMigration

		if err := transaction(ctx, conn, "public", true, func(tx *sql.Tx) error {
			applied, err = history(ctx, tx)
			return err
		}); err != nil {
			return err
		}
		if err := checkHistory(files, applied, true); err != nil {
			return err
		}
		current := 0
		if n := len(applied); n != 0 {
			current = applied[n-1].Version
		}
		lines = append(lines, fmt.Sprintf("Current version: %d", current))
		for i, item := range files {
			status := "pending"
			if i < len(applied) {
				status = "applied"
				if !applied[i].Checksum.Valid {
					status = "applied (baseline required)"
				}
			}
			lines = append(lines, fmt.Sprintf("%04d_%s: %s", item.Version, item.Name, status))
		}
		return nil
	})
	return strings.Join(lines, "\n"), err
}

func (m *Manager) Dev() error {
	return m.Deploy()
}

func (m *Manager) Deploy() error {
	return m.DeployContext(context.Background())
}

func (m *Manager) Reset() error {
	return m.ResetContext(context.Background(), os.Getenv("TORM_SHADOW_DATABASE_URL"))
}

func (m *Manager) Status() (string, error) {
	return m.StatusContext(context.Background())
}
