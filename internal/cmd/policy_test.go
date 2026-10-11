package cmd

import (
	"testing"

	"github.com/kubot64/gog-lite/internal/config"
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

	required, err = actionRequiresApproval("sheets.update")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !required {
		t.Fatal("sheets.update should require approval by default")
	}

	required, err = actionRequiresApproval("gmail.search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if required {
		t.Fatal("gmail.search should not require approval by default")
	}

	required, err = actionRequiresApproval("sheets.append")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if required {
		t.Fatal("sheets.append should not require approval by default")
	}
}

func TestActionRequiresApproval_PolicyAddsToDefaults(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// require_approval_actions adds to the defaults.
	if err := config.WritePolicy(config.PolicyFile{
		RequireApprovalActions: []string{"gmail.send"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	for _, action := range []string{
		"gmail.send",
		"calendar.delete",
		"docs.write.replace",
		"docs.find_replace",
		"sheets.update",
		"slides.write",
	} {
		required, err := actionRequiresApproval(action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !required {
			t.Fatalf("%s should require approval", action)
		}
	}

	required, err := actionRequiresApproval("gmail.search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if required {
		t.Fatal("gmail.search should not require approval")
	}
}

func TestActionRequiresApproval_ExemptDefaults(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		ExemptApprovalActions: []string{"calendar.delete", "docs.find_replace"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	for _, action := range []string{"calendar.delete", "docs.find_replace"} {
		required, err := actionRequiresApproval(action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if required {
			t.Fatalf("%s should be exempt from approval", action)
		}
	}

	for _, action := range []string{"docs.write.replace", "sheets.update", "slides.write"} {
		required, err := actionRequiresApproval(action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !required {
			t.Fatalf("%s should still require approval", action)
		}
	}
}

func TestActionRequiresApproval_RequireWinsOverExempt(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		RequireApprovalActions: []string{"gmail.send", "calendar.delete"},
		ExemptApprovalActions:  []string{"calendar.delete", "slides.write"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cases := []struct {
		action   string
		required bool
	}{
		{action: "calendar.delete", required: true},
		{action: "gmail.send", required: true},
		{action: "slides.write", required: false},
		{action: "sheets.update", required: true},
		{action: "gmail.search", required: false},
	}
	for _, tc := range cases {
		required, err := actionRequiresApproval(tc.action)
		if err != nil {
			t.Fatalf("%s: %v", tc.action, err)
		}
		if required != tc.required {
			t.Fatalf("%s required=%v, want %v", tc.action, required, tc.required)
		}
	}
}
