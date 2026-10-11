package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/kubot64/gog-lite/internal/config"
)

func TestRequireFileBackendPassword_RejectsBlank(t *testing.T) {
	for _, password := range []string{"", " ", "\t", " \n "} {
		t.Setenv(keyringPasswordEnv, password)
		if err := requireFileBackendPassword(); !errors.Is(err, errMissingKeyringPassword) {
			t.Fatalf("password %q: error = %v", password, err)
		}
	}
}

func TestRequireFileBackendPassword_AllowsWhenSet(t *testing.T) {
	t.Setenv(keyringPasswordEnv, "secret")
	if err := requireFileBackendPassword(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileKeyringPasswordFunc_RejectsBlankAndDoesNotPrompt(t *testing.T) {
	for _, password := range []string{"", " ", "\t"} {
		t.Setenv(keyringPasswordEnv, password)
		_, err := fileKeyringPasswordFunc()("unlock")
		if !errors.Is(err, errMissingKeyringPassword) {
			t.Fatalf("password %q: error = %v", password, err)
		}
	}

	const password = "secret-value"
	t.Setenv(keyringPasswordEnv, password)
	got, err := fileKeyringPasswordFunc()("unlock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != password {
		t.Fatal("file backend password func returned a different value")
	}
}

func TestOpenDefault_FileBackendRejectsBlankPassword(t *testing.T) {
	for _, tc := range []struct {
		name      string
		backend   string
		clearDBus bool
		linuxOnly bool
	}{
		{name: "explicit", backend: "file"},
		{name: "linux-no-dbus", backend: "", clearDBus: true, linuxOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.linuxOnly && runtime.GOOS != "linux" {
				t.Skip("D-Bus fallback is Linux-only")
			}
			useTempConfig(t)
			t.Setenv(keyringBackendEnv, tc.backend)
			if tc.clearDBus {
				t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
			}
			orig := keyringOpenFunc
			t.Cleanup(func() { keyringOpenFunc = orig })
			var opened bool
			keyringOpenFunc = func(keyring.Config) (keyring.Keyring, error) {
				opened = true
				return nil, errors.New("keyring opened")
			}

			for _, password := range []string{"", " ", "\t"} {
				opened = false
				t.Setenv(keyringPasswordEnv, password)
				err := openDefaultWithin(t, 2*time.Second)
				if opened {
					t.Fatal("keyring opened without a file-backend password")
				}
				if !errors.Is(err, errMissingKeyringPassword) {
					t.Fatalf("password %q: error = %v", password, err)
				}
				if err.Error() != errMissingKeyringPassword.Error() {
					t.Fatalf("error = %q", err.Error())
				}
			}
		})
	}
}

func TestOpenDefault_KeychainDoesNotRequireFilePassword(t *testing.T) {
	useTempConfig(t)
	t.Setenv(keyringBackendEnv, "keychain")
	t.Setenv(keyringPasswordEnv, "")

	_, err := OpenDefault()
	if errors.Is(err, errMissingKeyringPassword) {
		t.Fatal("keychain backend required a file keyring password")
	}
	if runtime.GOOS != "darwin" && err == nil {
		t.Fatal("expected keychain open to fail where keychain is unavailable")
	}
}

func TestMockFileBackendRoundTrip(t *testing.T) {
	const password = "mock-file-password"
	for _, tc := range []struct {
		name      string
		backend   string
		clearDBus bool
		linuxOnly bool
	}{
		{name: "explicit", backend: "file"},
		{name: "linux-no-dbus", backend: "", clearDBus: true, linuxOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.linuxOnly && runtime.GOOS != "linux" {
				t.Skip("D-Bus fallback is Linux-only")
			}
			useTempConfig(t)
			t.Setenv(keyringBackendEnv, tc.backend)
			t.Setenv(keyringPasswordEnv, password)
			if tc.clearDBus {
				t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
			}

			ring := newMemoryKeyring()
			orig := keyringOpenFunc
			t.Cleanup(func() { keyringOpenFunc = orig })
			keyringOpenFunc = func(cfg keyring.Config) (keyring.Keyring, error) {
				if len(cfg.AllowedBackends) != 1 || cfg.AllowedBackends[0] != keyring.FileBackend {
					t.Fatalf("backends = %v", cfg.AllowedBackends)
				}
				got, err := cfg.FilePasswordFunc("unlock")
				if err != nil {
					t.Fatalf("password func: %v", err)
				}
				if got != password {
					t.Fatal("file backend password func returned a different value")
				}
				return ring, nil
			}

			store, err := OpenDefault()
			if err != nil {
				t.Fatalf("OpenDefault: %v", err)
			}
			if err := store.SetToken("user@example.com", Token{
				RefreshToken: "refresh-token",
				Services:     []string{"gmail"},
			}); err != nil {
				t.Fatalf("SetToken: %v", err)
			}
			got, err := store.GetToken("User@Example.com")
			if err != nil {
				t.Fatalf("GetToken: %v", err)
			}
			if got.Email != "user@example.com" || got.RefreshToken != "refresh-token" || len(got.Services) != 1 || got.Services[0] != "gmail" {
				t.Fatalf("token = %+v", got)
			}
			if err := store.DeleteToken("user@example.com"); err != nil {
				t.Fatalf("DeleteToken: %v", err)
			}
			if _, err := store.GetToken("user@example.com"); err == nil {
				t.Fatal("expected deleted token to be missing")
			}

			secret := []byte("audit-key-material")
			if err := store.SetSecret("audit-hmac-key", secret); err != nil {
				t.Fatalf("SetSecret: %v", err)
			}
			gotSecret, err := store.GetSecret("audit-hmac-key")
			if err != nil {
				t.Fatalf("GetSecret: %v", err)
			}
			if !bytes.Equal(gotSecret, secret) {
				t.Fatal("secret round trip mismatch")
			}
		})
	}
}

