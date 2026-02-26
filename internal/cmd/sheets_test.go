package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/output"
)

func TestSheetsUpdateCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// AllowedActions set to unrelated action → sheets.update is denied.
	if err := config.WritePolicy(config.PolicyFile{
		AllowedActions: []string{"gmail.search"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `[["x"]]`,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
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
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "policy_denied" {
		t.Errorf("code = %q, want %q", payload.Code, "policy_denied")
	}
}

func TestSheetsUpdateCmd_InvalidValues(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `not-valid-json`,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON values, got nil")
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "invalid_values" {
		t.Errorf("code = %q, want %q", payload.Code, "invalid_values")
	}
}

func TestSheetsUpdateCmd_DryRun(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1:B1",
		Values:        `[["Alice",30]]`,
	}
	var stdout string
	var err error
	stdout = captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if !payload.DryRun {
		t.Error("expected dry_run=true")
	}
	if payload.Action != "sheets.update" {
		t.Errorf("action = %q, want %q", payload.Action, "sheets.update")
	}
}

func TestSheetsAppendCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		AllowedActions: []string{"gmail.search"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SheetsAppendCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1",
		Values:        `[["x"]]`,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
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
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "policy_denied" {
		t.Errorf("code = %q, want %q", payload.Code, "policy_denied")
	}
}

func TestSheetsAppendCmd_DryRun(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsAppendCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1",
		Values:        `[["Bob",25]]`,
	}
	var stdout string
	var err error
	stdout = captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if !payload.DryRun {
		t.Error("expected dry_run=true")
	}
	if payload.Action != "sheets.append" {
		t.Errorf("action = %q, want %q", payload.Action, "sheets.append")
	}
}
