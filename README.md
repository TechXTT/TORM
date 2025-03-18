TORM-Project - v0.0.7-alpha

TORM is a lightweight database management package written in Go. It provides an easy-to-use interface for interacting with PostgreSQL databases, including automatic table creation and basic query operations.

Features
	•	Database Connection Management: Easily connect to a PostgreSQL database.
	•	Auto Migration: Automatically creates tables based on Go structs.
	•	Select Queries: Fetch all rows from a table into a slice of structs.
	•	Unit Testing: Uses sqlmock for database unit tests.

Installation

To use TORM, you need Go installed. You can install the package using:
```bash
go get github.com/TechXTT/TORM
```
Setup
	1.	Ensure you have PostgreSQL installed and running.
	2.	Update the dataSourceName in your Go application:
```go
db, err := db.NewDB("postgres://user:password@localhost/dbname?sslmode=disable")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```
Usage

Initializing the Database
```go
package main

import (
    "fmt"
    "log"

    "github.com/TechXTT/TORM"
)

func main() {
    // Connect to the database
    db, err := db.NewDB("postgres://user:password@localhost/dbname?sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    fmt.Println("Database connection established!")
}
```
Auto Migration

To automatically create a table for a struct:
```go
type User struct {
    ID       int
    Username string
}

err = db.AutoMigrate(&User{})
if err != nil {
    log.Fatal(err)
}
fmt.Println("Table migrated successfully!")
```
Selecting Data
```go
var users []User
err = db.Select(&users)
if err != nil {
    log.Fatal(err)
}

for _, user := range users {
    fmt.Printf("User: %d - %s\n", user.ID, user.Username)
}
```
Testing

To run unit tests using sqlmock:
```bash
go test ./...
```
The tests include:
	•	Database connection (TestNewDB)
	•	Data retrieval (TestSelect)
	•	Table creation (TestAutoMigrate)

License

This project is licensed under the MIT License.

This README provides a structured guide for users and developers working on the project. Let me know if you need any modifications! 🚀