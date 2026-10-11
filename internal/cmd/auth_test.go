package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/googleauth"
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

func TestEmergencyRevokeCmd_BlocksAccountAfterRemoteRevoke(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	var gotToken string
	calls := 0
	stubRevoke(t, func(_ context.Context, token string) error {
		calls++
		gotToken = token
		if _, err := store.GetToken(account); err != nil {
			t.Errorf("local token was removed before remote revoke: %v", err)
		}
		return nil
	})

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	var err error
	stdout := captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err != nil {
		t.Fatalf("emergency-revoke failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("revoke calls = %d, want 1", calls)
	}
	if gotToken != refreshToken {
		t.Fatalf("revoked token = %q, want %q", gotToken, refreshToken)
	}

	var payload struct {
		Revoked        bool   `json:"revoked"`
		AlreadyRevoked bool   `json:"already_revoked"`
		Blocked        bool   `json:"blocked"`
		Email          string `json:"email"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.Revoked || payload.AlreadyRevoked || !payload.Blocked || payload.Email != account {
		t.Fatalf("payload = %+v", payload)
	}

	if _, err := store.GetToken(account); err == nil {
		t.Fatal("expected local token to be deleted after remote revoke")
	}

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

func TestEmergencyRevokeCmd_AlreadyRevokedDeletesAndBlocks(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	stubRevoke(t, func(context.Context, string) error {
		return googleauth.ErrAlreadyRevoked
	})

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	var err error
	stdout := captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err != nil {
		t.Fatalf("emergency-revoke failed: %v", err)
	}

	var payload struct {
		Revoked        bool   `json:"revoked"`
		AlreadyRevoked bool   `json:"already_revoked"`
		Blocked        bool   `json:"blocked"`
		Email          string `json:"email"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.Revoked || !payload.AlreadyRevoked || !payload.Blocked || payload.Email != account {
		t.Fatalf("payload = %+v", payload)
	}
	if _, err := store.GetToken(account); err == nil {
		t.Fatal("expected local token to be deleted when Google reports invalid_token")
	}
	assertAccountBlocked(t, account)
}

func TestEmergencyRevokeCmd_UnavailableBlocksAndKeepsToken(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	stubRevoke(t, func(context.Context, string) error {
		return fmt.Errorf("%w: HTTP 503", googleauth.ErrRevokeUnavailable)
	})

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d", output.ExitCode(err), output.ExitCodeError)
	}

	payload := decodeErrorPayload(t, stderr)
	if payload.Code != "remote_revoke_failed" {
		t.Errorf("code = %q, want %q", payload.Code, "remote_revoke_failed")
	}
	if !strings.Contains(payload.Error, "リモート失効に失敗した") || !strings.Contains(payload.Error, "ブロック済み") || !strings.Contains(payload.Error, "再実行") {
		t.Errorf("error = %q", payload.Error)
	}
	if strings.Contains(payload.Error, refreshToken) {
		t.Fatalf("error leaked refresh token: %q", payload.Error)
	}

	got, err := store.GetToken(account)
	if err != nil {
		t.Fatalf("local token was deleted after unavailable revoke: %v", err)
	}
	if got.RefreshToken != refreshToken {
		t.Fatalf("refresh token = %q, want %q", got.RefreshToken, refreshToken)
	}
	assertAccountBlocked(t, account)
}

func TestEmergencyRevokeCmd_RejectedRevokeBlocksAndKeepsToken(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	stubRevoke(t, func(_ context.Context, token string) error {
		if token != refreshToken {
			t.Errorf("token = %q, want %q", token, refreshToken)
		}
		return fmt.Errorf("%w: HTTP 400 invalid_client", googleauth.ErrRevokeRejected)
	})

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d", output.ExitCode(err), output.ExitCodeError)
	}

	payload := decodeErrorPayload(t, stderr)
	if payload.Code != "remote_revoke_failed" {
		t.Errorf("code = %q, want %q", payload.Code, "remote_revoke_failed")
	}
	if !strings.Contains(payload.Error, "リモート失効に失敗した") || !strings.Contains(payload.Error, "ブロック済み") || !strings.Contains(payload.Error, "再実行") {
		t.Errorf("error = %q", payload.Error)
	}
	if !strings.Contains(payload.Error, "invalid_client") {
		t.Errorf("error = %q, want the Google error code", payload.Error)
	}
	if strings.Contains(payload.Error, refreshToken) {
		t.Fatalf("error leaked refresh token: %q", payload.Error)
	}

	got, err := store.GetToken(account)
	if err != nil {
		t.Fatalf("local token was deleted after rejected revoke: %v", err)
	}
	if got.RefreshToken != refreshToken {
		t.Fatalf("refresh token = %q, want %q", got.RefreshToken, refreshToken)
	}
	assertAccountBlocked(t, account)
}

