# TORM – v0.5.0-alpha

TORM is a lightweight, idiomatic Go ORM for PostgreSQL, providing:

- **Schema DSL & Code Generation**: Define your schema in Go, generate models and migrations.
- **Introspective Migrations**: Auto-generate `CREATE TABLE` and `ALTER TABLE` stubs by comparing your DSL to the live database.
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
# Apply and auto-generate new migrations from your schema (dev database)
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

- `--schema` (optional): path to your Prisma-style schema file (default `prisma/schema.prisma`).
- `--dir` (optional): directory holding your `.up.sql`/`.down.sql` scripts (default `migrations/`).

### Code Generation

Generate Go models from your DSL schema definitions:

```bash
torm codegen \
  --schema-dir schema_defs \
  --out pkg/schema
```

- `--schema-dir`: directory containing `.schema` files.
- `--out`: target directory for generated Go model files.

---

## Defining Your Schema (DSL)

Place your `.schema` files under `schema_defs/`:

```go
// schema_defs/post.schema
entity.Post().
  Field("ID", "uuid.UUID").PrimaryKey().Default("uuid_generate_v4()").
  Field("Title", "string").NotNull().
  Field("Content", "string").
  Field("Published", "bool").Default("false").
  Field("CreatedAt", "time.Time").Default("now()").
  Field("UpdatedAt", "time.Time")
```

Each `Field` chain supports:

- `.PrimaryKey()`, `.AutoIncrement()`
- `.NotNull()`, `.Default("<expr>")`
- `.Enum("A","B","C")`
- Types: `string`, `int`, `bool`, `float64`, `time.Time`, `uuid.UUID`, and slices (`[]Type`).

---

## Typical Workflow

1. **Edit** your DSL files in `schema_defs/`.
2. **Run** migrations for your dev database (auto-generates SQL stubs):
   ```bash
   torm migrate dev --schema prisma/schema.prisma --dir migrations
   ```
3. **Inspect** and hand-tweak the generated `migrations/000X_*.up.sql` and `.down.sql` if needed.
4. **Regenerate** Go models:
   ```bash
   torm codegen --schema-dir schema_defs --out pkg/schema
   ```
5. **Use** TORM in your application:

   ```go
   import "github.com/TechXTT/TORM/pkg/torm"
   import "your/project/pkg/schema"

   db, err := torm.Open(dsn)
   defer db.Close()

   posts, err := db.
     Query[schema.Post]().
     From(schema.PostTable).
     All(ctx)
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