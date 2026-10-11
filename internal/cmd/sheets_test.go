package cmd

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

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
		Params struct {
			ValueInput string `json:"value_input"`
		} `json:"params"`
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
	if payload.Params.ValueInput != "RAW" {
		t.Errorf("value_input = %q, want RAW", payload.Params.ValueInput)
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
		Params struct {
			ValueInput string `json:"value_input"`
		} `json:"params"`
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
	if payload.Params.ValueInput != "RAW" {
		t.Errorf("value_input = %q, want RAW", payload.Params.ValueInput)
	}
}

func TestSheetsUpdateCmd_RequiresConfirmation(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `[["=SUM(A1)"]]`,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err == nil {
		t.Fatal("expected error when --confirm-write is not set")
	}
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("expected ExitCodeError, got %d", output.ExitCode(err))
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "write_requires_confirmation" {
		t.Errorf("code = %q, want %q", payload.Code, "write_requires_confirmation")
	}
}

func TestSheetsUpdateCmd_RequiresApprovalToken(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `[["x"]]`,
		ConfirmWrite:  true,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err == nil {
		t.Fatal("expected error when approval token is missing")
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
	if payload.Code != "approval_required" {
		t.Errorf("code = %q, want %q", payload.Code, "approval_required")
	}
}

func TestSheetsUpdateCmd_DryRunSkipsConfirmAndApproval(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `[["=SUM(A1)"]]`,
		ValueInput:    "USER_ENTERED",
	}
	var stdout string
	var err error
	stdout = captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	var payload struct {
		Params struct {
			ValueInput string `json:"value_input"`
		} `json:"params"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if payload.Params.ValueInput != "USER_ENTERED" {
		t.Fatalf("value_input = %q, want USER_ENTERED", payload.Params.ValueInput)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if runErr == nil {
		t.Fatal("expected confirmation error after dry-run")
	}
	var errPayload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errPayload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if errPayload.Code != "write_requires_confirmation" {
		t.Fatalf("code = %q, want write_requires_confirmation (dry-run must not consume approval)", errPayload.Code)
	}
}

func TestSheetsUpdateCmd_DryRunDoesNotConsumeApprovalToken(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")

	token, _, err := issueApprovalToken("a@example.com", "sheets.update", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `[["x"]]`,
		ConfirmWrite:  true,
		ApprovalToken: token,
	}
	if err := cmd.Run(context.Background(), &RootFlags{DryRun: true}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if runErr == nil {
		t.Fatal("expected an error after the token is accepted")
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code == "approval_required" {
		t.Fatalf("dry-run consumed the approval token: %s", stderr)
	}

	stderr = captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "approval_required" {
		t.Fatalf("code = %q, want approval_required after the token is used", payload.Code)
	}
}

func TestSheetsUpdateCmd_InvalidValueInput(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1!A1",
		Values:        `[["x"]]`,
		ValueInput:    "INPUT_VALUE",
		ConfirmWrite:  true,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err == nil {
		t.Fatal("expected error for invalid value input")
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "invalid_value_input" {
		t.Errorf("code = %q, want %q", payload.Code, "invalid_value_input")
	}
}

func TestSheetsAppendCmd_UserEnteredDryRun(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SheetsAppendCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp-123",
		Range:         "Sheet1",
		Values:        `[["=SUM(A1)"]]`,
		ValueInput:    "USER_ENTERED",
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
		Params struct {
			ValueInput string `json:"value_input"`
		} `json:"params"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if payload.Params.ValueInput != "USER_ENTERED" {
		t.Errorf("value_input = %q, want USER_ENTERED", payload.Params.ValueInput)
	}
}

func TestExecute_InvalidSheetsValueInput(t *testing.T) {
	prevArgs := os.Args
	t.Cleanup(func() { os.Args = prevArgs })
	os.Args = []string{
		"gog-lite", "--dry-run", "sheets", "update",
		"--account", "a@example.com",
		"--spreadsheet-id", "sp",
		"--range", "Sheet1!A1",
		"--values", `[["a"]]`,
		"--value-input", "FORMULA",
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = Execute(context.Background(), "dev")
	})
	if runErr == nil {
		t.Fatal("expected invalid --value-input to be rejected")
	}
	if output.ExitCode(runErr) != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d", output.ExitCode(runErr), output.ExitCodeError)
	}
	if code := stderrErrorCode(t, stderr); code != "invalid_arguments" {
		t.Fatalf("code = %q, want invalid_arguments (kong enum rejects the flag before Run)", code)
	}
}

func TestToInterfaceSliceCopiesRows(t *testing.T) {
	rows := [][]any{
		{"a", 1},
		nil,
		{},
	}
	got := toInterfaceSlice(rows)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if len(got[0]) != 2 || got[0][0] != "a" || got[0][1] != 1 {
		t.Fatalf("row 0 = %#v", got[0])
	}
	if got[1] == nil || len(got[1]) != 0 {
		t.Fatalf("nil row = %#v, want empty non-nil slice", got[1])
	}
	if got[2] == nil || len(got[2]) != 0 {
		t.Fatalf("empty row = %#v, want empty non-nil slice", got[2])
	}

	rows[0][0] = "changed"
	if got[0][0] != "a" {
		t.Fatalf("row 0 shares the input backing array: %#v", got[0])
	}
}