func TestEmergencyRevokeCmd_UnexpectedRevokeErrorBlocksAndKeepsToken(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	stubRevoke(t, func(context.Context, string) error {
		return fmt.Errorf("unexpected revoke failure")
	})

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d", output.ExitCode(err), output.ExitCodeError)
	}

	payload := decodeErrorPayload(t, stderr)
	if payload.Code != "remote_revoke_failed" {
		t.Errorf("code = %q, want %q", payload.Code, "remote_revoke_failed")
	}
	if !strings.Contains(payload.Error, "リモート失効に失敗した") || !strings.Contains(payload.Error, "ブロック済み") || !strings.Contains(payload.Error, "再実行") {
		t.Errorf("error = %q", payload.Error)
	}
	if strings.Contains(payload.Error, refreshToken) {
		t.Fatalf("error leaked refresh token: %q", payload.Error)
	}

	got, err := store.GetToken(account)
	if err != nil {
		t.Fatalf("local token was deleted after unexpected revoke error: %v", err)
	}
	if got.RefreshToken != refreshToken {
		t.Fatalf("refresh token = %q, want %q", got.RefreshToken, refreshToken)
	}
	assertAccountBlocked(t, account)
}

func TestEmergencyRevokeCmd_DryRunSkipsRevoke(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	calls := 0
	stubRevoke(t, func(context.Context, string) error {
		calls++
		return nil
	})

	cmd := &AuthEmergencyRevokeCmd{Account: account}
	var err error
	stdout := captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if calls != 0 {
		t.Fatalf("revoke calls = %d, want 0", calls)
	}

	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
		Params struct {
			Account string `json:"account"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.DryRun || payload.Action != "auth.emergency_revoke" || payload.Params.Account != account {
		t.Fatalf("payload = %+v", payload)
	}

	got, err := store.GetToken(account)
	if err != nil {
		t.Fatalf("dry-run deleted the token: %v", err)
	}
	if got.RefreshToken != refreshToken {
		t.Fatalf("refresh token = %q, want %q", got.RefreshToken, refreshToken)
	}
	assertAccountNotBlocked(t, account)

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	raw, err := os.ReadFile(configDir + "/gog-lite/audit.log")
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var entry struct {
		Action string `json:"action"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &entry); err != nil {
		t.Fatalf("parse audit log: %v (got %q)", err, raw)
	}
	if entry.Action != "auth.emergency_revoke" || !entry.DryRun {
		t.Fatalf("audit entry = %+v", entry)
	}
}

