# TORM

TORM is a Go ORM for PostgreSQL. Define models in Prisma Schema Language, generate a typed Go client, and review the SQL migrations before running them. TORM runs as a Go binary and uses Go's `database/sql` at runtime.

## Quickstart

Use Go 1.23 or newer and PostgreSQL 16–18. The CI matrix keeps the previous Go minimum for build compatibility. Use a Go release that still receives security updates for production builds.

1. From the repository root, run `go install ./cmd/torm` and make sure the Go binary directory is on your PATH.
2. Create two empty databases, `torm_example` and `torm_shadow`, on the same PostgreSQL major version. Give the development role access to both and permission to create a schema in the shadow database. In `example/`, copy `.env.example` to `.env` and set `DATABASE_URL` and `TORM_SHADOW_DATABASE_URL`. Use a dedicated shadow database with a distinct name and an empty `public` schema.
3. Validate the schema, regenerate the Go client, and write the initial migration.

   ```sh
   torm validate
   torm generate --out-models models
   torm migrate plan --name initial
   ```

4. Review `torm/migrations/0001_initial.up.sql`, apply it, then start the example. The HTTP app serves `/creators` and `/posts` on port 8080.

   ```sh
   torm migrate deploy
   torm migrate status
   go run .
   ```

Use the same published v2 tag for the CLI and runtime dependency in a downstream project. The example module uses a local replacement so it builds against the checkout. Check in the reviewed SQL migrations. Generated code can also be checked in and verified by a regeneration check in CI.

## Commands

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

Schema commands default to `prisma/schema.prisma`. Set `--schema` to a file or a directory of `.prisma` files. The migration directory defaults to `torm/migrations`. Override it with `--out-migrations` or `--dir`. Deployment and status can run from reviewed SQL files and a connection string alone.

TORM reads connection settings from `--dsn`, `TORM_DATABASE_URL`, `DATABASE_URL`, or the legacy schema's datasource URL, in that order. Database commands load `.env` from the working directory. Set the shadow connection with `TORM_SHADOW_DATABASE_URL` or `--shadow-dsn`. Use a PostgreSQL DSN for Go and leave Prisma CLI-only query parameters, such as `schema`, in Prisma's configuration. Generated `Open(dsn)` and `NewClientFromDB(db)` take explicit connections. `Open` owns its pool, and `NewClientFromDB` borrows the caller's pool.

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

## Generated Go API

The generated package provides typed keys, filters, sorting, relationship includes, and create/update inputs. A zero input leaves a value omitted. `runtime.Set` sends an explicit Go value, including `false`, `0`, or `""`. `runtime.Null[T]()` writes SQL `NULL` for a nullable field.

```go
client, err := models.Open(dsn)
if err != nil {
    return err
}
defer client.Close()

creators, err := client.CreatorService()
if err != nil {
    return err
}
creator, err := creators.Insert(ctx, models.CreatorCreate{
    Username: runtime.Set("ada"),
    Email: runtime.Set("ada@example.test"),
})
if err != nil {
    return err
}
withPosts, err := creators.Fetch(ctx, models.CreatorById(creator.Id),
    models.CreatorInclude{Posts: true})
```

`Find` accepts typed filters, order, offset, and limit. `Fetch`, `Patch`, `UpsertBy`, and `DeleteBy` use a declared unique key. `UpsertBy` uses PostgreSQL `ON CONFLICT`. `Client.Transaction` provides transaction-scoped services. Use `CreateBatch` for atomic multi-row writes and `runtime.All[Model]()` to explicitly target all rows for a bulk mutation. Relation includes load in batches. Implicit many-to-many services expose typed `Connect<Relation>` and `Disconnect<Relation>` methods, and explicit join models use regular CRUD.

## Migration review and recovery

1. Run `migrate plan` after changing the schema. The planner replays applied files in a temporary shadow schema, compares it with the live database, replays pending files, and verifies the proposed SQL. It expects history to describe the managed `public` schema. Preserve a separate, dedicated shadow database for this purpose.
2. Use `--name` to write a reviewed plan. Column and table removal, type conversion, and changes that need new non-null values require `--accept-data-loss` to write or apply an automatic migration. For a rename or data backfill, write an explicit `NNNN_name.up.sql` using unqualified application table names and check its result against the desired schema. Supported migration files run inside TORM-managed transactions.
3. Commit the reviewed migration files and deploy them in order. Each applied version records a checksum of its up SQL and name. Review any failure before retrying. A failed transactional migration leaves its SQL and version unapplied, so an unapplied file can be corrected and retried.
4. Preserve applied files. For a production correction, add a forward migration. Before a destructive change, take a restorable backup and verify the operation against representative data. When replaying an old setup that requires extensions, provision the same extensions or functions in the target and shadow databases.
5. Use `reset --force` only with a disposable database. TORM verifies and rebuilds the managed schema, then replays reviewed migrations. It preserves unchanged applied files and can resume from the last successfully applied version if replay fails.

For enum changes that add labels and immediately use them in another database object, put the enum addition and its first use in separate migrations. Removing or reordering enum values requires reviewed manual SQL.

## Upgrade from v1.0.0

1. Keep the original `.up.sql` and `.down.sql` files. Preserve physical names with `@map` and `@@map`, or use `--legacy-names` for the v1 lowercase convention. A legacy datasource URL also selects lowercase names. Existing custom join layouts can be declared as explicit join models or migrated with reviewed SQL.
2. Point TORM at the original migration directory. Run `migrate status` and then `migrate baseline` using a disposable shadow database. Baselining checks schema objects against replayed SQL before updating the migration ledger. Verify any historical data backfills separately. For a database without TORM history, use `--version` to select the verified prefix if newer files are still pending. Repair catalog drift and resolve history discrepancies before baselining.
3. Plan and review the changes to types and constraints. The [bundled v1 example upgrade](example/migrations/0003_upgrade_schema.up.sql) keeps existing creator names and posts, backfills usernames from IDs, adds required constraints, and converts post author IDs to UUIDs. It requires valid email values and author references. Its historical migrations require `uuid-ossp` in the shadow database. The fresh quickstart uses a separate migration directory and client-generated UUIDs.
4. Regenerate and recompile your application. The v2 Go module has the `/v2` suffix. The generated API retains the v1 service methods where possible. Update callers for nullable fields that now use pointers, relationship slices that contain pointers, and filters that require declared field or unique-key names. Use `Open` or `NewClientFromDB` to replace runtime schema-file loading.

## Verification

1. Run `go test ./...` and `go vet ./...` in the root and in `example/`.
2. Set `TORM_TEST_DATABASE_URL` to a disposable PostgreSQL server connection whose role can create databases, then run `go test -count=1 ./...` at the root. The integration suite creates its own databases and exercises fresh installs, repeat planning, failed and concurrent migrations, drift, the v1 data upgrade, and generated queries.
3. Run `npx --yes prisma@7.10.1 validate --schema example/prisma/schema.prisma` and `npx --yes prisma@7.10.1 validate --schema pkg/internal/schema/testdata/postgres` to check the shared Prisma fixtures. This is the last Prisma 7 release, pinned because Prisma 8 uses a different contract format.
4. The GitHub Actions workflow tests supported Go and PostgreSQL configurations, verifies checked-in generated output, and packages binaries when a v2 tag is pushed.

## License

[MIT](LICENSE)
