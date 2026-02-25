package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/term"

	"github.com/morikubo-takashi/gog-lite/internal/config"
)

const (
	keyringPasswordEnv = "GOG_LITE_KEYRING_PASSWORD" //nolint:gosec
	keyringBackendEnv  = "GOG_LITE_KEYRING_BACKEND"  //nolint:gosec
)

var (
	errMissingEmail        = errors.New("missing email")
	errMissingRefreshToken = errors.New("missing refresh token")
	errNoTTY               = errors.New("no TTY available for keyring file backend password prompt")
	errKeyringTimeout      = errors.New("keyring connection timed out")
	keyringOpenFunc        = keyring.Open
)

// Token is stored in the keyring for each account.
type Token struct {
	Email        string    `json:"email"`
	Services     []string  `json:"services,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	RefreshToken string    `json:"-"`
}

type storedToken struct {
	RefreshToken string    `json:"refresh_token"`
	Services     []string  `json:"services,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

// Store manages OAuth tokens in the system keyring.
type Store struct {
	ring keyring.Keyring
}

const keyringOpenTimeout = 5 * time.Second

func fileKeyringPasswordFunc() keyring.PromptFunc {
	password, passwordSet := os.LookupEnv(keyringPasswordEnv)
	if passwordSet {
		return keyring.FixedStringPrompt(password)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		return keyring.TerminalPrompt
	}

	return func(_ string) (string, error) {
		return "", fmt.Errorf("%w; set %s", errNoTTY, keyringPasswordEnv)
	}
}

func openKeyring() (keyring.Keyring, error) {
	keyringDir, err := config.EnsureKeyringDir()
	if err != nil {
		return nil, fmt.Errorf("ensure keyring dir: %w", err)
	}

	backendEnv := strings.ToLower(strings.TrimSpace(os.Getenv(keyringBackendEnv)))

	var backends []keyring.BackendType

	switch backendEnv {
	case "keychain":
		backends = []keyring.BackendType{keyring.KeychainBackend}
	case "file":
		backends = []keyring.BackendType{keyring.FileBackend}
	default:
		// On Linux with no D-Bus, force file backend to avoid hangs.
		dbusAddr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
		if runtime.GOOS == "linux" && dbusAddr == "" {
			backends = []keyring.BackendType{keyring.FileBackend}
		}
	}

	cfg := keyring.Config{
		ServiceName:             config.AppName,
		KeychainTrustApplication: false,
		AllowedBackends:          backends,
		FileDir:                  keyringDir,
		FilePasswordFunc:         fileKeyringPasswordFunc(),
	}

	// On Linux with D-Bus, use timeout to avoid hangs from unresponsive SecretService.
	dbusAddr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if runtime.GOOS == "linux" && dbusAddr != "" && backendEnv == "" {
		return openKeyringWithTimeout(cfg, keyringOpenTimeout)
	}

	ring, err := keyringOpenFunc(cfg)
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	return ring, nil
}

type keyringResult struct {
	ring keyring.Keyring
	err  error
}

func openKeyringWithTimeout(cfg keyring.Config, timeout time.Duration) (keyring.Keyring, error) {
	ch := make(chan keyringResult, 1)

	go func() {
		ring, err := keyringOpenFunc(cfg)
		ch <- keyringResult{ring, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("open keyring: %w", res.err)
		}

		return res.ring, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("%w after %v; set %s=file and %s=<password> to use file storage",
			errKeyringTimeout, timeout, keyringBackendEnv, keyringPasswordEnv)
	}
}

// OpenDefault opens the default keyring store.
func OpenDefault() (*Store, error) {
	ring, err := openKeyring()
	if err != nil {
		return nil, err
	}

	return &Store{ring: ring}, nil
}

func tokenKey(email string) string {
	return fmt.Sprintf("token:%s", strings.ToLower(strings.TrimSpace(email)))
}

func keyringItem(key string, data []byte) keyring.Item {
	return keyring.Item{
		Key:   key,
		Data:  data,
		Label: config.AppName,
	}
}

// SetToken stores a token for the given email.
func (s *Store) SetToken(email string, tok Token) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errMissingEmail
	}

	if tok.RefreshToken == "" {
		return errMissingRefreshToken
	}

	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(storedToken{
		RefreshToken: tok.RefreshToken,
		Services:     tok.Services,
		Scopes:       tok.Scopes,
		CreatedAt:    tok.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}

	if err := s.ring.Set(keyringItem(tokenKey(email), payload)); err != nil {
		return fmt.Errorf("store token: %w", err)
	}

	return nil
}

// GetToken retrieves the token for the given email.
func (s *Store) GetToken(email string) (Token, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return Token{}, errMissingEmail
	}

	item, err := s.ring.Get(tokenKey(email))
	if err != nil {
		return Token{}, fmt.Errorf("read token: %w", err)
	}

	var st storedToken
	if err := json.Unmarshal(item.Data, &st); err != nil {
		return Token{}, fmt.Errorf("decode token: %w", err)
	}

	return Token{
		Email:        email,
		Services:     st.Services,
		Scopes:       st.Scopes,
		CreatedAt:    st.CreatedAt,
		RefreshToken: st.RefreshToken,
	}, nil
}

// DeleteToken removes the token for the given email.
func (s *Store) DeleteToken(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errMissingEmail
	}

	if err := s.ring.Remove(tokenKey(email)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("delete token: %w", err)
	}

	return nil
}

// ListTokens returns all stored tokens.
func (s *Store) ListTokens() ([]Token, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("list keyring keys: %w", err)
	}

	out := make([]Token, 0)
	seen := make(map[string]struct{})

	for _, k := range keys {
		if !strings.HasPrefix(k, "token:") {
			continue
		}

		email := strings.TrimPrefix(k, "token:")
		if email == "" {
			continue
		}

		if _, ok := seen[email]; ok {
			continue
		}

		tok, err := s.GetToken(email)
		if err != nil {
			continue
		}

		seen[email] = struct{}{}
		out = append(out, tok)
	}

	return out, nil
}
