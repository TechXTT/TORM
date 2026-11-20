package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationFilesMatchHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0001_initial.up.sql")
	if err := os.WriteFile(path, []byte("CREATE TABLE sample (id int);\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := loadMigrations(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("load: %v, %v", files, err)
	}
	if err := os.WriteFile(path, []byte("CREATE TABLE sample (id int);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unix, err := loadMigrations(dir)
	if err != nil || unix[0].Checksum != files[0].Checksum {
		t.Fatalf("line ending changes must preserve checksums: %v, %v", unix, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001_conflict.up.sql"), []byte(unix[0].Up), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMigrations(dir); err == nil || !strings.Contains(err.Error(), "conflicting names") {
		t.Fatalf("duplicate version returned %v", err)
	}
}

func TestMigrationTransactionBoundaries(t *testing.T) {
	for _, input := range []string{
		"SELECT 'COMMIT;'; -- ROLLBACK\nSELECT 1;",
		"/* nested /* SET */ ignored */ DO $body$ BEGIN PERFORM 1; END $body$;",
	} {
		if err := validateSQL(input); err != nil {
			t.Errorf("valid migration %s: %v", input, err)
		}
	}
	for _, input := range []string{"BEGIN; SELECT 1;", "SELECT 1; /* comment */ COMMIT;", "ROLLBACK;", "SET search_path TO other;"} {
		if err := validateSQL(input); err == nil {
			t.Errorf("accepted session control in %s", input)
		}
	}
}
