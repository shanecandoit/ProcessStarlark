package main

import (
	"context"
	"os"
	"os/signal"

	"park/internal/cli"

	"golang.org/x/term"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	num := int(os.Stdin.Fd())
	c := &cli.CLI{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Terminal: term.IsTerminal(num)}
	os.Exit(c.Run(ctx, os.Args[1:]))
}
