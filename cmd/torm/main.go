package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/TechXTT/TORM/internal/dsl"
	"github.com/TechXTT/TORM/pkg/migrate"
	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: torm <command> [options]")
		fmt.Println("Commands: migrate, codegen")
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ExitOnError)
		dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN")
		dir := fs.String("dir", "migrations", "migrations directory")
		action := fs.String("action", "up", "migration action: up or down")
		fs.Parse(os.Args[2:])

		db, err := sql.Open("postgres", *dsn)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		defer db.Close()

		mgr, err := migrate.NewManager(db, *dir)
		if err != nil {
			log.Fatalf("load migrations: %v", err)
		}

		switch *action {
		case "up":
			if err := mgr.Up(); err != nil {
				log.Fatalf("migrate up failed: %v", err)
			}
		case "down":
			if err := mgr.Down(); err != nil {
				log.Fatalf("migrate down failed: %v", err)
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown migrate action %q\n", *action)
			os.Exit(1)
		}
		fmt.Println("Migration complete.")

	case "codegen":
		fs := flag.NewFlagSet("codegen", flag.ExitOnError)
		schemaDir := fs.String("schema-dir", "schema_defs", "schema definitions directory")
		outDir := fs.String("out", "pkg/schema", "output directory for generated code")
		fs.Parse(os.Args[2:])

		entries, err := ioutil.ReadDir(*schemaDir)
		if err != nil {
			log.Fatalf("read schema dir: %v", err)
		}

		var allEntities []dsl.Entity
		for _, fi := range entries {
			if fi.IsDir() || filepath.Ext(fi.Name()) != ".schema" {
				continue
			}
			data, err := ioutil.ReadFile(filepath.Join(*schemaDir, fi.Name()))
			if err != nil {
				log.Fatalf("read schema file %s: %v", fi.Name(), err)
			}
			ast, err := dsl.ParseSchema(data)
			if err != nil {
				log.Fatalf("parse schema %s: %v", fi.Name(), err)
			}
			allEntities = append(allEntities, ast.Entities...)
		}

		generator, err := dsl.NewGenerator()
		if err != nil {
			log.Fatalf("initialize code generator: %v", err)
		}
		ast := dsl.AST{Entities: allEntities}
		if err := generator.Generate(ast, *outDir); err != nil {
			log.Fatalf("code generation failed: %v", err)
		}
		fmt.Println("Code generation complete.")

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(1)
	}
}
