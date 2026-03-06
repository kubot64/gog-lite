package cmd

import (
	"encoding/json"
	"os"
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
	errs := make(chan error, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := consumeApprovalToken("you@example.com", "calendar.delete", token); err == nil {
				successCount.Add(1)
			} else {
				errs <- err
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	if got := successCount.Load(); got != 1 {
		t.Fatalf("successCount = %d, want 1", got)
	}

	var failureCount int
	for err := range errs {
		failureCount++
		if !strings.Contains(err.Error(), "already used") {
			t.Fatalf("unexpected loser error: %v", err)
		}
	}
	if failureCount != 7 {
		t.Fatalf("failureCount = %d, want 7", failureCount)
	}

	path, err := approvalTokenPath(token)
	if err != nil {
		t.Fatalf("approvalTokenPath: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}

	var st approvalTokenState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("parse token file: %v", err)
	}
	if !st.Used {
		t.Fatalf("expected token state to be used: %+v", st)
	}
}
