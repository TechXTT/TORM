package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/TechXTT/TORM/v2/pkg/internal/generator"
	"github.com/TechXTT/TORM/v2/pkg/internal/schema"
	"github.com/spf13/cobra"
)

const defaultSchema = "prisma/schema.prisma"
const defaultModels = "torm/models"

var releaseVersion string

func version() string {
	if releaseVersion != "" {
		return releaseVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

type schemaFlags struct {
	path        string
	legacyNames bool
}

func (options *schemaFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&options.path, "schema", defaultSchema, "Prisma schema file or directory")
	cmd.Flags().BoolVar(&options.legacyNames, "legacy-names", false, "Use lowercase SQL names for unmapped models and fields")
}

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "Print the version number", Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version())
		},
	}
}

func NewHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use: "help [command]", Short: "Help about any command",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _, err := cmd.Root().Find(args)
			if err != nil {
				return err
			}
			return target.Help()
		},
	}
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use: "torm", Short: "Generate Go clients and manage PostgreSQL migrations",
		SilenceUsage: true,
		Version:      version(),
	}
	root.SetHelpCommand(NewHelpCmd())
	root.AddCommand(NewMigrateCmd(), NewVersionCmd(), newValidateCmd(), newGenerateCmd())
	return root
}

func newValidateCmd() *cobra.Command {
	var options schemaFlags

	cmd := &cobra.Command{
		Use: "validate", Short: "Validate a Prisma schema without connecting to PostgreSQL", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := schema.Load(options.path, options.legacyNames)
			if err != nil {
				return err
			}
			if err := generator.Validate(source); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Validated %d models\n", len(source.Models))
			return nil
		},
	}
	options.add(cmd)
	return cmd
}

func newGenerateCmd() *cobra.Command {
	var options schemaFlags
	var output string
	var packageName string

	cmd := &cobra.Command{
		Use: "generate", Short: "Generate a Go client without connecting to PostgreSQL", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := schema.Load(options.path, options.legacyNames)
			if err != nil {
				return err
			}
			if err := generator.Compile(source, output, packageName); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Generated %d models in %s\n", len(source.Models), output)
			return nil
		},
	}
	options.add(cmd)
	cmd.Flags().StringVar(&output, "out-models", defaultModels, "Output directory for generated Go models")
	cmd.Flags().StringVar(&packageName, "package", "models", "Output Go package name")
	return cmd
}
