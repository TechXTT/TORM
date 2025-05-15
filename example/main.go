package main

import (
	"fmt"
	"os"
	"time"

	"github.com/TechXTT/TORM/internal/runtime"
	"github.com/google/uuid"
)

func main() {
	// 1) Connect to the database
	dsn := os.Getenv("DATABASE_URL")
	db, err := runtime.Connect(dsn)
	if err != nil {
		panic(fmt.Errorf("connect: %w", err))
	}

	// 2) Run migrations in "migrations/" (dev mode)
	mgr, err := runtime.NewManager(db, "../migrations")
	if err != nil {
		panic(fmt.Errorf("load migrations: %w", err))
	}
	if err := mgr.Up(); err != nil {
		panic(fmt.Errorf("migrate up: %w", err))
	}
	fmt.Println("✅ Migrations applied")

	// 3) Create a new user via raw SQL
	id := uuid.New()
	now := time.Now()
	_, err = db.Exec(
		`INSERT INTO users
           (id, first_name, last_name, email, password, created_at, updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, "Alice", "Smith", "alice@example.com", "hunter2", now, now,
	)
	if err != nil {
		panic(fmt.Errorf("insert user: %w", err))
	}
	fmt.Printf("✅ Created user %s\n", id)

	// 4) Query it back
	var (
		firstName, lastName, email, password string
		createdAt, updatedAt                 time.Time
		fetchedID                            uuid.UUID
	)
	row := db.QueryRow(
		`SELECT id, first_name, last_name, email, password, created_at, updated_at
         FROM users WHERE email = $1`,
		"alice@example.com",
	)
	if err := row.Scan(&fetchedID, &firstName, &lastName, &email, &password, &createdAt, &updatedAt); err != nil {
		panic(fmt.Errorf("fetch user: %w", err))
	}
	fmt.Printf("✅ Fetched user: %s %s %s (created %s)\n",
		fetchedID, firstName, lastName, createdAt.Format(time.RFC3339),
	)
}
