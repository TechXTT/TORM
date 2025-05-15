package dsl_test

import (
	"flag"
	"go/format"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TechXTT/TORM/internal/dsl"
	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

func TestGenerate_SimpleEntities(t *testing.T) {
	flag.Parse()
	// 1. Load and parse the schema file
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	schemaPath := filepath.Join(cwd, "test", "user.schema")
	schemaBytes, err := ioutil.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("failed to read schema: %v", err)
	}
	ast, err := dsl.ParseSchema(schemaBytes)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}
	// 2. Run generator
	tmp := t.TempDir()
	gen, err := dsl.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}
	if err := gen.Generate(ast, tmp); err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// 3. Compare against golden
	for _, ent := range ast.Entities {
		file := strings.ToLower(ent.Name) + ".go"
		gotPath := filepath.Join(tmp, file)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get current dir: %v", err)
		}
		wantPath := filepath.Join(cwd, "test", file+".golden")
		t.Logf("Comparing %s with %s", gotPath, wantPath)
		got, err := ioutil.ReadFile(gotPath)
		if err != nil {
			t.Fatalf("read generated %s: %v", file, err)
		}
		// Replace invalid package names (starting with digit) for formatting
		gotStr := string(got)
		parts := strings.SplitN(gotStr, "\n", 2)
		if len(parts) > 0 && strings.HasPrefix(parts[0], "package ") {
			parts[0] = "package test"
			got = []byte(strings.Join(parts, "\n"))
		}
		// Format the generated code using go/format
		formattedGot, err := format.Source(got)
		if err != nil {
			t.Fatalf("failed to format generated code: %v", err)
		}
		got = formattedGot
		// Regenerate golden files if requested
		if *update {
			if err := os.MkdirAll(filepath.Dir(wantPath), 0o755); err != nil {
				t.Fatalf("failed to create testdata dir: %v", err)
			}
			if err := ioutil.WriteFile(wantPath, got, 0o644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}
		}
		want, err := ioutil.ReadFile(wantPath)
		if err != nil {
			t.Fatalf("read golden %s: %v", file+".golden", err)
		}

		// Normalize whitespace per line for comparison
		normalize := func(s string) string {
			lines := strings.Split(s, "\n")
			for i, l := range lines {
				lines[i] = strings.TrimSpace(l)
			}
			return strings.Join(lines, "\n")
		}
		gotNorm := normalize(string(got))
		wantNorm := normalize(string(want))
		if diff := cmp.Diff(wantNorm, gotNorm); diff != "" {
			t.Errorf("output mismatch for %s:\n%s", file, diff)
		}
	}
}
