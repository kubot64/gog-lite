package googleauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/kubot64/gog-lite/internal/config"
)

func TestMain(m *testing.M) {
	if os.Getenv(callbackHoldEnv) == "1" {
		handled, err := RunCallbackHolderIfRequested()
		if !handled || err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestParseRedirectURL_Valid(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		wantCode  string
		wantState string
		wantURI   string
	}{
		{
			name:      "standard callback with state",
			rawURL:    "http://127.0.0.1:12345/oauth2/callback?code=mycode&state=mystate",
			wantCode:  "mycode",
			wantState: "mystate",
			wantURI:   "http://127.0.0.1:12345/oauth2/callback",
		},
		{
			name:     "no state param",
			rawURL:   "http://127.0.0.1:9999/oauth2/callback?code=abc",
			wantCode: "abc",
			wantURI:  "http://127.0.0.1:9999/oauth2/callback",
		},
		{
			name:     "leading and trailing whitespace",
			rawURL:   "  http://127.0.0.1:8080/oauth2/callback?code=xyz  ",
			wantCode: "xyz",
			wantURI:  "http://127.0.0.1:8080/oauth2/callback",
		},
		{
			name:      "extra query params are ignored in URI",
			rawURL:    "http://127.0.0.1:5000/oauth2/callback?code=c1&state=s1&scope=email",
			wantCode:  "c1",
			wantState: "s1",
			wantURI:   "http://127.0.0.1:5000/oauth2/callback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, state, uri, err := parseRedirectURL(tt.rawURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if state != tt.wantState {
				t.Errorf("state = %q, want %q", state, tt.wantState)
			}
			if uri != tt.wantURI {
				t.Errorf("redirectURI = %q, want %q", uri, tt.wantURI)
			}
		})
	}
}

func TestParseRedirectURL_NoCode(t *testing.T) {
	_, _, _, err := parseRedirectURL("http://127.0.0.1:1234/oauth2/callback?state=s")
	if err == nil {
		t.Error("expected error when code is missing, got nil")
	}
}

func TestParseRedirectURL_InvalidURL(t *testing.T) {
	invalids := []string{
		"",
		"not-a-url",
		"/relative/path?code=x",
		"://missing-scheme?code=x",
	}
	for _, raw := range invalids {
		_, _, _, err := parseRedirectURL(raw)
		if err == nil {
			t.Errorf("parseRedirectURL(%q): expected error, got nil", raw)
		}
	}
}

func TestNormalizeScopes_Sorted(t *testing.T) {
	got := normalizeScopes([]string{"c", "a", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, s := range got {
		if s != want[i] {
			t.Errorf("index %d: got %q, want %q", i, s, want[i])
		}
	}
}

func TestNormalizeScopes_Nil(t *testing.T) {
	if got := normalizeScopes(nil); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestNormalizeScopes_Empty(t *testing.T) {
	if got := normalizeScopes([]string{}); got != nil {
		t.Errorf("want nil for empty slice, got %v", got)
	}
}

func TestNormalizeScopes_DoesNotMutateInput(t *testing.T) {
	input := []string{"z", "a"}
	_ = normalizeScopes(input)
	if input[0] != "z" {
		t.Error("normalizeScopes mutated the input slice")
	}
}

func TestScopesEqual_True(t *testing.T) {
	tests := []struct {
		a, b []string
	}{
		{[]string{"a", "b"}, []string{"b", "a"}},
		{[]string{"x"}, []string{"x"}},
		{nil, nil},
		{[]string{}, nil},
	}
	for _, tt := range tests {
		if !scopesEqual(tt.a, tt.b) {
			t.Errorf("scopesEqual(%v, %v) = false, want true", tt.a, tt.b)
		}
	}
}

func TestScopesEqual_False(t *testing.T) {
	tests := []struct {
		a, b []string
	}{
		{[]string{"a", "b"}, []string{"a"}},
		{[]string{"a"}, []string{"b"}},
		{[]string{"a"}, nil},
	}
	for _, tt := range tests {
		if scopesEqual(tt.a, tt.b) {
			t.Errorf("scopesEqual(%v, %v) = true, want false", tt.a, tt.b)
		}
	}
}

func TestEmailFromUserInfo_Valid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email":"you@example.com","email_verified":true}`))
	}))
	defer server.Close()

	old := oauthUserInfoEndpoint
	oauthUserInfoEndpoint = server.URL
	defer func() { oauthUserInfoEndpoint = old }()

	tok := &oauth2.Token{AccessToken: "test-token"}
	email, err := emailFromUserInfo(context.Background(), tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "you@example.com" {
		t.Fatalf("email = %q, want %q", email, "you@example.com")
	}
}