func TestFileBackendRoundTripWithoutDBus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("D-Bus fallback is Linux-only")
	}

	const (
		password     = "roundtrip-password"
		refreshToken = "refresh-token-roundtrip"
	)
	useTempConfig(t)
	t.Setenv(keyringBackendEnv, "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv(keyringPasswordEnv, password)

	store, err := OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	if err := store.SetToken("user@example.com", Token{RefreshToken: refreshToken}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	secret := []byte("audit-key-material")
	if err := store.SetSecret("audit-hmac-key", secret); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	reopened, err := OpenDefault()
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := reopened.GetToken("user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got.RefreshToken != refreshToken {
		t.Fatal("refresh token did not round-trip")
	}
	gotSecret, err := reopened.GetSecret("audit-hmac-key")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if !bytes.Equal(gotSecret, secret) {
		t.Fatal("secret did not round-trip")
	}

	dir, err := config.KeyringDir()
	if err != nil {
		t.Fatalf("KeyringDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected encrypted keyring files")
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		text := string(body)
		if strings.Contains(text, password) || strings.Contains(text, refreshToken) || strings.Contains(text, string(secret)) {
			t.Fatalf("keyring file %s contains plaintext", entry.Name())
		}
	}
}

func useTempConfig(t *testing.T) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
}

func openDefaultWithin(t *testing.T, limit time.Duration) error {
	t.Helper()
	ch := make(chan error, 1)
	go func() {
		_, err := OpenDefault()
		ch <- err
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(limit):
		t.Fatal("OpenDefault hung")
		return nil
	}
}

type memoryKeyring struct {
	items map[string]keyring.Item
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{items: map[string]keyring.Item{}}
}

func (m *memoryKeyring) Get(key string) (keyring.Item, error) {
	item, ok := m.items[key]
	if !ok {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}
	cloned := item
	cloned.Data = append([]byte(nil), item.Data...)
	return cloned, nil
}

func (m *memoryKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	item, err := m.Get(key)
	if err != nil {
		return keyring.Metadata{}, err
	}
	item.Data = nil
	return keyring.Metadata{Item: &item}, nil
}

func (m *memoryKeyring) Set(item keyring.Item) error {
	cloned := item
	cloned.Data = append([]byte(nil), item.Data...)
	m.items[item.Key] = cloned
	return nil
}

func (m *memoryKeyring) Remove(key string) error {
	delete(m.items, key)
	return nil
}

func (m *memoryKeyring) Keys() ([]string, error) {
	keys := make([]string, 0, len(m.items))
	for key := range m.items {
		keys = append(keys, key)
	}
	return keys, nil
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
