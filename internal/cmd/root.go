package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

// RootFlags are flags available to all commands.
type RootFlags struct {
	Verbose  bool   `name:"verbose" short:"v" help:"Enable verbose logging."`
	DryRun   bool   `name:"dry-run" short:"n" help:"Print what would be done without executing."`
	AuditLog string `name:"audit-log" help:"Append write-action audit logs as JSON lines to this file path."`
}

// CLI is the top-level command structure.
type CLI struct {
	RootFlags `embed:""`
	Version   kong.VersionFlag `name:"version" help:"Print version and exit."`
	Auth      AuthCmd          `cmd:"" help:"Manage Google account authentication."`
	Gmail     GmailCmd         `cmd:"" help:"Gmail operations."`
	Calendar  CalendarCmd      `cmd:"" help:"Google Calendar operations."`
	Docs      DocsCmd          `cmd:"" help:"Google Docs operations."`
}

// Execute parses CLI arguments and runs the selected command.
func Execute(ctx context.Context) error {
	cli := &CLI{}

	k, err := kong.New(cli,
		kong.Name("gog-lite"),
		kong.Description("AI-agent-friendly CLI for Gmail, Calendar, and Docs."),
		kong.UsageOnError(),
		kong.Vars{"version": "0.1.0"},
	)
	if err != nil {
		return fmt.Errorf("create parser: %w", err)
	}

	kctx, err := k.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	// Bind context.Context as the interface type (not the concrete type).
	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)

	return kctx.Run()
}

// collectAllPages calls fn repeatedly until nextPageToken is empty or allPages is false.
// fn receives the current page token and returns (nextPageToken, items, error).
func collectAllPages[T any](allPages bool, fn func(pageToken string) (string, []T, error)) ([]T, string, error) {
	var result []T
	pageToken := ""

	for {
		next, items, err := fn(pageToken)
		if err != nil {
			return nil, "", err
		}

		result = append(result, items...)

		if !allPages || next == "" {
			return result, next, nil
		}

		pageToken = next
	}
}
