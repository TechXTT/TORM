package cli

import (
	"github.com/spf13/cobra"
)

func version() string {
	return "v1.0.0"
}

func help() string {
	return `TORM is a tool for managing database migrations and code generation.
It provides commands for applying, reverting, and checking the status of migrations,
as well as generating code based on a Prisma schema.
Usage:
  torm <command> [flags]
Available Commands:
  migrate     Run database migrations	
	dev         Run migrations in development mode
	deploy      Run migrations in deployment mode
	reset       Reset the database to its initial state
	status      Show the current migration status
Flags:
  -h, --help   help for torm
  -v, --version   print the version number
Use "torm [command] --help" for more information about a command.
Examples:
  torm migrate dev --schema prisma/schema.prisma --dir migrations
  torm migrate deploy --schema prisma/schema.prisma --dir migrations
  torm migrate reset --schema prisma/schema.prisma --dir migrations
  torm migrate status --schema prisma/schema.prisma --dir migrations`
}

// NewVersionCmd builds the `version` command.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(version())
		},
	}
}

// NewHelpCmd builds the `help` command.
func NewHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help",
		Short: "Print help information",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(help())
		},
	}
}

// NewRootCmd builds the top–level `torm` command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "torm",
		Short: "TORM — migrations and code generation",
	}
	root.AddCommand(NewMigrateCmd())
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewHelpCmd())
	return root
}
