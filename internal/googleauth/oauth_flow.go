package googleauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/morikubo-takashi/gog-lite/internal/config"
)

var (
	errMissingCode        = errors.New("missing authorization code")
	errNoCodeInURL        = errors.New("no code found in URL")
	errNoRefreshToken     = errors.New("no refresh token received; try again with --force-consent")
	errInvalidRedirectURL = errors.New("invalid redirect URL")
	errMissingScopes      = errors.New("missing scopes")
)

var oauthEndpoint = google.Endpoint

// AuthorizeOptions configures a 2-step headless OAuth flow.
type AuthorizeOptions struct {
	Scopes       []string
	ForceConsent bool
	Client       string // unused in gog-lite (single client), kept for compatibility
}

// Step1Result is returned on the first call when no redirect URL is provided.
type Step1Result struct {
	AuthURL  string `json:"auth_url"`
	NextStep string `json:"next_step"`
}

// Step2Result is returned after successfully exchanging the code.
type Step2Result struct {
	RefreshToken string
}

// manualState stores OAuth state between step 1 and step 2.
type manualState struct {
	State       string    `json:"state"`
	RedirectURI string    `json:"redirect_uri"`
	Scopes      []string  `json:"scopes"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	manualStateFilePrefix = "oauth-manual-state-"
	manualStateFileSuffix = ".json"
	manualStateTTL        = 10 * time.Minute
)

func manualStateDir() (string, error) {
	return config.EnsureDir()
}

func manualStatePath(state string) (string, error) {
	dir, err := manualStateDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, manualStateFilePrefix+state+manualStateFileSuffix), nil
}

func saveManualState(st manualState) error {
	path, err := manualStatePath(st.State)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manual auth state: %w", err)
	}

	data = append(data, '\n')
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write manual auth state: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit manual auth state: %w", err)
	}

	return nil
}

func loadManualState(scopes []string) (manualState, bool, error) {
	dir, err := manualStateDir()
	if err != nil {
		return manualState{}, false, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return manualState{}, false, fmt.Errorf("read manual auth state dir: %w", err)
	}

	normalizedScopes := normalizeScopes(scopes)

	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}

		name := ent.Name()
		if !strings.HasPrefix(name, manualStateFilePrefix) || !strings.HasSuffix(name, manualStateFileSuffix) {
			continue
		}

		path := filepath.Join(dir, name)

		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			continue
		}

		var st manualState
		if err := json.Unmarshal(data, &st); err != nil {
			_ = os.Remove(path)
			continue
		}

		if st.State == "" || st.RedirectURI == "" {
			_ = os.Remove(path)
			continue
		}

		if time.Since(st.CreatedAt) > manualStateTTL {
			_ = os.Remove(path)
			continue
		}

		if !scopesEqual(st.Scopes, normalizedScopes) {
			continue
		}

		return st, true, nil
	}

	return manualState{}, false, nil
}

func clearManualState(state string) error {
	path, err := manualStatePath(state)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove manual auth state: %w", err)
	}

	return nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomRedirectURI(ctx context.Context) (string, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen for redirect port: %w", err)
	}

	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port

	return fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port), nil
}

func parseRedirectURL(rawURL string) (code, state, redirectURI string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", "", fmt.Errorf("parse redirect url: %w", err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", "", fmt.Errorf("parse redirect url: %w", errInvalidRedirectURL)
	}

	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}

	redirectURI = fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, path)
	code = parsed.Query().Get("code")

	if code == "" {
		return "", "", "", errNoCodeInURL
	}

	state = parsed.Query().Get("state")

	return code, state, redirectURI, nil
}

func authURLParams(forceConsent bool) []oauth2.AuthCodeOption {
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	}
	if forceConsent {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "consent"))
	}

	return opts
}

// Step1 generates the authorization URL. The caller should present auth_url to the user.
// The user authorizes and is redirected to a loopback URL that won't load.
// They should copy that URL and pass it to Step2.
func Step1(ctx context.Context, creds config.ClientCredentials, opts AuthorizeOptions) (Step1Result, error) {
	if len(opts.Scopes) == 0 {
		return Step1Result{}, errMissingScopes
	}

	scopes := normalizeScopes(opts.Scopes)

	// Reuse existing state if available.
	if st, ok, err := loadManualState(scopes); err == nil && ok {
		cfg := oauth2.Config{
			ClientID:     creds.ClientID,
			ClientSecret: creds.ClientSecret,
			Endpoint:     oauthEndpoint,
			RedirectURL:  st.RedirectURI,
			Scopes:       scopes,
		}

		authURL := cfg.AuthCodeURL(st.State, authURLParams(opts.ForceConsent)...)

		return Step1Result{
			AuthURL:  authURL,
			NextStep: "run again with --auth-url <redirect URL from browser>",
		}, nil
	}

	redirectURI, err := randomRedirectURI(ctx)
	if err != nil {
		return Step1Result{}, err
	}

	state, err := randomState()
	if err != nil {
		return Step1Result{}, err
	}

	st := manualState{
		State:       state,
		RedirectURI: redirectURI,
		Scopes:      scopes,
		CreatedAt:   time.Now().UTC(),
	}

	if err := saveManualState(st); err != nil {
		return Step1Result{}, err
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	authURL := cfg.AuthCodeURL(state, authURLParams(opts.ForceConsent)...)

	return Step1Result{
		AuthURL:  authURL,
		NextStep: "run again with --auth-url <redirect URL from browser>",
	}, nil
}

// Step2 exchanges the redirect URL (containing the auth code) for a refresh token.
func Step2(ctx context.Context, creds config.ClientCredentials, opts AuthorizeOptions, redirectURLFromBrowser string) (Step2Result, error) {
	if len(opts.Scopes) == 0 {
		return Step2Result{}, errMissingScopes
	}

	code, _, redirectURI, err := parseRedirectURL(redirectURLFromBrowser)
	if err != nil {
		return Step2Result{}, err
	}

	scopes := normalizeScopes(opts.Scopes)

	// Load saved state to get the redirect URI we registered.
	var storedRedirectURI string

	if st, ok, loadErr := loadManualState(scopes); loadErr == nil && ok {
		storedRedirectURI = st.RedirectURI
		// Clean up state file after successful load.
		defer func() { _ = clearManualState(st.State) }()
	}

	// Use stored redirect URI if available (more reliable), otherwise use parsed one.
	if storedRedirectURI != "" {
		redirectURI = storedRedirectURI
	}

	if redirectURI == "" {
		return Step2Result{}, fmt.Errorf("missing redirect URI; run step 1 again")
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return Step2Result{}, fmt.Errorf("exchange code: %w", err)
	}

	if tok.RefreshToken == "" {
		return Step2Result{}, errNoRefreshToken
	}

	return Step2Result{RefreshToken: tok.RefreshToken}, nil
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}

	out := append([]string(nil), scopes...)
	sort.Strings(out)

	return out
}

func scopesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	na := normalizeScopes(a)
	nb := normalizeScopes(b)

	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}

	return true
}
