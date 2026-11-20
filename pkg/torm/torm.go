package torm

import (
	"os"

	"github.com/TechXTT/TORM/v2/pkg/cli"
)

// Deprecated: use the CLI or cli.NewRootCmd.
func RunMigrate(args []string) {
	root := cli.NewRootCmd()
	root.SetArgs(append([]string{"migrate"}, args...))
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
