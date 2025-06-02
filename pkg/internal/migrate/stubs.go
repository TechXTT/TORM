package migrate

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/TechXTT/TORM/pkg/internal/generator"
	"github.com/TechXTT/TORM/pkg/internal/typeconv"
)

func EnsureStubs(db *sql.DB, schemaPath, migrationsDir string) error {
	// Parse the Prisma schema into an AST
	data, err := ioutil.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	ast, err := generator.ParseSchema(data)
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
					colType := typeconv.MapGoTypeToSQL(f.Type)
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
					expected := typeconv.MapGoTypeToSQL(f.Type)
					actual := types[col]
					// Normalize both sides for comparison
					if typeconv.CanonicalType(expected) != typeconv.CanonicalType(actual) {
						// Use canonical expected type in migration
						alters = append(alters,
							fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tableName, col, typeconv.CanonicalType(expected)))
						// Use actual UDT for rollback
						drops = append(drops,
							fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tableName, col, typeconv.CanonicalType(actual)))
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

func generateCreateTableSQL(ent generator.Entity) string {
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
				colType = typeconv.MapGoTypeToSQL(f.Type)
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
		colType := typeconv.MapGoTypeToSQL(f.Type)

		// Default clause
		defaultClause := ""
		if f.Default != nil {
			defaultClause = fmt.Sprintf(" DEFAULT %s", *f.Default)
		}

		lines = append(lines, fmt.Sprintf("    %s %s%s", col, colType, defaultClause))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);", tableName, strings.Join(lines, ",\n"))
}
