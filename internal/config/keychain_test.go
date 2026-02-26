package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCredentials_KeychainFallbackOnDarwin(t *testing.T) {
	t.Setenv(clientIDEnvVar, "")
	t.Setenv(clientSecretEnvVar, "")

	origOS := currentGOOS
	origLookup := lookupKeychainItem
	currentGOOS = "darwin"
	lookupKeychainItem = func(service string) (string, error) {
		switch service {
		case keychainClientID:
			return "keychain-id", nil
		case keychainClientSecret:
			return "keychain-secret", nil
		default:
			return "", errors.New("unexpected service")
		}
	}
	defer func() {
		currentGOOS = origOS
		lookupKeychainItem = origLookup
	}()

	creds, err := ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "keychain-id" {
		t.Fatalf("ClientID = %q, want %q", creds.ClientID, "keychain-id")
	}
	if creds.ClientSecret != "keychain-secret" {
		t.Fatalf("ClientSecret = %q, want %q", creds.ClientSecret, "keychain-secret")
	}
}

func TestReadCredentials_KeychainFailureFallsBackToFile(t *testing.T) {
	t.Setenv(clientIDEnvVar, "")
	t.Setenv(clientSecretEnvVar, "")

	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"client_id":"file-id","client_secret":"file-secret"}`), 0o600); err != nil {
		t.Fatalf("write file creds: %v", err)
	}

	origOS := currentGOOS
	origLookup := lookupKeychainItem
	currentGOOS = "darwin"
	lookupKeychainItem = func(service string) (string, error) {
		if service == keychainClientSecret {
			return "", errors.New("not found")
		}
		return "keychain-id", nil
	}
	defer func() {
		currentGOOS = origOS
		lookupKeychainItem = origLookup
	}()

	creds, err := ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "file-id" || creds.ClientSecret != "file-secret" {
		t.Fatalf("got %+v, want file creds", creds)
	}
}
