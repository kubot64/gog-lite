package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubot64/gog-lite/internal/config"
)

func TestReadCredentials_EnvVarsTakePrecedence(t *testing.T) {
	t.Setenv("GOG_LITE_CLIENT_ID", "env-client-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "env-client-secret")

	// Even if credentials.json doesn't exist, env vars should work.
	creds, err := config.ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "env-client-id" {
		t.Errorf("ClientID = %q, want %q", creds.ClientID, "env-client-id")
	}
	if creds.ClientSecret != "env-client-secret" {
		t.Errorf("ClientSecret = %q, want %q", creds.ClientSecret, "env-client-secret")
	}
}

func TestReadCredentials_EnvVarsTrimSpace(t *testing.T) {
	t.Setenv("GOG_LITE_CLIENT_ID", "  id-with-spaces  ")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "  secret-with-spaces  ")

	creds, err := config.ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "id-with-spaces" {
		t.Errorf("ClientID = %q, want %q", creds.ClientID, "id-with-spaces")
	}
	if creds.ClientSecret != "secret-with-spaces" {
		t.Errorf("ClientSecret = %q, want %q", creds.ClientSecret, "secret-with-spaces")
	}
}

func TestReadCredentials_PartialEnvVarFallsBackToFile(t *testing.T) {
	// Only one env var set → must not use env vars, should fall back to file.
	t.Setenv("GOG_LITE_CLIENT_ID", "only-id")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome) // Linux: override XDG config home

	// Use config.CredentialsPath() to find the platform-correct path.
	credsPath, err := config.CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(credsPath, []byte(`{"client_id":"file-id","client_secret":"file-secret"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	creds, err := config.ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "file-id" {
		t.Errorf("ClientID = %q, want %q (should use file)", creds.ClientID, "file-id")
	}
}

func TestReadCredentials_FileGoogleFormat(t *testing.T) {
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome) // Linux: override XDG config home

	credsPath, err := config.CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	googleJSON := `{"installed":{"client_id":"google-id","client_secret":"google-secret","redirect_uris":["urn:ietf:wg:oauth:2.0:oob"]}}`
	if err := os.WriteFile(credsPath, []byte(googleJSON), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	creds, err := config.ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "google-id" {
		t.Errorf("ClientID = %q, want %q", creds.ClientID, "google-id")
	}
	if creds.ClientSecret != "google-secret" {
		t.Errorf("ClientSecret = %q, want %q", creds.ClientSecret, "google-secret")
	}
}

func TestReadCredentials_MissingFileError(t *testing.T) {
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome) // Linux: override XDG config home

	_, err := config.ReadCredentials()
	if err == nil {
		t.Fatal("expected error when credentials.json is missing, got nil")
	}
}
