package main

import (
	"fmt"
	"os"

	"github.com/TechXTT/TORM/pkg/torm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: torm <command> [options]")
		fmt.Println("Commands: migrate, db")
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "version":
		fmt.Println("TORM version @0.5.0-alpha")
	case "help":
		fmt.Println("Usage: torm <command> [options]")
		fmt.Println("Commands: migrate, db")
		fmt.Println("Options for each command:")
		fmt.Println("  migrate <dev|deploy|reset|status> [--dir <migrations-dir>]")
		fmt.Println("  db <command> [options]")
		// fmt.Println("  generate <command> [options]")
		// fmt.Println("  studio")
		fmt.Println("  help")
		fmt.Println("  version")
		os.Exit(0)
	case "migrate":
		torm.RunMigrate(os.Args[2:])
	case "db":
		torm.RunDB(os.Args[2:])
	// case "generate":
	// 	torm.RunGenerate(os.Args[2:])
	// case "studio":
	// 	torm.RunStudio()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		os.Exit(1)
	}
}
