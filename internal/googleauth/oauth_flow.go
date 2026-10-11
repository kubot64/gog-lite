package googleauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/kubot64/gog-lite/internal/config"
)

var (
	errMissingState        = errors.New("missing state in redirect URL")
	errNoCodeInURL         = errors.New("no code found in URL")
	errNoEmailInToken      = errors.New("no email found in userinfo")
	errEmailNotVerified    = errors.New("userinfo email is not verified")
	errNoRefreshToken      = errors.New("no refresh token received; try again with --force-consent")
	errInvalidRedirectURL  = errors.New("invalid redirect URL")
	errMissingScopes       = errors.New("missing scopes")
	errMissingCodeVerifier = errors.New("missing pkce code verifier; run step 1 again")
)

var oauthEndpoint = google.Endpoint
var oauthUserInfoEndpoint = "https://openidconnect.googleapis.com/v1/userinfo"

// SetOAuthEndpointForTest points the authorization-code exchange at endpoint
// until restore is called.
func SetOAuthEndpointForTest(endpoint oauth2.Endpoint) (restore func()) {
	prev := oauthEndpoint
	oauthEndpoint = endpoint
	return func() { oauthEndpoint = prev }
}

// SetUserInfoEndpointForTest points the OpenID userinfo call at endpoint
// until restore is called.
func SetUserInfoEndpointForTest(endpoint string) (restore func()) {
	prev := oauthUserInfoEndpoint
	oauthUserInfoEndpoint = endpoint
	return func() { oauthUserInfoEndpoint = prev }
}

// userInfoStatusError is a non-200 OpenID userinfo response. The body is not
// retained: it can echo the access token or an error_description.
type userInfoStatusError struct {
	status int
}

func (e *userInfoStatusError) Error() string {
	return userInfoFailureText(e.status)
}

const userInfoFailurePrefix = "userinfo request failed"

func userInfoFailureText(status int) string {
	return fmt.Sprintf("%s (HTTP %d)", userInfoFailurePrefix, status)
}

// UserInfoFailureMessage is the stderr text for a non-200 userinfo response.
// It is the fixed sentence plus the HTTP status. The response body is not
// included. ok is false when err is a different failure.
func UserInfoFailureMessage(err error) (string, bool) {
	var statusErr *userInfoStatusError
	if !errors.As(err, &statusErr) || statusErr == nil {
		return "", false
	}
	return statusErr.Error(), true
}

const (
	oauthUserInfoBodyLimit = 1 << 20
	// userInfoTimeout bounds the OpenID userinfo call. http.DefaultClient has
	// no timeout, so a hung connection would block the command until the
	// process is killed. This matches the Google API client timeout.
	userInfoTimeout = 30 * time.Second
)

// userInfoHTTPClient calls the OpenID userinfo endpoint with userInfoTimeout.
var userInfoHTTPClient = &http.Client{Timeout: userInfoTimeout}

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
	Email        string
}

