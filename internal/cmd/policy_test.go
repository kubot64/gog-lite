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
