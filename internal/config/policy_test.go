package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubot64/gog-lite/internal/config"
)

func TestReadWritePolicy(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	p := config.PolicyFile{
		AllowedActions:         []string{"gmail.send", "calendar.create"},
		BlockedAccounts:        []string{"bad@example.com"},
		RequireApprovalActions: []string{"Calendar.Delete", "calendar.delete"},
		ExemptApprovalActions:  []string{" Slides.Write ", "slides.write"},
	}
	if err := config.WritePolicy(p); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	got, err := config.ReadPolicy()
	if err != nil {
		t.Fatalf("ReadPolicy: %v", err)
	}
	if len(got.AllowedActions) != 2 {
		t.Fatalf("allowed_actions len=%d", len(got.AllowedActions))
	}
	if len(got.RequireApprovalActions) != 1 || got.RequireApprovalActions[0] != "calendar.delete" {
		t.Fatalf("require_approval_actions = %#v", got.RequireApprovalActions)
	}
	if len(got.ExemptApprovalActions) != 1 || got.ExemptApprovalActions[0] != "slides.write" {
		t.Fatalf("exempt_approval_actions = %#v", got.ExemptApprovalActions)
	}
}

func TestReadPolicy_MissingFile(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := config.PolicyPath()
	if err != nil {
		t.Fatalf("PolicyPath: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected missing file at %s", path)
	}

	got, err := config.ReadPolicy()
	if err != nil {
		t.Fatalf("ReadPolicy: %v", err)
	}
	if got.AllowedActions != nil {
		t.Fatalf("expected nil defaults")
	}

	_ = filepath.Join(cfgHome, "noop")
}
