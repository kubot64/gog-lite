package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/output"
	"github.com/kubot64/gog-lite/internal/secrets"
)

func TestWriteGoogleAPIError_TooManyRequestsStaysCommandError(t *testing.T) {
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("delete_error", &gapi.Error{Code: http.StatusTooManyRequests, Message: "rate limit"})
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
	assertErrorCode(t, stderr, "delete_error")
}

func TestWriteGoogleAPIError_Unauthorized(t *testing.T) {
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", &gapi.Error{Code: http.StatusUnauthorized, Message: "Invalid Credentials"})
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeAuth)
	}
	assertErrorCode(t, stderr, "auth_required")
	if strings.Contains(stderr, tokenEndpointFailureMessage) {
		t.Fatalf("google API 401 was treated as a token endpoint failure: %q", stderr)
	}
	if !strings.Contains(stderr, "Invalid Credentials") {
		t.Fatalf("stderr = %q, want google API message", stderr)
	}
}

func TestWriteGoogleAPIError_UnauthorizedWrapped(t *testing.T) {
	var runErr error
	stderr := captureStderr(t, func() {
		base := &gapi.Error{Code: http.StatusUnauthorized, Message: "Invalid Credentials"}
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("gmail labels: %w", base))
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeAuth)
	}
	assertErrorCode(t, stderr, "auth_required")
}

func TestWriteGoogleAPIError_UnauthorizedFromClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"Invalid Credentials"}}`))
	}))
	t.Cleanup(server.Close)

	svc, err := gmail.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("gmail.NewService: %v", err)
	}

	_, err = svc.Users.Labels.List("me").Do()
	if err == nil {
		t.Fatal("expected gmail client error, got nil")
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("gmail labels: %w", err))
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, output.ExitCodeAuth, err)
	}
	assertErrorCode(t, stderr, "auth_required")
}

func TestWriteGoogleAPIError_NotFound(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", &gapi.Error{Code: 404, Message: "not found"})
	})
	if got := output.ExitCode(err); got != output.ExitCodeNotFound {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeNotFound)
	}
}

func TestWriteGoogleAPIError_ForbiddenWrapped(t *testing.T) {
	err := withMutedStderr(t, func() error {
		base := &gapi.Error{Code: 403, Message: "forbidden"}
		return writeGoogleAPIError("get_error", fmt.Errorf("wrapped: %w", base))
	})
	if got := output.ExitCode(err); got != output.ExitCodePermission {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodePermission)
	}
}

func TestWriteGoogleAPIError_OtherGoogleAPIError(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", &gapi.Error{Code: 500, Message: "internal"})
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func TestWriteGoogleAPIError_NonGoogleError(t *testing.T) {
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", errors.New("plain error"))
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
	if !strings.Contains(stderr, "plain error") {
		t.Fatalf("stderr = %q, want plain error", stderr)
	}
}

func TestWriteGoogleAPIError_BadRequestStaysGeneric(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", &gapi.Error{Code: http.StatusBadRequest, Message: "invalid_grant"})
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func TestWriteGoogleAPIError_InvalidGrant(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	retrieve := &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:             []byte(`{"error":"invalid_grant","error_description":"echo ` + refreshToken + `"}`),
		ErrorCode:        "invalid_grant",
		ErrorDescription: "echo " + refreshToken,
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("round trip: %w", retrieve))
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_InvalidGrantUnparsedJSON(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	retrieve := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:     []byte(`{"error":"invalid_grant","error_description":"echo ` + refreshToken + `","refresh_token":"` + refreshToken + `"}`),
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", retrieve)
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_InvalidGrantUnparsedForm(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	retrieve := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:     []byte("error=invalid_grant&error_description=echo-" + refreshToken),
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", fmt.Errorf("round trip: %w", retrieve))
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_InvalidGrantFromTokenEndpoint(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		if vals.Get("refresh_token") != refreshToken {
			t.Errorf("refresh_token = %q", vals.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"echo `+refreshToken+`"}`)
	}))
	t.Cleanup(server.Close)

	cfg := oauth2.Config{
		ClientID:     "dummy-id",
		ClientSecret: "dummy-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, server.Client())
	_, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err == nil {
		t.Fatal("expected token error")
	}
	var retrieve *oauth2.RetrieveError
	if !errors.As(err, &retrieve) || retrieve.ErrorCode != "invalid_grant" {
		t.Fatalf("error = %v, want invalid_grant RetrieveError", err)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("round trip: %w", err))
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_OtherRetrieveError(t *testing.T) {
	const marker = "leak-marker-9f3c2a"
	retrieve := &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"},
		Body:             []byte(`{"error":"invalid_client","error_description":"echo ` + marker + `","refresh_token":"` + marker + `"}`),
		ErrorCode:        "invalid_client",
		ErrorDescription: "echo " + marker,
		ErrorURI:         "https://example.invalid/" + marker,
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", fmt.Errorf("round trip: %w", retrieve))
	})
	assertTokenEndpointFailure(t, runErr, stderr, "get_error", "invalid_client", marker)
}

