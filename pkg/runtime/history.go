package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type migration struct {
	Version  int
	Name     string
	Up       string
	Down     string
	Checksum string
}

type appliedMigration struct {
	Version  int
	Checksum sql.NullString
}

var filenamePattern = regexp.MustCompile(`^([0-9]+)_([A-Za-z0-9_-]+)\.(up|down)\.sql$`)
var migrationName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func loadMigrations(dir string) ([]migration, error) {
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	migrations := make(map[int]migration)
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || (!strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".down.sql")) {
			continue
		}
		match := filenamePattern.FindStringSubmatch(name)
		if match == nil || file.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("invalid migration file %q", name)
		}
		version, err := strconv.ParseInt(match[1], 10, 32)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid migration version in %q", name)
		}
		entry, found := migrations[int(version)]
		if found && entry.Name != match[2] {
			return nil, fmt.Errorf("conflicting names for migration %d", version)
		}
		entry.Version, entry.Name = int(version), match[2]
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		// Normalize line endings for cross-platform checksums.
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		if strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("empty migration file %q", name)
		}
		if match[3] == "up" {
			if err := validateSQL(content); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			if entry.Up != "" {
				return nil, fmt.Errorf("duplicate up migration %d", version)
			}
			entry.Up = content
		} else {
			if entry.Down != "" {
				return nil, fmt.Errorf("duplicate down migration %d", version)
			}
			entry.Down = content
		}
		migrations[int(version)] = entry
	}
	result := make([]migration, 0, len(migrations))
	for _, entry := range migrations {
		if entry.Up == "" {
			return nil, fmt.Errorf("migration %d_%s has no up file", entry.Version, entry.Name)
		}
		digest := sha256.Sum256([]byte(entry.Name + "\x00" + entry.Up))
		entry.Checksum = hex.EncodeToString(digest[:])
		result = append(result, entry)
	}
	slices.SortFunc(result, func(a, b migration) int { return a.Version - b.Version })
	return result, nil
}

func history(ctx context.Context, tx *sql.Tx) ([]appliedMigration, error) {
	var exists bool
	var hasChecksum bool
	var result []appliedMigration

	err := tx.QueryRowContext(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL,
		EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = to_regclass('public.schema_migrations') AND attname = 'checksum' AND NOT attisdropped)`).Scan(&exists, &hasChecksum)
	if err != nil || !exists {
		return nil, err
	}
	checksum := "NULL::text"
	if hasChecksum {
		checksum = "checksum"
	}
	rows, err := tx.QueryContext(ctx, "SELECT version, "+checksum+" FROM public.schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry appliedMigration

		if err := rows.Scan(&entry.Version, &entry.Checksum); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func checkHistory(files []migration, applied []appliedMigration, allowLegacy bool) error {
	if len(applied) > len(files) {
		return fmt.Errorf("database records migrations missing from disk")
	}
	for i, entry := range applied {
		if files[i].Version != entry.Version {
			return fmt.Errorf("migration history diverges at version %d (database has %d)", files[i].Version, entry.Version)
		}
		if !entry.Checksum.Valid {
			if !allowLegacy {
				return fmt.Errorf("migration %d has no checksum; run migrate baseline with a shadow database", entry.Version)
			}
			continue
		}
		if entry.Checksum.String != files[i].Checksum {
			return fmt.Errorf("checksum mismatch for applied migration %d_%s", entry.Version, files[i].Name)
		}
	}
	return nil
}

func ensureHistory(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version INTEGER PRIMARY KEY,
		checksum TEXT,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	ALTER TABLE public.schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT;
	ALTER TABLE public.schema_migrations ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;`)
	return err
}

func record(ctx context.Context, tx *sql.Tx, item migration) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO public.schema_migrations (version, checksum) VALUES ($1, $2)
		ON CONFLICT (version) DO UPDATE SET checksum = excluded.checksum WHERE schema_migrations.checksum IS NULL`, item.Version, item.Checksum)
	return err
}

func writePlan(dir, name string, files []migration, plan MigrationPlan) (string, error) {
	if !migrationName.MatchString(name) || len(name) > 80 {
		return "", fmt.Errorf("migration name must be 1–80 letters, digits, underscores, or hyphens")
	}
	version := 1
	if len(files) != 0 {
		version = files[len(files)-1].Version + 1
	}
	if version > 2147483647 {
		return "", fmt.Errorf("migration version limit reached")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	lockPath := filepath.Join(dir, ".torm-plan.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("lock migration directory: %w", err)
	}
	defer os.Remove(lockPath)
	if err := lock.Close(); err != nil {
		return "", err
	}
	current, err := loadMigrations(dir)
	if err != nil {
		return "", err
	}
	if !slices.EqualFunc(files, current, func(a, b migration) bool {
		return a.Version == b.Version && a.Checksum == b.Checksum
	}) {
		return "", fmt.Errorf("migration files changed during planning; run the plan again")
	}
	path := filepath.Join(dir, fmt.Sprintf("%04d_%s.up.sql", version, name))
	file, err := os.CreateTemp(dir, ".torm-plan-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(plan.SQL)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "", err
	}
	// Publish atomically without replacing an existing migration.
	if err := os.Link(file.Name(), path); err != nil {
		return "", fmt.Errorf("publish migration %s: %w", path, err)
	}
	return path, nil
}
