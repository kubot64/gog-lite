package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/output"
)

func TestGmailSearchCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"calendar.list"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &GmailSearchCmd{
		Account: "a@example.com",
		Query:   "is:unread",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestGmailThreadCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"calendar.list"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &GmailThreadCmd{
		Account:  "a@example.com",
		ThreadID: "thread-123",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestGmailLabelsCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"calendar.list"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &GmailLabelsCmd{
		Account: "a@example.com",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestCalendarListCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &CalendarListCmd{
		Account:    "a@example.com",
		CalendarID: "primary",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestCalendarCalendarsCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &CalendarCalendarsCmd{
		Account: "a@example.com",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestCalendarGetCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &CalendarGetCmd{
		Account:    "a@example.com",
		EventID:    "event-123",
		CalendarID: "primary",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestDocsCatCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &DocsCatCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestDocsInfoCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &DocsInfoCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestSheetsGetCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SheetsGetCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1:B2",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestSheetsInfoCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SheetsInfoCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestSlidesGetCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SlidesGetCmd{
		Account:        "a@example.com",
		PresentationID: "pres-123",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func TestSlidesInfoCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{AllowedActions: []string{"gmail.search"}}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SlidesInfoCmd{
		Account:        "a@example.com",
		PresentationID: "pres-123",
	}
	assertPolicyDenied(t, func() error {
		return cmd.Run(context.Background(), &RootFlags{})
	})
}

func assertPolicyDenied(t *testing.T, run func() error) {
	t.Helper()

	var err error
	stderr := captureStderr(t, func() {
		err = run()
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output.ExitCode(err) != output.ExitCodePermission {
		t.Fatalf("expected ExitCodePermission, got %d", output.ExitCode(err))
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	if payload.Code != "policy_denied" {
		t.Fatalf("code = %q, want %q", payload.Code, "policy_denied")
	}
}
