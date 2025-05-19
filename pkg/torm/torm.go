package torm

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/TechXTT/TORM/pkg/torm/generator"
	"github.com/TechXTT/TORM/pkg/torm/internal/metadata"
	"github.com/TechXTT/TORM/pkg/torm/runtime"
	"github.com/joho/godotenv"
)

// canonicalType normalizes SQL types for comparison.
func canonicalType(typ string) string {
	t := strings.ToUpper(typ)
	switch t {
	case "INT4", "INT8", "INTEGER":
		return "INTEGER"
	case "BOOL", "BOOLEAN":
		return "BOOLEAN"
	case "TEXT":
		return "TEXT"
	case "REAL", "FLOAT4", "FLOAT8":
		return "REAL"
	case "TIMESTAMP", "TIMESTAMPTZ":
		return "TIMESTAMP"
	case "UUID":
		return "UUID"
	default:
		return t
	}
}

// Prisma Migrate enables you to:

// Keep your database schema in sync with your Prisma schema as it evolves and
// Maintain existing data in your database
// Prisma Migrate generates a history of .sql migration files, and plays a role in both development and production.

// Prisma Migrate can be considered a hybrid database schema migration tool, meaning it has both of declarative and imperative elements:

// Declarative: The data model is described in a declarative way in the Prisma schema. Prisma Migrate generates SQL migration files from that data model.
// Imperative: All generated SQL migration files are fully customizable. Prisma Migrate hence provides the flexibility of an imperative migration tool by enabling you to modify what and how migrations are executed (and allows you to run custom SQL to e.g. make use of native database feature, perform data migrations, ...).

func RunMigrate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: torm migrate <dev|deploy|reset|status> [--dsn <dsn>] [--dir <migrations-dir>] [--schema <schema>]")
		os.Exit(1)
	}
	sub := args[0]
	fs := flag.NewFlagSet("migrate "+sub, flag.ExitOnError)

	// Define flags for migrate command
	dir := fs.String("dir", "./migrations", "migrations directory")
	schemaFile := fs.String("schema", "prisma/schema.prisma", "path to Prisma schema file")

	// Parse command-line args for migrate
	fs.Parse(args[1:])

	// Read DSN from Prisma schema datasource
	data, err := ioutil.ReadFile(*schemaFile)
	if err != nil {
		log.Fatalf("failed to read schema file %s: %v", *schemaFile, err)
	}
	re := regexp.MustCompile(`url\s*=\s*(?:env\("([^"]+)"\)|"([^"]+)")`)
	matches := re.FindStringSubmatch(string(data))
	godotenv.Load()
	var dsn string
	if len(matches) == 3 {
		if matches[1] != "" {
			dsn = os.Getenv(matches[1])
		} else {
			dsn = matches[2]
		}
	} else {
		log.Fatalf("could not parse datasource url from schema: %s", *schemaFile)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	mgr, err := runtime.NewManager(db, *dir)
	if err != nil {
		log.Fatalf("new manager: %v", err)
	}

	switch sub {
	case "dev":
		// Generate SQL stubs for any new models in the schema
		if err := ensureStubs(db, *schemaFile, *dir); err != nil {
			log.Fatalf("ensure stubs failed: %v", err)
		}

		if err := mgr.Dev(); err != nil {
			log.Fatalf("dev: %v", err)
		}

		// Regenerate Go models for updated schema
		generator.Generate(*schemaFile, "models") // replace with actual model output dir

	case "deploy":
		if err := mgr.Deploy(); err != nil {
			log.Fatalf("deploy: %v", err)
		}
	case "reset":
		if err := mgr.Reset(); err != nil {
			log.Fatalf("reset: %v", err)
		}
	case "status":
		status, err := mgr.Status()
		if err != nil {
			log.Fatalf("status: %v", err)
		}
		fmt.Println(status)
	default:
		fmt.Fprintf(os.Stderr, "Unknown migrate action %q\n", sub)
		os.Exit(1)
	}
}

