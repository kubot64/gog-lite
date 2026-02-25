package googleauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

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
		State:       "state-1",
		RedirectURI: "http://127.0.0.1:1234/oauth2/callback",
		Scopes:      []string{"a", "b"},
		CreatedAt:   time.Now().UTC(),
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
