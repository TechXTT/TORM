package migrate

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUp_AppliesPendingMigrations(t *testing.T) {
	// 1) create temp migrations dir
	dir, err := ioutil.TempDir("", "migtest")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// write one migration pair
	upSQL := "CREATE TABLE foo();"
	downSQL := "DROP TABLE foo;"
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(dir, "0001_foo.up.sql"),
		[]byte(upSQL), 0644))
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(dir, "0001_foo.down.sql"),
		[]byte(downSQL), 0644))

	// 2) prepare sqlmock
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect ensure version table
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// currentVersion: no rows -> NULL -> 0
	mock.ExpectQuery(`SELECT MAX\(version\) FROM schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	// Exec the UP SQL
	mock.ExpectExec(fmt.Sprintf("^%s$", regexp.QuoteMeta(upSQL))).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// recordVersion
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mgr, err := NewManager(db, dir)
	require.NoError(t, err)

	// 3) run Up and verify
	require.NoError(t, mgr.Up())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDown_RollsBackLatestMigration(t *testing.T) {
	// same setup for temp dir
	dir, err := ioutil.TempDir("", "migtest")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	ioutil.WriteFile(filepath.Join(dir, "0001_foo.up.sql"), []byte("X"), 0644)
	ioutil.WriteFile(filepath.Join(dir, "0001_foo.down.sql"), []byte("Y"), 0644)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// ensure table
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// currentVersion returns 1
	mock.ExpectQuery(`SELECT MAX\(version\) FROM schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(1))
	// Exec the DOWN SQL
	mock.ExpectExec("^Y$").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// deleteVersion
	mock.ExpectExec(`DELETE FROM schema_migrations WHERE version = \$1`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mgr, err := NewManager(db, dir)
	require.NoError(t, err)

	require.NoError(t, mgr.Down())
	require.NoError(t, mock.ExpectationsWereMet())
}
