package main

import (
	"context"
	"os"
	"os/signal"

	"park/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := &cli.CLI{Out: os.Stdout, Err: os.Stderr}
	os.Exit(c.Run(ctx, os.Args[1:]))
}
