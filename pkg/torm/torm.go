package torm

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/TechXTT/TORM/pkg/config"
	"github.com/TechXTT/TORM/pkg/internal/generator"
	"github.com/TechXTT/TORM/pkg/internal/migrate"
	"github.com/TechXTT/TORM/pkg/runtime"
)

func RunMigrate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: torm migrate <dev|deploy|reset|status> [--dsn <dsn>] [--dir <migrations-dir>] [--schema <schema>]")
		os.Exit(1)
	}
	sub := args[0]
	fs := flag.NewFlagSet("migrate "+sub, flag.ExitOnError)

	// Define flags for migrate command
	dir := fs.String("dir", "./migrations", "migrations directory")
	schemaFile := fs.String("schema", "prisma/schema.prisma", "path to Prisma schema file")

	// Parse command-line args for migrate
	fs.Parse(args[1:])

	configDSN, err := config.Load(*schemaFile)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := sql.Open("postgres", configDSN.DSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	mgr, err := runtime.NewManager(configDSN.DSN, *dir)
	if err != nil {
		log.Fatalf("new manager: %v", err)
	}

	switch sub {
	case "dev":
		// Generate SQL stubs for any new models in the schema
		if err := migrate.EnsureStubs(db, *schemaFile, *dir); err != nil {
			log.Fatalf("ensure stubs failed: %v", err)
		}

		if err := mgr.Dev(); err != nil {
			log.Fatalf("dev: %v", err)
		}

		// Regenerate Go models for updated schema
		generator.Generate(*schemaFile, "models") // replace with actual model output dir

	case "deploy":
		if err := mgr.Deploy(); err != nil {
			log.Fatalf("deploy: %v", err)
		}
	case "reset":
		if err := mgr.Reset(); err != nil {
			log.Fatalf("reset: %v", err)
		}
	case "status":
		status, err := mgr.Status()
		if err != nil {
			log.Fatalf("status: %v", err)
		}
		fmt.Println(status)
	default:
		fmt.Fprintf(os.Stderr, "Unknown migrate action %q\n", sub)
		os.Exit(1)
	}
}