func TestWriteGoogleAPIError_UnparsedBodyIsNotInvalidGrant(t *testing.T) {
	const marker = "leak-marker-9f3c2a"
	retrieve := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:     []byte("not-json mentions invalid_grant " + marker),
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", retrieve)
	})
	assertTokenEndpointFailure(t, runErr, stderr, "get_error", "", marker)
}

func TestWriteGoogleAPIError_TokenEndpointOmitsBody(t *testing.T) {
	const marker = "leak-marker-9f3c2a"
	longCode := strings.Repeat("b", 65)

	tests := []struct {
		name      string
		err       error
		wantCLI   string
		wantOAuth string
	}{
		{
			name: "json body without parsed code",
			err: &oauth2.RetrieveError{
				Body: []byte(`{"error":"invalid_client","error_description":"echo ` + marker + `","refresh_token":"` + marker + `"}`),
			},
			wantCLI:   "labels_error",
			wantOAuth: "invalid_client",
		},
		{
			name: "form body without parsed code",
			err: &oauth2.RetrieveError{
				Body: []byte("error=invalid_request&error_description=echo-" + marker + "&refresh_token=" + marker),
			},
			wantCLI:   "get_error",
			wantOAuth: "invalid_request",
		},
		{
			name: "parsed code when body is only the marker",
			err: &oauth2.RetrieveError{
				ErrorCode:        "server_error",
				ErrorDescription: marker,
				Body:             []byte(marker),
			},
			wantCLI:   "get_error",
			wantOAuth: "server_error",
		},
		{
			name: "unsafe parsed code is dropped",
			err: fmt.Errorf("round trip: %w", &oauth2.RetrieveError{
				ErrorCode:        "bad " + marker,
				ErrorDescription: marker,
				Body:             []byte(`{"error":"bad ` + marker + `","error_description":"` + marker + `"}`),
			}),
			wantCLI:   "draft_error",
			wantOAuth: "",
		},
		{
			name: "hyphenated error code is dropped",
			err: &oauth2.RetrieveError{
				Body: []byte(`{"error":"` + marker + `","error_description":"` + marker + `"}`),
			},
			wantCLI:   "get_error",
			wantOAuth: "",
		},
		{
			name: "overlong error code is dropped",
			err: &oauth2.RetrieveError{
				Body: []byte(`{"error":"` + longCode + `","error_description":"` + marker + `"}`),
			},
			wantCLI:   "get_error",
			wantOAuth: "",
		},
		{
			name: "nil response would panic in Error",
			err: &oauth2.RetrieveError{
				Body: []byte(marker),
			},
			wantCLI:   "get_error",
			wantOAuth: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runErr error
			stderr := captureStderr(t, func() {
				runErr = writeGoogleAPIError(tt.wantCLI, tt.err)
			})
			assertTokenEndpointFailure(t, runErr, stderr, tt.wantCLI, tt.wantOAuth, marker)
			if tt.wantOAuth == "" && strings.Contains(stderr, longCode) {
				t.Fatalf("stderr included overlong error code: %q", stderr)
			}
		})
	}
}

