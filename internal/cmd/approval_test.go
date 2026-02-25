package cmd

import (
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