// manualState stores OAuth state between step 1 and step 2.
// CodeVerifier is the PKCE secret. The file is mode 0600.
type manualState struct {
	State        string    `json:"state"`
	RedirectURI  string    `json:"redirect_uri"`
	Scopes       []string  `json:"scopes"`
	CreatedAt    time.Time `json:"created_at"`
	CodeVerifier string    `json:"code_verifier"`
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

	// WriteFile is masked by umask. Chmod sets the mode exactly.
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod manual auth state: %w", err)
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

		if !validCodeVerifier(st.CodeVerifier) {
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

func loadManualStateByState(state string, scopes []string) (manualState, bool, error) {
	if strings.TrimSpace(state) == "" {
		return manualState{}, false, nil
	}

	path, err := manualStatePath(state)
	if err != nil {
		return manualState{}, false, err
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return manualState{}, false, nil
		}

		return manualState{}, false, fmt.Errorf("read manual auth state: %w", err)
	}

	var st manualState
	if err := json.Unmarshal(data, &st); err != nil {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	if st.State == "" || st.RedirectURI == "" {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	if time.Since(st.CreatedAt) > manualStateTTL {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	if !validCodeVerifier(st.CodeVerifier) {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	normalizedScopes := normalizeScopes(scopes)
	if !scopesEqual(st.Scopes, normalizedScopes) {
		return manualState{}, false, nil
	}

	return st, true, nil
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

func listenLoopback(ctx context.Context) (net.Listener, string, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("listen for redirect port: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port

	return ln, fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port), nil
}

func validCodeVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}

	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '.' || c == '_' || c == '~':
		default:
			return false
		}
	}

	return true
}

func buildAuthURL(creds config.ClientCredentials, redirectURI, state, verifier string, scopes []string, forceConsent bool) string {
	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}
	opts := append(authURLParams(forceConsent), oauth2.S256ChallengeOption(verifier))

	return cfg.AuthCodeURL(state, opts...)
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

	// Reuse an unexpired state when its callback port is still served by this
	// program. The stored verifier must be the one that produced the original
	// code_challenge. If another process holds the port, drop that state and
	// mint a new port, state, and verifier below.
	for {
		st, ok, err := loadManualState(scopes)
		if err != nil {
			return Step1Result{}, err
		}
		if !ok {
			break
		}

		keep, err := ensureCallbackPort(st.RedirectURI)
		if err != nil {
			return Step1Result{}, err
		}
		if keep {
			return Step1Result{
				AuthURL:  buildAuthURL(creds, st.RedirectURI, st.State, st.CodeVerifier, scopes, opts.ForceConsent),
				NextStep: "run again with --auth-url <redirect URL from browser>",
			}, nil
		}

		if err := clearManualState(st.State); err != nil {
			return Step1Result{}, err
		}
	}

	ln, redirectURI, err := listenLoopback(ctx)
	if err != nil {
		return Step1Result{}, err
	}

	state, err := randomState()
	if err != nil {
		_ = ln.Close()
		return Step1Result{}, err
	}

	verifier := oauth2.GenerateVerifier()
	st := manualState{
		State:        state,
		RedirectURI:  redirectURI,
		Scopes:       scopes,
		CreatedAt:    time.Now().UTC(),
		CodeVerifier: verifier,
	}

	if err := saveManualState(st); err != nil {
		_ = ln.Close()
		return Step1Result{}, err
	}

	if err := holdCallbackPort(ln); err != nil {
		_ = ln.Close()
		_ = clearManualState(state)
		return Step1Result{}, err
	}

	return Step1Result{
		AuthURL:  buildAuthURL(creds, redirectURI, state, verifier, scopes, opts.ForceConsent),
		NextStep: "run again with --auth-url <redirect URL from browser>",
	}, nil
}

// Step2 exchanges the redirect URL (containing the auth code) for a refresh token.
func Step2(ctx context.Context, creds config.ClientCredentials, opts AuthorizeOptions, redirectURLFromBrowser string) (Step2Result, error) {
	if len(opts.Scopes) == 0 {
		return Step2Result{}, errMissingScopes
	}

	code, state, redirectURI, err := parseRedirectURL(redirectURLFromBrowser)
	if err != nil {
		return Step2Result{}, err
	}
	if strings.TrimSpace(state) == "" {
		return Step2Result{}, errMissingState
	}

	scopes := normalizeScopes(opts.Scopes)

	st, ok, err := loadManualStateByState(state, scopes)
	if err != nil {
		return Step2Result{}, err
	}
	if !ok {
		return Step2Result{}, fmt.Errorf("state mismatch or expired state; run step 1 again")
	}

	// Use stored redirect URI if available (more reliable), otherwise use parsed one.
	if st.RedirectURI != "" {
		redirectURI = st.RedirectURI
	}

	if redirectURI == "" {
		return Step2Result{}, fmt.Errorf("missing redirect URI; run step 1 again")
	}

	if !validCodeVerifier(st.CodeVerifier) {
		return Step2Result{}, errMissingCodeVerifier
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(st.CodeVerifier))
	if err != nil {
		return Step2Result{}, fmt.Errorf("exchange code: %w", err)
	}

	if tok.RefreshToken == "" {
		return Step2Result{}, errNoRefreshToken
	}

	email, err := emailFromUserInfo(ctx, tok)
	if err != nil {
		return Step2Result{}, fmt.Errorf("extract email from userinfo: %w", err)
	}

	// Drop the state only after the code exchange and userinfo both succeed.
	// A network failure leaves the file so the same redirect URL can be retried.
	_ = clearManualState(st.State)

	return Step2Result{
		RefreshToken: tok.RefreshToken,
		Email:        email,
	}, nil
}

func emailFromUserInfo(ctx context.Context, tok *oauth2.Token) (string, error) {
	if tok == nil {
		return "", errNoEmailInToken
	}

	accessToken := strings.TrimSpace(tok.AccessToken)
	if accessToken == "" {
		return "", errNoEmailInToken
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthUserInfoEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := userInfoHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Discard the body. It is not part of the error.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, oauthUserInfoBodyLimit))
		return "", &userInfoStatusError{status: resp.StatusCode}
	}

	var claims struct {
		Email string `json:"email"`
		// Google returns this field in OpenID userinfo response.
		EmailVerified bool `json:"email_verified"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, oauthUserInfoBodyLimit)).Decode(&claims); err != nil {
		return "", errNoEmailInToken
	}

	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return "", errNoEmailInToken
	}
	if !claims.EmailVerified {
		return "", errEmailNotVerified
	}

	return email, nil
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