// RunDB handles the "torm db" command.
func RunDB(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: torm db <push|pull> [--schema <schema>]")
		os.Exit(1)
	}
	sub := args[0]
	fs := flag.NewFlagSet("db "+sub, flag.ExitOnError)
	schemaFile := fs.String("schema", "schema.prisma", "schema file path")
	fs.Parse(args[1:])

	switch sub {
	case "push": // The db push command pushes the state of your Prisma schema to the database without using migrations. It creates the database if the database does not exist.
		log.Printf("Executing db push against schema %s", *schemaFile)
		// TODO: implement push
	case "pull": // The db pull command connects to your database and adds Prisma models to your Prisma schema that reflect the current database schema.
		log.Printf("Executing db pull against schema %s", *schemaFile)
		// TODO: implement pull
	default:
		fmt.Fprintf(os.Stderr, "Unknown db subcommand %q\n", sub)
		os.Exit(1)
	}
}

func ensureStubs(db *sql.DB, schemaPath, migrationsDir string) error {
	// Parse the Prisma schema into an AST
	data, err := ioutil.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	ast, err := metadata.ParseSchema(data)
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}

	// Introspect live database for existing columns and types
	existingCols := map[string]map[string]bool{}    // tableName -> set of columns
	existingTypes := map[string]map[string]string{} // tableName -> column -> udt_name
	for _, ent := range ast.Entities {
		table := strings.ToLower(ent.Name)
		existingCols[table] = map[string]bool{}
		existingTypes[table] = map[string]string{}

		rows, err := db.Query(
			`SELECT column_name, udt_name
             FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = $1`,
			table,
		)
		if err != nil {
			return fmt.Errorf("introspect table %s: %w", table, err)
		}
		defer rows.Close()

		for rows.Next() {
			var col, udtName string
			if err := rows.Scan(&col, &udtName); err != nil {
				return fmt.Errorf("scan column for %s: %w", table, err)
			}
			existingCols[table][col] = true
			existingTypes[table][col] = strings.ToUpper(udtName)
		}
	}

	// Read existing migration files
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Track seen entities and highest version
	seen := map[string]bool{}
	var versionNums []int
	reUp := regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if m := reUp.FindStringSubmatch(f.Name()); m != nil {
			seen[m[2]] = true
			if v, err := strconv.Atoi(m[1]); err == nil {
				versionNums = append(versionNums, v)
			}
		}
	}
	maxVer := 0
	if len(versionNums) > 0 {
		sort.Ints(versionNums)
		maxVer = versionNums[len(versionNums)-1]
	}

	// Generate migrations per entity
	for _, ent := range ast.Entities {
		tableName := strings.ToLower(ent.Name)
		existing := existingCols[tableName]
		types := existingTypes[tableName]

		if !seen[ent.Name] {
			// New table: CREATE TABLE stub
			maxVer++
			upFile := fmt.Sprintf("%04d_%s.up.sql", maxVer, ent.Name)
			downFile := fmt.Sprintf("%04d_%s.down.sql", maxVer, ent.Name)
			upPath := filepath.Join(migrationsDir, upFile)
			downPath := filepath.Join(migrationsDir, downFile)
			upSQL := generateCreateTableSQL(ent)
			downSQL := fmt.Sprintf("DROP TABLE %s;", tableName)
			if err := ioutil.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
				return fmt.Errorf("write up stub: %w", err)
			}
			if err := ioutil.WriteFile(downPath, []byte(downSQL), 0644); err != nil {
				return fmt.Errorf("write down stub: %w", err)
			}
			fmt.Printf("Generated migration stubs %s and %s\n", upFile, downFile)
		} else {
			// Existing table: detect adds, drops, and type changes
			var alters []string
			var drops []string

			// Added columns
			for _, f := range ent.Fields {
				col := strings.ToLower(f.Name)
				if !existing[col] {
					colType := mapGoTypeToSQL(f.Type)
					null := ""
					if f.Default == nil {
						null = " NOT NULL"
					}
					def := ""
					if f.Default != nil {
						def = fmt.Sprintf(" DEFAULT %s", *f.Default)
					}
					alters = append(alters, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s%s;", tableName, col, colType, null, def))
					drops = append(drops, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", tableName, col))
				}
			}

			// Changed types
			for _, f := range ent.Fields {
				col := strings.ToLower(f.Name)
				if existing[col] {
					expected := mapGoTypeToSQL(f.Type)
					actual := types[col]
					// Normalize both sides for comparison
					if canonicalType(expected) != canonicalType(actual) {
						// Use canonical expected type in migration
						alters = append(alters,
							fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tableName, col, canonicalType(expected)))
						// Use actual UDT for rollback
						drops = append(drops,
							fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tableName, col, canonicalType(actual)))
					}
				}
			}

			// Removed columns
			for col := range existing {
				found := false
				for _, f := range ent.Fields {
					if strings.ToLower(f.Name) == col {
						found = true
						break
					}
				}
				if !found {
					alters = append(alters, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", tableName, col))
					drops = append(drops, fmt.Sprintf("-- note: column %s dropped; manual re-add may be required", col))
				}
			}

			// Write alteration stubs if any
			if len(alters) > 0 {
				maxVer++
				upFile := fmt.Sprintf("%04d_%s.up.sql", maxVer, ent.Name)
				downFile := fmt.Sprintf("%04d_%s.down.sql", maxVer, ent.Name)
				upPath := filepath.Join(migrationsDir, upFile)
				downPath := filepath.Join(migrationsDir, downFile)
				upSQL := strings.Join(alters, "\n")
				downSQL := strings.Join(drops, "\n")
				if err := ioutil.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
					return fmt.Errorf("write alteration up stub: %w", err)
				}
				if err := ioutil.WriteFile(downPath, []byte(downSQL), 0644); err != nil {
					return fmt.Errorf("write alteration down stub: %w", err)
				}
				fmt.Printf("Generated ALTER migration stubs %s and %s\n", upFile, downFile)
			}
		}
	}
	return nil
}

