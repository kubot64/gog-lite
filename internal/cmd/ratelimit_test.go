package cmd

import (
	"testing"
	"time"
)

func TestEnforceRateLimit(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nowUTC = func() time.Time { return base }
	t.Cleanup(func() { nowUTC = func() time.Time { return time.Now().UTC() } })

	if err := enforceRateLimit("gmail.send", 2, time.Minute); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := enforceRateLimit("gmail.send", 2, time.Minute); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if err := enforceRateLimit("gmail.send", 2, time.Minute); err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
}
