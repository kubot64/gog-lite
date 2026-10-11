package secrets

import (
	"bytes"
	"errors"
	"testing"
)

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

func TestGetOrCreateSecret_StableAndHiddenFromTokenList(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv(keyringBackendEnv, "file")
	t.Setenv(keyringPasswordEnv, "test-password")

	store, err := OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}

	first, err := store.GetOrCreateSecret("audit-hmac-key", 32)
	if err != nil {
		t.Fatalf("GetOrCreateSecret: %v", err)
	}
	second, err := store.GetOrCreateSecret("audit-hmac-key", 32)
	if err != nil {
		t.Fatalf("GetOrCreateSecret again: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("secret changed between reads")
	}
	if _, err := store.GetOrCreateSecret("audit-hmac-key", 16); !errors.Is(err, errUnexpectedSecretLength) {
		t.Fatalf("length mismatch error = %v", err)
	}
	if err := store.SetSecret("token:a@example.com", []byte("nope")); !errors.Is(err, errReservedSecretName) {
		t.Fatalf("reserved name error = %v", err)
	}
	if err := store.SetSecret("audit-tail", nil); !errors.Is(err, errEmptySecret) {
		t.Fatalf("empty secret error = %v", err)
	}
	if _, err := store.GetSecret("missing-secret"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing secret error = %v", err)
	}

	if err := store.SetToken("a@example.com", Token{RefreshToken: "refresh-token"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	tokens, err := store.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Email != "a@example.com" {
		t.Fatalf("tokens = %+v", tokens)
	}
}
