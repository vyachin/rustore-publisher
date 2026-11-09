// Команда rustore-publisher публикует Android App Bundle в RuStore.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/vyachin/rustore-publisher/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx, cli.DefaultApp(), os.Args[1:]))
}
