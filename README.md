# TORM

[![Go Reference](https://pkg.go.dev/badge/github.com/TechXTT/TORM/v2.svg)](https://pkg.go.dev/github.com/TechXTT/TORM/v2)
[![Release](https://img.shields.io/github/v/release/TechXTT/TORM?label=version)](https://github.com/TechXTT/TORM/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/TechXTT/TORM/v2)](https://goreportcard.com/report/github.com/TechXTT/TORM/v2)

TORM is a schema-driven Go ORM for PostgreSQL. Define your models and relationships in Prisma Schema Language, generate a typed Go client, and review SQL migrations before applying them. The CLI is a Go binary, and the generated client uses Go’s `database/sql`.

## Table of contents

1. [Features](#features)
2. [Installation](#installation)
3. [Getting started](#getting-started)
4. [Command reference](#command-reference)
5. [Configuration](#configuration)
6. [PostgreSQL schema support](#postgresql-schema-support)
7. [Migration review and recovery](#migration-review-and-recovery)
8. [Upgrading from v1](#upgrading-from-v1)
9. [Contributing](#contributing)
10. [License](#license)

## Features

1. **Typed Go client.** Generate model structs, unique keys, filters, and create and update inputs from your schema. Validate and generate offline.
2. **Relationships.** Fetch related records in batches, work with explicit join models, or connect records through implicit many-to-many relations.
3. **Reviewable migrations.** Preview schema changes as SQL, detect drift against migration history, and deploy checked-in migrations under a database lock.

## Installation

TORM requires Go 1.23 or newer and supports PostgreSQL 16–18. Use a supported Go release for production builds. Install the CLI and add the matching runtime dependency from your Go module directory.

```sh
go install github.com/TechXTT/TORM/v2/cmd/torm@v2.0.0
go get github.com/TechXTT/TORM/v2/pkg/runtime@v2.0.0
```

Add your Go binary directory to `PATH` so you can run `torm`. When working from a checkout, install the CLI with `go install ./cmd/torm` from the repository root. The bundled example already points its Go dependency at the checkout.

## Getting started

The [example application](example/) serves creators and their posts over HTTP. Follow the steps below from a checkout to generate its client and create a fresh development database schema.

### Define the schema

TORM reads `prisma/schema.prisma` by default. In the example, each creator has a UUID primary key, a unique username, and a collection of posts. Here is an excerpt from the [complete schema](example/prisma/schema.prisma).

```prisma
model Creator {
  id        String   @id @default(uuid()) @db.Uuid
  username  String   @unique
  email     String   @unique
  firstName String?  @map("firstname")
  lastName  String?  @map("lastname")
  createdAt DateTime @default(now()) @map("createdat") @db.Timestamp(6)
  updatedAt DateTime @updatedAt @map("updatedat") @db.Timestamp(6)
  posts     Post[]

  @@map("creator")
}
```

### Generate the Go client

From the repository root, switch to the example directory. Run the remaining example commands there.

```sh
cd example
torm validate
torm generate --out-models models
```

This writes the models and services to `models/torm.gen.go`. You can check this file into version control and verify regeneration in CI.

### Create and apply the first migration

1. Create two empty PostgreSQL databases, `torm_example` for the application and `torm_shadow` for migration planning. Use the same PostgreSQL major version for both. Give the development role access to both databases and permission to create schemas in the shadow database. Keep the shadow database dedicated to TORM, with an empty `public` schema.
2. Copy `.env.example` to `.env`, set `DATABASE_URL` and `TORM_SHADOW_DATABASE_URL` to those connections, and generate the initial migration.

   ```sh
   torm migrate plan --name initial
   ```

3. Review `torm/migrations/0001_initial.up.sql`, then apply it and start the example.

   ```sh
   torm migrate deploy
   torm migrate status
   go run .
   ```

The app listens on port 8080. In another terminal, run `curl http://localhost:8080/creators` to list creators. The corresponding posts endpoint is `/posts`.

### Use the generated client

The generated package provides typed keys, filters, sorting, relationship includes, and create/update inputs. A zero input leaves a value omitted. `runtime.Set` sends an explicit Go value, including `false`, `0`, or `""`. `runtime.Null[T]()` writes SQL `NULL` for a nullable field.

```go
func createCreator(ctx context.Context, dsn string) (*models.Creator, error) {
	client, err := models.Open(dsn)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	creators, err := client.CreatorService()
	if err != nil {
		return nil, err
	}
	creator, err := creators.Insert(ctx, models.CreatorCreate{
		Username: runtime.Set("ada"),
		Email:    runtime.Set("ada@example.test"),
	})
	if err != nil {
		return nil, err
	}
	return creators.Fetch(ctx, models.CreatorById(creator.Id),
		models.CreatorInclude{Posts: true})
}
```

`Find` accepts typed filters, order, offset, and limit. `Fetch`, `Patch`, `UpsertBy`, and `DeleteBy` use a declared unique key. `UpsertBy` uses PostgreSQL `ON CONFLICT`. `Client.Transaction` provides transaction-scoped services. Use `CreateBatch` for atomic multi-row writes and `runtime.All[Model]()` to explicitly target all rows for a bulk mutation. Relation includes load in batches. Implicit many-to-many services expose typed `Connect<Relation>` and `Disconnect<Relation>` methods, and explicit join models use regular CRUD.

## Command reference

| Command | Behavior |
| --- | --- |
| `torm validate` | Validate schema syntax, PostgreSQL semantics, and generated Go names without a database. |
| `torm generate` | Generate `torm.gen.go` without a database. Use `--out-models` and `--package` to configure the package. |
| `torm migrate plan` | Compare replayed migration history with the desired schema, verify live drift, and print SQL. Add `--name <name>` to save the plan. |
| `torm migrate dev` | Plan, write and apply changes to a development database, then regenerate the client. |
| `torm migrate deploy` | Apply reviewed, pending SQL files with per-migration transactions and a database advisory lock. |
| `torm migrate status` | Read applied and pending versions and check their history. |
| `torm migrate baseline` | Replay and verify existing history before recording checksums on an existing database. |
| `torm migrate reset --force` | Rebuild a disposable development database from migration history. The rebuild discards application rows. |

Run `torm --help` for the full command list or `torm migrate <command> --help` for migration options.

## Configuration

| Setting | Default or lookup order |
| --- | --- |
| Schema | `prisma/schema.prisma`. Use `--schema` for another file or a directory of `.prisma` files. |
| Generated client | `torm/models/torm.gen.go`, in package `models`. Override with `--out-models` and `--package`. |
| Migration directory | `torm/migrations`. Override with `--out-migrations` or its alias `--dir`. |
| Target connection | `--dsn`, then `TORM_DATABASE_URL`, `DATABASE_URL`, and finally the URL in a legacy datasource block. Database commands load `.env` from the working directory. |
| Shadow connection | `--shadow-dsn` or `TORM_SHADOW_DATABASE_URL`. Deployment and status need only the target connection and reviewed SQL files. |

Use a PostgreSQL DSN suitable for Go. Keep Prisma CLI-only parameters such as `schema` in Prisma’s configuration. In application code, `models.Open(dsn)` owns its connection pool. Use `models.NewClientFromDB(db)` to share a pool that your application manages.

## PostgreSQL schema support

TORM uses its own validator and code generator. CI validates the reference schemas with a pinned [Prisma ORM 7 CLI](https://www.prisma.io/docs/orm/v7/reference/prisma-schema-reference). Datasource and generator blocks may coexist with Prisma tooling. TORM treats generator settings as metadata. For Prisma 7 projects, set the Prisma CLI's connection in [Prisma Config](https://www.prisma.io/docs/orm/v7/reference/prisma-config-reference) and TORM's connection through the settings above.

| Area | Supported behavior |
| --- | --- |
| Input | PostgreSQL datasource, single-file or multi-file schema, models, enums, comments, and file and line diagnostics. |
| Scalars | `String`, `Int`, `BigInt`, `Float`, `Decimal`, `Boolean`, `DateTime`, `Bytes`, `Json`, enums, and PostgreSQL scalar arrays. Use `runtime.Decimal` for exact numeric values. Optional fields become Go pointers. TORM declares scalar lists non-null. |
| Native types | `Uuid`, `Text`, `VarChar`, `Char`, `SmallInt`, `Integer`, `BigInt`, `Real`, `DoublePrecision`, `Boolean`, `Decimal`, `Date`, `Timestamp`, `Timestamptz`, `Time`, `Timetz`, `ByteA`, `Json`, and `JsonB`. |
| Naming and constraints | `@map` and `@@map`, mapped enum labels, `@id`, `@@id`, `@unique`, `@@unique`, and plain `@@index`. Compound selectors can use `name` and database objects can use `map`. |
| Defaults | Compatible literals, `autoincrement()`, `now()`, client-generated `uuid()` and `uuid(7)`, and client-managed `@updatedAt`. Native UUID database defaults support `dbgenerated("gen_random_uuid()")` and the legacy `uuid_generate_v4()` form when the database supplies it. |
| Relations | PostgreSQL foreign keys, referential actions, one-to-one, one-to-many, explicit join models, and Prisma-style implicit many-to-many join tables. Relations can reference declared composite unique keys. |
| Current scope | PostgreSQL `public` schema, foreign-key relation mode, and ordinary tables. Other providers, multi-schema models, advanced index options, relation emulation, and Prisma 8 contracts require additional support. |

## Migration review and recovery

1. Run `migrate plan` after changing the schema. The planner replays applied files in a temporary shadow schema, compares it with the live database, replays pending files, and verifies the proposed SQL. It expects history to describe the managed `public` schema. Preserve a separate, dedicated shadow database for this purpose.
2. Use `--name` to write a reviewed plan. Column and table removal, type conversion, and changes that need new non-null values require `--accept-data-loss` to write or apply an automatic migration. For a rename or data backfill, write an explicit `NNNN_name.up.sql` using unqualified application table names and check its result against the desired schema. Supported migration files run inside TORM-managed transactions.
3. Commit the reviewed migration files and deploy them in order. Each applied version records a checksum of its up SQL and name. Review any failure before retrying. A failed transactional migration leaves its SQL and version unapplied, so an unapplied file can be corrected and retried.
4. Preserve applied files. For a production correction, add a forward migration. Before a destructive change, take a restorable backup and verify the operation against representative data. When replaying an old setup that requires extensions, provision the same extensions or functions in the target and shadow databases.
5. Use `reset --force` only with a disposable database. TORM verifies and rebuilds the managed schema, then replays reviewed migrations. It preserves unchanged applied files and can resume from the last successfully applied version if replay fails.

For enum changes that add labels and immediately use them in another database object, put the enum addition and its first use in separate migrations. Removing or reordering enum values requires reviewed manual SQL.

## Upgrading from v1

1. Keep the original `.up.sql` and `.down.sql` files. Preserve physical names with `@map` and `@@map`, or use `--legacy-names` for the v1 lowercase convention. A legacy datasource URL also selects lowercase names. Existing custom join layouts can be declared as explicit join models or migrated with reviewed SQL.
2. Point TORM at the original migration directory. Run `migrate status` and then `migrate baseline` using a disposable shadow database. Baselining checks schema objects against replayed SQL before updating the migration ledger. Verify any historical data backfills separately. For a database without TORM history, use `--version` to select the verified prefix if newer files are still pending. Repair catalog drift and resolve history discrepancies before baselining.
3. Plan and review the changes to types and constraints. The [bundled v1 example upgrade](example/migrations/0003_upgrade_schema.up.sql) keeps existing creator names and posts, backfills usernames from IDs, adds required constraints, and converts post author IDs to UUIDs. It requires valid email values and author references. Its historical migrations require `uuid-ossp` in the shadow database. The fresh quickstart uses a separate migration directory and client-generated UUIDs.
4. Regenerate and recompile your application. The v2 Go module has the `/v2` suffix. The generated API retains the v1 service methods where possible. Update callers for nullable fields that now use pointers, relationship slices that contain pointers, and filters that require declared field or unique-key names. Use `Open` or `NewClientFromDB` to replace runtime schema-file loading.

## Contributing

1. Run `go test ./...` and `go vet ./...` in both the repository root and `example/`.
2. To exercise migrations and generated queries against PostgreSQL, set `TORM_TEST_DATABASE_URL` to a disposable server connection whose role can create databases. Run `go test -count=1 ./...` at the root.
3. CI also checks that the generated example client is up to date and validates both reference schemas with the pinned Prisma 7 CLI. See the [build workflow](.github/workflows/build.yaml) for the exact commands and test matrix.

## License

TORM is released under the [MIT License](LICENSE).
