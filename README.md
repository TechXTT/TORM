# TORM – v0.5.3-alpha

TORM is a lightweight, idiomatic Go ORM for PostgreSQL, providing:

- **Prisma-based Schema & Code Generation**: Define your schema in Prisma's DSL, generate models and migrations.
- **Introspective Migrations**: Auto-generate `CREATE TABLE` and `ALTER TABLE` stubs by comparing your Prisma schema to the live database.
- **Subcommand-based CLI**: Manage migrations (`dev`, `deploy`, `reset`, `status`) and codegen with a single `torm` binary.
- **Type-Safe Query Builder**: Fluent, generic-driven queries (`db.Query[T]()`).
- **Plugin & Hook System**: Extend lifecycle events, tracing, and middleware.
- **Context & Tracing**: Full `context.Context` support and optional OpenTelemetry integration.
- **Testing Support**: Unit-test your database logic with `sqlmock` and run integration tests against a live Postgres.

---

## Installation

```bash
go get github.com/TechXTT/TORM
go install github.com/TechXTT/TORM/cmd/torm@latest
```

Ensure that `$GOBIN` is on your `PATH` so the `torm` command is available.

---

## CLI Usage

### Migrations

TORM’s migration CLI uses subcommands:

```bash
# Apply and auto-generate new migrations from your Prisma schema (dev database)
torm migrate dev \
  --schema prisma/schema.prisma \
  --dir migrations

# Apply all pending migrations without schema diffs
torm migrate deploy \
  --schema prisma/schema.prisma \
  --dir migrations

# Drop and reapply all migrations
torm migrate reset \
  --schema prisma/schema.prisma \
  --dir migrations

# Show current version and pending/applied status
torm migrate status \
  --schema prisma/schema.prisma \
  --dir migrations
```

- `--schema` (optional): path to your Prisma schema file (default `prisma/schema.prisma`).
- `--dir` (optional): directory holding your `.up.sql`/`.down.sql` scripts (default `migrations/`).

### Code Generation

Generate Go models and client from your Prisma schema definitions:

```bash
torm codegen \
  --schema prisma/schema.prisma \
  --out pkg/schema
```

- `--schema`: path to your Prisma schema file.
- `--out`: target directory for generated Go model and client files.

---

## Defining Your Schema (Prisma)

Place your Prisma schema file under `prisma/schema.prisma`:

```prisma
// prisma/schema.prisma
datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

generator client {
  provider = "torm"
  output   = "../pkg/schema"
}

model Post {
  id        String   @id @default(uuid())
  title     String
  content   String?
  published Boolean  @default(false)
  createdAt DateTime @default(now())
  updatedAt DateTime
}
```

Define your database schema using Prisma's declarative DSL, including models, fields, relations, and enums.

---

## Typical Workflow

1. **Edit** your Prisma schema file `prisma/schema.prisma`.
2. **Run** migrations for your dev database (auto-generates SQL stubs):
   ```bash
   torm migrate dev --schema prisma/schema.prisma --dir migrations
   ```
3. **Inspect** and hand-tweak the generated `migrations/000X_*.up.sql` and `.down.sql` files if needed.
4. **Regenerate** Go models and client:
   ```bash
   torm codegen --schema prisma/schema.prisma --out pkg/schema
   ```
5. **Use** TORM in your application:

   ```go
   import (
     "context"
     "github.com/TechXTT/TORM/pkg/schema/models"
   )

   func main() {
     client := models.NewClient()
     ctx := context.Background()

     posts, err := client.Post.Query().All(ctx)
     if err != nil {
       // handle error
     }

     // use posts
   }
   ```

---

## Testing

- **Unit tests** with `sqlmock`: mock database calls in Go.
- **Integration tests**: configure Postgres in CI, run `torm migrate dev`, then exercise your code.

```bash
go test ./pkg/... ./internal/... -v
```

---

## License

MIT License.  