func TestEmergencyRevokeCmd_RerunDoesNotChangePolicy(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"
	const refreshToken = "dummy-refresh"

	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	stubRevoke(t, func(context.Context, string) error {
		return fmt.Errorf("%w: HTTP 400 invalid_client", googleauth.ErrRevokeRejected)
	})

	run := func() {
		t.Helper()
		cmd := &AuthEmergencyRevokeCmd{Account: "Victim@Example.com"}
		var err error
		stderr := captureStderr(t, func() {
			err = cmd.Run(context.Background(), &RootFlags{})
		})
		if output.ExitCode(err) != output.ExitCodeError {
			t.Fatalf("exit code = %d, want %d", output.ExitCode(err), output.ExitCodeError)
		}
		payload := decodeErrorPayload(t, stderr)
		if payload.Code != "remote_revoke_failed" {
			t.Fatalf("code = %q, want %q", payload.Code, "remote_revoke_failed")
		}
	}

	run()

	path, err := config.PolicyPath()
	if err != nil {
		t.Fatalf("PolicyPath: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}

	run()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("policy changed on rerun\nbefore: %s\nafter: %s", before, after)
	}

	p, err := config.ReadPolicy()
	if err != nil {
		t.Fatalf("ReadPolicy: %v", err)
	}
	count := 0
	for _, blocked := range p.BlockedAccounts {
		if blocked == account {
			count++
		}
	}
	if count != 1 || len(p.BlockedAccounts) != 1 {
		t.Fatalf("blocked_accounts = %v, want [%s]", p.BlockedAccounts, account)
	}

	got, err := store.GetToken(account)
	if err != nil {
		t.Fatalf("local token was deleted: %v", err)
	}
	if got.RefreshToken != refreshToken {
		t.Fatalf("refresh token = %q, want %q", got.RefreshToken, refreshToken)
	}
}

func TestEmergencyRevokeCmd_AlreadyBlockedLeavesPolicyUnchanged(t *testing.T) {
	setupFileKeyring(t)
	account := "victim@example.com"

	if _, err := config.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	path, err := config.PolicyPath()
	if err != nil {
		t.Fatalf("PolicyPath: %v", err)
	}
	raw := []byte("{\n  \"blocked_accounts\": [\"Victim@Example.com\", \"other@example.com\"]\n}\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	calls := 0
	stubRevoke(t, func(context.Context, string) error {
		calls++
		return nil
	})

	cmd := &AuthEmergencyRevokeCmd{Account: "Victim@Example.com"}
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if output.ExitCode(runErr) != output.ExitCodeNotFound {
		t.Fatalf("exit code = %d, want %d (stderr %q)", output.ExitCode(runErr), output.ExitCodeNotFound, stderr)
	}
	if calls != 0 {
		t.Fatalf("revoke calls = %d, want 0", calls)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if string(after) != string(raw) {
		t.Fatalf("policy changed although the address was already blocked\nbefore: %s\nafter: %s", raw, after)
	}
	assertAccountBlocked(t, account)
}

func TestEmergencyRevokeCmd_MissingTokenBlocksAccount(t *testing.T) {
	setupFileKeyring(t)

	calls := 0
	stubRevoke(t, func(context.Context, string) error {
		calls++
		return nil
	})

	cmd := &AuthEmergencyRevokeCmd{Account: "missing@example.com"}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err == nil {
		t.Fatal("expected error when refresh token is missing")
	}
	if output.ExitCode(err) != output.ExitCodeNotFound {
		t.Fatalf("exit code = %d, want %d", output.ExitCode(err), output.ExitCodeNotFound)
	}
	if calls != 0 {
		t.Fatalf("revoke calls = %d, want 0", calls)
	}

	var payload struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "remote_revoke_failed" {
		t.Errorf("code = %q, want %q", payload.Code, "remote_revoke_failed")
	}
	if !strings.Contains(payload.Error, "リモート失効に失敗した") || !strings.Contains(payload.Error, "ブロック済み") {
		t.Errorf("error = %q", payload.Error)
	}

	assertAccountBlocked(t, "missing@example.com")
}

func decodeErrorPayload(t *testing.T, stderr string) struct {
	Error string `json:"error"`
	Code  string `json:"code"`
} {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	return payload
}

func assertAccountBlocked(t *testing.T, account string) {
	t.Helper()
	p, err := config.ReadPolicy()
	if err != nil {
		t.Fatalf("ReadPolicy: %v", err)
	}
	for _, blocked := range p.BlockedAccounts {
		if blocked == account {
			return
		}
	}
	t.Fatalf("expected %q in blocked_accounts, got %v", account, p.BlockedAccounts)
}

func assertAccountNotBlocked(t *testing.T, account string) {
	t.Helper()
	p, err := config.ReadPolicy()
	if err != nil {
		t.Fatalf("ReadPolicy: %v", err)
	}
	for _, blocked := range p.BlockedAccounts {
		if blocked == account {
			t.Fatalf("expected %q not to be blocked, got %v", account, p.BlockedAccounts)
		}
	}
}

func TestAuthExchangeErrorMessage_TokenEndpointAndOtherErrors(t *testing.T) {
	const marker = "leak-marker-9f3c2a"

	plain := errors.New("state mismatch or expired state; run step 1 again")
	if got := authExchangeErrorMessage(plain); got != plain.Error() {
		t.Fatalf("plain error = %q, want %q", got, plain.Error())
	}

	wrapped := fmt.Errorf("exchange code: %w", &oauth2.RetrieveError{
		ErrorCode:        "invalid_client",
		ErrorDescription: "echo " + marker,
		ErrorURI:         "https://example.invalid/" + marker,
		Body:             []byte(`{"error":"invalid_client","error_description":"echo ` + marker + `"}`),
	})
	got := authExchangeErrorMessage(wrapped)
	if got != tokenEndpointFailureMessage+" (invalid_client)" {
		t.Fatalf("retrieve error = %q", got)
	}
	if strings.Contains(got, marker) || strings.Contains(got, "exchange code:") || strings.Contains(got, "error_description") {
		t.Fatalf("retrieve error leaked details: %q", got)
	}

	invalidGrant := &oauth2.RetrieveError{
		Body: []byte(`{"error":"invalid_grant","error_description":"echo ` + marker + `"}`),
	}
	got = authExchangeErrorMessage(invalidGrant)
	if got != tokenEndpointFailureMessage+" (invalid_grant)" || got == invalidGrantMessage || strings.Contains(got, marker) {
		t.Fatalf("invalid_grant = %q, want fixed token endpoint message", got)
	}

	// Response is nil. RetrieveError.Error would panic while formatting the body.
	unparsed := &oauth2.RetrieveError{Body: []byte("not-json " + marker)}
	got = authExchangeErrorMessage(unparsed)
	if got != tokenEndpointFailureMessage || strings.Contains(got, marker) {
		t.Fatalf("unparsed = %q, want fixed sentence", got)
	}
}

func TestExecute_AuthLoginExchangeOmitsBody(t *testing.T) {
	const marker = "leak-marker-9f3c2a"
	const clientSecret = "super-secret-client-secret"
	const account = "test@example.com"
	const state = "teststate"
	const authCode = "auth-code-" + marker
	const redirectURI = "http://127.0.0.1:9/oauth2/callback"

	tests := []struct {
		name        string
		contentType string
		body        func(code, verifier string) string
		wantOAuth   string
	}{
		{
			name:        "invalid_client",
			contentType: "application/json",
			body: func(code, verifier string) string {
				return `{"error":"invalid_client","error_description":"echo ` + marker + ` ` + code + ` ` + verifier + `","error_uri":"https://example.invalid/` + marker + `","client_secret":"` + clientSecret + `"}`
			},
			wantOAuth: "invalid_client",
		},
		{
			name:        "form invalid_request",
			contentType: "application/x-www-form-urlencoded",
			body: func(code, verifier string) string {
				vals := url.Values{}
				vals.Set("error", "invalid_request")
				vals.Set("error_description", "echo "+marker+" "+code+" "+verifier+" "+clientSecret)
				return vals.Encode()
			},
			wantOAuth: "invalid_request",
		},
		{
			name:        "invalid_grant stays auth_exchange_error",
			contentType: "application/json",
			body: func(code, verifier string) string {
				return `{"error":"invalid_grant","error_description":"echo ` + marker + ` ` + code + `","refresh_token":"` + verifier + `","client_secret":"` + clientSecret + `"}`
			},
			wantOAuth: "invalid_grant",
		},
		{
			name:        "unparsed body",
			contentType: "application/json",
			body: func(code, verifier string) string {
				return "not-json " + marker + " " + code + " " + verifier + " " + clientSecret
			},
			wantOAuth: "",
		},
		{
			name:        "hyphenated error code is dropped",
			contentType: "application/json",
			body: func(code, verifier string) string {
				return `{"error":"` + marker + `","error_description":"` + code + " " + verifier + " " + clientSecret + `"}`
			},
			wantOAuth: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFileKeyring(t)
			t.Setenv("GOG_LITE_CLIENT_SECRET", clientSecret)

			scopes, err := googleauth.ScopesForServices([]googleauth.Service{googleauth.ServiceGmail})
			if err != nil {
				t.Fatalf("ScopesForServices: %v", err)
			}
			verifier := oauth2.GenerateVerifier()
			writeOAuthManualState(t, state, redirectURI, verifier, scopes)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				vals, err := url.ParseQuery(string(raw))
				if err != nil {
					t.Errorf("parse form: %v", err)
				}
				if vals.Get("grant_type") != "authorization_code" {
					t.Errorf("grant_type = %q", vals.Get("grant_type"))
				}
				if vals.Get("code") != authCode {
					t.Errorf("code = %q", vals.Get("code"))
				}
				if vals.Get("code_verifier") != verifier {
					t.Errorf("code_verifier = %q", vals.Get("code_verifier"))
				}
				if vals.Get("client_secret") != clientSecret {
					t.Errorf("client_secret = %q", vals.Get("client_secret"))
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, tt.body(vals.Get("code"), vals.Get("code_verifier")))
			}))
			t.Cleanup(server.Close)

			restore := googleauth.SetOAuthEndpointForTest(oauth2.Endpoint{
				AuthURL:   "https://accounts.google.com/o/oauth2/auth",
				TokenURL:  server.URL,
				AuthStyle: oauth2.AuthStyleInParams,
			})
			t.Cleanup(restore)

			authURL := redirectURI + "?code=" + url.QueryEscape(authCode) + "&state=" + url.QueryEscape(state)
			prevArgs := os.Args
			t.Cleanup(func() { os.Args = prevArgs })
			os.Args = []string{
				"gog-lite", "auth", "login",
				"--account", account,
				"--services", "gmail",
				"--auth-url", authURL,
			}

			var runErr error
			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					runErr = Execute(context.Background(), "dev")
				})
			})
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			assertTokenEndpointFailure(t, runErr, stderr, "auth_exchange_error", tt.wantOAuth, marker)
			if strings.Contains(stderr, invalidGrantMessage) {
				t.Fatalf("stderr used the refresh invalid_grant message: %q", stderr)
			}
			for _, secret := range []string{authCode, verifier, clientSecret, "exchange code:", "error_uri"} {
				if strings.Contains(stderr, secret) {
					t.Fatalf("stderr leaked %q: %q", secret, stderr)
				}
			}

			dir, err := config.Dir()
			if err != nil {
				t.Fatalf("Dir: %v", err)
			}
			statePath := filepath.Join(dir, "oauth-manual-state-"+state+".json")
			if _, err := os.Stat(statePath); err != nil {
				t.Fatalf("state file after failed exchange: %v", err)
			}
		})
	}
}

