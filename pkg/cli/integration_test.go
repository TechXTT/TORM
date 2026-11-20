package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TechXTT/TORM/v2/pkg/runtime"
	"github.com/lib/pq"
)

func testDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	connection := os.Getenv("TORM_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TORM_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	dsn, err := url.Parse(connection)
	if err != nil || (dsn.Scheme != "postgresql" && dsn.Scheme != "postgres") {
		t.Fatal("TORM_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	admin, err := runtime.Connect(connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var random [8]byte

	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "torm_test_" + hex.EncodeToString(random[:])
	if _, err := admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + pq.QuoteIdentifier(name)); err != nil {
			t.Errorf("remove disposable database: %v", err)
		}
	})
	dsn.Path = "/" + name
	db, err := runtime.Connect(dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dsn.String()
}

func command(dsn, shadow, dir string, args ...string) (string, error) {
	var output bytes.Buffer

	root := NewRootCmd()
	root.SilenceErrors = true
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(append(args, "--dsn", dsn, "--shadow-dsn", shadow, "--out-migrations", dir))
	err := root.ExecuteContext(context.Background())
	return output.String(), err
}

func requireCommand(t *testing.T, dsn, shadow, dir string, args ...string) string {
	t.Helper()
	output, err := command(dsn, shadow, dir, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, output)
	}
	return output
}

func exampleDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "example"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func checkTable(t *testing.T, db *sql.DB, table string, expected bool) {
	t.Helper()
	var exists bool

	if err := db.QueryRow("SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil || exists != expected {
		t.Fatalf("expected table %s to exist=%t, got %t (%v)", table, expected, exists, err)
	}
}

