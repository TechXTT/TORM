package main

import (
	"fmt"
	"os"

	"github.com/TechXTT/TORM/pkg/torm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: torm <command> [options]")
		fmt.Println("Commands: migrate, db, generate, studio")
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "version":
		fmt.Println("TORM version 0.4.1")
	case "help":
		fmt.Println("Usage: torm <command> [options]")
		fmt.Println("Commands: migrate, codegen")
		fmt.Println("Options for migrate:")
		fmt.Println("  -dsn string")
		fmt.Println("        Postgres DSN (default $DATABASE_URL)")
		fmt.Println("  -dir string")
		fmt.Println("        migrations directory (default \"migrations\")")
		fmt.Println("  -action string")
		fmt.Println("        migration action: up or down (default \"up\")")
		fmt.Println("Options for codegen:")
		fmt.Println("  -schema-dir string")
		fmt.Println("        schema definitions directory (default \"schema_defs\")")
		fmt.Println("  -out string")
		fmt.Println("        output directory for generated code (default \"pkg/schema\")")
		fmt.Println("Options for version:")
		fmt.Println("  -version")
		fmt.Println("        print version information")
		fmt.Println("Options for help:")
		fmt.Println("  -help")
		fmt.Println("        print help information")
	case "migrate":
		torm.RunMigrate(os.Args[2:])
	case "db":
		torm.RunDB(os.Args[2:])
	case "generate":
		torm.RunGenerate(os.Args[2:])
	case "studio":
		torm.RunStudio()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		os.Exit(1)
	}
}
