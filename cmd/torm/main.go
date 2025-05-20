package main

import (
	"os"

	"github.com/TechXTT/TORM/pkg/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
