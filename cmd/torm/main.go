package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/TechXTT/TORM/v2/pkg/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
