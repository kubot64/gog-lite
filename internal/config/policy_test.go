package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/morikubo-takashi/gog-lite/internal/config"
)

func TestReadWritePolicy(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	p := config.PolicyFile{
		AllowedActions:         []string{"gmail.send", "calendar.create"},
		BlockedAccounts:        []string{"bad@example.com"},
		RequireApprovalActions: []string{"calendar.delete"},
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
