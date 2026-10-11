package cmd

import (
	"crypto/sha256"
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
