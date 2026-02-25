package cmd

import (
	"encoding/json"
	"os"
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
