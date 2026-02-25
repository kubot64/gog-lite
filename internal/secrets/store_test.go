package secrets

import "testing"

func TestRequireFileBackendPassword_RequiresEnv(t *testing.T) {
	t.Setenv(keyringPasswordEnv, "")
	if err := requireFileBackendPassword("file"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRequireFileBackendPassword_NotRequiredForOtherBackends(t *testing.T) {
	t.Setenv(keyringPasswordEnv, "")
	if err := requireFileBackendPassword("keychain"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireFileBackendPassword_AllowsWhenSet(t *testing.T) {
	t.Setenv(keyringPasswordEnv, "secret")
	if err := requireFileBackendPassword("file"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
