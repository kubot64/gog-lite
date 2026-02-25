package main

import (
	"context"
	"os"

	"github.com/morikubo-takashi/gog-lite/internal/cmd"
	"github.com/morikubo-takashi/gog-lite/internal/output"
)

func main() {
	ctx := context.Background()
	if err := cmd.Execute(ctx); err != nil {
		os.Exit(output.ExitCode(err))
	}
}
