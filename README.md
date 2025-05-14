# TORM - v0.2.1-beta

TORM is a modern, lightweight ORM for Go, designed for PostgreSQL with a focus on type-safety, migrations, and extensibility.

## Features

- **Database Connection**: Connect to PostgreSQL using `torm.Open`.
- **Migrations CLI**: Manage schema changes via the `torm` binary (`migrate up/down`).
- **Schema DSL & Code Generation**: Define schemas in Go and generate models & migrations.
- **Type-Safe Query Builder**: Use generics for compile-time-safe queries (`Query[T]`).
- **Plugin & Hook System**: Customize behavior with lifecycle hooks and middleware.
- **Context & Tracing**: Full `context.Context` support and optional OpenTelemetry integration.
- **Unit Testing**: Easily mock DB interactions with `sqlmock`.

## Installation

Install the library and CLI:

```bash
go get github.com/TechXTT/TORM
go install github.com/TechXTT/TORM/cmd/torm@latest
```

## CLI Usage

```bash
# Apply all pending migrations
torm migrate --dsn "postgres://user:pass@localhost/db?sslmode=disable" --dir ./migrations up

# Roll back the latest migration
torm migrate --dsn ... down

# Generate schema code from DSL files
torm codegen --schema-dir schema_defs --out ./pkg/schema
```

## Getting Started

### 1. Define Your Schema (DSL)

Create a DSL file:

```go
// schema_defs/user.schema
entity.User().
    Field("ID", "int").
    Field("Name", "string").NotNull().
    Field("Email", "string").NotNull().
    Field("CreatedAt", "time.Time").Default("now()")
```

Generate models:

```bash
torm codegen --schema-dir schema_defs --out pkg/schema
```

### 2. Write Migrations

Add SQL files under `migrations/`:

```
migrations/
  0001_create_users.up.sql
  0001_create_users.down.sql
```

### 3. Run Migrations

```bash
torm migrate --dsn "$DATABASE_URL" --dir migrations up
```

### 4. Use TORM in Go

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/TechXTT/TORM/pkg/torm"
    "example/pkg/schema"
)

func main() {
    dsn := "postgres://user:pass@localhost/db?sslmode=disable"
    db, err := torm.Open(dsn)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Insert a user
    _, err = db.Exec(context.Background(),
        "INSERT INTO users(name, email) VALUES($1, $2)",
        "Alice", "alice@example.com",
    )
    if err != nil {
        log.Fatal(err)
    }

    // Query users
    users, err := db.Query[schema.User]().
        From(schema.UserTable).
        Select(schema.User{}.Fields()...).
        Where("email = $1", "alice@example.com").
        All(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    for _, u := range users {
        fmt.Printf("%+v\n", u)
    }
}
```

## Testing

```bash
go test ./...
```

## License

MIT License.