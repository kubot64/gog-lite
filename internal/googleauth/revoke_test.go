package googleauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRevokeRefreshToken_PostsRefreshToken(t *testing.T) {
	const refreshToken = "abc+def/ghi="

	var gotMethod, gotContentType, gotToken, rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		rawQuery = r.URL.RawQuery
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotToken = values.Get("token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := swapRevokeEndpoint(t, server.URL)
	defer restore()

	if err := RevokeRefreshToken(context.Background(), "  "+refreshToken+"  "); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", gotContentType)
	}
	if gotToken != refreshToken {
		t.Errorf("token = %q, want %q", gotToken, refreshToken)
	}
	if strings.Contains(rawQuery, refreshToken) || strings.Contains(rawQuery, "abc") {
		t.Errorf("token leaked into query %q", rawQuery)
	}
}

func TestRevokeRefreshToken_NonOKKeepsTokenOutOfError(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_token","echo":"`+refreshToken+`"}`)
	}))
	defer server.Close()

	restore := swapRevokeEndpoint(t, server.URL)
	defer restore()

	err := RevokeRefreshToken(context.Background(), refreshToken)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error = %q, want HTTP 400", err.Error())
	}
	if strings.Contains(err.Error(), refreshToken) {
		t.Fatalf("error leaked refresh token: %q", err.Error())
	}
}

func TestRevokeRefreshToken_EmptyTokenSkipsHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := swapRevokeEndpoint(t, server.URL)
	defer restore()

	if err := RevokeRefreshToken(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty token")
	}
	if called {
		t.Fatal("empty token must not call the revoke endpoint")
	}
}

func TestRevokeRefreshToken_DoesNotFollowRedirect(t *testing.T) {
	const refreshToken = "redirect-secret-token"

	hits := 0
	leak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if strings.Contains(string(body), refreshToken) || strings.Contains(r.URL.RawQuery, refreshToken) {
			t.Errorf("refresh token was forwarded to redirect target")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer leak.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, leak.URL, http.StatusFound)
	}))
	defer server.Close()

	restore := swapRevokeEndpoint(t, server.URL)
	defer restore()

	err := RevokeRefreshToken(context.Background(), refreshToken)
	if err == nil {
		t.Fatal("expected error for redirect")
	}
	if strings.Contains(err.Error(), refreshToken) {
		t.Fatalf("error leaked refresh token: %q", err.Error())
	}
	if hits != 0 {
		t.Fatalf("redirect target was called %d times", hits)
	}
}

func TestRevokeRefreshToken_TransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close()

	restore := swapRevokeEndpoint(t, server.URL)
	defer restore()

	err := RevokeRefreshToken(context.Background(), "refresh-token")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), "refresh-token") {
		t.Fatalf("error leaked refresh token: %q", err.Error())
	}
}

func TestRevokeRefreshToken_CanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := swapRevokeEndpoint(t, server.URL)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RevokeRefreshToken(ctx, "refresh-token")
	if err == nil {
		t.Fatal("expected canceled context to fail")
	}
	if strings.Contains(err.Error(), "refresh-token") {
		t.Fatalf("error leaked refresh token: %q", err.Error())
	}
}

func swapRevokeEndpoint(t *testing.T, endpoint string) func() {
	t.Helper()
	old := tokenRevokeEndpoint
	tokenRevokeEndpoint = endpoint
	return func() { tokenRevokeEndpoint = old }
}
