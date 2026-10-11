package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIssueAndConsumeApprovalToken(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err != nil {
		t.Fatalf("consumeApprovalToken: %v", err)
	}
	if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err == nil {
		t.Fatal("expected second consume to fail")
	}
}

func TestApprovalToken_Expired(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// Issue a token, then overwrite the file with a past expiry.
	token, _, err := issueApprovalToken("a@example.com", "docs.write.replace", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	expired := approvalTokenState{
		Token:     token,
		Account:   "a@example.com",
		Action:    "docs.write.replace",
		ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	}
	b, _ := json.Marshal(expired)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("overwrite token file: %v", err)
	}

	if err := consumeApprovalToken("a@example.com", "docs.write.replace", token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestApprovalToken_AccountMismatch(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	token, _, err := issueApprovalToken("owner@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	if err := consumeApprovalToken("other@example.com", "calendar.delete", token); err == nil {
		t.Fatal("expected account mismatch to be rejected")
	}
}

func TestApprovalToken_ActionMismatch(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	token, _, err := issueApprovalToken("a@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	if err := consumeApprovalToken("a@example.com", "docs.write.replace", token); err == nil {
		t.Fatal("expected action mismatch to be rejected")
	}
}

func TestApprovalTokenPath_FilenameIsHash(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}

	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}

	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:]) + ".json"
	if got := filepath.Base(path); got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	if strings.Contains(filepath.Base(path), token) {
		t.Fatalf("filename %q contains the token", filepath.Base(path))
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read approvals dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), token) {
			t.Fatalf("directory entry %q contains the token", entry.Name())
		}
	}
}

func TestApprovalTokenPath_RejectsTraversal(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	for _, token := range []string{"../evil", "/tmp/evil", "a/b", "bad token"} {
		if _, err := approvalTokenPath(token); err == nil {
			t.Fatalf("expected %q to be rejected", token)
		}
	}
}

