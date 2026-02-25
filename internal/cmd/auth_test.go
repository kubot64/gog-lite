package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/morikubo-takashi/gog-lite/internal/output"
)

func TestApprovalTokenCmd_InvalidTTL(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "not-a-duration",
	}
	err := cmd.Run(context.Background(), &RootFlags{})
	if err == nil {
		t.Fatal("expected error for invalid TTL, got nil")
	}
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("expected exit code %d, got %d", output.ExitCodeError, output.ExitCode(err))
	}
}

func TestApprovalTokenCmd_Valid(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	// Set env vars to avoid credentials file lookup in audit path.
	t.Setenv("GOG_LITE_CLIENT_ID", "dummy-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "dummy-secret")

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "10m",
	}
	// Run writes JSON to stdout; we only care that it doesn't error.
	if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIssueApprovalToken_NegativeTTL(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	_, _, err := issueApprovalToken("a@example.com", "calendar.delete", -time.Minute)
	if err == nil {
		t.Fatal("expected error for negative TTL")
	}
}
