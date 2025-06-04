

# TORM

[![Go Reference](https://pkg.go.dev/badge/github.com/TechXTT/TORM.svg)](https://pkg.go.dev/github.com/TechXTT/TORM)
[![Release](https://img.shields.io/github/v/release/TechXTT/TORM?label=version)](https://github.com/TechXTT/TORM/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/TechXTT/TORM)](https://goreportcard.com/report/github.com/TechXTT/TORM)

TORM is a lightweight Go ORM inspired by Prisma, providing schema-driven code generation, type-safe query builders, and automated migrations. It leverages a Prisma-style schema file (`schema.prisma`) as a single source of truth for database models and relationships. TORM generates Go structs, a `Client` for database operations, and SQL migration files automatically.

**Current Version:** v1.0.0

---

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Project Structure](#project-structure)
- [Getting Started](#getting-started)
  - [Prisma Schema](#prisma-schema)
  - [Generating Models and Client](#generating-models-and-client)
  - [Database Migrations](#database-migrations)
  - [Using the Generated Client](#using-the-generated-client)
- [Example](#example)
- [Commands Reference](#commands-reference)
- [Configuration](#configuration)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Schema-driven**  
  Define your models and relationships in a `schema.prisma` file. TORM parses the schema and generates Go code accordingly.

- **Code Generation**  
  Generates Go structs for each model, a `Client` to connect to the database, and per-model service methods (CRUD, filtering, relations, aggregates, etc.).

- **Type-Safe Query Builder**  
  Query methods accept Go maps or typed values to construct SQL queries safely.

- **Automated Migrations**  
  - `migrate deploy`: Apply pending migrations to the database.  
  - `migrate dev`: Generate new migration files from schema diffs, apply them, and update models.  
  - `migrate reset`: Rollback all migrations and reapply from scratch.  
  - `migrate status`: Show current migration status.

- **Many-to-Many Relations Support**  
  Automatically creates connector tables for m2m relations and generates appropriate Go fields and service methods.

- **Enum Support**  
  Maps Prisma `enum` definitions to Go `type` and `const` declarations.

- **UUID & Auto-Increment**  
  Supports `@db.Uuid()` for Postgres UUID defaults and `@default(autoincrement())` for integer primary keys.

- **Zero-value Handling**  
  Automatically treats `NULL` values for `time.Time`, pointers, and optional fields, returning Go zero values instead of panics.

---

## Prerequisites

- Go 1.23 or higher  
- PostgreSQL (or compatible) 12 or higher
- A valid `schema.prisma` file

---

## Installation

1. Clone the repository:

   ```bash
   go install github.com/TechXTT/TORM/cmd/torm@latest
   ```  

---

## Project Structure

```
TORM/
├── cmd/                      # CLI entrypoint
│   └── torm/                 
│       └── main.go           # `torm` command: migrate & codegen
├── example/                  # Example application
│   ├── migrations/           # Pre-generated migration SQL files
│   ├── prisma/               # Prisma schema for the example
│   │   └── schema.prisma     
│   ├── models/               # Generated Go models & client
│   ├── .env                  # Environment variables (e.g., DATABASE_URL)
│   └── main.go               # Example usage of generated client
├── pkg/                      
│   ├── config/               # DSN extraction logic
│   │   └── dsn.go            
│   ├── torm/                 # Public API (user‐facing types & functions)
│   │   └── torm.go           # High‐level orchestration (migrate & codegen)
│   ├── runtime/              # Core runtime: connector, session, migrations
│   │   ├── connector.go      
│   │   ├── session.go        
│   │   └── migrate.go        
│   └── cli/                  # Cobra CLI definitions
│       ├── root.go           
│       └── migrate.go        
├── pkg/internal/             
│   ├── typeconv/             # Type‐conversion utilities
│   │   └── types.go          
│   ├── generator/            # Schema parser & code generator
│   │   ├── parser.go         
│   │   └── generator.go      
│   └── migrate/              # Migration stub logic
│       └── stubs.go          
├── go.mod                    
├── go.sum                    
├── README.md                 
└── .gitignore               
```

---

## Getting Started

### Prisma Schema

TORM expects a Prisma-style schema file at `prisma/schema.prisma` (default path). Example:

```prisma
datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

generator torm {
  provider = "torm"
}

model User {
  id        Int       @id @default(autoincrement())
  email     String    @unique
  name      String?
  posts     Post[]
}

model Post {
  id        Int       @id @default(autoincrement())
  title     String
  content   String?
  author    User?     @relation(fields: [authorId], references: [id])
  authorId  Int?
  tags      Tag[]     @relation("PostTags", references: [id])
}

model Tag {
  id    Int     @id @default(autoincrement())
  name  String  @unique
  posts Post[]  @relation("PostTags", references: [id])
}

enum ProjectType {
  personal
  team
  enterprise
}
```

- **Many-to-Many**  
  In the example above, `Post` and `Tag` have a many-to-many relation. TORM will generate a `post_tags` join table (or customized join table name).

### Generating Models and Client

Run the migration and code generation commands to create Go models and a client for your database schema.

```bash
torm migrate dev 
```

This will:

- Parse `schema.prisma` (default path or specified with `--schema`)
- Generate Go structs (`models/*.go`)  
- Generate a `client.go` with a `Client` struct and per-model services  
- Run `go mod tidy` in the output directory

### Database Migrations

TORM automatically generates SQL migration files by comparing your current `schema.prisma` with the latest applied schema.

- **Initialize Migrations Directory**  
  On first run, TORM will create `migrations/0001_<Model>.up.sql` and `0001_<Model>.down.sql` for each model.

- **Develop Workflow**  
  ```bash
  torm migrate dev \
    --schema prisma/schema.prisma \
    --dir migrations
  ```
  - Compares the current Prisma schema to the last migration.  
  - If differences exist, generates new `NNNN_Model.up.sql` and `NNNN_Model.down.sql` stubs.  
  - Applies the new migration.  
  - Regenerates models and client.

- **Deploy Workflow**  
  ```bash
  torm migrate deploy \
    --schema prisma/schema.prisma \
    --dir migrations
  ```
  - Applies all pending migrations in order.

- **Reset All Migrations**  
  ```bash
  torm migrate reset \
    --schema prisma/schema.prisma \
    --dir migrations
  ```
  - Rolls back all migrations (in reverse order)  
  - Reapplies from the first migration  
  - Regenerates models and client

- **Migration Status**  
  ```bash
  torm migrate status \
    --schema prisma/schema.prisma \
    --dir migrations
  ```

### Using the Generated Client

In your Go application:

```go
package main

import (
    "context"
    "fmt"
    "github.com/TechXTT/TORM/internal/model" // adjust import path
)

func main() {
    // Instantiate the generated client
    client := model.NewClient()

    // Example: Create a new User
    ctx := context.Background()
    newUser := &model.User{
        Email: "alice@example.com",
        Name:  "Alice",
    }
    err := client.UserService().Create(ctx, newUser)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Created user with ID: %d\n", newUser.ID)

    // Example: Fetch posts with related tags (many-to-many)
    posts, err := client.PostService().FindMany(ctx, map[string]interface{}{"authorId": newUser.ID}, nil, 0, 10)
    if err != nil {
        panic(err)
    }
    for _, post := range posts {
        fmt.Printf("Post: %s, Tags: %+v\n", post.Title, post.Tags)
    }
}
```

- **Per-Model Service Methods**  
  - `FindUnique(ctx, where map[string]interface{}) (*Model, error)`  
  - `FindFirst(ctx, where, orderBy []string, skip, take int) ([]*Model, error)`  
  - `FindMany(ctx, where, orderBy []string, skip, take int) ([]*Model, error)`  
  - `Create(ctx, data *Model) error`  
  - `Update(ctx, where, data *Model) error`  
  - `Upsert(ctx, where, createData, updateData *Model) error`  
  - `Delete(ctx, where map[string]interface{}) error`  
  - `Count(ctx, where map[string]interface{}) (int64, error)`  
  - `CreateMany(ctx, data []*Model) (int64, error)`  
  - `UpdateMany(ctx, where map[string]interface{}, data map[string]interface{}) (int64, error)`  
  - `DeleteMany(ctx, where map[string]interface{}) (int64, error)`  
  - `Aggregate(ctx, where map[string]interface{}, agg map[string][]string) (map[string]interface{}, error)`  
  - `GroupBy(ctx, by []string, where map[string]interface{}, agg map[string][]string) ([]map[string]interface{}, error)`

---

## Example

Assume you have defined `Project` and `Creator` models in your Prisma schema:

```prisma
model Project {
  id          Int        @id @default(autoincrement())
  name        String
  description String?
  creators    Creator[]  @relation("CreatorProjects")
}

model Creator {
  id       Int       @id @default(autoincrement())
  name     String
  projects Project[] @relation("CreatorProjects")
}
```

1. **Generate code and migrations:**
   ```
   torm migrate dev
   ```

2. **Use in application:**  
   ```go
   client := model.NewClient()
   ctx := context.Background()

   // Create a creator
   creator := &model.Creator{Name: "Bob"}
   client.CreatorService().Create(ctx, creator)

   // Create a project and link to creator
   project := &model.Project{Name: "TORM Demo", Description: "Demonstration of TORM"}
   project.Creators = []*model.Creator{creator}
   client.ProjectService().Create(ctx, project)

   // Fetch project with creators (many-to-many)
   proj, _ := client.ProjectService().FindUnique(ctx, map[string]interface{}{"id": project.ID})
   fmt.Println(proj.Creators) 
   ```

---

## Commands Reference

- **Generate Models and Client**  
  ```
  torm migrate dev \
       --schema <schema.prisma> \
       --dir <migrations_dir>
  ```

- **Migrations**  
  ```
  torm migrate [dev|deploy|reset|status] \
       --schema <schema.prisma> \
       --dir <migrations_dir>
  ```

- **Help**  
  ```
  torm --help
  torm migrate --help
  ```

---

## Configuration

- **Prisma Schema File**  
  By default, TORM looks for `prisma/schema.prisma`. Override with `--schema` flag.

- **Output Directory**  
  By default, TORM generates code in `models/`.

- **Environment Variables**  
  - `DATABASE_URL`: Database connection string (Postgres).  
  - If `sslmode` is not specified, TORM automatically appends `sslmode=disable` for local development.

---

## Contributing

Contributions are welcome! Please:

1. Fork the repository  
2. Create a feature branch (`git checkout -b feature/xyz`)  
3. Commit your changes (`git commit -m "[ADD] feature"`)  
4. Push to your branch (`git push origin feature/xyz`)  
5. Open a Pull Request

Ensure that:

- All new code is covered by tests.  
- `go fmt` and `go mod tidy` pass.  
- Linting (`golangci-lint`) reports no issues.

---

## License

TORM is released under the [MIT License](LICENSE).
