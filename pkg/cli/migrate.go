package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/TechXTT/TORM/v2/pkg/config"
	"github.com/TechXTT/TORM/v2/pkg/internal/generator"
	"github.com/TechXTT/TORM/v2/pkg/internal/schema"
	"github.com/TechXTT/TORM/v2/pkg/runtime"
	"github.com/spf13/cobra"
)

type migrationFlags struct {
	schemaFlags
	dsn       string
	shadowDSN string
	dir       string
	legacyDir string
}

func (options migrationFlags) manager(cmd *cobra.Command, requireSchema bool) (*runtime.Manager, *schema.Schema, error) {
	config.LoadEnv()
	var source schema.Source
	var validated *schema.Schema

	if requireSchema || (options.dsn == "" && os.Getenv("TORM_DATABASE_URL") == "" && os.Getenv("DATABASE_URL") == "") {
		var err error

		validated, err = schema.Load(options.path, options.legacyNames)
		if err != nil && (requireSchema || !errors.Is(err, os.ErrNotExist)) {
			return nil, nil, err
		}
		if validated != nil {
			source = validated.Source
		}
	}
	dsn, err := config.ResolveDSN(options.dsn, source.Environment, source.URL)
	if err != nil {
		return nil, nil, err
	}
	dir := options.dir
	if options.legacyDir != "" {
		if cmd.Flags().Changed("out-migrations") && options.legacyDir != dir {
			return nil, nil, fmt.Errorf("choose either --dir or --out-migrations")
		}
		dir = options.legacyDir
	}
	manager, err := runtime.NewManager(dsn, dir)
	return manager, validated, err
}

func (options migrationFlags) shadow() string {
	if options.shadowDSN != "" {
		return options.shadowDSN
	}
	return os.Getenv("TORM_SHADOW_DATABASE_URL")
}

func NewMigrateCmd() *cobra.Command {
	var options migrationFlags
	var planName string
	var devName string
	var output string
	var packageName string
	var planRisk bool
	var devRisk bool
	var force bool
	var baselineVersion int

	root := &cobra.Command{Use: "migrate", Short: "Plan, apply, and inspect migrations"}
	root.PersistentFlags().StringVar(&options.path, "schema", defaultSchema, "Prisma schema file or directory")
	root.PersistentFlags().BoolVar(&options.legacyNames, "legacy-names", false, "Use lowercase names for unmapped models and fields")
	root.PersistentFlags().StringVar(&options.dir, "out-migrations", "torm/migrations", "Migration directory")
	root.PersistentFlags().StringVar(&options.legacyDir, "dir", "", "Alias for --out-migrations")
	root.PersistentFlags().StringVar(&options.dsn, "dsn", "", "Target PostgreSQL connection string")
	root.PersistentFlags().StringVar(&options.shadowDSN, "shadow-dsn", "", "Separate shadow PostgreSQL connection string")
	plan := &cobra.Command{
		Use: "plan", Short: "Preview SQL or write a named migration for review", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, source, err := options.manager(cmd, true)
			if err != nil {
				return err
			}
			defer manager.Close()
			result, path, err := manager.Plan(cmd.Context(), options.shadow(), source.DDL(), planName, planRisk)
			for _, notice := range result.Review {
				fmt.Fprintln(cmd.ErrOrStderr(), notice)
			}
			if result.SQL != "" {
				fmt.Fprint(cmd.OutOrStdout(), result.SQL)
			}
			if err != nil {
				return err
			}
			if path != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "Wrote", path)
			}
			if result.SQL == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No schema changes")
			}
			return nil
		},
	}
	plan.Flags().StringVar(&planName, "name", "", "Write a migration with this name")
	plan.Flags().BoolVar(&planRisk, "accept-data-loss", false, "Allow writing SQL that requires data review")

	dev := &cobra.Command{
		Use: "dev", Short: "Generate and apply migrations on a development database", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, source, err := options.manager(cmd, true)
			if err != nil {
				return err
			}
			defer manager.Close()
			if err := generator.Validate(source); err != nil {
				return err
			}
			result, path, err := manager.Develop(cmd.Context(), options.shadow(), source.DDL(), devName, devRisk)
			for _, notice := range result.Review {
				fmt.Fprintln(cmd.ErrOrStderr(), notice)
			}
			if path != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "Wrote", path)
			}
			if err != nil {
				return err
			}
			return generator.Compile(source, output, packageName)
		},
	}
	dev.Flags().StringVar(&devName, "name", "schema", "Name for a new migration")
	dev.Flags().BoolVar(&devRisk, "accept-data-loss", false, "Allow schema changes that require data review")
	dev.Flags().StringVar(&output, "out-models", defaultModels, "Output directory for generated Go models")
	dev.Flags().StringVar(&packageName, "package", "models", "Output Go package name")

	deploy := &cobra.Command{
		Use: "deploy", Short: "Apply pending reviewed SQL migrations", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := options.manager(cmd, false)
			if err != nil {
				return err
			}
			defer manager.Close()
			return manager.DeployContext(cmd.Context())
		},
	}
	status := &cobra.Command{
		Use: "status", Short: "Show migration history without modifying the database", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := options.manager(cmd, false)
			if err != nil {
				return err
			}
			defer manager.Close()
			value, err := manager.StatusContext(cmd.Context())
			if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return err
		},
	}
	baseline := &cobra.Command{
		Use: "baseline", Short: "Verify and record already-applied migration SQL", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := options.manager(cmd, false)
			if err != nil {
				return err
			}
			defer manager.Close()
			return manager.Baseline(cmd.Context(), options.shadow(), baselineVersion)
		},
	}
	baseline.Flags().IntVar(&baselineVersion, "version", 0, "Last already-applied version for an existing database")

	reset := &cobra.Command{
		Use: "reset", Short: "Rebuild a disposable development database and discard its data", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return fmt.Errorf("reset removes application data; pass --force for a disposable database")
			}
			manager, _, err := options.manager(cmd, false)
			if err != nil {
				return err
			}
			defer manager.Close()
			return manager.ResetContext(cmd.Context(), options.shadow())
		},
	}
	reset.Flags().BoolVar(&force, "force", false, "Confirm that the target is disposable")
	root.AddCommand(plan, dev, deploy, status, baseline, reset)
	return root
}
