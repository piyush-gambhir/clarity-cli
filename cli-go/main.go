package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/piyush-gambhir/clarity-cli/cli-go/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cmd.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
