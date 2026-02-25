package main

import (
	"context"
	"os"

	"github.com/kubot64/gog-lite/internal/cmd"
	"github.com/kubot64/gog-lite/internal/output"
)

func main() {
	ctx := context.Background()
	if err := cmd.Execute(ctx); err != nil {
		os.Exit(output.ExitCode(err))
	}
}
