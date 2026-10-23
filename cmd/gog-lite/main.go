package main

import (
	"context"
	"os"

	"github.com/kubot64/gog-lite/internal/cmd"
	"github.com/kubot64/gog-lite/internal/googleauth"
	"github.com/kubot64/gog-lite/internal/output"
)

// version is injected at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	if handled, err := googleauth.RunCallbackHolderIfRequested(); handled {
		if err != nil {
			_ = output.WriteError(output.ExitCodeError, "oauth_callback_error", err.Error())
			os.Exit(output.ExitCodeError)
		}
		return
	}

	ctx := context.Background()
	if err := cmd.Execute(ctx, version); err != nil {
		os.Exit(output.ExitCode(err))
	}
}
