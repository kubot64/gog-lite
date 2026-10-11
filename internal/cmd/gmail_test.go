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

func TestValidateHeaderValue_Valid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "subject", value: "Weekly report"},
		{name: "to", value: "alice@example.com"},
		{name: "cc", value: "alice@example.com,bob@example.com"},
		{name: "account", value: "sender@example.com"},
		{name: "bcc", value: ""},
	} {
		if err := validateHeaderValue(tc.name, tc.value); err != nil {
			t.Errorf("validateHeaderValue(%q, %q): unexpected error: %v", tc.name, tc.value, err)
		}
	}
}

func TestValidateHeaderValue_RejectsCRLF(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "subject", value: "ok\r\nBcc: evil@example.com"},
		{name: "to", value: "alice@example.com\nCc:evil@example.com"},
		{name: "account", value: "sender@example.com\rbad"},
	} {
		if err := validateHeaderValue(tc.name, tc.value); err == nil {
			t.Errorf("validateHeaderValue(%q, %q): expected error, got nil", tc.name, tc.value)
		}
	}
}

func TestValidateAddressList_Valid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "to", value: "alice@example.com"},
		{name: "cc", value: "Alice <alice@example.com>, bob@example.com"},
		{name: "bcc", value: ""},
	} {
		if err := validateAddressList(tc.name, tc.value); err != nil {
			t.Errorf("validateAddressList(%q, %q): unexpected error: %v", tc.name, tc.value, err)
		}
	}
}

func TestValidateAddressList_Invalid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "to", value: ""},
		{name: "to", value: "not-an-email"},
		{name: "cc", value: "alice@example.com, bad"},
	} {
		if err := validateAddressList(tc.name, tc.value); err == nil {
			t.Errorf("validateAddressList(%q, %q): expected error, got nil", tc.name, tc.value)
		}
	}
}

func TestGmailSendCmd_DryRun(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &GmailSendCmd{
		Account: "Sender@Example.com",
		To:      "alice@example.com",
		CC:      "carol@example.com",
		BCC:     "dave@example.com",
		Subject: "Weekly report",
		Body:    "hello",
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
			Account    string `json:"account"`
			To         string `json:"to"`
			CC         string `json:"cc"`
			BCC        string `json:"bcc"`
			Subject    string `json:"subject"`
			BodyLength int    `json:"body_length"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.DryRun {
		t.Error("expected dry_run=true")
	}
	if payload.Action != "gmail.draft" {
		t.Errorf("action = %q, want %q", payload.Action, "gmail.draft")
	}
	if payload.Params.Account != "Sender@Example.com" || payload.Params.To != "alice@example.com" {
		t.Errorf("params account/to = %+v", payload.Params)
	}
	if payload.Params.CC != "carol@example.com" || payload.Params.BCC != "dave@example.com" {
		t.Errorf("params cc/bcc = %+v", payload.Params)
	}
	if payload.Params.Subject != "Weekly report" || payload.Params.BodyLength != len("hello") {
		t.Errorf("params subject/body_length = %+v", payload.Params)
	}

	auditPath, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}
	b, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var entry auditEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &entry); err != nil {
		t.Fatalf("parse audit log: %v (got %q)", err, b)
	}
	if entry.Action != "gmail.draft" || !entry.DryRun {
		t.Errorf("audit entry = %+v, want action gmail.draft dry_run=true", entry)
	}
	if entry.Account != "sender@example.com" || entry.Target != "alice@example.com" {
		t.Errorf("audit account/target = %q / %q", entry.Account, entry.Target)
	}

	ratePath, err := rateLimitPath("gmail.send")
	if err != nil {
		t.Fatalf("rateLimitPath: %v", err)
	}
	if _, err := os.Stat(ratePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not write rate limit state, stat err=%v", err)
	}
}

func TestGmailSendCmd_DryRunBodyStdin(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &GmailSendCmd{
		Account:   "sender@example.com",
		To:        "alice@example.com",
		Subject:   "report",
		Body:      "ignored",
		BodyStdin: true,
	}

	var stdout string
	var runErr error
	_, err := withStdin(t, "from stdin", func() (string, error) {
		stdout = captureStdout(t, func() {
			runErr = cmd.Run(context.Background(), &RootFlags{DryRun: true})
		})
		return "", runErr
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload struct {
		DryRun bool `json:"dry_run"`
		Params struct {
			BodyLength int `json:"body_length"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.DryRun {
		t.Error("expected dry_run=true")
	}
	if payload.Params.BodyLength != len("from stdin") {
		t.Errorf("body_length = %d, want %d", payload.Params.BodyLength, len("from stdin"))
	}
}

func TestGmailSendCmd_DryRunRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  GmailSendCmd
		code string
	}{
		{
			name: "header",
			cmd: GmailSendCmd{
				Account: "sender@example.com",
				To:      "alice@example.com",
				Subject: "ok\r\nBcc: evil@example.com",
				Body:    "hello",
			},
			code: "invalid_header",
		},
		{
			name: "recipient",
			cmd: GmailSendCmd{
				Account: "sender@example.com",
				To:      "not-an-email",
				Subject: "report",
				Body:    "hello",
			},
			code: "invalid_recipient",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfgHome := t.TempDir()
			t.Setenv("HOME", cfgHome)
			t.Setenv("XDG_CONFIG_HOME", cfgHome)

			var err error
			stderr := captureStderr(t, func() {
				err = tc.cmd.Run(context.Background(), &RootFlags{DryRun: true})
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if output.ExitCode(err) != output.ExitCodeError {
				t.Fatalf("expected ExitCodeError, got %d", output.ExitCode(err))
			}
			var payload struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
				t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
			}
			if payload.Code != tc.code {
				t.Errorf("code = %q, want %q", payload.Code, tc.code)
			}

			auditPath, err := resolveAuditLogPath("")
			if err != nil {
				t.Fatalf("resolveAuditLogPath: %v", err)
			}
			if _, err := os.Stat(auditPath); !os.IsNotExist(err) {
				t.Fatalf("invalid input should not write audit log, stat err=%v", err)
			}
		})
	}
}

func TestGmailSendCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		AllowedActions: []string{"gmail.search"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &GmailSendCmd{
		Account: "sender@example.com",
		To:      "alice@example.com",
		Subject: "report",
		Body:    "hello",
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
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
		t.Errorf("code = %q, want %q", payload.Code, "policy_denied")
	}
}

func TestGmailSendCmd_RateLimited(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := rateLimitPath("gmail.send")
	if err != nil {
		t.Fatalf("rateLimitPath: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	stamps := make([]string, 20)
	for i := range stamps {
		stamps[i] = now
	}
	if err := saveRateLimitState(path, rateLimitState{Timestamps: stamps}); err != nil {
		t.Fatalf("saveRateLimitState: %v", err)
	}

	cmd := &GmailSendCmd{
		Account: "sender@example.com",
		To:      "alice@example.com",
		Subject: "report",
		Body:    "hello",
	}
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{DryRun: false})
	})
	if runErr == nil {
		t.Fatal("expected error, got nil")
	}
	if output.ExitCode(runErr) != output.ExitCodeError {
		t.Fatalf("expected ExitCodeError, got %d", output.ExitCode(runErr))
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	if payload.Code != "rate_limited" {
		t.Errorf("code = %q, want %q", payload.Code, "rate_limited")
	}
}