func TestEmailFromUserInfo_MissingAccessToken(t *testing.T) {
	_, err := emailFromUserInfo(context.Background(), &oauth2.Token{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUserInfoHTTPClient_HasTimeout(t *testing.T) {
	if userInfoHTTPClient == nil {
		t.Fatal("userinfo client is nil")
	}
	if userInfoHTTPClient.Timeout != userInfoTimeout {
		t.Fatalf("timeout = %s, want %s", userInfoHTTPClient.Timeout, userInfoTimeout)
	}
	if userInfoHTTPClient.Timeout <= 0 {
		t.Fatal("userinfo client has no timeout")
	}
}

func TestEmailFromUserInfo_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	oldEndpoint := oauthUserInfoEndpoint
	oauthUserInfoEndpoint = server.URL
	defer func() { oauthUserInfoEndpoint = oldEndpoint }()

	oldClient := userInfoHTTPClient
	userInfoHTTPClient = &http.Client{Timeout: 100 * time.Millisecond}
	defer func() { userInfoHTTPClient = oldClient }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err := emailFromUserInfo(ctx, &oauth2.Token{AccessToken: "test-token"})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Fatalf("userinfo hung for %s: %v", elapsed, err)
	}
	if _, ok := UserInfoFailureMessage(err); ok {
		t.Fatalf("timeout was treated as an HTTP status failure: %v", err)
	}
}

func TestEmailFromUserInfo_NonOKOmitsBody(t *testing.T) {
	const marker = "leak-marker-9f3c2a"
	const accessToken = "access-token-" + marker

	tests := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "unauthorized oauth error",
			status: http.StatusUnauthorized,
			body:   `{"error":"invalid_token","error_description":"echo ` + marker + ` ` + accessToken + `"}`,
		},
		{
			name:   "plain text",
			status: http.StatusBadRequest,
			body:   "invalid credentials " + marker + " " + accessToken,
		},
		{
			name:   "server error",
			status: http.StatusBadGateway,
			body:   marker + " " + accessToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
					t.Errorf("authorization = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			restore := SetUserInfoEndpointForTest(server.URL)
			t.Cleanup(restore)

			_, err := emailFromUserInfo(context.Background(), &oauth2.Token{AccessToken: accessToken})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			want := userInfoFailureText(tt.status)
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
			wrapped := fmt.Errorf("extract email from userinfo: %w", err)
			msg, ok := UserInfoFailureMessage(wrapped)
			if !ok || msg != want {
				t.Fatalf("UserInfoFailureMessage = %q, %v, want %q", msg, ok, want)
			}
			for _, secret := range []string{marker, accessToken, "error_description", tt.body} {
				if secret != "" && strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked %q: %q", secret, err.Error())
				}
				if secret != "" && strings.Contains(msg, secret) {
					t.Fatalf("stderr text leaked %q: %q", secret, msg)
				}
			}
		})
	}
}

func TestUserInfoFailureMessage_OtherErrors(t *testing.T) {
	if _, ok := UserInfoFailureMessage(errNoEmailInToken); ok {
		t.Fatal("missing email was treated as an HTTP status failure")
	}
	wrapped := fmt.Errorf("extract email from userinfo: %w", errEmailNotVerified)
	if _, ok := UserInfoFailureMessage(wrapped); ok {
		t.Fatal("unverified email was treated as an HTTP status failure")
	}
}

