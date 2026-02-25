package cmd

import (
	"testing"

	"github.com/morikubo-takashi/gog-lite/internal/config"
)

func TestEnforceActionPolicy(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		AllowedActions:  []string{"gmail.send"},
		BlockedAccounts: []string{"blocked@example.com"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	if err := enforceActionPolicy("ok@example.com", "gmail.send"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := enforceActionPolicy("ok@example.com", "calendar.create"); err == nil {
		t.Fatal("expected denial for disallowed action")
	}
	if err := enforceActionPolicy("blocked@example.com", "gmail.send"); err == nil {
		t.Fatal("expected denial for blocked account")
	}
}

func TestEnforceActionPolicy_NoAllowedActions(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// No policy file → allowed_actions is empty → all actions are permitted.
	if err := enforceActionPolicy("anyone@example.com", "calendar.create"); err != nil {
		t.Fatalf("expected no restriction when allowed_actions is empty: %v", err)
	}
}

func TestActionRequiresApproval_Default(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// No policy → uses defaultApprovalActions.
	required, err := actionRequiresApproval("calendar.delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !required {
		t.Fatal("calendar.delete should require approval by default")
	}

	required, err = actionRequiresApproval("gmail.search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if required {
		t.Fatal("gmail.search should not require approval by default")
	}
}

func TestActionRequiresApproval_PolicyOverride(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// Policy sets require_approval_actions → replaces defaults.
	if err := config.WritePolicy(config.PolicyFile{
		RequireApprovalActions: []string{"gmail.send"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	required, err := actionRequiresApproval("gmail.send")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !required {
		t.Fatal("gmail.send should require approval when listed in policy")
	}

	// calendar.delete is in defaults but NOT in the override list.
	required, err = actionRequiresApproval("calendar.delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if required {
		t.Fatal("calendar.delete should NOT require approval when not in policy override")
	}
}
