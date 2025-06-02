// pkg/internal/migrate/stubs_test.go

package migrate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// minimalPrismaSchema_New defines two models: Author and Book (both new)
const minimalPrismaSchema_New = `
model Author {
  id   String @id @default(uuid()) @db.Uuid
  name String
}

model Book {
  id    String @id @default(uuid()) @db.Uuid
  title String
}
`

// minimalPrismaSchema_Alter defines the same two models,
// but Book has an extra “pages” field to trigger ALTER logic.
const minimalPrismaSchema_Alter = `
model Author {
  id    String @id @default(uuid()) @db.Uuid
  email String @unique
}

model Book {
  id     String @id @default(uuid()) @db.Uuid
  title  String
  pages  Int
}
`

// TestEnsureStubs_NewTables verifies that when no tables exist in the DB,
// EnsureStubs emits CREATE TABLE stubs for both Author and Book.
func TestEnsureStubs_NewTables(t *testing.T) {
	// 1) Set up sqlmock to simulate an empty database—no columns in any table.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("unexpected error opening stub database: %v", err)
	}
	defer db.Close()

	// Both Author and Book should return zero rows from information_schema.columns
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT column_name, udt_name
             FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = $1`,
	)).WillReturnRows(sqlmock.NewRows([]string{"column_name", "udt_name"}))
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT column_name, udt_name
             FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = $1`,
	)).WillReturnRows(sqlmock.NewRows([]string{"column_name", "udt_name"}))

	// 2) Create a temporary directory to hold schema.prisma and migrations/
	tmpDir, err := ioutil.TempDir("", "torm-stubs-new")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write schema.prisma
	schemaPath := filepath.Join(tmpDir, "schema.prisma")
	if err := ioutil.WriteFile(schemaPath, []byte(minimalPrismaSchema_New), 0644); err != nil {
		t.Fatalf("failed to write schema.prisma: %v", err)
	}

	// Create migrations directory
	migrationsDir := filepath.Join(tmpDir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}

	// 3) Call EnsureStubs
	if err := EnsureStubs(db, schemaPath, migrationsDir); err != nil {
		t.Fatalf("EnsureStubs failed: %v", err)
	}

	// 4) Check that exactly 4 files were written: 2 models × (up + down)
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("failed to read migrations dir: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 stub files, got %d", len(files))
	}

	// 5) Verify each “.up.sql” contains a `CREATE TABLE <table>` statement
	upRegex := regexp.MustCompile(`^\d{4}_(Author|Book)\.up\.sql$`)
	downRegex := regexp.MustCompile(`^\d{4}_(Author|Book)\.down\.sql$`)
	foundUp := map[string]bool{"Author": false, "Book": false}
	foundDown := map[string]bool{"Author": false, "Book": false}

	for _, f := range files {
		name := f.Name()
		switch {
		case upRegex.MatchString(name):
			model := upRegex.FindStringSubmatch(name)[1]
			foundUp[model] = true

			contents, _ := ioutil.ReadFile(filepath.Join(migrationsDir, name))
			if !strings.Contains(string(contents), "CREATE TABLE "+strings.ToLower(model)) {
				t.Errorf("up stub %s missing CREATE TABLE, got:\n%s", name, string(contents))
			}

		case downRegex.MatchString(name):
			model := downRegex.FindStringSubmatch(name)[1]
			foundDown[model] = true

			contents, _ := ioutil.ReadFile(filepath.Join(migrationsDir, name))
			if !strings.Contains(string(contents), "DROP TABLE "+strings.ToLower(model)) {
				t.Errorf("down stub %s missing DROP TABLE, got:\n%s", name, string(contents))
			}

		default:
			t.Errorf("unexpected file in migrations: %s", name)
		}
	}

	for model, ok := range foundUp {
		if !ok {
			t.Errorf("missing up stub for model %s", model)
		}
	}
	for model, ok := range foundDown {
		if !ok {
			t.Errorf("missing down stub for model %s", model)
		}
	}

	// 6) Ensure sqlmock expectations were satisfied
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled SQL mock expectations: %s", err)
	}
}