func TestExecute_AuthLoginExchangeKeepsOtherErrors(t *testing.T) {
	setupFileKeyring(t)
	const marker = "leak-marker-9f3c2a"

	prevArgs := os.Args
	t.Cleanup(func() { os.Args = prevArgs })
	os.Args = []string{
		"gog-lite", "auth", "login",
		"--account", "test@example.com",
		"--services", "gmail",
		"--auth-url", "http://127.0.0.1:9/oauth2/callback?code=" + marker + "&state=missing-state",
	}

	var runErr error
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			runErr = Execute(context.Background(), "dev")
		})
	})
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if got := output.ExitCode(runErr); got != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d (stderr %q)", got, output.ExitCodeError, stderr)
	}
	assertErrorCode(t, stderr, "auth_exchange_error")
	if !strings.Contains(stderr, "state mismatch") {
		t.Fatalf("stderr = %q, want state mismatch", stderr)
	}
	if strings.Contains(stderr, marker) {
		t.Fatalf("stderr leaked auth url marker: %q", stderr)
	}
	if strings.Contains(stderr, tokenEndpointFailureMessage) {
		t.Fatalf("non-token failure was rewritten: %q", stderr)
	}
}

func TestExecute_AuthLoginUserInfoOmitsBody(t *testing.T) {
	const marker = "leak-marker-9f3c2a"
	const clientSecret = "super-secret-client-secret"
	const account = "test@example.com"
	const state = "teststate"
	const authCode = "auth-code-" + marker
	const accessToken = "access-token-" + marker
	const refreshToken = "refresh-token-" + marker
	const redirectURI = "http://127.0.0.1:9/oauth2/callback"

	tests := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "unauthorized oauth error",
			status: http.StatusUnauthorized,
			body:   `{"error":"invalid_token","error_description":"echo ` + marker + ` ` + accessToken + `","access_token":"` + accessToken + `"}`,
		},
		{
			name:   "plain text",
			status: http.StatusBadRequest,
			body:   "not-json " + marker + " " + accessToken + " " + refreshToken + " " + clientSecret,
		},
		{
			name:   "server error",
			status: http.StatusBadGateway,
			body:   marker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFileKeyring(t)
			t.Setenv("GOG_LITE_CLIENT_SECRET", clientSecret)

			scopes, err := googleauth.ScopesForServices([]googleauth.Service{googleauth.ServiceGmail})
			if err != nil {
				t.Fatalf("ScopesForServices: %v", err)
			}
			verifier := oauth2.GenerateVerifier()
			writeOAuthManualState(t, state, redirectURI, verifier, scopes)

			tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				vals, err := url.ParseQuery(string(raw))
				if err != nil {
					t.Errorf("parse form: %v", err)
				}
				if vals.Get("grant_type") != "authorization_code" {
					t.Errorf("grant_type = %q", vals.Get("grant_type"))
				}
				if vals.Get("code") != authCode {
					t.Errorf("code = %q", vals.Get("code"))
				}
				if vals.Get("code_verifier") != verifier {
					t.Errorf("code_verifier = %q", vals.Get("code_verifier"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"`+accessToken+`","refresh_token":"`+refreshToken+`","token_type":"Bearer","expires_in":3600}`)
			}))
			t.Cleanup(tokenSrv.Close)

			userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
					t.Errorf("authorization = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(userinfoSrv.Close)

			restoreToken := googleauth.SetOAuthEndpointForTest(oauth2.Endpoint{
				AuthURL:   "https://accounts.google.com/o/oauth2/auth",
				TokenURL:  tokenSrv.URL,
				AuthStyle: oauth2.AuthStyleInParams,
			})
			t.Cleanup(restoreToken)
			restoreUserInfo := googleauth.SetUserInfoEndpointForTest(userinfoSrv.URL)
			t.Cleanup(restoreUserInfo)

			authURL := redirectURI + "?code=" + url.QueryEscape(authCode) + "&state=" + url.QueryEscape(state)
			prevArgs := os.Args
			t.Cleanup(func() { os.Args = prevArgs })
			os.Args = []string{
				"gog-lite", "auth", "login",
				"--account", account,
				"--services", "gmail",
				"--auth-url", authURL,
			}

			var runErr error
			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					runErr = Execute(context.Background(), "dev")
				})
			})
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if got := output.ExitCode(runErr); got != output.ExitCodeError {
				t.Fatalf("exit code = %d, want %d (stderr %q)", got, output.ExitCodeError, stderr)
			}
			assertErrorCode(t, stderr, "auth_exchange_error")
			payload := decodeErrorPayload(t, stderr)
			want := "userinfo request failed (HTTP " + fmt.Sprint(tt.status) + ")"
			if payload.Error != want {
				t.Fatalf("error = %q, want %q", payload.Error, want)
			}
			for _, secret := range []string{
				marker, accessToken, refreshToken, authCode, verifier, clientSecret,
				"error_description", "extract email from userinfo", "userinfo status", tt.body,
			} {
				if secret != "" && strings.Contains(stderr, secret) {
					t.Fatalf("stderr leaked %q: %q", secret, stderr)
				}
			}

			dir, err := config.Dir()
			if err != nil {
				t.Fatalf("Dir: %v", err)
			}
			statePath := filepath.Join(dir, "oauth-manual-state-"+state+".json")
			if _, err := os.Stat(statePath); err != nil {
				t.Fatalf("state file after failed userinfo: %v", err)
			}
		})
	}
}

func writeOAuthManualState(t *testing.T, state, redirectURI, verifier string, scopes []string) {
	t.Helper()
	dir, err := config.EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	payload := map[string]any{
		"state":         state,
		"redirect_uri":  redirectURI,
		"scopes":        scopes,
		"created_at":    time.Now().UTC(),
		"code_verifier": verifier,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	path := filepath.Join(dir, "oauth-manual-state-"+state+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func setupFileKeyring(t *testing.T) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	t.Setenv("GOG_LITE_KEYRING_PASSWORD", "test-password")
	t.Setenv("GOG_LITE_CLIENT_ID", "dummy-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "dummy-secret")
}

func openTestStore(t *testing.T) *secrets.Store {
	t.Helper()
	store, err := secrets.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	return store
}

func stubRevoke(t *testing.T, fn func(context.Context, string) error) {
	t.Helper()
	orig := revokeRefreshToken
	revokeRefreshToken = fn
	t.Cleanup(func() { revokeRefreshToken = orig })
}
