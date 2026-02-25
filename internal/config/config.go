package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppName        = "gog-lite"
	clientIDEnvVar = "GOG_LITE_CLIENT_ID"     //nolint:gosec
	clientSecretEnvVar = "GOG_LITE_CLIENT_SECRET" //nolint:gosec
)

// File is the configuration stored at ~/.config/gog-lite/config.json.
type File struct {
	KeyringBackend string `json:"keyring_backend,omitempty"`
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(base, AppName), nil
}

func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	return dir, nil
}

func KeyringDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "keyring"), nil
}

func EnsureKeyringDir() (string, error) {
	dir, err := KeyringDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure keyring dir: %w", err)
	}

	return dir, nil
}

func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

func ReadConfig() (File, error) {
	path, err := ConfigPath()
	if err != nil {
		return File{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}

		return File{}, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json.Unmarshal(b, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

// CredentialsPath returns the path to the OAuth client credentials file.
func CredentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "credentials.json"), nil
}

// ClientCredentials holds the OAuth2 client ID and secret.
type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type googleCredentialsFile struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
	Web *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"web"`
}

// ReadCredentials reads OAuth client credentials.
// Environment variables GOG_LITE_CLIENT_ID and GOG_LITE_CLIENT_SECRET take
// precedence over credentials.json, enabling headless/container deployments.
func ReadCredentials() (ClientCredentials, error) {
	clientID := strings.TrimSpace(os.Getenv(clientIDEnvVar))
	clientSecret := strings.TrimSpace(os.Getenv(clientSecretEnvVar))
	if clientID != "" && clientSecret != "" {
		return ClientCredentials{ClientID: clientID, ClientSecret: clientSecret}, nil
	}

	path, err := CredentialsPath()
	if err != nil {
		return ClientCredentials{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return ClientCredentials{}, &CredentialsMissingError{Path: path, Cause: err}
		}

		return ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
	}

	// Try parsing as a Google OAuth credentials file first.
	var gf googleCredentialsFile
	if err := json.Unmarshal(b, &gf); err == nil {
		var clientID, clientSecret string
		if gf.Installed != nil {
			clientID, clientSecret = gf.Installed.ClientID, gf.Installed.ClientSecret
		} else if gf.Web != nil {
			clientID, clientSecret = gf.Web.ClientID, gf.Web.ClientSecret
		}

		if clientID != "" && clientSecret != "" {
			return ClientCredentials{ClientID: clientID, ClientSecret: clientSecret}, nil
		}
	}

	// Fall back to parsing as a simple ClientCredentials JSON.
	var c ClientCredentials
	if err := json.Unmarshal(b, &c); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials: %w", err)
	}

	if c.ClientID == "" || c.ClientSecret == "" {
		return ClientCredentials{}, fmt.Errorf("credentials.json is missing client_id or client_secret")
	}

	return c, nil
}

// WriteCredentials writes the credentials to ~/.config/gog-lite/credentials.json.
func WriteCredentials(c ClientCredentials) error {
	_, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := CredentialsPath()
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}

	b = append(b, '\n')
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit credentials: %w", err)
	}

	return nil
}

// CredentialsMissingError is returned when credentials.json is not found.
type CredentialsMissingError struct {
	Path  string
	Cause error
}

func (e *CredentialsMissingError) Error() string {
	return fmt.Sprintf("credentials.json not found at %s; place your Google OAuth credentials there", e.Path)
}

func (e *CredentialsMissingError) Unwrap() error {
	return e.Cause
}
