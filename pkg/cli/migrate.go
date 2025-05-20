package cli

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/TechXTT/TORM/pkg/config"
	"github.com/TechXTT/TORM/pkg/internal/generator"
	"github.com/TechXTT/TORM/pkg/internal/migrate"
	"github.com/TechXTT/TORM/pkg/runtime"
	"github.com/spf13/cobra"
)

func NewMigrateCmd() *cobra.Command {
	var (
		schemaFile string
		migrations string
	)

	cmd := &cobra.Command{
		Use:       "migrate [dev|deploy|reset|status]",
		Short:     "Run database migrations",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"dev", "deploy", "reset", "status"},
		RunE: func(cmd *cobra.Command, args []string) error {
			action := args[0]
			cfg, err := config.Load(schemaFile)
			if err != nil {
				return err
			}
			mgr, err := runtime.NewManager(cfg.DSN, migrations)
			if err != nil {
				return err
			}

			// Open the database connection
			db, err := sql.Open("postgres", cfg.DSN)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer db.Close()

			switch action {
			case "dev":
				// Open the database connection
				db, err := sql.Open("postgres", cfg.DSN)
				if err != nil {
					log.Fatalf("open db: %v", err)
				}
				defer db.Close()
				// Generate SQL stubs for any new models in the schema
				if err := migrate.EnsureStubs(db, schemaFile, migrations); err != nil {
					log.Fatalf("ensure stubs failed: %v", err)
				}
				if err := mgr.Dev(); err != nil {
					return err
				}
				// codegen after migrations
				return generator.Generate(cfg.SchemaDir, cfg.ModelOutDir)
			case "deploy":
				return mgr.Deploy()
			case "reset":
				return mgr.Reset()
			case "status":
				status, err := mgr.Status()
				if err != nil {
					return err
				}
				fmt.Println(status)
				return nil
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&schemaFile, "schema", "prisma/schema.prisma", "Prisma schema path")
	cmd.Flags().StringVar(&migrations, "dir", "migrations", "Migrations directory")
	return cmd
}