func generateCreateTableSQL(ent metadata.Entity) string {
	tableName := strings.ToLower(ent.Name)
	var lines []string

	// Primary key column first
	for _, f := range ent.Fields {
		if f.PrimaryKey {
			col := strings.ToLower(f.Name)
			var colType string
			var defaultClause string

			// Handle UUID primary keys
			if f.Type == "uuid.UUID" {
				colType = "UUID"
				defaultClause = " DEFAULT uuid_generate_v4()"
			} else if f.AutoIncrement && (f.Type == "int" || f.Type == "int32" || f.Type == "int64") {
				colType = "SERIAL"
			} else {
				colType = mapGoTypeToSQL(f.Type)
			}

			lines = append(lines, fmt.Sprintf("    %s %s PRIMARY KEY%s", col, colType, defaultClause))
			break
		}
	}

	// Other columns
	for _, f := range ent.Fields {
		if f.PrimaryKey {
			continue
		}
		col := strings.ToLower(f.Name)
		colType := mapGoTypeToSQL(f.Type)

		// NOT NULL unless a default value is provided
		notNull := ""
		if f.Default == nil {
			notNull = " NOT NULL"
		}

		// Default clause
		defaultClause := ""
		if f.Default != nil {
			defaultClause = fmt.Sprintf(" DEFAULT %s", *f.Default)
		}

		lines = append(lines, fmt.Sprintf("    %s %s%s%s", col, colType, notNull, defaultClause))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);", tableName, strings.Join(lines, ",\n"))
}

func mapGoTypeToSQL(goType string) string {
	switch goType {
	case "int", "int32", "int64":
		return "INTEGER"
	case "string":
		return "TEXT"
	case "bool":
		return "BOOLEAN"
	case "float32", "float64":
		return "REAL"
	case "time.Time":
		return "TIMESTAMP"
	default:
		return "TEXT"
	}
}