func TestExecute_OtherTokenEndpointErrorOmitsBody(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	const marker = "leak-marker-9f3c2a"
	const account = "test@example.com"

	tests := []struct {
		name        string
		contentType string
		body        string
		wantOAuth   string
	}{
		{
			name:        "invalid_client",
			contentType: "application/json",
			body:        `{"error":"invalid_client","error_description":"echo ` + marker + `","refresh_token":"` + refreshToken + `"}`,
			wantOAuth:   "invalid_client",
		},
		{
			name:        "form invalid_request",
			contentType: "application/x-www-form-urlencoded",
			body:        "error=invalid_request&error_description=echo-" + marker + "&refresh_token=" + refreshToken,
			wantOAuth:   "invalid_request",
		},
		{
			name:        "unparsed body",
			contentType: "application/json",
			body:        "not-json " + marker + " " + refreshToken,
			wantOAuth:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFileKeyring(t)
			store := openTestStore(t)
			if err := store.SetToken(account, secrets.Token{
				Email:        account,
				RefreshToken: refreshToken,
				Services:     []string{"gmail"},
			}); err != nil {
				t.Fatalf("SetToken: %v", err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				vals, err := url.ParseQuery(string(body))
				if err != nil {
					t.Errorf("parse form: %v", err)
				}
				if vals.Get("refresh_token") != refreshToken {
					t.Errorf("refresh_token = %q", vals.Get("refresh_token"))
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			restore := googleapi.SetOAuthEndpointForTest(oauth2.Endpoint{
				TokenURL:  server.URL,
				AuthStyle: oauth2.AuthStyleInParams,
			})
			t.Cleanup(restore)

			prevArgs := os.Args
			t.Cleanup(func() { os.Args = prevArgs })
			os.Args = []string{"gog-lite", "gmail", "labels", "--account", account}

			var runErr error
			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					runErr = Execute(context.Background(), "dev")
				})
			})
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			assertTokenEndpointFailure(t, runErr, stderr, "labels_error", tt.wantOAuth, marker)
			if strings.Contains(stderr, refreshToken) {
				t.Fatalf("stderr leaked refresh token: %q", stderr)
			}
		})
	}
}

func TestExecute_InvalidGrantIsAuthRequired(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	const account = "test@example.com"

	setupFileKeyring(t)
	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{
		Email:        account,
		RefreshToken: refreshToken,
		Services:     []string{"gmail"},
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case called <- struct{}{}:
		default:
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		if vals.Get("refresh_token") != refreshToken {
			t.Errorf("refresh_token = %q", vals.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"echo `+refreshToken+`","refresh_token":"`+refreshToken+`"}`)
	}))
	t.Cleanup(server.Close)

	restore := googleapi.SetOAuthEndpointForTest(oauth2.Endpoint{
		TokenURL:  server.URL,
		AuthStyle: oauth2.AuthStyleInParams,
	})
	t.Cleanup(restore)

	prevArgs := os.Args
	t.Cleanup(func() { os.Args = prevArgs })
	os.Args = []string{"gog-lite", "gmail", "labels", "--account", account}

	var runErr error
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			runErr = Execute(context.Background(), "dev")
		})
	})
	select {
	case <-called:
	default:
		t.Fatal("token endpoint was not called")
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func assertInvalidGrant(t *testing.T, runErr error, stderr, secret string) {
	t.Helper()
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, output.ExitCodeAuth, runErr)
	}
	assertErrorCode(t, stderr, "auth_required")

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("decode stderr %q: %v", stderr, err)
	}
	if payload.Error != invalidGrantMessage {
		t.Errorf("error = %q, want %q", payload.Error, invalidGrantMessage)
	}
	if strings.Contains(stderr, secret) {
		t.Fatalf("stderr leaked refresh token: %q", stderr)
	}
}

func assertTokenEndpointFailure(t *testing.T, runErr error, stderr, wantCLI, wantOAuth, marker string) {
	t.Helper()
	if got := output.ExitCode(runErr); got != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, output.ExitCodeError, runErr)
	}
	assertErrorCode(t, stderr, wantCLI)

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("decode stderr %q: %v", stderr, err)
	}
	want := tokenEndpointFailureMessage
	if wantOAuth != "" {
		want += " (" + wantOAuth + ")"
	}
	if payload.Error != want {
		t.Errorf("error = %q, want %q", payload.Error, want)
	}
	if marker != "" && strings.Contains(stderr, marker) {
		t.Fatalf("stderr leaked token endpoint body: %q", stderr)
	}
	if strings.Contains(stderr, "error_description") || strings.Contains(stderr, "Response:") || strings.Contains(stderr, "round trip:") {
		t.Fatalf("stderr included token endpoint details: %q", stderr)
	}
}

func assertErrorCode(t *testing.T, stderr, want string) {
	t.Helper()

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("decode stderr %q: %v", stderr, err)
	}
	if payload.Code != want {
		t.Errorf("code = %q, want %q", payload.Code, want)
	}
}

func withMutedStderr(t *testing.T, fn func() error) error {
	t.Helper()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stderr = w
	defer func() { os.Stderr = old }()

	runErr := fn()

	_ = w.Close()
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()

	return runErr
}
