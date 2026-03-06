package output_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kubot64/gog-lite/internal/output"
)

func TestWriteJSON_NoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	err := output.WriteJSON(&buf, map[string]string{"url": "https://example.com/a?b=1&c=2<3"})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, `\u003c`) {
		t.Errorf("URL was HTML-escaped: %s", got)
	}
	if !strings.Contains(got, "https://example.com/a?b=1&c=2<3") {
		t.Errorf("URL not preserved in output: %s", got)
	}
}

func TestWriteJSON_Indented(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "  ") {
		t.Errorf("expected indented JSON, got: %s", buf.String())
	}
}

func TestWriteJSON_AddsSharedSuccessMetadata(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]any{
		"messages":      []map[string]string{{"id": "m1"}},
		"nextPageToken": "token-123",
	}); err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK            bool   `json:"ok"`
		ResourceType  string `json:"resource_type"`
		DryRun        bool   `json:"dry_run"`
		HasMore       bool   `json:"has_more"`
		NextPageToken string `json:"nextPageToken"`
		NextPageSnake string `json:"next_page_token"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if !payload.OK {
		t.Fatal("expected ok=true")
	}
	if payload.ResourceType != "message" {
		t.Fatalf("resource_type = %q", payload.ResourceType)
	}
	if payload.DryRun {
		t.Fatal("expected dry_run=false default")
	}
	if !payload.HasMore || payload.NextPageToken != "token-123" || payload.NextPageSnake != "token-123" {
		t.Fatalf("unexpected pagination metadata: %+v", payload)
	}
}

func TestWriteJSON_InfersActionAccountAndTarget(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]any{
		"dry_run": true,
		"action":  "docs.export",
		"params": map[string]any{
			"account": "you@example.com",
			"output":  "/tmp/out.pdf",
		},
	}); err != nil {
		t.Fatal(err)
	}

	var payload struct {
		Action  string `json:"action"`
		Account string `json:"account"`
		Target  string `json:"target"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if payload.Action != "docs.export" || payload.Account != "you@example.com" || payload.Target != "/tmp/out.pdf" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestExitCode_Nil(t *testing.T) {
	if code := output.ExitCode(nil); code != output.ExitCodeOK {
		t.Errorf("want %d, got %d", output.ExitCodeOK, code)
	}
}

func TestExitCode_ExitCodeErr(t *testing.T) {
	err := output.NewError(output.ExitCodeAuth, fmt.Errorf("auth failed"))
	if code := output.ExitCode(err); code != output.ExitCodeAuth {
		t.Errorf("want %d, got %d", output.ExitCodeAuth, code)
	}
}

func TestExitCode_WrappedExitCodeErr(t *testing.T) {
	inner := output.NewError(output.ExitCodeNotFound, fmt.Errorf("not found"))
	wrapped := fmt.Errorf("outer: %w", inner)
	if code := output.ExitCode(wrapped); code != output.ExitCodeNotFound {
		t.Errorf("want %d, got %d", output.ExitCodeNotFound, code)
	}
}

func TestExitCode_PlainError(t *testing.T) {
	if code := output.ExitCode(fmt.Errorf("some error")); code != output.ExitCodeError {
		t.Errorf("want %d, got %d", output.ExitCodeError, code)
	}
}

func TestExitCodeErr_ErrorString(t *testing.T) {
	err := output.NewError(output.ExitCodeAuth, fmt.Errorf("auth failed"))
	if err.Error() != "auth failed" {
		t.Errorf("want %q, got %q", "auth failed", err.Error())
	}
}

func TestExitCodeErr_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	err := output.NewError(output.ExitCodeError, inner)
	if err.Unwrap() != inner {
		t.Errorf("Unwrap() should return the inner error")
	}
}

func TestWriteError_AddsRecoveryMetadata(t *testing.T) {
	oldStderr := outputStderrSwap(t)
	defer oldStderr()

	err := output.WriteError(output.ExitCodePermission, "approval_required", "approval token required")
	if code := output.ExitCode(err); code != output.ExitCodePermission {
		t.Fatalf("exit code = %d", code)
	}

	var payload struct {
		Code          string   `json:"code"`
		Retryable     bool     `json:"retryable"`
		NextAction    string   `json:"next_action"`
		MissingTokens []string `json:"missing_tokens"`
	}
	if err := json.Unmarshal(stderrBuffer.Bytes(), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v", err)
	}
	if payload.Code != "approval_required" || payload.Retryable || payload.NextAction != "request_approval_token" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(payload.MissingTokens) != 1 || payload.MissingTokens[0] != "approval-token" {
		t.Fatalf("unexpected missing_tokens: %+v", payload.MissingTokens)
	}
}

func TestWriteError_AddsCommandHintForAuthRequired(t *testing.T) {
	oldStderr := outputStderrSwap(t)
	defer oldStderr()

	_ = output.WriteError(output.ExitCodeAuth, "auth_required", "auth required for gmail you@example.com; run: gog-lite auth login --account you@example.com --services gmail")

	var payload struct {
		Code        string `json:"code"`
		NextAction  string `json:"next_action"`
		CommandHint string `json:"command_hint"`
	}
	if err := json.Unmarshal(stderrBuffer.Bytes(), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v", err)
	}
	if payload.Code != "auth_required" || payload.NextAction != "authenticate_account" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.CommandHint != "gog-lite auth login --account you@example.com --services gmail" {
		t.Fatalf("command_hint = %q", payload.CommandHint)
	}
}

var stderrBuffer bytes.Buffer

func outputStderrSwap(t *testing.T) func() {
	t.Helper()
	stderrBuffer.Reset()
	orig := output.Stderr
	output.Stderr = &stderrBuffer

	return func() {
		output.Stderr = orig
	}
}