// TestGenerateStubs_AlterTable verifies that when “author” does not exist
// but “book” does exist with columns id+title, EnsureStubs emits:
//   - CREATE TABLE stub for Author
//   - ALTER TABLE … ADD COLUMN pages for Book
func TestGenerateStubs_AlterTable(t *testing.T) {
	// 1) Set up sqlmock
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("unexpected error opening stub database: %v", err)
	}
	defer db.Close()

	// First ExpectQuery: “author” table does not exist → zero rows
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT column_name, udt_name
             FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = $1`,
	)).WithArgs("author").WillReturnRows(sqlmock.NewRows([]string{"column_name", "udt_name"}))

	// Second ExpectQuery: “book” table has two existing columns: id, title
	bookRows := sqlmock.NewRows([]string{"column_name", "udt_name"}).
		AddRow("id", "UUID").
		AddRow("title", "TEXT")
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT column_name, udt_name
             FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = $1`,
	)).WithArgs("book").WillReturnRows(bookRows)

	// 2) Create a temp directory
	tmpDir, err := ioutil.TempDir("", "torm-stubs-alter")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write schema.prisma with Author+Book (Book has “pages” field)
	schemaPath := filepath.Join(tmpDir, "schema.prisma")
	if err := ioutil.WriteFile(schemaPath, []byte(minimalPrismaSchema_Alter), 0644); err != nil {
		t.Fatalf("failed to write schema.prisma: %v", err)
	}

	// Create migrations directory
	migrationsDir := filepath.Join(tmpDir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}

	// Create a dummy up file so that Book is considered existing
	dummyUp := filepath.Join(migrationsDir, "0001_Book.up.sql")
	ioutil.WriteFile(dummyUp, []byte(""), 0644)
	// Also create a dummy down file for Book
	dummyDown := filepath.Join(migrationsDir, "0001_Book.down.sql")
	ioutil.WriteFile(dummyDown, []byte(""), 0644)

	// 3) Run EnsureStubs
	if err := EnsureStubs(db, schemaPath, migrationsDir); err != nil {
		t.Fatalf("EnsureStubs error: %v", err)
	}

	// 4) Expect four files:
	//    0001_Author.up.sql   # CREATE TABLE author ...
	//    0001_Author.down.sql # DROP TABLE author
	//    0002_Book.up.sql     # ALTER TABLE book ADD COLUMN pages
	//    0002_Book.down.sql   # DROP COLUMN or type revert
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("failed to read migrations dir: %v", err)
	}
	if len(files) != 6 {
		t.Fatalf("expected 6 stub files, got %d", len(files))
	}

	// 5) Verify correctness (skip the dummy 0001_Book.up.sql)
	authorUpRe := regexp.MustCompile(`^\d{4}_Author\.up\.sql$`)
	bookUpRe := regexp.MustCompile(`^[0-9]{4}_Book\.up\.sql$`)
	foundAuthorUp := false
	foundBookUp := false

	for _, f := range files {
		name := f.Name()

		// Skip the dummy stub at version 0001
		if name == "0001_Book.up.sql" {
			continue
		}

		if authorUpRe.MatchString(name) {
			foundAuthorUp = true
			contents, _ := ioutil.ReadFile(filepath.Join(migrationsDir, name))
			if !strings.Contains(string(contents), "CREATE TABLE author") {
				t.Errorf("Author up stub missing CREATE TABLE author, got:\n%s", string(contents))
			}
		}
		// Only consider Book stubs at version > 0001
		if bookUpRe.MatchString(name) && !strings.HasPrefix(name, "0001_") {
			foundBookUp = true
			contents, _ := ioutil.ReadFile(filepath.Join(migrationsDir, name))
			if !strings.Contains(string(contents), "ALTER TABLE book ADD COLUMN pages") {
				t.Errorf("Book up stub missing ALTER ADD COLUMN pages, got:\n%s", string(contents))
			}
		}
	}

	if !foundAuthorUp {
		t.Errorf("missing Author up stub file")
	}
	if !foundBookUp {
		t.Errorf("missing Book up stub file")
	}

	// 6) Validate all sqlmock expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled SQL mock expectations: %s", err)
	}
}
