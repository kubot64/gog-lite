package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/output"
	"github.com/kubot64/gog-lite/internal/secrets"
)

// captureStderr redirects os.Stderr for the duration of fn and returns the captured output.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	fn()

	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	os.Stderr = orig
	return string(b)
}

// captureStdout redirects os.Stdout for the duration of fn and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	fn()

	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	os.Stdout = orig
	return string(b)
}

func TestApprovalTokenCmd_InvalidTTL(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "not-a-duration",
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})

	if err == nil {
		t.Fatal("expected error for invalid TTL, got nil")
	}
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("expected exit code %d, got %d", output.ExitCodeError, output.ExitCode(err))
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "invalid_ttl" {
		t.Errorf("code = %q, want %q", payload.Code, "invalid_ttl")
	}
}

func TestApprovalTokenCmd_Valid(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "dummy-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "dummy-secret")

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "10m",
	}
	if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApprovalTokenCmd_ApprovalNotRequired(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "gmail.search",
		TTL:     "10m",
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
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
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "approval_not_required" {
		t.Errorf("code = %q, want %q", payload.Code, "approval_not_required")
	}
}

func TestApprovalTokenCmd_TargetActionPolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		AllowedActions: []string{"auth.approval_token"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "10m",
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

func TestApprovalTokenCmd_DryRun(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "10m",
	}
	var err error
	stdout := captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.DryRun {
		t.Fatal("expected dry_run=true")
	}
	if payload.Action != "auth.approval_token" {
		t.Fatalf("action = %q, want %q", payload.Action, "auth.approval_token")
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

func TestPreflightCmd_CredentialsMissing(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")

	cmd := &AuthPreflightCmd{Account: "test@example.com"}
	var err error
	stdout := captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})

	if err != nil {
		t.Fatalf("preflight should not return error even when not ready: %v", err)
	}
	var result struct {
		Ready  bool `json:"ready"`
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if result.Ready {
		t.Error("expected ready=false when credentials are missing")
	}
	if len(result.Checks) == 0 {
		t.Error("expected at least one check in result")
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == "credentials" && !c.OK {
			found = true
		}
	}
	if !found {
		t.Errorf("expected credentials check to fail; checks=%+v", result.Checks)
	}
}

func TestEmergencyRevokeCmd_BlocksAccount(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	t.Setenv("GOG_LITE_KEYRING_PASSWORD", "test-password")
	t.Setenv("GOG_LITE_CLIENT_ID", "dummy-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "dummy-secret")

	account := "victim@example.com"

	// Pre-store a dummy token so DeleteToken doesn't fail on a missing file.
	store, err := secrets.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	if err := store.SetToken(account, secrets.Token{RefreshToken: "dummy-refresh"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Errorf("emergency-revoke failed: %v", err)
		}
	})

	p, err := config.ReadPolicy()
	if err != nil {
		t.Fatalf("ReadPolicy: %v", err)
	}
	for _, blocked := range p.BlockedAccounts {
		if blocked == account {
			return
		}
	}
	t.Errorf("expected %q to be in blocked_accounts, got %v", account, p.BlockedAccounts)
}
