package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/TechXTT/TORM/pkg/torm/runtime"
	"github.com/google/uuid"
)

func main() {
	// Load .env file (if using godotenv)
	// eg: godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	db, err := runtime.Connect(dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	// Run migrations
	mgr, err := runtime.NewManager(db, "prisma/migrations")
	if err != nil {
		log.Fatalf("load migrations: %v", err)
	}
	if err := mgr.Up(); err != nil {
		log.Fatalf("migrate up: %v", err)
	}
	fmt.Println("✅ Migrations applied")

	// Initialize Prisma client
	client := prisma.NewClient()
	if err := client.Prisma.Connect(); err != nil {
		log.Fatalf("prisma connect: %v", err)
	}
	defer client.Prisma.Disconnect()

	// Create a user
	now := time.Now()
	u, err := client.User.CreateOne(
		prisma.User.ID.Set(uuid.New()),
		prisma.User.FirstName.Set("Alice"),
		prisma.User.LastName.Set("Smith"),
		prisma.User.Email.Set("alice@example.com"),
		prisma.User.Password.Set("hunter2"),
		prisma.User.CreatedAt.Set(now),
		prisma.User.UpdatedAt.Set(now),
	).Exec(context.Background())
	if err != nil {
		log.Fatalf("create user: %v", err)
	}
	fmt.Printf("✅ Created user: %s %s %s\n", u.ID, u.FirstName, u.LastName)

	// Fetch the same user
	fetched, err := client.User.FindUnique(
		prisma.User.Email.Equals("alice@example.com"),
	).Exec(context.Background())
	if err != nil {
		log.Fatalf("fetch user: %v", err)
	}
	fmt.Printf("✅ Fetched user by email: %s %s %s\n", fetched.ID, fetched.FirstName, fetched.LastName)
}
