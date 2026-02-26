package main

import (
	"context"
	"os"

	"github.com/kubot64/gog-lite/internal/cmd"
	"github.com/kubot64/gog-lite/internal/output"
)

// version is injected at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	ctx := context.Background()
	if err := cmd.Execute(ctx, version); err != nil {
		os.Exit(output.ExitCode(err))
	}
}
