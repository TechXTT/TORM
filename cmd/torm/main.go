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