func TestEmailFromUserInfo_Unverified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email":"you@example.com","email_verified":false}`))
	}))
	defer server.Close()

	old := oauthUserInfoEndpoint
	oauthUserInfoEndpoint = server.URL
	defer func() { oauthUserInfoEndpoint = old }()

	_, err := emailFromUserInfo(context.Background(), &oauth2.Token{AccessToken: "test-token"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadManualStateByState_Match(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	st := manualState{
		State:        "state-1",
		RedirectURI:  "http://127.0.0.1:1234/oauth2/callback",
		Scopes:       []string{"a", "b"},
		CreatedAt:    time.Now().UTC(),
		CodeVerifier: strings.Repeat("a", 43),
	}
	if err := saveManualState(st); err != nil {
		t.Fatalf("saveManualState: %v", err)
	}

	got, ok, err := loadManualStateByState("state-1", []string{"b", "a"})
	if err != nil {
		t.Fatalf("loadManualStateByState: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.RedirectURI != st.RedirectURI {
		t.Fatalf("redirectURI = %q, want %q", got.RedirectURI, st.RedirectURI)
	}
}

func TestLoadManualStateByState_NotFound(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	_, ok, err := loadManualStateByState("missing", []string{"a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestLoadManualStateByState_RejectsMissingVerifier(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	st := manualState{
		State:       "state-old",
		RedirectURI: "http://127.0.0.1:1234/oauth2/callback",
		Scopes:      []string{"a"},
		CreatedAt:   time.Now().UTC(),
	}
	if err := saveManualState(st); err != nil {
		t.Fatalf("saveManualState: %v", err)
	}

	_, ok, err := loadManualStateByState("state-old", []string{"a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for state without a PKCE verifier")
	}

	path, err := manualStatePath("state-old")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected invalid state file to be removed, stat err=%v", statErr)
	}
}

func TestServeCallback_DoesNotEchoCode(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- serveCallback(ctx, ln)
	}()

	base := "http://" + ln.Addr().String()
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err = http.Get(base + "/oauth2/callback?code=secret-code&state=secret-state")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("callback server did not respond: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "secret-code") || strings.Contains(string(body), "secret-state") {
		t.Fatalf("response echoed the authorization query: %s", body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}

	other, err := http.Get(base + "/other")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Body.Close()
	if other.StatusCode != http.StatusNotFound {
		t.Fatalf("other path status = %d", other.StatusCode)
	}

	occupied, err := net.Listen("tcp", ln.Addr().String())
	if err == nil {
		_ = occupied.Close()
		t.Fatal("callback port was not held")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback server did not stop")
	}
}

func TestStep2_KeepsStateWhenExchangeFails(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	oldTimeout := callbackHoldTimeout
	callbackHoldTimeout = 30 * time.Second
	t.Cleanup(func() { callbackHoldTimeout = oldTimeout })

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(tokenSrv.Close)

	oldEndpoint := oauthEndpoint
	oauthEndpoint = oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/auth", TokenURL: tokenSrv.URL}
	t.Cleanup(func() { oauthEndpoint = oldEndpoint })

	creds := config.ClientCredentials{ClientID: "client-id", ClientSecret: "client-secret"}
	opts := AuthorizeOptions{Scopes: []string{"email"}}

	step1, err := Step1(context.Background(), creds, opts)
	if err != nil {
		t.Fatalf("Step1: %v", err)
	}

	authURL, err := url.Parse(step1.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	state := authURL.Query().Get("state")
	path, err := manualStatePath(state)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	redirectURL := authURL.Query().Get("redirect_uri") + "?code=auth-code&state=" + url.QueryEscape(state)
	if _, err := Step2(context.Background(), creds, opts, redirectURL); err == nil {
		t.Fatal("expected exchange failure")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("state file removed after failed exchange: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("failed exchange rewrote the state file")
	}
}

func TestStep1AndStep2_PKCEAndHeldPort(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	oldTimeout := callbackHoldTimeout
	callbackHoldTimeout = 30 * time.Second
	t.Cleanup(func() { callbackHoldTimeout = oldTimeout })

	var gotVerifier, gotCode, gotRedirect string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token form: %v", err)
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		gotVerifier = r.Form.Get("code_verifier")
		gotCode = r.Form.Get("code")
		gotRedirect = r.Form.Get("redirect_uri")
		if r.Form.Get("grant_type") != "authorization_code" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("authorization = %q", got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email":"you@example.com","email_verified":true}`))
	}))
	t.Cleanup(userinfoSrv.Close)

	oldEndpoint := oauthEndpoint
	oauthEndpoint = oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/auth", TokenURL: tokenSrv.URL}
	t.Cleanup(func() { oauthEndpoint = oldEndpoint })

	oldUserInfo := oauthUserInfoEndpoint
	oauthUserInfoEndpoint = userinfoSrv.URL
	t.Cleanup(func() { oauthUserInfoEndpoint = oldUserInfo })

	creds := config.ClientCredentials{ClientID: "client-id", ClientSecret: "client-secret"}
	opts := AuthorizeOptions{Scopes: []string{"email", "openid"}, ForceConsent: true}

	step1, err := Step1(context.Background(), creds, opts)
	if err != nil {
		t.Fatalf("Step1: %v", err)
	}

	authURL, err := url.Parse(step1.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	q := authURL.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	if q.Get("code_challenge") == "" {
		t.Fatal("missing code_challenge")
	}
	if q.Get("prompt") != "consent" {
		t.Fatalf("prompt = %q", q.Get("prompt"))
	}
	if strings.Contains(step1.AuthURL, "code_verifier=") {
		t.Fatal("auth URL contains the PKCE verifier")
	}

	state := q.Get("state")
	path, err := manualStatePath(state)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode = %o, want 600", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var st manualState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if !validCodeVerifier(st.CodeVerifier) {
		t.Fatalf("stored verifier is invalid: %q", st.CodeVerifier)
	}
	if oauth2.S256ChallengeFromVerifier(st.CodeVerifier) != q.Get("code_challenge") {
		t.Fatal("stored verifier does not match code_challenge")
	}
	if strings.Contains(step1.AuthURL, st.CodeVerifier) {
		t.Fatal("auth URL contains the raw verifier")
	}

	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	if ln, listenErr := net.Listen("tcp", redirect.Host); listenErr == nil {
		_ = ln.Close()
		t.Fatal("redirect port was not held after Step1 returned")
	}

	resp, err := http.Get(redirect.String() + "?code=secret-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "secret-code") || strings.Contains(string(body), st.CodeVerifier) {
		t.Fatalf("callback page leaked secrets: %s", body)
	}

	again, err := Step1(context.Background(), creds, opts)
	if err != nil {
		t.Fatalf("Step1 reuse: %v", err)
	}
	againURL, err := url.Parse(again.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	if againURL.Query().Get("code_challenge") != q.Get("code_challenge") {
		t.Fatal("reused Step1 changed the PKCE challenge")
	}
	if againURL.Query().Get("state") != state {
		t.Fatal("reused Step1 changed the state")
	}

	redirectURL := redirect.String() + "?code=auth-code&state=" + url.QueryEscape(state)
	step2, err := Step2(context.Background(), creds, opts, redirectURL)
	if err != nil {
		t.Fatalf("Step2: %v", err)
	}
	if step2.RefreshToken != "refresh-token" || step2.Email != "you@example.com" {
		t.Fatalf("Step2 result = %+v", step2)
	}
	if gotCode != "auth-code" {
		t.Fatalf("exchanged code = %q", gotCode)
	}
	if gotVerifier != st.CodeVerifier {
		t.Fatal("token request did not send the stored code_verifier")
	}
	if gotRedirect != st.RedirectURI {
		t.Fatalf("redirect_uri = %q, want %q", gotRedirect, st.RedirectURI)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("state file still exists after Step2, stat err=%v", statErr)
	}
}
