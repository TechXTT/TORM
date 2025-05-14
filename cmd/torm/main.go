// cmd/torm/main.go
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/TechXTT/TORM/pkg/migrate"
	_ "github.com/lib/pq"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN")
	dir := flag.String("dir", "migrations", "migrations directory")
	action := flag.String("action", "up", "migration action: up or down")
	flag.Parse()
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
		fmt.Fprintf(os.Stderr, "unknown action %q\n", *action)
		os.Exit(1)
	}

	fmt.Println("Done.")
}
