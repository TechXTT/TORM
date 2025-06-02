package generator

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalPrismaSchema is a tiny schema.prisma used for testing
const minimalPrismaSchema = `
datasource db {
  provider = "postgresql"
  url      = "postgres://user:pass@localhost:5432/db?sslmode=disable"
}

model Book {
  id     String   @id @default(uuid()) @db.Uuid
  title  String
  pages  Int
  read   Boolean @default(false)
}
`

func TestGenerate_WritesModelAndClient(t *testing.T) {
	// Create a temporary directory to act as output
	tmpDir, err := ioutil.TempDir("", "torm-gen-test")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a temporary prisma file
	schemaPath := filepath.Join(tmpDir, "schema.prisma")
	if err := ioutil.WriteFile(schemaPath, []byte(minimalPrismaSchema), 0644); err != nil {
		t.Fatalf("failed to write schema.prisma: %v", err)
	}

	// Create a go.mod in the outDir so that go mod tidy can run without error
	goModContent := []byte("module example.com/tormtest\n\ngo 1.21")
	if err := ioutil.WriteFile(filepath.Join(tmpDir, "go.mod"), goModContent, 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Call Generate
	if err := Generate(schemaPath, tmpDir); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Check that book.go (model) exists
	bookPath := filepath.Join(tmpDir, "book.go")
	if _, err := os.Stat(bookPath); os.IsNotExist(err) {
		t.Fatalf("expected model file %s to be created, but it does not exist", bookPath)
	}

	// Check that client.go exists
	clientPath := filepath.Join(tmpDir, "client.go")
	if _, err := os.Stat(clientPath); os.IsNotExist(err) {
		t.Fatalf("expected client file %s to be created, but it does not exist", clientPath)
	}

	// Read the generated Book model and verify that it contains the correct struct name
	bookContents, err := ioutil.ReadFile(bookPath)
	if err != nil {
		t.Fatalf("failed to read book.go: %v", err)
	}
	if !strings.Contains(string(bookContents), "type Book struct") {
		t.Errorf("book.go does not contain expected \"type Book struct\"; got:\n%s", string(bookContents))
	}

	// Verify that UUID import appears in book.go if needed
	if !strings.Contains(string(bookContents), "\"github.com/google/uuid\"") {
		t.Errorf("book.go missing uuid import; got:\n%s", string(bookContents))
	}

	// Verify that required imports and service definitions appear in client.go
	clientContents, err := ioutil.ReadFile(clientPath)
	if err != nil {
		t.Fatalf("failed to read client.go: %v", err)
	}
	if !strings.Contains(string(clientContents), "\"database/sql\"") {
		t.Errorf("client.go missing database/sql import; got:\n%s", string(clientContents))
	}
	if !strings.Contains(string(clientContents), "type BookService") {
		t.Errorf("client.go missing BookService definition; got:\n%s", string(clientContents))
	}
}