func TestMigrationLifecycle(t *testing.T) {
	db, dsn := testDatabase(t)
	_, shadow := testDatabase(t)
	dir := t.TempDir()
	example := exampleDirectory(t)
	schema := filepath.Join(example, "prisma", "schema.prisma")
	requireCommand(t, dsn, shadow, dir, "migrate", "status")
	checkTable(t, db, "schema_migrations", false)
	if _, err := command(dsn, dsn, dir, "migrate", "plan", "--schema", schema); err == nil || !strings.Contains(err.Error(), "different database name") {
		t.Fatalf("expected the runner to reject a shared target and shadow database: %v", err)
	}
	requireCommand(t, dsn, shadow, dir, "migrate", "plan", "--schema", schema, "--name", "initial")
	checkTable(t, db, "creator", false)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := command(dsn, shadow, dir, "migrate", "deploy")
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if output := requireCommand(t, dsn, shadow, dir, "migrate", "plan", "--schema", schema); !strings.Contains(output, "No schema changes") {
		t.Fatalf("first repeat plan: %s", output)
	}
	client := exec.Command("go", "test", "-count=1", "./...")
	client.Dir = example
	client.Env = append(os.Environ(), "TORM_EXAMPLE_DATABASE_URL="+dsn)
	if output, err := client.CombinedOutput(); err != nil {
		t.Fatalf("example: %v\n%s", err, output)
	}
	input, err := os.ReadFile(schema)
	if err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(t.TempDir(), "changed.prisma")
	doomed := strings.Replace(string(input), `  firstName String?  @map("firstname")`, "", 1)
	if err := os.WriteFile(changed, []byte(doomed), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := command(dsn, shadow, dir, "migrate", "plan", "--schema", changed, "--name", "remove")
	if err == nil || !strings.Contains(output, "drop column creator.firstname") {
		t.Fatalf("expected a data review error, got %v\n%s", err, output)
	}
	if _, err := db.Exec("ALTER TABLE post ADD COLUMN drift text"); err != nil {
		t.Fatal(err)
	}
	_, err = command(dsn, shadow, dir, "migrate", "plan", "--schema", schema)
	if err == nil || !strings.Contains(err.Error(), "schema drift") {
		t.Fatalf("expected schema drift, got %v", err)
	}
	if _, err := db.Exec("ALTER TABLE post DROP COLUMN drift"); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "0002_reviewed.up.sql")
	if err := os.WriteFile(second, []byte("CREATE TABLE atomic_probe (id int); SELECT torm_nonexistent_function();"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = command(dsn, shadow, dir, "migrate", "deploy")
	if err == nil || !strings.Contains(err.Error(), "apply migration 0002_reviewed") {
		t.Fatalf("expected migration failure, got %v", err)
	}
	checkTable(t, db, "atomic_probe", false)
	if err := os.WriteFile(second, []byte("CREATE TABLE atomic_probe (id int);"), 0o644); err != nil {
		t.Fatal(err)
	}
	requireCommand(t, dsn, shadow, dir, "migrate", "deploy")
	checkTable(t, db, "atomic_probe", true)
	if _, err := command(dsn, shadow, dir, "migrate", "reset"); err == nil {
		t.Fatal("reset must require confirmation through --force")
	}
	requireCommand(t, dsn, shadow, dir, "migrate", "reset", "--force")
	checkTable(t, db, "atomic_probe", true)
	var count int

	if err := db.QueryRow("SELECT COUNT(*) FROM creator").Scan(&count); err != nil || count != 0 {
		t.Fatalf("reset must discard application rows: %d, %v", count, err)
	}
	if err := os.WriteFile(second, []byte("CREATE TABLE atomic_probe (id text);"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = command(dsn, shadow, dir, "migrate", "deploy")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestUpgradeReleasedHistory(t *testing.T) {
	db, dsn := testDatabase(t)
	shadowDB, shadow := testDatabase(t)
	example := exampleDirectory(t)
	dir := filepath.Join(example, "migrations")
	for _, connection := range []*sql.DB{db, shadowDB} {
		if _, err := connection.Exec(`CREATE EXTENSION "uuid-ossp"`); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"0001_Creator.up.sql", "0002_Post.up.sql"} {
		content, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations(version integer PRIMARY KEY);
		INSERT INTO schema_migrations VALUES (1), (2);
		INSERT INTO creator (id, firstname, email) VALUES ('0dbb9ed7-6a6f-4a46-ac8e-c362a02f5ecd', 'Ada', 'ada@example.test');
		INSERT INTO post (title, content, authorid) VALUES ('Original post', '', '0dbb9ed7-6a6f-4a46-ac8e-c362a02f5ecd');`); err != nil {
		t.Fatal(err)
	}
	_, err := command(dsn, shadow, dir, "migrate", "deploy")
	if err == nil || !strings.Contains(err.Error(), "run migrate baseline") {
		t.Fatalf("expected baseline requirement, got %v", err)
	}
	requireCommand(t, dsn, shadow, dir, "migrate", "baseline")
	requireCommand(t, dsn, shadow, dir, "migrate", "deploy")
	var author string
	var title string

	if err := db.QueryRow("SELECT creator.firstname, post.title FROM creator JOIN post ON creator.id = post.authorid").Scan(&author, &title); err != nil {
		t.Fatal(err)
	}
	if author != "Ada" || title != "Original post" {
		t.Fatal(fmt.Errorf("legacy rows changed: %s, %s", author, title))
	}
	output := requireCommand(t, dsn, shadow, dir, "migrate", "plan", "--schema", filepath.Join(example, "prisma", "schema.prisma"))
	if !strings.Contains(output, "No schema changes") {
		t.Fatalf("upgraded database differs: %s", output)
	}
}

func TestGeneratedSchemaWithRelations(t *testing.T) {
	_, dsn := testDatabase(t)
	_, shadow := testDatabase(t)
	root := filepath.Dir(exampleDirectory(t))
	dir := t.TempDir()
	schema := filepath.Join(root, "pkg", "internal", "schema", "testdata", "postgres")
	migrations := filepath.Join(dir, "migrations")
	requireCommand(t, dsn, shadow, migrations, "migrate", "dev", "--schema", schema, "--out-models", filepath.Join(dir, "models"))
	requireCommand(t, dsn, shadow, migrations, "migrate", "dev", "--schema", schema, "--out-models", filepath.Join(dir, "models"))
	files, err := os.ReadDir(migrations)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one migration, got %v (%v)", files, err)
	}
	source, err := os.ReadFile(filepath.Join("testdata", "client_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module example.test/generated\n\ngo 1.23\n\nrequire (\n github.com/TechXTT/TORM/v2 v2.0.0\n github.com/google/uuid v1.6.0\n)\nreplace github.com/TechXTT/TORM/v2 => " + strconv.Quote(filepath.ToSlash(root)) + "\n"
	for name, content := range map[string][]byte{"client_test.go": source, "go.mod": []byte(mod)} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	test := exec.Command("go", "test", "-mod=mod", "-count=1", "./...")
	test.Dir = dir
	test.Env = append(os.Environ(), "TORM_EXAMPLE_DATABASE_URL="+dsn)
	if output, err := test.CombinedOutput(); err != nil {
		t.Fatalf("generated schema client: %v\n%s", err, output)
	}
}
