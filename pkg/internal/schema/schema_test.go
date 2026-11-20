package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePostgreSQLSchema(t *testing.T) {
	parsed, err := Load(filepath.Join("testdata", "postgres"), false)
	if err != nil {
		t.Fatal(err)
	}
	ddl := parsed.DDL()
	for _, expected := range []string{
		`CREATE TYPE "visibility" AS ENUM (E'public', E'private')`,
		`"amounts" numeric(12,2)[] NOT NULL`,
		`"rank" bigint NOT NULL`,
		`"changedAt" timestamp(6) with time zone NOT NULL`,
		`PRIMARY KEY ("tenant", "slug")`,
		`REFERENCES "entries" ("tenant", "slug") ON DELETE CASCADE`,
		`CREATE UNIQUE INDEX "_AuthorTags_AB_unique"`,
		`"contact_address" text NOT NULL`,
	} {
		if !strings.Contains(ddl, expected) {
			t.Errorf("DDL is missing %s\n%s", expected, ddl)
		}
	}
	if strings.Contains(ddl, "uuid_generate") {
		t.Error("uuid() should be generated in the Go client")
	}
}

func TestSchemaDiagnostics(t *testing.T) {
	cases := []struct {
		input   string
		message string
	}{
		{"model Book {\n id Int @id\n pages Missing\n}", "unknown or unsupported type"},
		{"model Book {\n id Int @id\n other Int @db.VarChar(2)\n}", "requires String"},
		{"model Book {\n id Int @id\n @@index([gone])\n}", "unknown, duplicate, or unsupported field"},
		{"model Book {\n id Int?\n}", "requires a primary key"},
		{"model Book {\n id Int @id\n titles String[] @default(uuid())\n}", "list default must be an array"},
		{"model Book {\n id Int @id\n}", ""},
	}
	for _, item := range cases {
		path := filepath.Join(t.TempDir(), "schema.prisma")
		if err := os.WriteFile(path, []byte(item.input), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path, false)
		if item.message == "" {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "schema.prisma:") || !strings.Contains(err.Error(), item.message) {
			t.Fatalf("diagnostic for %q: %v", item.input, err)
		}
	}
}