func TestConsumeApprovalToken_RejectsTraversal(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	err := consumeApprovalToken("a@example.com", "calendar.delete", "../evil")
	if err == nil {
		t.Fatal("expected traversal token to be rejected")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConsumeApprovalToken_DoesNotStealAgedLock(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := withFileLock(path, func() error {
			close(started)
			<-release
			return nil
		})
		if err != nil {
			t.Errorf("holder: %v", err)
		}
	}()
	<-started

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path+".lock", old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	setLockAcquireTimeout(t, 200*time.Millisecond)

	err = consumeApprovalToken("you@example.com", "calendar.delete", token)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("consume while held = %v, want timeout", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	var st approvalTokenState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if st.Used {
		t.Fatal("timed-out consume marked the token used")
	}

	close(release)
	wg.Wait()

	if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err != nil {
		t.Fatalf("consume after release: %v", err)
	}
	if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err == nil {
		t.Fatal("expected a second consume to fail")
	}
}

func TestConsumeApprovalToken_ConcurrentSingleSuccess(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err == nil {
				successCount.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := successCount.Load(); got != 1 {
		t.Fatalf("successCount = %d, want 1", got)
	}
}

func TestRandomTokenFrom_SkipsLeadingHyphen(t *testing.T) {
	hyphen := bytes.Repeat([]byte{0xF8}, approvalTokenEntropyBytes)
	safe := bytes.Repeat([]byte{0x00}, approvalTokenEntropyBytes)
	if got := base64.RawURLEncoding.EncodeToString(hyphen); !strings.HasPrefix(got, "-") {
		t.Fatalf("fixture encoding = %q, want a leading hyphen", got)
	}

	raw := append(append([]byte{}, hyphen...), safe...)
	got, err := randomTokenFrom(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("randomTokenFrom: %v", err)
	}
	if strings.HasPrefix(got, "-") {
		t.Fatal("issued token starts with '-'")
	}
	want := base64.RawURLEncoding.EncodeToString(safe)
	if got != want {
		t.Fatalf("token = %q, want the second draw", got)
	}
}

func TestRandomTokenFrom_ErrorsWhenEveryDrawStartsWithHyphen(t *testing.T) {
	raw := bytes.Repeat([]byte{0xF8}, approvalTokenEntropyBytes*approvalTokenMaxAttempts)
	if _, err := randomTokenFrom(bytes.NewReader(raw)); err == nil {
		t.Fatal("expected an error when every draw starts with '-'")
	}
}

func TestRandomTokenFrom_ShortRead(t *testing.T) {
	if _, err := randomTokenFrom(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("expected an error on a short read")
	}
}

func TestRandomToken_DoesNotStartWithHyphen(t *testing.T) {
	for i := 0; i < 32; i++ {
		token, err := randomToken()
		if err != nil {
			t.Fatalf("randomToken: %v", err)
		}
		if strings.HasPrefix(token, "-") {
			t.Fatal("issued token starts with '-'")
		}
	}
}

func TestConsumeApprovalToken_LeadingHyphenStillValid(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	const token = "-legacyToken_09"
	writeLegacyApprovalToken(t, token, "you@example.com", "calendar.delete")
	if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err != nil {
		t.Fatalf("consume legacy token: %v", err)
	}
}

func TestExecute_IssuedApprovalTokenSpaceSeparated(t *testing.T) {
	setApprovalCLIEnv(t)

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	if token == "" || strings.HasPrefix(token, "-") {
		t.Fatal("issued token is empty or starts with '-'")
	}

	code := runCalendarDelete(t, "--approval-token", token)
	if code == "invalid_arguments" {
		t.Fatal("space-separated approval token was parsed as a flag")
	}
	if code != "auth_required" {
		t.Fatalf("code = %q, want auth_required", code)
	}
	if !approvalTokenUsed(t, token) {
		t.Fatal("space-separated token was not consumed")
	}
}

func TestExecute_LegacyHyphenApprovalToken(t *testing.T) {
	setApprovalCLIEnv(t)

	const token = "-legacyToken_09"
	writeLegacyApprovalToken(t, token, "you@example.com", "calendar.delete")

	code := runCalendarDelete(t, "--approval-token", token)
	if code != "invalid_arguments" {
		t.Fatalf("code = %q, want invalid_arguments", code)
	}
	if approvalTokenUsed(t, token) {
		t.Fatal("flag-misread token was consumed")
	}

	code = runCalendarDelete(t, "--approval-token="+token)
	if code != "auth_required" {
		t.Fatalf("equals form code = %q, want auth_required", code)
	}
	if !approvalTokenUsed(t, token) {
		t.Fatal("equals-form legacy token was not consumed")
	}
}

func TestExecute_ApprovalTokenHelp(t *testing.T) {
	commands := [][]string{
		{"gog-lite", "calendar", "delete", "--help"},
		{"gog-lite", "docs", "write", "--help"},
		{"gog-lite", "docs", "find-replace", "--help"},
		{"gog-lite", "sheets", "update", "--help"},
		{"gog-lite", "slides", "write", "--help"},
	}
	for _, args := range commands {
		t.Run(strings.Join(args[1:len(args)-1], " "), func(t *testing.T) {
			prev := os.Args
			t.Cleanup(func() { os.Args = prev })
			os.Args = args

			stdout := captureStdout(t, func() {
				if err := Execute(context.Background(), "dev"); err != nil {
					t.Fatalf("Execute: %v", err)
				}
			})

			var payload struct {
				Help string `json:"help"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
				t.Fatalf("parse stdout JSON: %v", err)
			}
			if !strings.Contains(payload.Help, "Newly issued tokens do not start with '-'") {
				t.Fatal("help does not mention that new tokens avoid a leading hyphen")
			}
		})
	}
}

func setApprovalCLIEnv(t *testing.T) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "dummy-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "dummy-secret")
	t.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	t.Setenv("GOG_LITE_KEYRING_PASSWORD", "ci-test-password")
}

func writeLegacyApprovalToken(t *testing.T, token, account, action string) {
	t.Helper()
	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	st := approvalTokenState{
		Token:     token,
		Account:   account,
		Action:    action,
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}
	if err := writeApprovalTokenState(path, st); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

func runCalendarDelete(t *testing.T, tokenArgs ...string) string {
	t.Helper()
	prev := os.Args
	t.Cleanup(func() { os.Args = prev })
	os.Args = append([]string{
		"gog-lite", "calendar", "delete",
		"--account", "you@example.com",
		"--event-id", "evt",
		"--confirm-delete",
	}, tokenArgs...)

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = Execute(context.Background(), "dev")
	})
	if runErr == nil {
		t.Fatal("expected calendar delete to fail")
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatal("stderr was not JSON")
	}
	return payload.Code
}

func approvalTokenUsed(t *testing.T, token string) bool {
	t.Helper()
	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	var st approvalTokenState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	return st.Used
}
