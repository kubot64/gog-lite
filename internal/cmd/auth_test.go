package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

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

func TestEmergencyRevokeCmd_RejectedRevokeKeepsTokenAndDoesNotBlock(t *testing.T) {
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
		return fmt.Errorf("%w: HTTP 400 invalid_request", googleauth.ErrRevokeRejected)
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
	if !strings.Contains(payload.Error, "リモート失効に失敗した") {
		t.Errorf("error = %q, want remote revoke failure", payload.Error)
	}
	if strings.Contains(payload.Error, "ブロック済み") {
		t.Errorf("error claims a block that should not happen: %q", payload.Error)
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
	assertAccountNotBlocked(t, account)
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
