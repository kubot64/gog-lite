package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func issueApprovalTokenWithFile(t *testing.T, ttl time.Duration) (string, string) {
	t.Helper()

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", ttl)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	path, err := writeIssuedApprovalTokenFile(token)
	if err != nil {
		t.Fatalf("writeIssuedApprovalTokenFile: %v", err)
	}

	return token, path
}

func expireApprovalTokenState(t *testing.T, token string) {
	t.Helper()

	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st approvalTokenState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	st.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := writeApprovalTokenState(path, st); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func assertFileExists(t *testing.T, path string, want bool) {
	t.Helper()

	_, err := os.Lstat(path)
	switch {
	case want && err != nil:
		t.Fatalf("expected %s to exist: %v", path, err)
	case !want && !os.IsNotExist(err):
		t.Fatalf("expected %s to be removed, got err=%v", path, err)
	}
}

func TestIssuedApprovalTokenFile_NameDoesNotRevealToken(t *testing.T) {
	isolateTestConfig(t)

	token, path := issueApprovalTokenWithFile(t, time.Minute)
	if filepath.Base(path) == token || filepath.Base(path) == token+".token" {
		t.Fatalf("issued file name reveals the token: %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read issued file: %v", err)
	}
	if got, err := resolveApprovalTokenInput("", path); err != nil || got != token {
		t.Fatalf("resolve issued file = %q, %v; content %q", got, err, b)
	}
}

func TestConsumeApprovalToken_RemovesIssuedFile(t *testing.T) {
	isolateTestConfig(t)

	token, path := issueApprovalTokenWithFile(t, time.Minute)
	if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err != nil {
		t.Fatalf("consumeApprovalToken: %v", err)
	}

	assertFileExists(t, path, false)
}

func TestConsumeApprovalToken_FailureKeepsIssuedFile(t *testing.T) {
	isolateTestConfig(t)

	token, path := issueApprovalTokenWithFile(t, time.Minute)
	if err := consumeApprovalToken("other@example.com", "calendar.delete", token); err == nil {
		t.Fatal("expected account mismatch to fail")
	}

	assertFileExists(t, path, true)
}

func TestConsumeApprovalToken_LeavesFileOutsideIssuedDir(t *testing.T) {
	isolateTestConfig(t)

	token, _, err := issueApprovalToken("you@example.com", "calendar.delete", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "my-token.txt")
	if err := os.WriteFile(outside, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	resolved, err := resolveApprovalTokenInput("", outside)
	if err != nil {
		t.Fatalf("resolveApprovalTokenInput: %v", err)
	}
	if err := consumeApprovalToken("you@example.com", "calendar.delete", resolved); err != nil {
		t.Fatalf("consumeApprovalToken: %v", err)
	}

	assertFileExists(t, outside, true)
}

func TestSweepIssuedApprovalTokenFiles(t *testing.T) {
	isolateTestConfig(t)

	_, validPath := issueApprovalTokenWithFile(t, time.Minute)

	expiredToken, expiredPath := issueApprovalTokenWithFile(t, time.Minute)
	expireApprovalTokenState(t, expiredToken)

	// A used token whose issued file survived (e.g. a crash after marking used).
	usedToken, usedPath := issueApprovalTokenWithFile(t, time.Minute)
	statePath, err := approvalTokenPath(usedToken)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	if err := writeApprovalTokenState(statePath, approvalTokenState{
		Token: usedToken, Account: "you@example.com", Action: "calendar.delete",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), Used: true,
	}); err != nil {
		t.Fatalf("write used state: %v", err)
	}

	// An issued file whose state is gone.
	orphanToken, orphanPath := issueApprovalTokenWithFile(t, time.Minute)
	orphanState, err := approvalTokenPath(orphanToken)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	if err := os.Remove(orphanState); err != nil {
		t.Fatalf("remove orphan state: %v", err)
	}

	// Files the sweep does not own must stay.
	unrelated := filepath.Join(filepath.Dir(validPath), "notes.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	sweepIssuedApprovalTokenFiles(time.Now().UTC())

	assertFileExists(t, validPath, true)
	assertFileExists(t, expiredPath, false)
	assertFileExists(t, usedPath, false)
	assertFileExists(t, orphanPath, false)
	assertFileExists(t, unrelated, true)
}

func TestAuthApprovalTokenCmd_SweepsStaleIssuedFiles(t *testing.T) {
	setApprovalCLIEnv(t)

	expiredToken, expiredPath := issueApprovalTokenWithFile(t, time.Minute)
	expireApprovalTokenState(t, expiredToken)

	captureStdout(t, func() {
		cmd := &AuthApprovalTokenCmd{Account: "you@example.com", Action: "calendar.delete", TTL: "10m"}
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	assertFileExists(t, expiredPath, false)
}
