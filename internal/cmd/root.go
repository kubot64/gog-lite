package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/kubot64/gog-lite/internal/output"
)

// RootFlags are flags available to all commands.
type RootFlags struct {
	Verbose          bool   `name:"verbose" short:"v" help:"Enable verbose logging."`
	DryRun           bool   `name:"dry-run" short:"n" help:"Print what would be done without executing."`
	AuditLog         string `name:"audit-log" help:"Append write-action audit logs as JSON lines to this file path."`
	AllowedOutputDir string `name:"allowed-output-dir" help:"Restrict write outputs to this directory (and its children)."`
}

// CLI is the top-level command structure.
type CLI struct {
	RootFlags `embed:""`
	Auth      AuthCmd     `cmd:"" help:"Manage Google account authentication."`
	Gmail     GmailCmd    `cmd:"" help:"Gmail operations."`
	Calendar  CalendarCmd `cmd:"" help:"Google Calendar operations."`
	Docs      DocsCmd     `cmd:"" help:"Google Docs operations."`
	Sheets    SheetsCmd   `cmd:"" help:"Google Sheets operations."`
	Slides    SlidesCmd   `cmd:"" help:"Google Slides operations."`
}

// Execute parses CLI arguments and runs the selected command.
func Execute(ctx context.Context, version string) error {
	cli := &CLI{}
	args := os.Args[1:]
	resolvedVersion := resolveVersion(version)
	var parserStdout bytes.Buffer
	var parserStderr bytes.Buffer

	if hasAnyFlag(args, "--version") {
		return output.WriteJSON(os.Stdout, map[string]any{
			"version": resolvedVersion,
		})
	}

	k, err := kong.New(cli,
		kong.Name("gog-lite"),
		kong.Description("AI-agent-friendly CLI for Gmail, Calendar, and Docs."),
		kong.NoDefaultHelp(),
		kong.UsageOnError(),
		kong.Writers(&parserStdout, &parserStderr),
		kong.Vars{"version": resolvedVersion},
	)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "parser_error", fmt.Sprintf("create parser: %v", err))
	}

	if hasAnyFlag(args, "--help", "-h") {
		helpArgs := filterFlags(args, "--help", "-h")
		helpCtx, parseErr := k.Parse(helpArgs)
		if parseErr != nil {
			var pe *kong.ParseError
			if errors.As(parseErr, &pe) && pe.Context != nil {
				helpCtx = pe.Context
			}
		}
		if helpCtx == nil {
			return output.WriteError(output.ExitCodeError, "help_error", "build help context")
		}

		var help bytes.Buffer
		prevStdout := k.Stdout
		k.Stdout = &help
		printErr := helpCtx.PrintUsage(false)
		k.Stdout = prevStdout
		if printErr != nil {
			return output.WriteError(output.ExitCodeError, "help_error", printErr.Error())
		}

		return output.WriteJSON(os.Stdout, map[string]any{
			"help": strings.TrimRight(help.String(), "\n"),
		})
	}

	kctx, err := k.Parse(args)
	if err != nil {
		msg := strings.TrimSpace(parserStderr.String())
		if msg == "" {
			msg = err.Error()
		}

		return output.WriteError(output.ExitCodeError, "invalid_arguments", msg)
	}

	// Bind context.Context as the interface type (not the concrete type).
	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)

	return kctx.Run()
}

func resolveVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "dev"
	}

	return v
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag {
				return true
			}
		}
	}

	return false
}

func filterFlags(args []string, flags ...string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		drop := false
		for _, flag := range flags {
			if arg == flag {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, arg)
		}
	}

	return out
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
